package pmmcp

var (
	pTask   = prop("string", "ticket key such as ENG-12, or a task id")
	pLimit  = prop("integer", "max items to return (default 20, max 50)")
	pCursor = prop("string", "next_cursor from the previous result")
	pDetail = prop("boolean", "true to return full text instead of a short excerpt")
)

// buildTools declares the tools. Descriptions are short because they are sent to the model on every session.
func (s *Server) buildTools() []tool {
	return []tool{
		{"list_projects", "List projects you can read.", obj(nil, map[string]any{
			"query": prop("string", "filter by name or key"), "status": prop("string", "active (default) or archived"), "limit": pLimit, "cursor": pCursor}), s.listProjects},
		{"list_tasks", "List tasks of a project (compact).", obj([]string{"project"}, map[string]any{
			"project": prop("string", "project key (ENG) or id"), "column": prop("string", "column name, e.g. In progress"), "priority": prop("string", "none|low|medium|high|urgent"),
			"assignee": prop("string", "user id"), "query": prop("string", "text filter"), "overdue": prop("boolean", "only overdue"), "blocked": prop("boolean", "only blocked"),
			"sort": prop("string", "rank|created|updated|due|priority|number"), "limit": pLimit, "cursor": pCursor}), s.listTasks},
		{"get_task", "Get one task.", obj([]string{"task"}, map[string]any{"task": pTask, "detail": pDetail}), s.getTask},
		{"create_task", "Create a task.", obj([]string{"project", "title"}, map[string]any{
			"project": prop("string", "project key or id"), "title": prop("string", "1-300 chars"), "description": prop("string", "markdown"),
			"priority": prop("string", "none|low|medium|high|urgent"), "due_at": prop("string", "RFC 3339 time")}), s.createTask},
		{"update_task", "Change title, description, priority or due date of a task.", obj([]string{"task"}, map[string]any{
			"task": pTask, "title": prop("string", "new title"), "description": prop("string", "new description"), "priority": prop("string", "priority"), "due_at": prop("string", "RFC 3339 time")}), s.updateTask},
		{"move_task", "Move a task to a column by name.", obj([]string{"task", "column"}, map[string]any{"task": pTask, "column": prop("string", "column name or key")}), s.moveTask},
		{"add_comment", "Comment on a task.", obj([]string{"task", "body"}, map[string]any{"task": pTask, "body": prop("string", "markdown, max 10000 chars")}), s.addComment},
		{"list_comments", "List comments of a task (excerpts).", obj([]string{"task"}, map[string]any{"task": pTask, "detail": pDetail, "limit": pLimit, "cursor": pCursor}), s.listComments},
		{"my_work", "Tasks assigned to you, grouped by due date, with counts.", obj(nil, map[string]any{"limit": pLimit}), s.myWork},
		{"search", "Search tasks across readable projects.", obj([]string{"query"}, map[string]any{"query": prop("string", "words or a ticket key"), "limit": pLimit}), s.search},
		{"log_time", "Log time spent on a task.", obj([]string{"task", "minutes"}, map[string]any{"task": pTask, "minutes": prop("integer", "minutes spent"), "note": prop("string", "short note")}), s.logTime},
	}
}
