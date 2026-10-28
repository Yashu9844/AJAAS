package project

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// handler adapts a use case to gin: it resolves the actor and the :id path parameter.
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

// Register mounts the project routes on g (already guarded by tenant resolution, authentication and the flag middleware).
func (s *Service) Register(g *gin.RouterGroup) {
	h := handler{s: s}
	h.projectRoutes(g)
	h.memberRoutes(g)
	h.roleRoutes(g)
	h.columnRoutes(g)
	h.labelRoutes(g)
	h.milestoneRoutes(g)
	h.groupRoutes(g)
}

func (h handler) projectRoutes(g *gin.RouterGroup) {
	g.POST("/projects", act(func(c *gin.Context, a api.Actor) {
		body(c, func(in CreateInput) {
			p, err := h.s.Create(c.Request.Context(), a, in)
			kernel.Respond(c, http.StatusCreated, p, err)
		})
	}))
	g.GET("/projects", act(func(c *gin.Context, a api.Actor) {
		f := ListFilter{Status: c.Query("status"), Query: c.Query("q"), Mine: c.Query("mine") == "true", Favorite: c.Query("favorite") == "true",
			Cursor: c.Query("cursor"), Limit: kernel.ClampLimit(c.Query("limit"))}
		rows, next, err := h.s.List(c.Request.Context(), a, f)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, f.Limit, next), err)
	}))
	g.GET("/projects/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		p, err := h.s.Get(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, p, err)
	}))
	g.PATCH("/projects/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in UpdateInput) {
			if v := kernel.IfMatchVersion(c); v > 0 && in.Version == nil {
				in.Version = &v
			}
			p, err := h.s.Update(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, p, err)
		})
	}))
	g.POST("/projects/:id/archive", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		p, err := h.s.Archive(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, p, err)
	}))
	g.POST("/projects/:id/restore", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		p, err := h.s.Restore(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, p, err)
	}))
	g.DELETE("/projects/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.Delete(c.Request.Context(), a, id))
	}))
	g.PUT("/projects/:id/favorite", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"favorite": true}, h.s.SetFavorite(c.Request.Context(), a, id, true))
	}))
	g.DELETE("/projects/:id/favorite", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"favorite": false}, h.s.SetFavorite(c.Request.Context(), a, id, false))
	}))
}

func (h handler) memberRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/members", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := h.s.Members(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/members", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in MemberInput) {
			m, err := h.s.AddMember(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, m, err)
		})
	}))
	g.PATCH("/projects/:id/members/:user_id", withSub("user_id", func(c *gin.Context, a api.Actor, id, user uuid.UUID) {
		body(c, func(in MemberInput) {
			m, err := h.s.ChangeMember(c.Request.Context(), a, id, user, in)
			kernel.Respond(c, http.StatusOK, m, err)
		})
	}))
	g.DELETE("/projects/:id/members/:user_id", withSub("user_id", func(c *gin.Context, a api.Actor, id, user uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"removed": true}, h.s.RemoveMember(c.Request.Context(), a, id, user))
	}))
}
