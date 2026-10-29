package pmmcp

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jaas/jaas/pkg/pmclient"
)

// asked reports why a list was cut: more rows on the server (cursor) or rows dropped locally.
func listResult(key string, items []pmclient.Map, next string, dropped int) map[string]any {
	out := map[string]any{key: items, "count": len(items)}
	if next != "" {
		out["next_cursor"] = next
	}
	if dropped > 0 {
		out["omitted"] = dropped
	}
	return out
}

// projectID accepts a project id or its key (ENG) and returns the id.
func (s *Server) projectID(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", errors.New("project is required (key or id)")
	}
	if _, err := uuid.Parse(ref); err == nil {
		return ref, nil
	}
	page, err := s.api.Projects(ctx, "", ref, "", maxLimit)
	if err != nil {
		return "", err
	}
	for _, p := range page.Items {
		if k, _ := p["key"].(string); strings.EqualFold(k, ref) {
			id, _ := p["id"].(string)
			return id, nil
		}
	}
	return "", errors.New("no project with key " + ref)
}

// columnID accepts a column id, key or name of the project.
func (s *Server) columnID(ctx context.Context, projectID, ref string) (string, error) {
	if _, err := uuid.Parse(ref); err == nil {
		return ref, nil
	}
	cols, err := s.api.Columns(ctx, projectID)
	if err != nil {
		return "", err
	}
	for _, c := range cols {
		name, _ := c["name"].(string)
		key, _ := c["key"].(string)
		if strings.EqualFold(name, ref) || strings.EqualFold(key, ref) {
			id, _ := c["id"].(string)
			return id, nil
		}
	}
	return "", errors.New("no column named " + ref + "; list_tasks shows column names of the project")
}

func (s *Server) listProjects(ctx context.Context, a args) (any, error) {
	page, err := s.api.Projects(ctx, a.str("status"), a.str("query"), a.str("cursor"), a.limit())
	if err != nil {
		return nil, err
	}
	items, dropped := pmclient.CompactAll(page.Items, a.limit(), pmclient.CompactProject)
	return listResult("projects", items, page.Next, dropped), nil
}

func (s *Server) listTasks(ctx context.Context, a args) (any, error) {
	pid, err := s.projectID(ctx, a.str("project"))
	if err != nil {
		return nil, err
	}
	q := url.Values{"limit": {strconv.Itoa(a.limit())}}
	for _, k := range []string{"priority", "assignee", "query", "cursor", "sort"} {
		if v := a.str(k); v != "" {
			key := k
			if k == "query" {
				key = "q"
			}
			q.Set(key, v)
		}
	}
	if col := a.str("column"); col != "" {
		id, cerr := s.columnID(ctx, pid, col)
		if cerr != nil {
			return nil, cerr
		}
		q.Set("column_id", id)
	}
	for _, flag := range []string{"overdue", "blocked"} {
		if a.flag(flag) {
			q.Set(flag, "true")
		}
	}
	page, err := s.api.Tasks(ctx, pid, q)
	if err != nil {
		return nil, err
	}
	items, dropped := pmclient.CompactAll(page.Items, a.limit(), func(m pmclient.Map) pmclient.Map { return pmclient.CompactTask(m, false) })
	return listResult("tasks", items, page.Next, dropped), nil
}

func (s *Server) getTask(ctx context.Context, a args) (any, error) {
	t, err := s.api.Task(ctx, a.str("task"))
	if err != nil {
		return nil, err
	}
	return pmclient.CompactTask(t, a.flag("detail")), nil
}

func (s *Server) createTask(ctx context.Context, a args) (any, error) {
	pid, err := s.projectID(ctx, a.str("project"))
	if err != nil {
		return nil, err
	}
	in := pmclient.Map{"title": a.str("title")}
	copyKeys(in, a, "description", "priority", "due_at")
	t, err := s.api.CreateTask(ctx, pid, in)
	if err != nil {
		return nil, err
	}
	return pmclient.CompactTask(t, false), nil
}

func (s *Server) updateTask(ctx context.Context, a args) (any, error) {
	in := pmclient.Map{}
	copyKeys(in, a, "title", "description", "priority", "due_at")
	if len(in) == 0 {
		return nil, errors.New("nothing to change: give title, description, priority or due_at")
	}
	t, err := s.api.UpdateTask(ctx, a.str("task"), in)
	if err != nil {
		return nil, err
	}
	return pmclient.CompactTask(t, false), nil
}

func (s *Server) moveTask(ctx context.Context, a args) (any, error) {
	t, err := s.api.Task(ctx, a.str("task"))
	if err != nil {
		return nil, err
	}
	pid, _ := t["project_id"].(string)
	col, err := s.columnID(ctx, pid, a.str("column"))
	if err != nil {
		return nil, err
	}
	moved, err := s.api.MoveTask(ctx, a.str("task"), col)
	if err != nil {
		return nil, err
	}
	return pmclient.CompactTask(moved, false), nil
}

func (s *Server) addComment(ctx context.Context, a args) (any, error) {
	c, err := s.api.Comment(ctx, a.str("task"), a.str("body"))
	if err != nil {
		return nil, err
	}
	return pmclient.CompactComment(c, false), nil
}

func (s *Server) listComments(ctx context.Context, a args) (any, error) {
	page, err := s.api.Comments(ctx, a.str("task"), a.str("cursor"), a.limit())
	if err != nil {
		return nil, err
	}
	items, dropped := pmclient.CompactAll(page.Items, a.limit(), func(m pmclient.Map) pmclient.Map { return pmclient.CompactComment(m, a.flag("detail")) })
	return listResult("comments", items, page.Next, dropped), nil
}

func (s *Server) search(ctx context.Context, a args) (any, error) {
	rows, err := s.api.Search(ctx, a.str("query"), a.limit())
	if err != nil {
		return nil, err
	}
	items, dropped := pmclient.CompactAll(rows, a.limit(), func(m pmclient.Map) pmclient.Map { return pmclient.CompactTask(m, false) })
	return listResult("results", items, "", dropped), nil
}

func (s *Server) logTime(ctx context.Context, a args) (any, error) {
	m := a.num("minutes", 0)
	if m <= 0 {
		return nil, errors.New("minutes must be a positive number")
	}
	e, err := s.api.LogTime(ctx, a.str("task"), m, a.str("note"))
	if err != nil {
		return nil, err
	}
	return map[string]any{"logged_minutes": m, "entry_id": e["id"]}, nil
}

func (s *Server) myWork(ctx context.Context, a args) (any, error) {
	w, err := s.api.MyWork(ctx)
	if err != nil {
		return nil, err
	}
	return compactMyWork(w, a.limit()), nil
}

func copyKeys(dst pmclient.Map, src args, keys ...string) {
	for _, k := range keys {
		if v, ok := src[k]; ok && v != "" {
			dst[k] = v
		}
	}
}

// compactMyWork keeps the group names, their counts and the first few tasks of each group.
func compactMyWork(w pmclient.Map, limit int) pmclient.Map {
	out := pmclient.Map{}
	for group, v := range w {
		items, ok := v.([]any)
		if !ok {
			continue
		}
		maps := make([]pmclient.Map, 0, len(items))
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				maps = append(maps, m)
			}
		}
		shown, dropped := pmclient.CompactAll(maps, limit/4+1, func(m pmclient.Map) pmclient.Map { return pmclient.CompactTask(m, false) })
		g := pmclient.Map{"count": len(maps), "tasks": shown}
		if dropped > 0 {
			g["omitted"] = dropped
		}
		out[group] = g
	}
	return out
}
