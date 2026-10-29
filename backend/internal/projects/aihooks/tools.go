// Package aihooks is the seam between Module 2 and a separate AI module (M9). It contains no model, no prompt and no
// provider: only what an AI needs from M2 and cannot safely get any other way - a machine-readable catalogue of the actions
// M2 offers (with the permission each needs and its risk), compact context packs sized for a model context, and an
// incremental document feed for indexing. Every read is authorised through the same Authorizer as the REST API, so an AI
// acting with a user token can see and do exactly what that user may - never more. See docs/modules/m9-ai-assistant/ai-integration.md.
package aihooks

import "github.com/jaas/jaas/internal/projects/api"

// Risk classes tell an AI runtime how careful to be before calling a tool.
const (
	RiskRead        = "read"        // no state change; safe to call freely
	RiskWrite       = "write"       // changes data the user owns; reversible by another call
	RiskDestructive = "destructive" // hard or impossible to undo; a runtime should ask the user first
)

// Route is the REST call behind a tool; {id} and {key} placeholders are filled from the tool arguments.
type Route struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

// Tool is one action an AI may take. It mirrors an existing REST operation - there is no separate AI-only write path.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Route       Route          `json:"route"`
	Requires    api.Perm       `json:"requires,omitempty"` // project permission; empty = any signed-in user
	Risk        string         `json:"risk"`
	Confirm     bool           `json:"confirm"` // the runtime should obtain user confirmation before calling
	Idempotent  bool           `json:"idempotency_key"`
	InputSchema map[string]any `json:"input_schema"`
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func schema(required []string, props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

var (
	taskArg    = str("task id or ticket key such as ENG-12")
	projectArg = str("project id")
)

// Catalogue is every tool, in a stable order. It is the single list both the MCP server and the AI module should offer;
// TestEveryToolRouteExists keeps it in step with the router.
func Catalogue() []Tool {
	return []Tool{
		{"list_projects", "List projects the user can read.", Route{"GET", "/projects"}, "", RiskRead, false, false,
			schema(nil, map[string]any{"q": str("name or key filter"), "limit": map[string]any{"type": "integer"}})},
		{"get_project_context", "Compact overview of a project: columns with counts, overdue/blocked totals, milestones, sprint, labels, custom fields, and what the user may do.",
			Route{"GET", "/ai-hooks/context/projects/{id}"}, api.ProjectRead, RiskRead, false, false, schema([]string{"id"}, map[string]any{"id": projectArg})},
		{"list_tasks", "List and filter the tasks of a project.", Route{"GET", "/projects/{id}/tasks"}, api.TaskRead, RiskRead, false, false,
			schema([]string{"id"}, map[string]any{"id": projectArg, "column_id": str("column"), "priority": str("priority"), "assignee": str("user id"), "q": str("text"), "limit": map[string]any{"type": "integer"}})},
		{"get_task", "One task by id or ticket key.", Route{"GET", "/tickets/{key}"}, api.TaskRead, RiskRead, false, false, schema([]string{"key"}, map[string]any{"key": taskArg})},
		{"get_task_context", "A task with its subtasks, relations, latest comments, activity and custom fields in one compact pack.",
			Route{"GET", "/ai-hooks/context/tasks/{id}"}, api.TaskRead, RiskRead, false, false, schema([]string{"id"}, map[string]any{"id": taskArg})},
		{"search", "Search tasks across the projects the user can read.", Route{"GET", "/search"}, "", RiskRead, false, false,
			schema([]string{"q"}, map[string]any{"q": str("words or a ticket key")})},
		{"my_work", "Tasks assigned to the user, grouped by due date.", Route{"GET", "/my-work"}, "", RiskRead, false, false, schema(nil, map[string]any{})},
		{"create_task", "Create a task.", Route{"POST", "/projects/{id}/tasks"}, api.TaskCreate, RiskWrite, false, true,
			schema([]string{"id", "title"}, map[string]any{"id": projectArg, "title": str("1-300 characters"), "description": str("markdown"), "priority": str("none|low|medium|high|urgent")})},
		{"update_task", "Change title, description, priority or dates; send the task version.", Route{"PATCH", "/tasks/{id}"}, api.TaskUpdate, RiskWrite, false, false,
			schema([]string{"id", "version"}, map[string]any{"id": taskArg, "version": map[string]any{"type": "integer"}, "title": str("title"), "description": str("description")})},
		{"move_task", "Move a task to a column.", Route{"POST", "/tasks/{id}/move"}, api.TaskUpdate, RiskWrite, false, false,
			schema([]string{"id", "column_id"}, map[string]any{"id": taskArg, "column_id": str("column id"), "version": map[string]any{"type": "integer"}})},
		{"assign_task", "Assign project members to a task.", Route{"POST", "/tasks/{id}/assignees"}, api.TaskAssign, RiskWrite, false, false,
			schema([]string{"id", "user_ids"}, map[string]any{"id": taskArg, "user_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}})},
		{"add_comment", "Comment on a task.", Route{"POST", "/tasks/{id}/comments"}, api.TaskComment, RiskWrite, false, true,
			schema([]string{"id", "body"}, map[string]any{"id": taskArg, "body": str("markdown, max 10000 characters")})},
		{"log_time", "Log time spent on a task.", Route{"POST", "/tasks/{id}/time"}, api.TimeLog, RiskWrite, false, true,
			schema([]string{"id", "started_at", "minutes"}, map[string]any{"id": taskArg, "started_at": str("RFC 3339"), "minutes": map[string]any{"type": "integer"}})},
		{"set_custom_fields", "Set or clear custom field values by key.", Route{"PUT", "/tasks/{id}/custom-fields"}, api.TaskUpdate, RiskWrite, false, false,
			schema([]string{"id", "values"}, map[string]any{"id": taskArg, "values": map[string]any{"type": "object"}})},
		{"bulk_update_tasks", "Change up to 100 tasks at once (column, priority, labels, assignees).", Route{"POST", "/projects/{id}/tasks/bulk"}, api.TaskUpdate, RiskWrite, true, true,
			schema([]string{"id", "task_ids"}, map[string]any{"id": projectArg, "task_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}})},
		{"archive_task", "Archive a task (reversible).", Route{"POST", "/tasks/{id}/archive"}, api.TaskUpdate, RiskWrite, false, false, schema([]string{"id"}, map[string]any{"id": taskArg})},
		{"delete_task", "Delete a task (soft delete; purged after 30 days).", Route{"DELETE", "/tasks/{id}"}, api.TaskDelete, RiskDestructive, true, false, schema([]string{"id"}, map[string]any{"id": taskArg})},
	}
}
