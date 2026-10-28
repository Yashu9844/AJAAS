package collab

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/shared/storage"
	"gorm.io/gorm"
)

// UploadTicket answers an init: the pending attachment and where the client PUTs the bytes (AT-01).
type UploadTicket struct {
	Attachment *Attachment       `json:"attachment"`
	Upload     storage.PutTarget `json:"upload"`
}

// store returns the object store, or 503 when none is configured.
func (s *Service) store() (storage.Store, error) {
	if s.p.Storage == nil {
		return nil, kernel.Err(http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "file storage is not configured")
	}
	return s.p.Storage, nil
}

func storageDown() error {
	return kernel.Err(http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "file storage did not answer; try again")
}

// StorageKey is the object key layout t/<tenant>/p/<project>/a/<attachment-id> (design section 5).
func StorageKey(tenantID, projectID, attachmentID uuid.UUID) string {
	return fmt.Sprintf("t/%s/p/%s/a/%s", tenantID, projectID, attachmentID)
}

// InitAttachment validates the file, reserves quota, writes a pending row and returns a presigned PUT (AT-01, AT-02).
// attachment.upload is required. Nothing is stored until the client uploads and completes.
func (s *Service) InitAttachment(ctx context.Context, a api.Actor, taskID uuid.UUID, in InitInput) (*UploadTicket, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	ref, _, err := s.taskAccess(ctx, a, taskID, api.AttachmentUpload)
	if err != nil {
		return nil, err
	}
	f, err := in.validate()
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	att := &Attachment{ID: id, TenantID: a.TenantID, ProjectID: ref.ProjectID, TaskID: taskID, StorageKey: StorageKey(a.TenantID, ref.ProjectID, id),
		Filename: f.filename, ContentType: f.contentType, Size: f.size, SHA256: f.sha, Status: "pending", UploadedBy: a.UserID, CreatedAt: s.now().UTC()}
	if err := s.inTx(ctx, func(tx *gorm.DB) error { return s.reserve(ctx, tx, att) }); err != nil {
		return nil, err
	}
	target, err := st.PresignPut(ctx, att.StorageKey, att.ContentType, att.Size, att.SHA256, UploadTTL)
	if err != nil {
		s.markDeleted(ctx, att.TenantID, att.ID)
		return nil, storageDown()
	}
	return &UploadTicket{Attachment: att, Upload: target}, nil
}

// reserve checks the per-task count and the tenant quota under a tenant lock, then inserts the pending row.
func (s *Service) reserve(ctx context.Context, tx *gorm.DB, att *Attachment) error {
	db := tx.WithContext(ctx)
	if err := db.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, "attachments:"+att.TenantID.String()).Error; err != nil {
		return err
	}
	var used struct{ Bytes, OnTask int64 }
	if err := db.Raw(`SELECT COALESCE(SUM(size), 0) AS bytes, COUNT(*) FILTER (WHERE task_id = ?) AS on_task
		FROM attachments WHERE tenant_id = ? AND status <> 'deleted'`, att.TaskID, att.TenantID).Scan(&used).Error; err != nil {
		return err
	}
	if used.OnTask >= MaxPerTask {
		return kernel.Conflict("TOO_MANY_ATTACHMENTS", "this task already has the maximum number of attachments")
	}
	if used.Bytes+att.Size > TenantQuotaBytes {
		return kernel.Conflict("QUOTA_EXCEEDED", "the storage quota of the tenant is used up")
	}
	return db.Create(att).Error
}

// markDeleted retires a row whose upload can no longer happen; the sweep removes row and object.
func (s *Service) markDeleted(ctx context.Context, tenantID, id uuid.UUID) {
	_ = s.db.WithContext(ctx).Exec(`UPDATE attachments SET status = 'deleted', deleted_at = ? WHERE id = ? AND tenant_id = ? AND status <> 'deleted'`,
		s.now().UTC(), id, tenantID).Error
}

type attachmentCursor struct {
	At time.Time `json:"a"`
	ID uuid.UUID `json:"i"`
}

// ListAttachments pages a task's ready attachments, oldest first (AT-01).
func (s *Service) ListAttachments(ctx context.Context, a api.Actor, taskID uuid.UUID, pr PageRequest) ([]Attachment, string, error) {
	if _, err := s.store(); err != nil {
		return nil, "", err
	}
	if _, _, err := s.taskAccess(ctx, a, taskID, api.TaskRead); err != nil {
		return nil, "", err
	}
	var cur attachmentCursor
	if err := kernel.DecodeCursor(pr.Cursor, &cur); err != nil {
		return nil, "", err
	}
	q := s.db.WithContext(ctx).Where("tenant_id = ? AND task_id = ? AND status = 'ready'", a.TenantID, taskID)
	if cur.ID != uuid.Nil {
		q = q.Where("(created_at, id) > (?, ?)", cur.At, cur.ID)
	}
	var rows []Attachment
	if err := q.Order("created_at, id").Limit(pr.limit() + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	rows, next := page(rows, pr.limit(), func(x Attachment) any { return attachmentCursor{At: x.CreatedAt, ID: x.ID} })
	return rows, next, nil
}

// loadAttachment reads a live (pending or ready) attachment of the tenant.
func (s *Service) loadAttachment(ctx context.Context, tenantID, id uuid.UUID) (Attachment, error) {
	var att Attachment
	err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND status <> 'deleted'", id, tenantID).Take(&att).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return att, kernel.NotFound("attachment")
	}
	return att, err
}

// attachmentContext finds an attachment and authorises the caller against the project of its task, never against the
// attachment id alone (PG8). A pending upload is invisible to everyone but its uploader.
func (s *Service) attachmentContext(ctx context.Context, a api.Actor, id uuid.UUID) (Attachment, api.TaskRef, api.Access, error) {
	att, err := s.loadAttachment(ctx, a.TenantID, id)
	if err != nil {
		return att, api.TaskRef{}, api.Access{}, err
	}
	ref, acc, err := s.taskAccess(ctx, a, att.TaskID, api.TaskRead)
	if err != nil {
		return att, ref, acc, err
	}
	if att.Status == "pending" && att.UploadedBy != a.UserID {
		return att, ref, acc, kernel.NotFound("attachment")
	}
	return att, ref, acc, nil
}
