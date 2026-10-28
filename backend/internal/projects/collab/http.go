package collab

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// handler adapts the use cases to gin: it resolves the actor and the :id path parameter.
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

func body[T any](c *gin.Context, run func(in T)) {
	var in T
	if kernel.Bind(c, &in) {
		run(in)
	}
}

func pageOf(c *gin.Context) PageRequest {
	return PageRequest{Cursor: c.Query("cursor"), Limit: kernel.ClampLimit(c.Query("limit"))}
}

// Register mounts the collaboration routes on g (already guarded by tenant resolution, authentication and the flag
// middleware). The task routes use :id for the task, like the task context.
func (s *Service) Register(g *gin.RouterGroup) {
	h := handler{s: s}
	h.commentRoutes(g)
	h.watcherRoutes(g)
	h.timeRoutes(g)
	h.timerRoutes(g)
	h.attachmentRoutes(g)
}

func (h handler) commentRoutes(g *gin.RouterGroup) {
	g.GET("/tasks/:id/comments", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		pr := pageOf(c)
		rows, next, err := h.s.ListComments(c.Request.Context(), a, id, pr)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, pr.Limit, next), err)
	}))
	g.POST("/tasks/:id/comments", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in CommentInput) {
			cm, err := h.s.CreateComment(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, cm, err)
		})
	}))
	g.PATCH("/comments/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in CommentInput) {
			cm, err := h.s.UpdateComment(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, cm, err)
		})
	}))
	g.DELETE("/comments/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.DeleteComment(c.Request.Context(), a, id))
	}))
}

func (h handler) watcherRoutes(g *gin.RouterGroup) {
	g.GET("/tasks/:id/watch", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		w, err := h.s.WatchStatus(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, w, err)
	}))
	g.PUT("/tasks/:id/watch", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		w, err := h.s.Watch(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, w, err)
	}))
	g.DELETE("/tasks/:id/watch", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, WatchState{}, h.s.Unwatch(c.Request.Context(), a, id))
	}))
	g.GET("/tasks/:id/watchers", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		pr := pageOf(c)
		rows, next, err := h.s.Watchers(c.Request.Context(), a, id, pr)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, pr.Limit, next), err)
	}))
}
