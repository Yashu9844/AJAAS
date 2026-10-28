package task

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func (h handler) linkRoutes(g *gin.RouterGroup) {
	g.POST("/tasks/:id/assignees", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in UserIDsInput) {
			v, err := h.s.AddAssignees(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.DELETE("/tasks/:id/assignees/:user_id", withSub("user_id", func(c *gin.Context, a api.Actor, id, user uuid.UUID) {
		v, err := h.s.RemoveAssignee(c.Request.Context(), a, id, user)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.POST("/tasks/:id/labels", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in LabelIDsInput) {
			v, err := h.s.AddLabels(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.DELETE("/tasks/:id/labels/:label_id", withSub("label_id", func(c *gin.Context, a api.Actor, id, label uuid.UUID) {
		v, err := h.s.RemoveLabel(c.Request.Context(), a, id, label)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
}

func (h handler) readRoutes(g *gin.RouterGroup) {
	g.GET("/tasks/:id/subtasks", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		f := ListFilter{Cursor: c.Query("cursor"), Limit: kernel.ClampLimit(c.Query("limit"))}
		rows, next, err := h.s.Subtasks(c.Request.Context(), a, id, f)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, f.Limit, next), err)
	}))
	g.GET("/tasks/:id/relations", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := h.s.Relations(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/tasks/:id/relations", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in RelationInput) {
			r, err := h.s.AddRelation(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, r, err)
		})
	}))
	g.DELETE("/tasks/:id/relations/:relation_id", withSub("relation_id", func(c *gin.Context, a api.Actor, id, rel uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"removed": true}, h.s.RemoveRelation(c.Request.Context(), a, id, rel))
	}))
	g.GET("/tasks/:id/activity", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		f := activityFilter(c)
		rows, next, err := h.s.TaskActivity(c.Request.Context(), a, id, f)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, f.Limit, next), err)
	}))
}

func activityFilter(c *gin.Context) ActivityFilter {
	return ActivityFilter{Cursor: c.Query("cursor"), Limit: kernel.ClampLimit(c.Query("limit")), Asc: c.Query("order") == "asc"}
}

// queryBool reads an optional boolean parameter; ok is false (and 400 written) when it is present but malformed.
func queryBool(c *gin.Context, name string) (*bool, bool) {
	switch c.Query(name) {
	case "":
		return nil, true
	case "true":
		t := true
		return &t, true
	case "false":
		f := false
		return &f, true
	}
	kernel.Fail(c, kernel.Invalid(name, "must be true or false"))
	return nil, false
}

func queryTime(c *gin.Context, name string) (*time.Time, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return &t, true
		}
	}
	kernel.Fail(c, kernel.Invalid(name, "must be an RFC 3339 time or YYYY-MM-DD"))
	return nil, false
}

// parseFilter reads the list query parameters; assignee=me means the caller.
func parseFilter(c *gin.Context, a api.Actor) (ListFilter, bool) {
	f := ListFilter{Category: c.Query("category"), Priority: c.Query("priority"), Query: strings.TrimSpace(c.Query("q")),
		Sort: c.Query("sort"), Dir: c.Query("dir"), Cursor: c.Query("cursor"), Limit: kernel.ClampLimit(c.Query("limit")),
		TopLevel: c.Query("top_level") == "true", Archived: c.Query("archived") == "true"}
	ids := []struct {
		name string
		dst  **uuid.UUID
	}{{"column_id", &f.ColumnID}, {"label_id", &f.LabelID}, {"milestone_id", &f.MilestoneID}, {"sprint_id", &f.SprintID}, {"parent_id", &f.ParentID}}
	for _, p := range ids {
		v, ok := kernel.QueryUUID(c, p.name)
		if !ok {
			return f, false
		}
		*p.dst = v
	}
	if c.Query("assignee") == "me" {
		f.AssigneeID = &a.UserID
	} else if v, ok := kernel.QueryUUID(c, "assignee"); ok {
		f.AssigneeID = v
	} else {
		return f, false
	}
	return parseFilterFlags(c, f)
}

func parseFilterFlags(c *gin.Context, f ListFilter) (ListFilter, bool) {
	var ok bool
	if f.DueFrom, ok = queryTime(c, "due_from"); !ok {
		return f, false
	}
	if f.DueTo, ok = queryTime(c, "due_to"); !ok {
		return f, false
	}
	if f.Overdue, ok = queryBool(c, "overdue"); !ok {
		return f, false
	}
	f.Blocked, ok = queryBool(c, "blocked")
	return f, ok
}
