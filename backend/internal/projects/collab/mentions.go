package collab

import (
	"context"
	"errors"
	"net/http"
	"regexp"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
	"gorm.io/gorm"
)

// maxMentions caps the distinct mentions kept per text, so one comment cannot fan out an unbounded number of events.
const maxMentions = 20

var mentionToken = regexp.MustCompile(`@\[([^\]\r\n]{1,120})\]\(user:([0-9A-Fa-f-]{36})\)`)

// ParseMentions extracts the distinct user ids of the @[Name](user:<uuid>) tokens in markdown, in order of appearance
// and capped at maxMentions (CM-02). Task descriptions use it too. It does not check that the users may read anything:
// callers pass the result through ReadableMentions before storing or announcing it.
func ParseMentions(text string) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	for _, m := range mentionToken.FindAllStringSubmatch(text, -1) {
		id, err := uuid.Parse(m[2])
		if err != nil || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == maxMentions {
			break
		}
	}
	return out
}

// ReadableMentions drops the ids that cannot read the project (CM-02, PG11), so a mention never leaks a task. The rule
// is evaluated for the mentioned user with an actor that carries no tenant-level flags: a tenant-visible project is
// readable by every active user of the tenant, a private project only by its active members. Users who can read a
// private project solely through projects:read_all are dropped too, which fails closed. Unknown, foreign-tenant and
// inactive users are dropped.
func (s *Service) ReadableMentions(ctx context.Context, tenantID, projectID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		ok, err := s.canRead(ctx, tenantID, projectID, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *Service) canRead(ctx context.Context, tenantID, projectID, userID uuid.UUID) (bool, error) {
	if s.p.Users != nil {
		u, err := s.p.Users.GetByID(ctx, s.db, tenantID, userID)
		if err != nil {
			return false, err
		}
		if !u.Active() {
			return false, nil
		}
	}
	acc, err := s.p.Authorizer.Access(ctx, s.db, api.Actor{TenantID: tenantID, UserID: userID}, projectID)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return acc.Can(api.TaskRead), nil
}

func isNotFound(err error) bool {
	var app *sharedErrors.AppError
	return errors.As(err, &app) && app.StatusCode == http.StatusNotFound
}

// MentionSet is the readable mentions of one comment or task description.
type MentionSet struct {
	TenantID uuid.UUID
	Kind     string // task or comment
	SourceID uuid.UUID
	TaskID   uuid.UUID
	Users    []uuid.UUID
}

// ReplaceMentions stores the set in the caller transaction and returns the users who were not mentioned before.
func ReplaceMentions(ctx context.Context, tx *gorm.DB, m MentionSet) ([]uuid.UUID, error) {
	db := tx.WithContext(ctx)
	var before []uuid.UUID
	if err := db.Raw(`SELECT user_id FROM mentions WHERE tenant_id = ? AND source_kind = ? AND source_id = ?`,
		m.TenantID, m.Kind, m.SourceID).Scan(&before).Error; err != nil {
		return nil, err
	}
	if err := db.Exec(`DELETE FROM mentions WHERE tenant_id = ? AND source_kind = ? AND source_id = ?`, m.TenantID, m.Kind, m.SourceID).Error; err != nil {
		return nil, err
	}
	known := map[uuid.UUID]bool{}
	for _, id := range before {
		known[id] = true
	}
	var added []uuid.UUID
	for _, id := range m.Users {
		if err := db.Exec(`INSERT INTO mentions (tenant_id, source_kind, source_id, task_id, user_id) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING`, m.TenantID, m.Kind, m.SourceID, m.TaskID, id).Error; err != nil {
			return nil, err
		}
		if !known[id] {
			added = append(added, id)
		}
	}
	return added, nil
}
