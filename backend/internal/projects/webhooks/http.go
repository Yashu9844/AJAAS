package webhooks

import (
	"io"
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

// actSub is act plus a second path id (webhook_id or integration_id).
func actSub(param string, fn func(c *gin.Context, a api.Actor, id, sub uuid.UUID)) gin.HandlerFunc {
	return act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		if sub, ok := kernel.ParamUUID(c, param); ok {
			fn(c, a, id, sub)
		}
	})
}

// Register mounts the webhook and integration routes on g (already guarded by tenant resolution and authentication).
func (s *Service) Register(g *gin.RouterGroup) {
	s.webhookRoutes(g)
	s.deliveryRoutes(g)
	s.integrationRoutes(g)
	g.GET("/tasks/:id/links", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := s.TaskLinks(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
}

func (s *Service) webhookRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/webhooks", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := s.Webhooks(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/webhooks", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		var in WebhookInput
		if kernel.Bind(c, &in) {
			w, err := s.CreateWebhook(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, w, err)
		}
	}))
	g.PATCH("/projects/:id/webhooks/:webhook_id", actSub("webhook_id", func(c *gin.Context, a api.Actor, id, wid uuid.UUID) {
		var in WebhookUpdate
		if kernel.Bind(c, &in) {
			w, err := s.UpdateWebhook(c.Request.Context(), a, id, wid, in)
			kernel.Respond(c, http.StatusOK, w, err)
		}
	}))
	g.DELETE("/projects/:id/webhooks/:webhook_id", actSub("webhook_id", func(c *gin.Context, a api.Actor, id, wid uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, s.DeleteWebhook(c.Request.Context(), a, id, wid))
	}))
	g.POST("/projects/:id/webhooks/:webhook_id/rotate-secret", actSub("webhook_id", func(c *gin.Context, a api.Actor, id, wid uuid.UUID) {
		w, err := s.RotateWebhookSecret(c.Request.Context(), a, id, wid)
		kernel.Respond(c, http.StatusOK, w, err)
	}))
}

func (s *Service) deliveryRoutes(g *gin.RouterGroup) {
	g.POST("/projects/:id/webhooks/:webhook_id/test", actSub("webhook_id", func(c *gin.Context, a api.Actor, id, wid uuid.UUID) {
		d, err := s.TestWebhook(c.Request.Context(), a, id, wid)
		kernel.Respond(c, http.StatusOK, d, err)
	}))
	g.GET("/projects/:id/webhooks/:webhook_id/deliveries", actSub("webhook_id", func(c *gin.Context, a api.Actor, id, wid uuid.UUID) {
		f := DeliveryFilter{Status: c.Query("status"), Cursor: c.Query("cursor"), Limit: kernel.ClampLimit(c.Query("limit"))}
		rows, next, err := s.Deliveries(c.Request.Context(), a, id, wid, f)
		kernel.RespondCursor(c, kernel.NewCursorPage(rows, f.Limit, next), err)
	}))
	g.POST("/projects/:id/webhooks/:webhook_id/deliveries/:delivery_id/redeliver", actSub("webhook_id", func(c *gin.Context, a api.Actor, id, wid uuid.UUID) {
		did, ok := kernel.ParamUUID(c, "delivery_id")
		if !ok {
			return
		}
		d, err := s.Redeliver(c.Request.Context(), a, id, wid, did)
		kernel.Respond(c, http.StatusOK, d, err)
	}))
}

func (s *Service) integrationRoutes(g *gin.RouterGroup) {
	g.GET("/projects/:id/integrations", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		rows, err := s.Integrations(c.Request.Context(), a, id)
		kernel.Respond(c, http.StatusOK, rows, err)
	}))
	g.POST("/projects/:id/integrations", act(func(c *gin.Context, a api.Actor, id uuid.UUID) {
		var in IntegrationInput
		if kernel.Bind(c, &in) {
			it, err := s.CreateIntegration(c.Request.Context(), a, id, in)
			kernel.Respond(c, http.StatusCreated, it, err)
		}
	}))
	g.DELETE("/projects/:id/integrations/:integration_id", actSub("integration_id", func(c *gin.Context, a api.Actor, id, iid uuid.UUID) {
		kernel.Respond(c, http.StatusOK, gin.H{"deleted": true}, s.DeleteIntegration(c.Request.Context(), a, id, iid))
	}))
	g.POST("/projects/:id/integrations/:integration_id/rotate-secret", actSub("integration_id", func(c *gin.Context, a api.Actor, id, iid uuid.UUID) {
		it, err := s.RotateIntegrationSecret(c.Request.Context(), a, id, iid)
		kernel.Respond(c, http.StatusOK, it, err)
	}))
}

// RegisterPublic mounts the one other route without a bearer token (besides the calendar feed): the provider call. It is
// authenticated by the HMAC of the raw body and rate limited per client IP.
func (s *Service) RegisterPublic(rg *gin.RouterGroup) {
	rg.POST("/integrations/:provider/:integration_id", s.limiter, s.serveIncoming)
}

func (s *Service) serveIncoming(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxIncomingBody)
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		kernel.Fail(c, kernel.Err(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "the request body is too large"))
		return
	}
	in := Incoming{Provider: c.Param("provider"), ID: c.Param("integration_id"), Body: raw}
	if in.Provider == ProviderGitea {
		in.Signature, in.Event = c.GetHeader("X-Gitea-Signature"), firstHeader(c, "X-Gitea-Event", "X-GitHub-Event")
	} else {
		in.Signature, in.Event = c.GetHeader("X-Hub-Signature-256"), c.GetHeader("X-GitHub-Event")
	}
	res, err := s.Receive(c.Request.Context(), in)
	kernel.Respond(c, http.StatusOK, res, err)
}

func firstHeader(c *gin.Context, names ...string) string {
	for _, n := range names {
		if v := c.GetHeader(n); v != "" {
			return v
		}
	}
	return ""
}
