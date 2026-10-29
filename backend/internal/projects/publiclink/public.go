package publiclink

import (
	"context"
	"crypto/subtle"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
	"gorm.io/gorm"
)

var (
	tokenShape = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	// mentionToken matches the @[Name](user:<uuid>) markdown a description may hold; it is replaced so no user id or name leaks.
	mentionToken = regexp.MustCompile(`@\[[^\]\r\n]{1,120}\]\(user:[0-9A-Fa-f-]{36}\)`)
)

func notFound() error { return kernel.NotFound("public project") }

// resolve finds the active link of a raw token. Unknown, revoked and malformed tokens all answer the same 404, and the
// stored hash is compared in constant time.
func (s *Service) resolve(ctx context.Context, raw string) (*Link, error) {
	if !tokenShape.MatchString(raw) {
		return nil, notFound()
	}
	want := hashToken(raw)
	var l Link
	err := s.db.WithContext(ctx).Where("token_hash = ? AND revoked_at IS NULL", want).Take(&l).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound()
	}
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(l.TokenHash), []byte(want)) != 1 {
		return nil, notFound()
	}
	return &l, nil
}

type pageCursor struct {
	After int `json:"n"`
}

// View renders the public page for a raw token. The tenant and the project come from the link row; an archived or
// deleted project answers the same 404 as an unknown token.
func (s *Service) View(ctx context.Context, raw, cursor string, limit int) (*Page, error) {
	l, err := s.resolve(ctx, raw)
	if err != nil {
		return nil, err
	}
	var cur pageCursor
	if err := kernel.DecodeCursor(cursor, &cur); err != nil {
		return nil, err
	}
	pr, err := s.project(ctx, l)
	if err != nil {
		return nil, err
	}
	cols, err := s.columns(ctx, l)
	if err != nil {
		return nil, err
	}
	tasks, next, err := s.tasks(ctx, l, pr.Key, cur.After, limit)
	if err != nil {
		return nil, err
	}
	return &Page{Project: *pr, Columns: cols, Tasks: tasks, Limit: limit, Next: next}, nil
}

func (s *Service) project(ctx context.Context, l *Link) (*Project, error) {
	var rows []Project
	err := s.db.WithContext(ctx).Raw(`SELECT key, name FROM projects WHERE id = ? AND tenant_id = ? AND deleted_at IS NULL AND status <> 'archived'`,
		l.ProjectID, l.TenantID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, notFound()
	}
	return &rows[0], nil
}

func (s *Service) columns(ctx context.Context, l *Link) ([]Column, error) {
	cols := []Column{}
	err := s.db.WithContext(ctx).Raw(`SELECT name, category, color FROM board_columns WHERE project_id = ? AND tenant_id = ? ORDER BY position, id`,
		l.ProjectID, l.TenantID).Scan(&cols).Error
	return cols, err
}

type taskRow struct {
	ID       uuid.UUID
	Number   int
	Title    string
	Column   string
	Category string
	Priority string
	DueAt    *time.Time
	Desc     string
	Total    int
	Done     int
}

// tasks reads one page of the live, unarchived tasks in ticket order, plus their label names.
func (s *Service) tasks(ctx context.Context, l *Link, key string, after, limit int) ([]Task, string, error) {
	var rows []taskRow
	err := s.db.WithContext(ctx).Raw(`SELECT t.id, t.number, t.title, c.name AS "column", c.category, t.priority, t.due_at, t.description AS "desc",
		(SELECT count(*) FROM tasks s WHERE s.parent_task_id = t.id AND s.deleted_at IS NULL) AS total,
		(SELECT count(*) FROM tasks s JOIN board_columns sc ON sc.id = s.column_id WHERE s.parent_task_id = t.id AND s.deleted_at IS NULL AND sc.category = 'done') AS done
		FROM tasks t JOIN board_columns c ON c.id = t.column_id
		WHERE t.project_id = ? AND t.tenant_id = ? AND t.deleted_at IS NULL AND t.archived_at IS NULL AND t.number > ?
		ORDER BY t.number LIMIT ?`, l.ProjectID, l.TenantID, after, limit+1).Scan(&rows).Error
	if err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, limit)
	labels, err := s.labelNames(ctx, l, rows)
	if err != nil {
		return nil, "", err
	}
	out := make([]Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, publicTask(r, key, labels[r.ID], l.ShowDescriptions))
	}
	next := ""
	if more {
		next = kernel.EncodeCursor(pageCursor{After: rows[len(rows)-1].Number})
	}
	return out, next, nil
}

func publicTask(r taskRow, key string, labels []string, withDesc bool) Task {
	t := Task{Key: key + "-" + itoa(r.Number), Title: r.Title, Column: r.Column, Priority: r.Priority, DueAt: r.DueAt, Labels: labels, Progress: progress(r)}
	if t.Labels == nil {
		t.Labels = []string{}
	}
	if withDesc {
		d := cleanDescription(r.Desc)
		t.Description = &d
	}
	return t
}

// progress is the share of finished subtasks, or 100 for a finished task without subtasks.
func progress(r taskRow) int {
	switch {
	case r.Total > 0:
		return r.Done * 100 / r.Total
	case r.Category == "done":
		return 100
	}
	return 0
}

// cleanDescription hides mentions (a user id and a name) and caps the length.
func cleanDescription(d string) string {
	d = mentionToken.ReplaceAllString(d, "@someone")
	if r := []rune(d); len(r) > MaxDescription {
		d = strings.TrimSpace(string(r[:MaxDescription]))
	}
	return d
}

func (s *Service) labelNames(ctx context.Context, l *Link, rows []taskRow) (map[uuid.UUID][]string, error) {
	out := map[uuid.UUID][]string{}
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	var found []struct {
		TaskID uuid.UUID
		Name   string
	}
	err := s.db.WithContext(ctx).Raw(`SELECT tl.task_id, l.name FROM task_labels tl JOIN labels l ON l.id = tl.label_id
		WHERE tl.tenant_id = ? AND tl.task_id IN ? ORDER BY lower(l.name), l.id`, l.TenantID, ids).Scan(&found).Error
	for _, f := range found {
		out[f.TaskID] = append(out[f.TaskID], f.Name)
	}
	return out, err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
