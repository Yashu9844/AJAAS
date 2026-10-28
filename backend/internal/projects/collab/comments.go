package collab

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

// CommentInput is the body of a comment create or edit.
type CommentInput struct {
	Body string `json:"body"`
}

// commentWrite is a comment ready to be stored together with what its author's markdown mentions.
type commentWrite struct {
	ref      api.TaskRef
	c        *Comment
	mentions []uuid.UUID
}

// prepareBody validates the markdown and drops the mentions that cannot read the project or are the author (CM-02).
func (s *Service) prepareBody(ctx context.Context, a api.Actor, ref api.TaskRef, in CommentInput) (string, []uuid.UUID, error) {
	body, err := cleanText("body", in.Body, 1, maxCommentBody)
	if err != nil {
		return "", nil, err
	}
	readable, err := s.ReadableMentions(ctx, a.TenantID, ref.ProjectID, ParseMentions(body))
	if err != nil {
		return "", nil, err
	}
	out := readable[:0]
	for _, id := range readable {
		if id != a.UserID {
			out = append(out, id)
		}
	}
	return body, out, nil
}

// CreateComment adds a comment to a task (CM-01). The commenter and the readable mentioned users start watching (CM-03).
func (s *Service) CreateComment(ctx context.Context, a api.Actor, taskID uuid.UUID, in CommentInput) (*Comment, error) {
	ref, _, err := s.taskAccess(ctx, a, taskID, api.TaskComment)
	if err != nil {
		return nil, err
	}
	body, mentions, err := s.prepareBody(ctx, a, ref, in)
	if err != nil {
		return nil, err
	}
	c := &Comment{ID: uuid.New(), TenantID: a.TenantID, TaskID: taskID, AuthorUserID: a.UserID, Body: body, CreatedAt: s.now().UTC(), Mentions: mentions}
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		return s.storeComment(ctx, tx, a, commentWrite{ref: ref, c: c, mentions: mentions})
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) storeComment(ctx context.Context, tx *gorm.DB, a api.Actor, w commentWrite) error {
	if err := tx.WithContext(ctx).Create(w.c).Error; err != nil {
		return err
	}
	if err := s.afterMentions(ctx, tx, a, w); err != nil {
		return err
	}
	if err := AddWatcher(ctx, tx, Watcher{TaskID: w.ref.ID, UserID: a.UserID, TenantID: a.TenantID, Reason: ReasonCommenter}); err != nil {
		return err
	}
	tid := w.ref.ID
	if err := kernel.LogActivity(ctx, tx, kernel.Activity{TenantID: a.TenantID, ProjectID: w.ref.ProjectID, TaskID: &tid, ActorID: &a.UserID,
		Type: "commented", Data: map[string]any{"comment_id": w.c.ID}}); err != nil {
		return err
	}
	s.audit(ctx, tx, a, auditEntry{Action: "comment.created", Resource: "comment", ID: w.c.ID, Meta: map[string]any{"task_id": w.ref.ID}})
	return kernel.Emit(ctx, tx, a.TenantID, "task.commented", map[string]any{"task_id": w.ref.ID, "project_id": w.ref.ProjectID,
		"comment_id": w.c.ID, "author_user_id": a.UserID, "task_version": w.ref.Version})
}

// afterMentions stores the mentions, makes the newly mentioned users watchers and announces them with ids only (PG11, PG12).
func (s *Service) afterMentions(ctx context.Context, tx *gorm.DB, a api.Actor, w commentWrite) error {
	added, err := ReplaceMentions(ctx, tx, MentionSet{TenantID: a.TenantID, Kind: "comment", SourceID: w.c.ID, TaskID: w.ref.ID, Users: w.mentions})
	if err != nil || len(added) == 0 {
		return err
	}
	if err := watchMany(ctx, tx, Watcher{TaskID: w.ref.ID, TenantID: a.TenantID, Reason: ReasonMentioned}, added); err != nil {
		return err
	}
	return kernel.Emit(ctx, tx, a.TenantID, "task.mentioned", map[string]any{"task_id": w.ref.ID, "project_id": w.ref.ProjectID,
		"source": "comment", "source_id": w.c.ID, "user_ids": added, "actor_user_id": a.UserID})
}

type commentCursor struct {
	At time.Time `json:"a"`
	ID uuid.UUID `json:"i"`
}

// ListComments pages a task's comments oldest first; deleted ones appear as tombstones with an empty body.
func (s *Service) ListComments(ctx context.Context, a api.Actor, taskID uuid.UUID, pr PageRequest) ([]Comment, string, error) {
	if _, _, err := s.taskAccess(ctx, a, taskID, api.TaskRead); err != nil {
		return nil, "", err
	}
	var cur commentCursor
	if err := kernel.DecodeCursor(pr.Cursor, &cur); err != nil {
		return nil, "", err
	}
	q := s.db.WithContext(ctx).Where("tenant_id = ? AND task_id = ?", a.TenantID, taskID)
	if cur.ID != uuid.Nil {
		q = q.Where("(created_at, id) > (?, ?)", cur.At, cur.ID)
	}
	var rows []Comment
	if err := q.Order("created_at, id").Limit(pr.limit() + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	rows, next := page(rows, pr.limit(), func(c Comment) any { return commentCursor{At: c.CreatedAt, ID: c.ID} })
	return rows, next, s.fillComments(ctx, a.TenantID, rows)
}

// fillComments marks tombstones and attaches the stored mentions of the page.
func (s *Service) fillComments(ctx context.Context, tenantID uuid.UUID, rows []Comment) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	var ms []struct{ SourceID, UserID uuid.UUID }
	if err := s.db.WithContext(ctx).Raw(`SELECT source_id, user_id FROM mentions WHERE tenant_id = ? AND source_kind = 'comment'
		AND source_id IN ? ORDER BY created_at, user_id`, tenantID, ids).Scan(&ms).Error; err != nil {
		return err
	}
	by := map[uuid.UUID][]uuid.UUID{}
	for _, m := range ms {
		by[m.SourceID] = append(by[m.SourceID], m.UserID)
	}
	for i := range rows {
		rows[i].Deleted = rows[i].DeletedAt != nil
		rows[i].Mentions = by[rows[i].ID]
		if rows[i].Mentions == nil {
			rows[i].Mentions = []uuid.UUID{}
		}
	}
	return nil
}

// loadComment reads a comment of the tenant (tombstones included).
func (s *Service) loadComment(ctx context.Context, tenantID, id uuid.UUID) (Comment, error) {
	var c Comment
	err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Take(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c, kernel.NotFound("comment")
	}
	return c, err
}

// commentContext finds a comment and authorises the caller against the project of its task.
func (s *Service) commentContext(ctx context.Context, a api.Actor, id uuid.UUID) (Comment, api.TaskRef, api.Access, error) {
	c, err := s.loadComment(ctx, a.TenantID, id)
	if err != nil {
		return c, api.TaskRef{}, api.Access{}, err
	}
	ref, acc, err := s.taskAccess(ctx, a, c.TaskID, api.TaskRead)
	return c, ref, acc, err
}

// UpdateComment lets the author edit a comment; the edit time is recorded (CM-01).
func (s *Service) UpdateComment(ctx context.Context, a api.Actor, id uuid.UUID, in CommentInput) (*Comment, error) {
	c, ref, acc, err := s.commentContext(ctx, a, id)
	if err != nil {
		return nil, err
	}
	if c.DeletedAt != nil {
		return nil, kernel.NotFound("comment")
	}
	if c.AuthorUserID != a.UserID {
		return nil, kernel.Err(http.StatusForbidden, "FORBIDDEN", "only the author can edit a comment")
	}
	if err := acc.Require(api.TaskComment); err != nil {
		return nil, err
	}
	body, mentions, err := s.prepareBody(ctx, a, ref, in)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	c.Body, c.EditedAt, c.Mentions = body, &now, mentions
	err = s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE comments SET body = ?, edited_at = ? WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL`, body, now, c.ID, a.TenantID)
		if res.Error != nil || res.RowsAffected == 0 {
			return orNotFound(res.Error, "comment")
		}
		s.audit(ctx, tx, a, auditEntry{Action: "comment.updated", Resource: "comment", ID: c.ID, Meta: map[string]any{"task_id": c.TaskID}})
		return s.afterMentions(ctx, tx, a, commentWrite{ref: ref, c: &c, mentions: mentions})
	})
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func orNotFound(err error, what string) error {
	if err != nil {
		return err
	}
	return kernel.NotFound(what)
}

// DeleteComment turns a comment into a tombstone. The author may delete their own; comment.moderate holders any (CM-01).
func (s *Service) DeleteComment(ctx context.Context, a api.Actor, id uuid.UUID) error {
	c, _, acc, err := s.commentContext(ctx, a, id)
	if err != nil {
		return err
	}
	if c.DeletedAt != nil {
		return kernel.NotFound("comment")
	}
	need := api.CommentModerate
	if c.AuthorUserID == a.UserID {
		need = api.TaskComment
	}
	if err := acc.Require(need); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *gorm.DB) error {
		res := tx.Exec(`UPDATE comments SET body = '', deleted_at = ?, deleted_by = ? WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL`,
			s.now().UTC(), a.UserID, c.ID, a.TenantID)
		if res.Error != nil || res.RowsAffected == 0 {
			return orNotFound(res.Error, "comment")
		}
		if err := tx.Exec(`DELETE FROM mentions WHERE tenant_id = ? AND source_kind = 'comment' AND source_id = ?`, a.TenantID, c.ID).Error; err != nil {
			return err
		}
		s.audit(ctx, tx, a, auditEntry{Action: "comment.deleted", Resource: "comment", ID: c.ID, Meta: map[string]any{"task_id": c.TaskID}})
		return nil
	})
}
