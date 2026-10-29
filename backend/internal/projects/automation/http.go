package automation

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func act(fn func(c *gin.Context, a api.Actor, id uuid.UUID)) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := api.ActorFrom(c)
		if !ok {
			return
		}
		if id, ok := kernel.ParamUUID(c, "id"); ok {
			fn(c, a, id)
		}
	}
}

// onRule adds the rule id of the path to a project handler.
func onRule(fn func(c *gin.Context, a api.Actor, id, rule uuid.UUID)) gin.HandlerFunc {
	return act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		if rule, ok := kernel.ParamUUID(c, "rule_id"); ok {
			fn(c, a, id, rule)
		}
	})
}

// testInput names the task a dry run checks the rule against.
type testInput struct {
	TaskID uuid.UUID `json:"task_id"`
}

// Register mounts the rule routes on g (already guarded by tenant resolution and authentication).
func (s *Service) Register(g *gin.RouterGroup) {
	g.GET("/projects/:id/rules", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := s.Rules(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/rules", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		var in RuleInput
		if kernel.Bind(c, &in) {
			r, err := s.CreateRule(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, r, err)
		}
	}))
	g.PATCH("/projects/:id/rules/:rule_id", onRule(func(c *gin.Context, a api.Actor, id, rule uuid.UUID) {
		var in RuleUpdate
		if !kernel.Bind(c, &in) {
			return
		}
		if v := kernel.IfMatchVersion(c); v > 0 && in.Version == nil {
			in.Version = &v
		}
		r, err := s.UpdateRule(c.Request.Context(), a, id, rule, in)
		kernel.Respond(c, http.StatusOK, r, err)
	}))
	g.DELETE("/projects/:id/rules/:rule_id", onRule(func(c *gin.Context, a api.Actor, id, rule uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, s.DeleteRule(c.Request.Context(), a, id, rule))
	}))
	g.GET("/projects/:id/rules/:rule_id/runs", onRule(func(c *gin.Context, a api.Actor, id, rule uuid.UUID) {
		f := RunFilter{Status: c.Query("status"), Cursor: c.Query("cursor"), Limit: kernel.ClampLimit(c.Query("limit"))}
		rows, next, err := s.Runs(c.Request.Context(), a, id, rule, f)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, f.Limit, next), err)
	}))
	g.POST("/projects/:id/rules/:rule_id/test", onRule(func(c *gin.Context, a api.Actor, id, rule uuid.UUID) {
		var in testInput
		if kernel.Bind(c, &in) {
			res, err := s.Test(c.Request.Context(), a, id, rule, in.TaskID)
			kernel.Respond(c, http.StatusOK, res, err)
		}
	}))
}
