package aihooks

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func act(fn func(c *gin.Context, a api.Actor)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if a, ok := api.ActorFrom(c); ok {
			fn(c, a)
		}
	}
}

// Register mounts the AI hooks on g (already guarded by tenant resolution and authentication). All routes are reads.
func (s *Service) Register(g *gin.RouterGroup) {
	g.GET("/ai-hooks/tools", act(func(c *gin.Context, a api.Actor) {
		pid, ok := kernel.QueryUUID(c, "project_id")
		if !ok {
			return
		}
		tools, err := s.ToolsFor(c.Request.Context(), a, pid)
		kernel.Respond(c, http.StatusOK, tools, err)
	}))
	g.GET("/ai-hooks/readable-projects", act(func(c *gin.Context, a api.Actor) {
		ids, err := s.ReadableProjects(c.Request.Context(), a)
		kernel.Respond(c, http.StatusOK, ids, err)
	}))
	g.GET("/ai-hooks/context/projects/:id", act(func(c *gin.Context, a api.Actor) {
		if id, ok := kernel.ParamUUID(c, "id"); ok {
			p, err := s.ProjectContext(c.Request.Context(), a, id)
			kernel.Respond(c, http.StatusOK, p, err)
		}
	}))
	g.GET("/ai-hooks/context/tasks/:id", act(func(c *gin.Context, a api.Actor) {
		p, err := s.TaskContext(c.Request.Context(), a, c.Param("id"))
		kernel.Respond(c, http.StatusOK, p, err)
	}))
	g.GET("/ai-hooks/documents", act(func(c *gin.Context, a api.Actor) {
		pid, ok := kernel.QueryUUID(c, "project_id")
		if !ok || pid == nil {
			if ok {
				kernel.Fail(c, kernel.Invalid("project_id", "is required"))
			}
			return
		}
		limit, _ := strconv.Atoi(c.Query("limit"))
		docs, next, err := s.Documents(c.Request.Context(), a, DocQuery{ProjectID: *pid, Kind: c.Query("kind"), Cursor: c.Query("cursor"), Limit: limit})
		kernel.RespondCursor(c, kernel.NewCursorPage(docs, clampLimit(limit), next), err)
	}))
}
