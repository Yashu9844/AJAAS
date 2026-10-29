package pmmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/jaas/jaas/pkg/pmclient"
)

// fakeAPI is an in-memory API; calls records what the tools asked for.
type fakeAPI struct {
	calls   []string
	columns []pmclient.Map
	fail    error
	tasks   []pmclient.Map
}

func (f *fakeAPI) rec(s string) { f.calls = append(f.calls, s) }

func (f *fakeAPI) Projects(_ context.Context, _, q, _ string, _ int) (pmclient.Page, error) {
	f.rec("projects:" + q)
	return pmclient.Page{Items: []pmclient.Map{{"id": "11111111-1111-4111-8111-111111111111", "key": "ENG", "name": "Engineering", "created_at": "x"}}}, f.fail
}
func (f *fakeAPI) Tasks(_ context.Context, pid string, q url.Values) (pmclient.Page, error) {
	f.rec("tasks:" + pid + "?" + q.Encode())
	return pmclient.Page{Items: f.tasks, Next: "tok"}, f.fail
}
func (f *fakeAPI) Task(_ context.Context, id string) (pmclient.Map, error) {
	f.rec("task:" + id)
	return pmclient.Map{"id": "t1", "key": "ENG-1", "title": "T", "project_id": "11111111-1111-4111-8111-111111111111", "version": 3.0,
		"description": strings.Repeat("x", 2000)}, f.fail
}
func (f *fakeAPI) CreateTask(_ context.Context, pid string, in pmclient.Map) (pmclient.Map, error) {
	f.rec("create:" + pid)
	return pmclient.Map{"id": "t2", "title": in["title"]}, f.fail
}
func (f *fakeAPI) UpdateTask(_ context.Context, id string, in pmclient.Map) (pmclient.Map, error) {
	f.rec("update:" + id)
	return pmclient.Map{"id": "t1", "title": in["title"]}, f.fail
}
func (f *fakeAPI) MoveTask(_ context.Context, id, col string) (pmclient.Map, error) {
	f.rec("move:" + id + ":" + col)
	return pmclient.Map{"id": "t1"}, f.fail
}
func (f *fakeAPI) Columns(context.Context, string) ([]pmclient.Map, error) {
	return f.columns, f.fail
}
func (f *fakeAPI) Comment(_ context.Context, id, body string) (pmclient.Map, error) {
	f.rec("comment:" + id)
	return pmclient.Map{"id": "c1", "body": body}, f.fail
}
func (f *fakeAPI) Comments(context.Context, string, string, int) (pmclient.Page, error) {
	return pmclient.Page{Items: []pmclient.Map{{"id": "c1", "body": strings.Repeat("y", 900)}}}, f.fail
}
func (f *fakeAPI) MyWork(context.Context) (pmclient.Map, error) {
	return pmclient.Map{"overdue": []any{map[string]any{"id": "t1", "key": "ENG-1", "title": "late"}}, "later": []any{}}, f.fail
}
func (f *fakeAPI) Search(context.Context, string, int) ([]pmclient.Map, error) {
	return []pmclient.Map{{"id": "t1", "key": "ENG-1", "title": "found"}}, f.fail
}
func (f *fakeAPI) LogTime(_ context.Context, id string, m int, _ string) (pmclient.Map, error) {
	f.rec("time:" + id)
	return pmclient.Map{"id": "e1"}, f.fail
}

func rpc(t *testing.T, s *Server, method string, params any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	out := s.Handle(context.Background(), raw)
	if out == nil {
		t.Fatalf("%s: no response", method)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("%s: %v (%s)", method, err, out)
	}
	return m
}

func call(t *testing.T, s *Server, name string, a map[string]any) (string, bool) {
	t.Helper()
	res := rpc(t, s, "tools/call", map[string]any{"name": name, "arguments": a})["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	return text, res["isError"].(bool)
}

func TestProtocolBasics(t *testing.T) {
	s := New(&fakeAPI{}, "jaas-pm", "test")
	init := rpc(t, s, "initialize", map[string]any{})["result"].(map[string]any)
	if init["protocolVersion"] != ProtocolVersion || !strings.Contains(init["instructions"].(string), "untrusted") {
		t.Fatalf("initialize: %v", init)
	}
	if s.Handle(context.Background(), []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)) != nil {
		t.Fatal("notifications get no answer")
	}
	if e := rpc(t, s, "nope", nil)["error"].(map[string]any); e["code"] != float64(-32601) {
		t.Fatalf("unknown method: %v", e)
	}
	bad := s.Handle(context.Background(), []byte(`not json`))
	if !strings.Contains(string(bad), "-32700") {
		t.Fatalf("parse error: %s", bad)
	}
	if rpc(t, s, "ping", nil)["result"] == nil {
		t.Fatal("ping")
	}
	tools := rpc(t, s, "tools/list", nil)["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 11 {
		t.Fatalf("tools: %d", len(tools))
	}
	for _, x := range tools {
		tool := x.(map[string]any)
		if tool["name"] == "" || tool["description"] == "" || tool["inputSchema"].(map[string]any)["type"] != "object" {
			t.Fatalf("incomplete tool %v", tool)
		}
	}
	if rpc(t, s, "tools/call", map[string]any{"name": "nope"})["error"] == nil || rpc(t, s, "tools/call", "x")["error"] == nil {
		t.Fatal("bad tool calls are protocol errors")
	}
}

func TestServeReadsLinesAndWritesResponses(t *testing.T) {
	s := New(&fakeAPI{}, "jaas-pm", "test")
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n" + `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n")
	var out bytes.Buffer
	if err := s.Serve(context.Background(), in, &out); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(out.String()), "\n"); len(lines) != 2 {
		t.Fatalf("one response per request: %q", out.String())
	}
}

func TestToolsAreCompactAndEconomical(t *testing.T) {
	api := &fakeAPI{tasks: make([]pmclient.Map, 60), columns: []pmclient.Map{{"id": "c-done", "name": "Done", "key": "done"}}}
	for i := range api.tasks {
		api.tasks[i] = pmclient.Map{"id": "x", "key": "ENG-1", "title": "t", "created_at": "drop me", "tenant": "drop me", "description": strings.Repeat("d", 5000)}
	}
	s := New(api, "jaas-pm", "test")

	text, isErr := call(t, s, "list_tasks", map[string]any{"project": "ENG", "column": "done", "limit": 500})
	if isErr || strings.Contains(text, "drop me") || !strings.Contains(text, `"next_cursor":"tok"`) || !strings.Contains(text, "omitted") {
		t.Fatalf("list_tasks must be compact, capped and paged: %.300s", text)
	}
	if len([]rune(text)) > maxResult+200 {
		t.Fatalf("results are bounded: %d", len([]rune(text)))
	}
	if !strings.Contains(api.calls[1], "column_id=c-done") || !strings.Contains(api.calls[1], "limit=50") {
		t.Fatalf("project and column names are resolved, limit capped: %v", api.calls)
	}
	text, _ = call(t, s, "get_task", map[string]any{"task": "ENG-1"})
	if !strings.Contains(text, "truncated") {
		t.Fatalf("long descriptions are truncated: %.200s", text)
	}
	text, _ = call(t, s, "get_task", map[string]any{"task": "ENG-1", "detail": true})
	if strings.Contains(text, "truncated") {
		t.Fatal("detail returns full text")
	}
}

func TestWriteToolsAndMoveByColumnName(t *testing.T) {
	api := &fakeAPI{columns: []pmclient.Map{{"id": "c-prog", "name": "In progress", "key": "in-progress"}}}
	s := New(api, "jaas-pm", "test")
	if _, e := call(t, s, "create_task", map[string]any{"project": "ENG", "title": "New"}); e {
		t.Fatal("create")
	}
	if _, e := call(t, s, "update_task", map[string]any{"task": "ENG-1"}); !e {
		t.Fatal("update without changes is an error")
	}
	if _, e := call(t, s, "update_task", map[string]any{"task": "ENG-1", "title": "Renamed"}); e {
		t.Fatal("update")
	}
	if _, e := call(t, s, "move_task", map[string]any{"task": "ENG-1", "column": "in progress"}); e || api.calls[len(api.calls)-1] != "move:ENG-1:c-prog" {
		t.Fatalf("move by name: %v", api.calls)
	}
	if text, e := call(t, s, "move_task", map[string]any{"task": "ENG-1", "column": "Nowhere"}); !e || !strings.Contains(text, "no column") {
		t.Fatalf("unknown column: %s", text)
	}
	if _, e := call(t, s, "add_comment", map[string]any{"task": "ENG-1", "body": "hi"}); e {
		t.Fatal("comment")
	}
	if text, e := call(t, s, "log_time", map[string]any{"task": "ENG-1", "minutes": 0}); !e || !strings.Contains(text, "positive") {
		t.Fatalf("log_time validation: %s", text)
	}
	if _, e := call(t, s, "log_time", map[string]any{"task": "ENG-1", "minutes": 30}); e {
		t.Fatal("log_time")
	}
	for _, name := range []string{"list_projects", "my_work", "search", "list_comments"} {
		if text, e := call(t, s, name, map[string]any{"query": "x", "task": "ENG-1"}); e || text == "" {
			t.Fatalf("%s: %s", name, text)
		}
	}
}

func TestAPIErrorsAreShortAndSafe(t *testing.T) {
	api := &fakeAPI{fail: &pmclient.APIError{Status: 403, Code: "FORBIDDEN", Message: "missing permission task.update"}}
	s := New(api, "jaas-pm", "test")
	text, isErr := call(t, s, "get_task", map[string]any{"task": "ENG-1"})
	if !isErr || text != "FORBIDDEN: missing permission task.update" {
		t.Fatalf("the model sees the API code and message only: %q", text)
	}
	api.fail = errors.New("dial tcp 10.0.0.1: refused")
	if text, isErr = call(t, s, "get_task", map[string]any{"task": "ENG-1"}); !isErr || !strings.HasPrefix(text, "error:") {
		t.Fatalf("transport errors: %q", text)
	}
	if _, e := call(t, s, "list_tasks", map[string]any{}); !e {
		t.Fatal("project is required")
	}
}

func TestCompactHelpers(t *testing.T) {
	if got := pmclient.Truncate("abcdef", 3); !strings.HasPrefix(got, "abc") || !strings.Contains(got, "truncated 3") {
		t.Fatalf("truncate: %s", got)
	}
	if pmclient.Truncate("abc", 10) != "abc" || pmclient.Truncate("abc", 0) != "abc" {
		t.Fatal("short text is untouched")
	}
	items, dropped := pmclient.CompactAll([]pmclient.Map{{"id": 1}, {"id": 2}, {"id": 3}}, 2, func(m pmclient.Map) pmclient.Map { return m })
	if len(items) != 2 || dropped != 1 {
		t.Fatal("compact all")
	}
	if !pmclient.IsTicketKey("ENG-12") || pmclient.IsTicketKey("eng") || pmclient.IsTicketKey("1-2") {
		t.Fatal("ticket key")
	}
}
