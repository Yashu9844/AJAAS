package pmmcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/jaas/jaas/pkg/pmclient"
)

// API is what the tools need from the REST client (satisfied by *pmclient.Client; tests use a fake).
type API interface {
	Projects(ctx context.Context, status, query, cursor string, limit int) (pmclient.Page, error)
	Tasks(ctx context.Context, projectID string, q url.Values) (pmclient.Page, error)
	Task(ctx context.Context, idOrKey string) (pmclient.Map, error)
	CreateTask(ctx context.Context, projectID string, in pmclient.Map) (pmclient.Map, error)
	UpdateTask(ctx context.Context, idOrKey string, in pmclient.Map) (pmclient.Map, error)
	MoveTask(ctx context.Context, idOrKey, columnID string) (pmclient.Map, error)
	Columns(ctx context.Context, projectID string) ([]pmclient.Map, error)
	Comment(ctx context.Context, idOrKey, body string) (pmclient.Map, error)
	Comments(ctx context.Context, idOrKey, cursor string, limit int) (pmclient.Page, error)
	MyWork(ctx context.Context) (pmclient.Map, error)
	Search(ctx context.Context, query string, limit int) ([]pmclient.Map, error)
	LogTime(ctx context.Context, idOrKey string, minutes int, note string) (pmclient.Map, error)
}

// Result limits (token economy).
const (
	defaultLimit = 20
	maxLimit     = 50
	maxResult    = 12000 // characters of one tool result
)

type tool struct {
	name        string
	description string
	schema      map[string]any
	run         func(ctx context.Context, args args) (any, error)
}

type args map[string]any

func (a args) str(k string) string { s, _ := a[k].(string); return s }
func (a args) flag(k string) bool  { b, _ := a[k].(bool); return b }
func (a args) num(k string, def int) int {
	if f, ok := a[k].(float64); ok && f > 0 {
		return int(f)
	}
	return def
}

func (a args) limit() int {
	n := a.num("limit", defaultLimit)
	if n > maxLimit {
		return maxLimit
	}
	return n
}

func obj(required []string, props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func prop(typ, desc string) map[string]any { return map[string]any{"type": typ, "description": desc} }

func (s *Server) listTools() []map[string]any {
	out := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, map[string]any{"name": t.name, "description": t.description, "inputSchema": t.schema})
	}
	return out
}

type callParams struct {
	Name      string `json:"name"`
	Arguments args   `json:"arguments"`
}

func (s *Server) callTool(ctx context.Context, raw json.RawMessage) (any, *rpcError) {
	var p callParams
	if err := json.Unmarshal(raw, &p); err != nil || p.Name == "" {
		return nil, &rpcError{Code: -32602, Message: "invalid params"}
	}
	for _, t := range s.tools {
		if t.name != p.Name {
			continue
		}
		if p.Arguments == nil {
			p.Arguments = args{}
		}
		res, err := t.run(ctx, p.Arguments)
		if err != nil {
			return toolResult(errorText(err), true), nil
		}
		return toolResult(render(res), false), nil
	}
	return nil, &rpcError{Code: -32602, Message: "unknown tool " + p.Name}
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": isError}
}

// errorText turns an API error into a short message a model can act on; internals are never included.
func errorText(err error) string {
	var ae *pmclient.APIError
	if errors.As(err, &ae) {
		return ae.Code + ": " + ae.Message
	}
	return "error: " + err.Error()
}

// render serialises a result compactly and caps it, telling the model how to narrow the request.
func render(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "error: result could not be encoded"
	}
	s := string(raw)
	if r := []rune(s); len(r) > maxResult {
		return string(r[:maxResult]) + "… [result truncated; narrow the request with limit, query or a specific task]"
	}
	return s
}
