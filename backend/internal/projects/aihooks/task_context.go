package aihooks

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Caps for the task pack.
const (
	capSubtasks   = 20
	capRelations  = 20
	capComments   = 5
	capActivity   = 10
	descMax       = 1500
	commentMax    = 300
	relationQuery = 41
)

type taskRow struct {
	ID        string       `json:"id"`
	Key       string       `json:"key"`
	Title     string       `json:"title"`
	Desc      string       `json:"description,omitempty" gorm:"column:description"`
	Priority  string       `json:"priority"`
	Column    string       `json:"column"`
	Category  string       `json:"column_category"`
	DueAt     *time.Time   `json:"due_at,omitempty"`
	StartOn   *kernel.Date `json:"start_on,omitempty"`
	Estimate  *int         `json:"estimate_minutes,omitempty"`
	Version   int          `json:"version"`
	Milestone *string      `json:"milestone,omitempty"`
	Sprint    *string      `json:"sprint,omitempty"`
	Parent    *string      `json:"parent,omitempty"`
}

type miniTask struct {
	Key      string `json:"key"`
	Title    string `json:"title"`
	Category string `json:"column_category"`
}

type relationRow struct {
	Type      string `json:"type"`
	Direction string `json:"direction"`
	Key       string `json:"key"`
	Title     string `json:"title"`
	Category  string `json:"column_category"`
	ProjectID string `json:"-"`
}

// resolveTask accepts a task id or a ticket key (ENG-12) and returns the id; it checks nothing about access.
func (s *Service) resolveTask(ctx context.Context, a api.Actor, ref string) (uuid.UUID, error) {
	if id, err := uuid.Parse(ref); err == nil {
		return id, nil
	}
	i := strings.LastIndex(ref, "-")
	n, err := strconv.Atoi(ref[i+1:])
	if i < 1 || err != nil {
		return uuid.Nil, kernel.NotFound("task")
	}
	var ids []string
	if err := s.db.WithContext(ctx).Raw(`SELECT t.id::text FROM tasks t JOIN projects p ON p.id = t.project_id
		WHERE p.tenant_id = ? AND p.key = upper(?) AND t.number = ? AND t.deleted_at IS NULL`, a.TenantID, ref[:i], n).Scan(&ids).Error; err != nil {
		return uuid.Nil, err
	}
	if len(ids) != 1 {
		return uuid.Nil, kernel.NotFound("task")
	}
	return uuid.Parse(ids[0])
}

// TaskContext returns a task with its surroundings in one compact pack. Everything in it is readable by the caller: the
// task resolver checks project access first, and relations to tasks in projects the caller cannot read are left out.
func (s *Service) TaskContext(ctx context.Context, a api.Actor, ref string) (Pack, error) {
	id, err := s.resolveTask(ctx, a, ref)
	if err != nil {
		return Pack{}, err
	}
	t, acc, err := s.tasks.Task(ctx, s.db, a, id)
	if err != nil {
		return Pack{}, err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return Pack{}, err
	}
	data, cut, err := s.taskData(ctx, a, t)
	if err != nil {
		return Pack{}, err
	}
	data["permissions"] = api.SortedPerms(acc.Perms)
	return s.seal("task", a, data, cut), nil
}

func (s *Service) taskData(ctx context.Context, a api.Actor, t api.TaskRef) (map[string]any, []string, error) {
	var rows []taskRow
	err := s.db.WithContext(ctx).Raw(`SELECT t.id::text AS id, p.key || '-' || t.number AS key, t.title, t.description, t.priority, c.name AS "column",
		c.category AS category, t.due_at, t.start_on, t.estimate_minutes, t.version, m.name AS milestone, s.name AS sprint, par.number::text AS parent
		FROM tasks t JOIN projects p ON p.id = t.project_id JOIN board_columns c ON c.id = t.column_id
		LEFT JOIN milestones m ON m.id = t.milestone_id LEFT JOIN sprints s ON s.id = t.sprint_id LEFT JOIN tasks par ON par.id = t.parent_task_id
		WHERE t.id = ? AND t.tenant_id = ? AND t.deleted_at IS NULL`, t.ID, a.TenantID).Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	if len(rows) != 1 {
		return nil, nil, kernel.NotFound("task")
	}
	var cut []string
	task := rows[0]
	if utf8.RuneCountInString(task.Desc) > descMax {
		task.Desc, cut = string([]rune(task.Desc)[:descMax]), append(cut, "description")
	}
	data := map[string]any{"task": task}
	if err := s.taskParts(ctx, a, t, data, &cut); err != nil {
		return nil, nil, err
	}
	return data, cut, nil
}

func capList[T any](items []T, n int, name string, cut *[]string) []T {
	if len(items) > n {
		*cut = append(*cut, name)
		return items[:n]
	}
	return items
}
