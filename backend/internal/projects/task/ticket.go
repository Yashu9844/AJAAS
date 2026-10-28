package task

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

var ticketPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]{1,9})-([0-9]{1,9})$`)

// ByKey resolves a ticket id such as KEY-12 inside the tenant (TK-12). A moved task keeps answering to its old ids.
func (s *Service) ByKey(ctx context.Context, a api.Actor, key string) (*View, error) {
	m := ticketPattern.FindStringSubmatch(strings.TrimSpace(key))
	if m == nil {
		return nil, kernel.Invalid("key", "must look like KEY-12")
	}
	number, _ := strconv.Atoi(m[2])
	id, err := s.resolveTicket(ctx, a.TenantID, strings.ToUpper(m[1]), number)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, a, id)
}

func (s *Service) resolveTicket(ctx context.Context, tenantID uuid.UUID, key string, number int) (uuid.UUID, error) {
	var ids []uuid.UUID
	db := s.db.WithContext(ctx)
	err := db.Raw(`SELECT t.id FROM tasks t JOIN projects p ON p.id = t.project_id WHERE t.tenant_id = ? AND p.key = ? AND t.number = ?
		AND t.deleted_at IS NULL LIMIT 1`, tenantID, key, number).Scan(&ids).Error
	if err != nil {
		return uuid.Nil, err
	}
	if len(ids) > 0 {
		return ids[0], nil
	}
	err = db.Raw(`SELECT t.id FROM task_ticket_alias al JOIN tasks t ON t.id = al.task_id WHERE al.tenant_id = ? AND al.old_key = ?
		AND al.old_number = ? AND t.deleted_at IS NULL LIMIT 1`, tenantID, key, number).Scan(&ids).Error
	if err != nil {
		return uuid.Nil, err
	}
	if len(ids) == 0 {
		return uuid.Nil, kernel.NotFound("ticket")
	}
	return ids[0], nil
}

// Subtasks lists the direct subtasks of a task, oldest ticket first, with the usual cursor.
func (s *Service) Subtasks(ctx context.Context, a api.Actor, id uuid.UUID, f ListFilter) ([]View, string, error) {
	t, _, err := s.locate(ctx, a, id)
	if err != nil {
		return nil, "", err
	}
	f.ParentID, f.Sort = &id, "number"
	if f.Dir == "" {
		f.Dir = "asc"
	}
	return s.List(ctx, a, t.ProjectID, f)
}
