package planning

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

const cursorPrefix = "cursor_"

// boardParams reads per_column, sprint and the per-column cursors (cursor_<columnId>) from the query string.
func boardParams(c *gin.Context) BoardParams {
	n, _ := strconv.Atoi(c.Query("per_column"))
	p := BoardParams{PerColumn: n, Sprint: c.Query("sprint"), Cursors: map[uuid.UUID]string{}}
	for k, v := range c.Request.URL.Query() {
		id, err := uuid.Parse(strings.TrimPrefix(k, cursorPrefix))
		if strings.HasPrefix(k, cursorPrefix) && err == nil && len(v) > 0 {
			p.Cursors[id] = v[0]
		}
	}
	return p
}

func (h handler) viewRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/board", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		b, err := h.s.Board(c.Request.Context(), a, id, boardParams(c))
		kernel.Respond(c, http.StatusOK, b, err)
	}))
	g.GET("/projects/:id/calendar", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		w, err := ParseWindow(c.Query("from"), c.Query("to"), maxWindowDays)
		if err != nil {
			kernel.Fail(c, err)
			return
		}
		v, err := h.s.ProjectCalendar(c.Request.Context(), a, id, w)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.GET("/projects/:id/gantt", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		p, err := ganttParams(c)
		if err != nil {
			kernel.Fail(c, err)
			return
		}
		v, err := h.s.Gantt(c.Request.Context(), a, id, p)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
}

// ganttParams reads the optional window (both ends or none), the cursor and the limit.
func ganttParams(c *gin.Context) (GanttParams, error) {
	p := GanttParams{Cursor: c.Query("cursor"), Limit: ClampGanttLimit(c.Query("limit"))}
	from, to := c.Query("from"), c.Query("to")
	if from == "" && to == "" {
		return p, nil
	}
	w, err := ParseWindow(from, to, maxGanttWindow)
	if err != nil {
		return p, err
	}
	p.Window = &w
	return p, nil
}

func (h handler) workRoutes(g *gin.RouterGroup) {
	g.GET("/my-work", act(func(c *gin.Context, a api.Actor) {
		n, _ := strconv.Atoi(c.Query("per_group"))
		v, err := h.s.MyWork(c.Request.Context(), a, WorkParams{Group: c.Query("group"), Cursor: c.Query("cursor"), PerGroup: n})
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.GET("/my-work/counts", act(func(c *gin.Context, a api.Actor) {
		v, err := h.s.MyWorkCounts(c.Request.Context(), a)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.GET("/calendar", act(func(c *gin.Context, a api.Actor) {
		w, err := ParseWindow(c.Query("from"), c.Query("to"), maxWindowDays)
		if err != nil {
			kernel.Fail(c, err)
			return
		}
		v, err := h.s.MyCalendar(c.Request.Context(), a, w)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.GET("/search", act(func(c *gin.Context, a api.Actor) {
		project, ok := kernel.QueryUUID(c, "project_id")
		if !ok {
			return
		}
		hits, err := h.s.Search(c.Request.Context(), a, SearchParams{Query: c.Query("q"), ProjectID: project, Limit: ClampSearchLimit(c.Query("limit"))})
		kernel.Respond(c, http.StatusOK, hits, err)
	}))
}
