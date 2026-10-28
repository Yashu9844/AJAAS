package collab

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// DownloadLink is a short-lived presigned GET (AT-03).
type DownloadLink struct {
	URL       string    `json:"url"`
	Filename  string    `json:"filename"`
	ExpiresAt time.Time `json:"expires_at"`
}

// DownloadAttachment authorises the caller against the project of the attachment's task (task.read) and returns a
// two-minute presigned URL. Only ready attachments download; unknown, foreign, pending and deleted ids are 404 (PG8).
// Downloads from private projects are written to the activity log (AT-03).
func (s *Service) DownloadAttachment(ctx context.Context, a api.Actor, id uuid.UUID) (*DownloadLink, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	att, ref, acc, err := s.attachmentContext(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if att.Status != "ready" {
		return nil, kernel.NotFound("attachment")
	}
	if acc.Project.Visibility != api.VisibilityTenant {
		err = s.inTx(ctx, func(tx *gorm.DB) error {
			tid := ref.ID
			return kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: ref.ProjectID, TaskID: &tid, ActorID: &a.UserID,
				Type: "attachment_downloaded", Data: map[string]any{"attachment_id": att.ID}})
		})
		if err != nil {
			return nil, err
		}
	}
	url, err := st.PresignGet(ctx, att.StorageKey, att.Filename, DownloadTTL)
	if err != nil {
		return nil, storageDown()
	}
	return &DownloadLink{URL: url, Filename: att.Filename, ExpiresAt: s.now().UTC().Add(DownloadTTL)}, nil
}

// DeleteAttachment retires an attachment and removes its object. The uploader may delete their own (attachment.upload);
// anyone else needs attachment.delete (AT-03). A failed object delete leaves the row for the sweep to retry.
func (s *Service) DeleteAttachment(ctx context.Context, a api.Actor, id uuid.UUID) error {
	st, err := s.store()
	if err != nil {
		return err
	}
	att, ref, acc, err := s.attachmentContext(ctx, a, id)
	if err != nil {
		return err
	}
	need := api.AttachmentDelete
	if att.UploadedBy == a.UserID {
		need = api.AttachmentUpload
	}
	if err := acc.Require(need); err != nil {
		return err
	}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE attachments SET status = 'deleted', deleted_at = ? WHERE id = ? AND tenant_id = ? AND status <> 'deleted'`,
			s.now().UTC(), att.ID, a.TenantID)
		if res.Error != nil || res.RowsAffected == 0 {
			return orNotFound(res.Error, "attachment")
		}
		s.audit(ctx, tx, a, auditEntry{Action: "attachment.removed", Resource: "attachment", ID: att.ID, Meta: map[string]any{"task_id": ref.ID}})
		if att.Status != "ready" {
			return nil
		}
		return kernel.Emit(ctx, tx, a.TenantID, "attachment.removed", map[string]any{"attachment_id": att.ID, "task_id": ref.ID,
			"project_id": ref.ProjectID, "removed_by": a.UserID})
	})
	if err != nil {
		return err
	}
	if st.Delete(ctx, att.StorageKey) == nil {
		_ = s.db.WithContext(ctx).Exec(`DELETE FROM attachments WHERE id = ? AND tenant_id = ? AND status = 'deleted'`, att.ID, a.TenantID).Error
	}
	return nil
}
