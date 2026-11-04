package content

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
)

// Register mounts the occurrence-content routes (specification section 3).
func (s *Service) Register(g *gin.RouterGroup) {
	s.agendaRoutes(g)
	s.pointRoutes(g)
	s.notesRoutes(g)
	s.decisionRoutes(g)
}

func (s *Service) agendaRoutes(g *gin.RouterGroup) {
	g.GET("/occurrences/:id/agenda-items", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.Agenda(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.POST("/occurrences/:id/agenda-items", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in AgendaInput) {
			v, err := s.AddAgendaItem(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.PATCH("/agenda-items/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in AgendaInput) {
			in.Version = kernel.Version(c, in.Version)
			v, err := s.UpdateAgendaItem(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.DELETE("/agenda-items/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.NoContent(c, s.DeleteAgendaItem(c.Request.Context(), a, id))
	}))
	g.POST("/agenda-items/:id/move", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in MoveInput) {
			v, err := s.MoveAgendaItem(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.GET("/occurrences/:id/attendance", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.AttendanceList(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.PUT("/occurrences/:id/attendance", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in AttendanceInput) {
			v, err := s.MarkAttendance(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
}

func (s *Service) pointRoutes(g *gin.RouterGroup) {
	g.GET("/occurrences/:id/private-points", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.PrivatePoints(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.POST("/occurrences/:id/private-points", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in PointInput) {
			v, err := s.AddPrivatePoint(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.PATCH("/private-points/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in PointInput) {
			v, err := s.UpdatePrivatePoint(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.DELETE("/private-points/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.NoContent(c, s.DeletePrivatePoint(c.Request.Context(), a, id))
	}))
	g.POST("/private-points/:id/promote", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.PromotePrivatePoint(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusCreated, v, err)
	}))
}

func (s *Service) notesRoutes(g *gin.RouterGroup) {
	g.GET("/occurrences/:id/notes", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.Notes(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.PUT("/occurrences/:id/notes/:section", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in NotesInput) {
			if v := kernel.IfMatchVersion(c); v > 0 {
				in.Version = v
			}
			v, err := s.SaveNotes(c.Request.Context(), a, id, c.Param("section"), in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.GET("/occurrences/:id/notes/:section/versions", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.NoteVersions(c.Request.Context(), a, id, c.Param("section"))
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.POST("/occurrences/:id/minutes/publish", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.Publish(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusCreated, v, err)
	}))
	g.POST("/occurrences/:id/minutes/corrections", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in PublishInput) {
			v, err := s.Correct(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.GET("/occurrences/:id/minutes", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.MinutesVersion(c.Request.Context(), a, id, 0)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.GET("/occurrences/:id/minutes/versions", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.MinutesVersions(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.GET("/occurrences/:id/minutes/versions/:version", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		n, err := strconv.Atoi(c.Param("version"))
		if err != nil || n < 1 {
			kernel.Fail(c, kernel.Invalid("version", "must be a positive number"))
			return
		}
		v, err := s.MinutesVersion(c.Request.Context(), a, id, n)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
}

func (s *Service) decisionRoutes(g *gin.RouterGroup) {
	g.GET("/occurrences/:id/decisions", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.Decisions(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.POST("/occurrences/:id/decisions", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in DecisionInput) {
			v, err := s.RecordDecision(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.PATCH("/decisions/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in DecisionInput) {
			in.Version = kernel.Version(c, in.Version)
			v, err := s.UpdateDecision(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
}
