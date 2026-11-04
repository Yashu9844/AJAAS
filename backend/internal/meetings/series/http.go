package series

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/kernel"
)

// Register mounts the series, occurrence and participant routes (specification section 3).
func (s *Service) Register(g *gin.RouterGroup) {
	s.seriesRoutes(g)
	s.participantRoutes(g)
	s.occurrenceRoutes(g)
}

// window reads ?from and ?to; without them it is the next 14 days.
func (s *Service) window(c *gin.Context) (time.Time, time.Time, bool) {
	from, ok := kernel.QueryTime(c, "from")
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	to, ok := kernel.QueryTime(c, "to")
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	f := s.now().UTC().Add(-time.Hour)
	if from != nil {
		f = *from
	}
	t := f.AddDate(0, 0, 14)
	if to != nil {
		t = *to
	}
	return f, t, true
}

func (s *Service) list(c *gin.Context, a api.Actor, f ListFilter) {
	from, to, ok := s.window(c)
	if !ok {
		return
	}
	f.From, f.To = from, to
	rows, err := s.List(c.Request.Context(), a, f)
	kernel.Respond(c, http.StatusOK, rows, err)
}

func (s *Service) seriesRoutes(g *gin.RouterGroup) {
	g.GET("/meetings", api.Act(func(c *gin.Context, a api.Actor) { s.list(c, a, ListFilter{Mine: c.Query("mine") == "true"}) }))
	g.GET("/me/meetings", api.Act(func(c *gin.Context, a api.Actor) { s.list(c, a, ListFilter{Mine: true}) }))
	g.POST("/meetings", api.Act(func(c *gin.Context, a api.Actor) {
		api.Body(c, func(in CreateInput) {
			v, err := s.Create(c.Request.Context(), a, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.GET("/meetings/search", api.Act(func(c *gin.Context, a api.Actor) {
		f := SearchFilter{Query: c.Query("q")}
		if raw := c.Query("participant_id"); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				kernel.Fail(c, kernel.Invalid("participant_id", "must be a UUID"))
				return
			}
			f.ParticipantID = &id
		}
		hits, err := s.Search(c.Request.Context(), a, f)
		kernel.Respond(c, http.StatusOK, hits, err)
	}))
	g.GET("/meetings/needs-organizer", api.Act(func(c *gin.Context, a api.Actor) {
		hits, err := s.NeedsOrganizer(c.Request.Context(), a)
		kernel.Respond(c, http.StatusOK, hits, err)
	}))
	g.GET("/meetings/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.Get(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.PATCH("/meetings/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in UpdateInput) {
			in.Version = kernel.Version(c, in.Version)
			v, err := s.Update(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.DELETE("/meetings/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		kernel.NoContent(c, s.Delete(c.Request.Context(), a, id, kernel.IfMatchVersion(c)))
	}))
	s.lifecycleRoutes(g)
}

func (s *Service) lifecycleRoutes(g *gin.RouterGroup) {
	g.POST("/meetings/:id/cancel", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in CancelInput) {
			in.Version = kernel.Version(c, in.Version)
			v, err := s.Cancel(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.POST("/meetings/:id/split", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in SplitInput) {
			in.Version = kernel.Version(c, in.Version)
			v, err := s.Split(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.GET("/meetings/:id/occurrences", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		if _, err := s.Access(c.Request.Context(), s.db, a, id); err != nil {
			kernel.Fail(c, err)
			return
		}
		s.list(c, a, ListFilter{SeriesID: &id})
	}))
	g.PUT("/meetings/:id/response", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in ResponseInput) { kernel.NoContent(c, s.Respond(c.Request.Context(), a, id, nil, in)) })
	}))
	g.GET("/meetings/:id/consent", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.Consents(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.PUT("/meetings/:id/consent", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in ConsentInput) {
			v, err := s.SetConsent(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
}

func (s *Service) participantRoutes(g *gin.RouterGroup) {
	g.GET("/meetings/:id/participants", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.Participants(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.POST("/meetings/:id/participants", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in ParticipantInput) {
			v, err := s.AddParticipant(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, v, err)
		})
	}))
	g.PATCH("/meetings/:id/participants/:user_id", api.WithSub("user_id", func(c *gin.Context, a api.Actor, id, user uuid.UUID) {
		api.Body(c, func(in ParticipantInput) {
			in.UserID = user
			v, err := s.UpdateParticipant(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.DELETE("/meetings/:id/participants/:user_id", api.WithSub("user_id", func(c *gin.Context, a api.Actor, id, user uuid.UUID) {
		kernel.NoContent(c, s.RemoveParticipant(c.Request.Context(), a, id, user))
	}))
	g.GET("/meetings/:id/roles", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.AssignableRoles(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
}

func (s *Service) occurrenceRoutes(g *gin.RouterGroup) {
	g.GET("/occurrences/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		v, err := s.GetOccurrence(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, v, err)
	}))
	g.PATCH("/occurrences/:id", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in OccurrenceInput) {
			in.Version = kernel.Version(c, in.Version)
			v, err := s.UpdateOccurrence(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.POST("/occurrences/:id/cancel", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in CancelInput) {
			in.Version = kernel.Version(c, in.Version)
			v, err := s.CancelOccurrence(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.POST("/occurrences/:id/status", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in StatusInput) {
			in.Version = kernel.Version(c, in.Version)
			v, err := s.SetStatus(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusOK, v, err)
		})
	}))
	g.PUT("/occurrences/:id/response", api.WithID(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		api.Body(c, func(in ResponseInput) { kernel.NoContent(c, s.RespondOccurrence(c.Request.Context(), a, id, in)) })
	}))
}
