package project

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
)

func (h handler) roleRoutes(g *gin.RouterGroup) {
	g.GET("/project-permissions", act(func(c *gin.Context, _ api.Actor) { kernel.Respond(c, http.StatusOK, PermissionCatalogue(), nil) }))
	g.GET("/project-roles", act(func(c *gin.Context, a api.Actor) {
		rows, err := h.s.Roles(c.Request.Context(), a)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/project-roles", act(func(c *gin.Context, a api.Actor) {
		body(c, func(in RoleInput) {
			r, err := h.s.CreateRole(c.Request.Context(), a, in)
			kernel.Respond(c, http.StatusCreated, r, err)
		})
	}))
	g.PATCH("/project-roles/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in RoleInput) {
			r, err := h.s.UpdateRole(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, r, err)
		})
	}))
	g.DELETE("/project-roles/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.DeleteRole(c.Request.Context(), a, id))
	}))
}

func (h handler) columnRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/columns", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := h.s.Columns(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/columns", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in ColumnInput) {
			r, err := h.s.CreateColumn(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, r, err)
		})
	}))
	g.PUT("/projects/:id/columns/order", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in struct {
			ColumnIDs []uuid.UUID `json:"column_ids"`
		}) {
			rows, err := h.s.ReorderColumns(c.Request.Context(), a, id, in.ColumnIDs)
			kernel.Respond(c, http.StatusOK, rows, err)
		})
	}))
	g.PATCH("/projects/:id/columns/:column_id", withSub("column_id", func(c *gin.Context, a api.Actor, id, col uuid.UUID) {
		body(c, func(in ColumnInput) {
			r, err := h.s.UpdateColumn(c.Request.Context(), a, id, col, in)
			kernel.Respond(c, http.StatusOK, r, err)
		})
	}))
	g.DELETE("/projects/:id/columns/:column_id", withSub("column_id", func(c *gin.Context, a api.Actor, id, col uuid.UUID) {
		target, ok := kernel.QueryUUID(c, "move_to")
		if ok {
			kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.DeleteColumn(c.Request.Context(), a, id, col, target))
		}
	}))
}

func (h handler) labelRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/labels", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := h.s.Labels(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/labels", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in LabelInput) {
			l, err := h.s.CreateLabel(c.Request.Context(), a, &id, in)
			kernel.Respond(c, http.StatusCreated, l, err)
		})
	}))
	g.PATCH("/projects/:id/labels/:label_id", withSub("label_id", func(c *gin.Context, a api.Actor, id, lid uuid.UUID) {
		body(c, func(in LabelInput) {
			l, err := h.s.UpdateLabel(c.Request.Context(), a, &id, lid, in)
			kernel.Respond(c, http.StatusOK, l, err)
		})
	}))
	g.DELETE("/projects/:id/labels/:label_id", withSub("label_id", func(c *gin.Context, a api.Actor, id, lid uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.DeleteLabel(c.Request.Context(), a, &id, lid))
	}))
	g.GET("/project-labels", act(func(c *gin.Context, a api.Actor) {
		rows, err := h.s.TenantLabels(c.Request.Context(), a)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/project-labels", act(func(c *gin.Context, a api.Actor) {
		body(c, func(in LabelInput) {
			l, err := h.s.CreateLabel(c.Request.Context(), a, nil, in)
			kernel.Respond(c, http.StatusCreated, l, err)
		})
	}))
	g.PATCH("/project-labels/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in LabelInput) {
			l, err := h.s.UpdateLabel(c.Request.Context(), a, nil, id, in)
			kernel.Respond(c, http.StatusOK, l, err)
		})
	}))
	g.DELETE("/project-labels/:id", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.DeleteLabel(c.Request.Context(), a, nil, id))
	}))
}

func (h handler) milestoneRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/milestones", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := h.s.Milestones(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/milestones", withID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		body(c, func(in MilestoneInput) {
			m, err := h.s.CreateMilestone(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, m, err)
		})
	}))
	g.PATCH("/projects/:id/milestones/:milestone_id", withSub("milestone_id", func(c *gin.Context, a api.Actor, id, mid uuid.UUID) {
		body(c, func(in MilestoneInput) {
			m, err := h.s.UpdateMilestone(c.Request.Context(), a, id, mid, in)
			kernel.Respond(c, http.StatusOK, m, err)
		})
	}))
	g.DELETE("/projects/:id/milestones/:milestone_id", withSub("milestone_id", func(c *gin.Context, a api.Actor, id, mid uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, h.s.DeleteMilestone(c.Request.Context(), a, id, mid))
	}))
}
