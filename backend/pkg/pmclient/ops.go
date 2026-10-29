package pmclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// Map is a decoded JSON object.
type Map = map[string]any

// Page is one page of a cursor-paged list.
type Page struct {
	Items []Map
	Next  string
}

var ticketKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{1,9}-[0-9]+$`)

// IsTicketKey reports whether s looks like KEY-12.
func IsTicketKey(s string) bool { return ticketKey.MatchString(s) }

func nextCursor(meta json.RawMessage) string {
	var m struct {
		Next *string `json:"next_cursor"`
	}
	if json.Unmarshal(meta, &m) == nil && m.Next != nil {
		return *m.Next
	}
	return ""
}

func (c *Client) list(ctx context.Context, path string, q url.Values) (Page, error) {
	var items []Map
	meta, err := c.Call(ctx, "GET", path, CallOpts{Query: q}, &items)
	return Page{Items: items, Next: nextCursor(meta)}, err
}

func (c *Client) one(ctx context.Context, method, path string, body any) (Map, error) {
	var m Map
	_, err := c.Call(ctx, method, path, CallOpts{Body: body}, &m)
	return m, err
}

// Projects lists the projects the user can read (status "" = active).
func (c *Client) Projects(ctx context.Context, status, query, cursor string, limit int) (Page, error) {
	q := url.Values{}
	set(q, "status", status)
	set(q, "q", query)
	set(q, "cursor", cursor)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return c.list(ctx, "/projects", q)
}

// Project returns one project.
func (c *Client) Project(ctx context.Context, id string) (Map, error) {
	return c.one(ctx, "GET", "/projects/"+url.PathEscape(id), nil)
}

// Columns lists the board columns of a project.
func (c *Client) Columns(ctx context.Context, projectID string) ([]Map, error) {
	var out []Map
	_, err := c.Call(ctx, "GET", "/projects/"+url.PathEscape(projectID)+"/columns", CallOpts{}, &out)
	return out, err
}

// Tasks lists the tasks of a project; q carries the API filters (column_id, assignee, priority, q, sort, ...).
func (c *Client) Tasks(ctx context.Context, projectID string, q url.Values) (Page, error) {
	return c.list(ctx, "/projects/"+url.PathEscape(projectID)+"/tasks", q)
}

// Task returns a task by id or by ticket key (KEY-12).
func (c *Client) Task(ctx context.Context, idOrKey string) (Map, error) {
	if IsTicketKey(idOrKey) {
		return c.one(ctx, "GET", "/tickets/"+url.PathEscape(idOrKey), nil)
	}
	return c.one(ctx, "GET", "/tasks/"+url.PathEscape(idOrKey), nil)
}

// CreateTask creates a task in a project.
func (c *Client) CreateTask(ctx context.Context, projectID string, in Map) (Map, error) {
	return c.one(ctx, "POST", "/projects/"+url.PathEscape(projectID)+"/tasks", in)
}

// taskID resolves a ticket key to the task id and its current version.
func (c *Client) taskID(ctx context.Context, idOrKey string) (string, float64, error) {
	t, err := c.Task(ctx, idOrKey)
	if err != nil {
		return "", 0, err
	}
	id, _ := t["id"].(string)
	v, _ := t["version"].(float64)
	if id == "" {
		return "", 0, errors.New("task has no id")
	}
	return id, v, nil
}

// UpdateTask edits a task; the current version is fetched when in has none (optimistic concurrency still applies).
func (c *Client) UpdateTask(ctx context.Context, idOrKey string, in Map) (Map, error) {
	id, v, err := c.taskID(ctx, idOrKey)
	if err != nil {
		return nil, err
	}
	if _, ok := in["version"]; !ok {
		in["version"] = v
	}
	return c.one(ctx, "PATCH", "/tasks/"+url.PathEscape(id), in)
}

// MoveTask moves a task to a column.
func (c *Client) MoveTask(ctx context.Context, idOrKey, columnID string) (Map, error) {
	id, v, err := c.taskID(ctx, idOrKey)
	if err != nil {
		return nil, err
	}
	return c.one(ctx, "POST", "/tasks/"+url.PathEscape(id)+"/move", Map{"column_id": columnID, "version": v})
}

// Comment adds a comment to a task.
func (c *Client) Comment(ctx context.Context, idOrKey, body string) (Map, error) {
	id, _, err := c.taskID(ctx, idOrKey)
	if err != nil {
		return nil, err
	}
	return c.one(ctx, "POST", "/tasks/"+url.PathEscape(id)+"/comments", Map{"body": body})
}

// Comments lists the comments of a task.
func (c *Client) Comments(ctx context.Context, idOrKey, cursor string, limit int) (Page, error) {
	id, _, err := c.taskID(ctx, idOrKey)
	if err != nil {
		return Page{}, err
	}
	q := url.Values{}
	set(q, "cursor", cursor)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return c.list(ctx, "/tasks/"+url.PathEscape(id)+"/comments", q)
}

// MyWork returns the grouped My work view.
func (c *Client) MyWork(ctx context.Context) (Map, error) { return c.one(ctx, "GET", "/my-work", nil) }

// Search runs a tenant-scoped search over the projects the user can read.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Map, error) {
	q := url.Values{"q": {query}}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	var out []Map
	_, err := c.Call(ctx, "GET", "/search", CallOpts{Query: q}, &out)
	return out, err
}

// LogTime logs minutes on a task.
func (c *Client) LogTime(ctx context.Context, idOrKey string, minutes int, note string) (Map, error) {
	id, _, err := c.taskID(ctx, idOrKey)
	if err != nil {
		return nil, err
	}
	started := time.Now().UTC().Add(-time.Duration(minutes) * time.Minute).Format(time.RFC3339)
	return c.one(ctx, "POST", "/tasks/"+url.PathEscape(id)+"/time", Map{"started_at": started, "minutes": minutes, "note": note})
}

func set(q url.Values, k, v string) {
	if v != "" {
		q.Set(k, v)
	}
}
