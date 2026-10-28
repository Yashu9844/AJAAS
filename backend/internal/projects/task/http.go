package task

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// handler adapts a use case to gin: it resolves the actor and the path parameters.
type handler struct{ s *Service }

func act(fn func(c *gin.Context, a api.Actor)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if a, ok := api.ActorFrom(c); ok {
			fn(c, a)
		}
	}
}

func withID(fn func(c *gin.Context, a api.Actor, id uuid.UUID)) gin.HandlerFunc {
	return act(func(c *gin.Context, a api.Actor) {
		if id, ok := kernel.ParamUUID(c, "id"); ok {
			fn(c, a, id)
		}
	})
}

// withSub resolves :id and a second path parameter.
func withSub(name string, fn func(c *gin.Context, a api.Actor, id, sub uuid.UUID)) gin.HandlerFunc {
	return withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		if sub, ok := kernel.ParamUUID(c, name); ok {
			fn(c, a, id, sub)
		}
	})
}

func body[T any](c *gin.Context, run func(in T)) {
	var in T
	if kernel.Bind(c, &in) {
		run(in)
	}
}

// withVersion fills a missing body version from the If-Match header.
func withVersion(c *gin.Context, v **int) {
	if n := kernel.IfMatchVersion(c); n > 0 && *v == nil {
		*v = &n
	}
}

// Register mounts the task routes on g (already guarded by tenant resolution, authentication and the flag middleware).
func (s *Service) Register(g *gin.RouterGroup) {
	h := handler{s: s}
	h.collectionRoutes(g)
	h.taskRoutes(g)
	h.moveRoutes(g)
	h.linkRoutes(g)
	h.readRoutes(g)
}

func (h handler) collectionRoutes(g *gin.RouterGroup) {
	g.POST("/projects/:id/tasks", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in CreateInput) {
			v, err := h.s.Create(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.GET("/projects/:id/tasks", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		f, ok := parseFilter(c, a)
		if !ok {
			return
		}
		rows, next, err := h.s.List(c.Request.Context(), a, id, f)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, f.Limit, next), err)
	}))
	g.POST("/projects/:id/tasks/bulk", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in BulkInput) {
			out, err := h.s.Bulk(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, out, err)
		})
	}))
	g.GET("/projects/:id/activity", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		f := activityFilter(c)
		rows, next, err := h.s.ProjectActivity(c.Request.Context(), a, id, f)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, f.Limit, next), err)
	}))
}

func (h handler) taskRoutes(g *gin.RouterGroup) {
	g.GET("/tasks/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := h.s.Get(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.GET("/tickets/:key", act(func(c *gin.Context, a api.Actor) {
		v, err := h.s.ByKey(c.Request.Context(), a, c.Param("key"))
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.PATCH("/tasks/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in UpdateInput) {
			withVersion(c, &in.Version)
			v, err := h.s.Update(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.DELETE("/tasks/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.Delete(c.Request.Context(), a, id))
	}))
	g.POST("/tasks/:id/archive", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := h.s.Archive(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.POST("/tasks/:id/restore", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := h.s.Restore(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.POST("/tasks/:id/duplicate", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		var in DuplicateInput
		if c.Request.ContentLength != 0 && !kernel.Bind(c, &in) {
			return
		}
		v, err := h.s.Duplicate(c.Request.Context(), a, id, in)
		kernel.Respond(c, http.StatusCreated, v, err)
	}))
}

func (h handler) moveRoutes(g *gin.RouterGroup) {
	g.POST("/tasks/:id/move", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in MoveInput) {
			withVersion(c, &in.Version)
			v, err := h.s.Move(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.POST("/tasks/:id/reorder", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in MoveInput) {
			withVersion(c, &in.Version)
			v, err := h.s.Reorder(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.POST("/tasks/:id/move-project", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in MoveProjectInput) {
			withVersion(c, &in.Version)
			v, err := h.s.MoveProject(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
}
