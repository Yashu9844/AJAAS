package aihooks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// docMax bounds the text of one indexed document; longer text is cut and flagged.
const docMax = 4000

// Document is one piece of text an AI runtime may index (for retrieval) together with what it needs to keep the index
// current: a stable id, a content hash, the last change time and a deletion marker.
type Document struct {
	Kind      string    `json:"kind"` // task or comment
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Key       string    `json:"key"`
	UpdatedAt time.Time `json:"updated_at"`
	Deleted   bool      `json:"deleted"`
	Hash      string    `json:"content_hash"`
	Truncated bool      `json:"truncated,omitempty"`
	Text      string    `json:"text,omitempty"`
}

// DocQuery selects a page of the document feed.
type DocQuery struct {
	ProjectID uuid.UUID
	Kind      string
	Cursor    string
	Limit     int
}

// docScan is the decoded position and page size of one feed query.
type docScan struct {
	projectID uuid.UUID
	cursor    docCursor
	n         int
}

type docCursor struct {
	At string `json:"at"`
	ID string `json:"id"`
}

type docRow struct {
	ID        string
	TaskID    string
	Key       string
	Body      string
	UpdatedAt time.Time
	Deleted   bool
}

// Documents pages through the tasks (kind task) or comments (kind comment) of one project that changed after the cursor,
// oldest change first, including tombstones for deleted items so an index can drop them. The caller needs task.read in the
// project; an index built from this feed must still filter its results by ReadableProjects when it answers a user.
func (s *Service) Documents(ctx context.Context, a api.Actor, q DocQuery) ([]Document, string, error) {
	projectID, kind, cursor, limit := q.ProjectID, q.Kind, q.Cursor, q.Limit
	acc, err := s.authz.Access(ctx, nil, a, projectID)
	if err != nil {
		return nil, "", err
	}
	if err := acc.Require(api.TaskRead); err != nil {
		return nil, "", err
	}
	var cur docCursor
	if err := kernel.DecodeCursor(cursor, &cur); err != nil {
		return nil, "", err
	}
	limit = clampLimit(limit)
	var rows []docRow
	switch kind {
	case "", "task":
		kind, err = "task", s.taskDocs(ctx, a, docScan{projectID, cur, limit + 1}, &rows)
	case "comment":
		err = s.commentDocs(ctx, a, docScan{projectID, cur, limit + 1}, &rows)
	default:
		return nil, "", kernel.Invalid("kind", "must be task or comment")
	}
	if err != nil {
		return nil, "", err
	}
	rows, more := kernel.Trim(rows, limit)
	next := ""
	if more {
		last := rows[len(rows)-1]
		next = kernel.EncodeCursor(docCursor{At: last.UpdatedAt.UTC().Format(time.RFC3339Nano), ID: last.ID})
	}
	return toDocuments(kind, rows), next, nil
}

func clampLimit(n int) int {
	if n < 1 {
		return 50
	}
	if n > 100 {
		return 100
	}
	return n
}

func (s *Service) taskDocs(ctx context.Context, a api.Actor, d docScan, out *[]docRow) error {
	pid, cur, n := d.projectID, d.cursor, d.n
	after := "TRUE"
	args := []any{pid, a.TenantID}
	if cur.ID != "" {
		after = "(t.updated_at, t.id) > (?::timestamptz, ?::uuid)"
		args = append(args, cur.At, cur.ID)
	}
	args = append(args, n)
	return s.db.WithContext(ctx).Raw(`SELECT t.id::text AS id, t.id::text AS task_id, p.key || '-' || t.number AS key,
		t.title || E'\n' || t.description AS body, t.updated_at AS updated_at, (t.deleted_at IS NOT NULL) AS deleted
		FROM tasks t JOIN projects p ON p.id = t.project_id
		WHERE t.project_id = ? AND t.tenant_id = ? AND (t.deleted_at IS NULL OR t.deleted_at > now() - interval '30 days') AND `+after+`
		ORDER BY t.updated_at, t.id LIMIT ?`, args...).Scan(out).Error
}

func (s *Service) commentDocs(ctx context.Context, a api.Actor, d docScan, out *[]docRow) error {
	pid, cur, n := d.projectID, d.cursor, d.n
	after := "TRUE"
	args := []any{pid, a.TenantID}
	if cur.ID != "" {
		after = "(COALESCE(c.edited_at, c.created_at), c.id) > (?::timestamptz, ?::uuid)"
		args = append(args, cur.At, cur.ID)
	}
	args = append(args, n)
	return s.db.WithContext(ctx).Raw(`SELECT c.id::text AS id, c.task_id::text AS task_id, p.key || '-' || t.number AS key,
		CASE WHEN c.deleted_at IS NULL THEN c.body ELSE '' END AS body, COALESCE(c.edited_at, c.created_at) AS updated_at, (c.deleted_at IS NOT NULL) AS deleted
		FROM comments c JOIN tasks t ON t.id = c.task_id JOIN projects p ON p.id = t.project_id
		WHERE t.project_id = ? AND c.tenant_id = ? AND t.deleted_at IS NULL AND `+after+`
		ORDER BY COALESCE(c.edited_at, c.created_at), c.id LIMIT ?`, args...).Scan(out).Error
}

func toDocuments(kind string, rows []docRow) []Document {
	out := make([]Document, 0, len(rows))
	for _, r := range rows {
		body := strings.TrimSpace(r.Body)
		d := Document{Kind: kind, ID: r.ID, TaskID: r.TaskID, Key: r.Key, UpdatedAt: r.UpdatedAt, Deleted: r.Deleted}
		sum := sha256.Sum256([]byte(body))
		d.Hash = hex.EncodeToString(sum[:8])
		if !r.Deleted {
			if utf8.RuneCountInString(body) > docMax {
				body, d.Truncated = string([]rune(body)[:docMax]), true
			}
			d.Text = body
		}
		out = append(out, d)
	}
	return out
}
