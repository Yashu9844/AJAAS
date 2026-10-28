package collab

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"github.com/jaas/jaas/internal/shared/storage"
	"gorm.io/gorm"
)

func uploadMismatch() error {
	return kernel.Conflict("UPLOAD_MISMATCH", "the uploaded object does not match the declared size, type or checksum")
}

// baseType lowercases a media type and drops its parameters.
func baseType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}

// CompleteAttachment verifies the uploaded object against what init declared, runs the scan hook and marks the
// attachment ready (AT-01, AT-02). A mismatch answers 409 UPLOAD_MISMATCH and the row stays pending. Completing a ready
// attachment again returns it unchanged.
func (s *Service) CompleteAttachment(ctx context.Context, a api.Actor, id uuid.UUID) (*Attachment, error) {
	st, err := s.store()
	if err != nil {
		return nil, err
	}
	att, ref, acc, err := s.attachmentContext(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if att.UploadedBy != a.UserID {
		return nil, kernel.Err(http.StatusForbidden, "FORBIDDEN", "only the uploader can complete an upload")
	}
	if err := acc.Require(api.AttachmentUpload); err != nil {
		return nil, err
	}
	if att.Status == "ready" {
		return &att, nil
	}
	if err := s.verifyUpload(ctx, st, &att); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE attachments SET status = 'ready', ready_at = ? WHERE id = ? AND tenant_id = ? AND status = 'pending'`, now, att.ID, a.TenantID)
		if res.Error != nil || res.RowsAffected == 0 {
			return orNotFound(res.Error, "attachment")
		}
		return s.announceAttached(ctx, tx, a, attachFact{ref: ref, att: att})
	})
	if err != nil {
		return nil, err
	}
	att.Status, att.ReadyAt = "ready", &now
	return &att, nil
}

// attachFact is an attachment that just became ready, with the task it belongs to.
type attachFact struct {
	ref api.TaskRef
	att Attachment
}

func (s *Service) announceAttached(ctx context.Context, tx *gorm.DB, a api.Actor, f attachFact) error {
	ref, att := f.ref, f.att
	tid := ref.ID
	if err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: ref.ProjectID, TaskID: &tid, ActorID: &a.UserID,
		Type: "attached", Data: map[string]any{"attachment_id": att.ID}}); err != nil {
		return err
	}
	s.audit(ctx, tx, a, auditEntry{Action: "attachment.added", Resource: "attachment", ID: att.ID, Meta: map[string]any{"task_id": ref.ID}})
	return kernel.Emit(ctx, tx, a.TenantID, "attachment.added", map[string]any{"attachment_id": att.ID, "task_id": ref.ID,
		"project_id": ref.ProjectID, "uploaded_by": att.UploadedBy})
}

// verifyUpload heads the object, compares it with the declaration and scans it. No database transaction is open (I/O).
func (s *Service) verifyUpload(ctx context.Context, st storage.Store, att *Attachment) error {
	info, err := st.Head(ctx, att.StorageKey)
	if errors.Is(err, storage.ErrNotFound) {
		return uploadMismatch()
	}
	if err != nil {
		return storageDown()
	}
	if info.Size != att.Size || baseType(info.ContentType) != att.ContentType {
		return uploadMismatch()
	}
	if att.SHA256 != "" && !strings.EqualFold(info.SHA256, att.SHA256) {
		return uploadMismatch()
	}
	clean, err := s.p.Scanner.Scan(ctx, att.StorageKey, info)
	if err != nil {
		return kernel.Err(http.StatusServiceUnavailable, "SCAN_UNAVAILABLE", "the file could not be scanned; try again")
	}
	if !clean {
		s.markDeleted(ctx, att.TenantID, att.ID)
		_ = st.Delete(ctx, att.StorageKey)
		return kernel.Conflict("MALWARE_DETECTED", "the file was rejected by the malware scan")
	}
	return nil
}
