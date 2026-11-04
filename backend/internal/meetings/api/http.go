package api

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/kernel"
)

// Act adapts a use case to gin: it resolves the actor first.
func Act(fn func(c *gin.Context, a Actor)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if a, ok := ActorFrom(c); ok {
			fn(c, a)
		}
	}
}

// WithID resolves the actor and the :id path parameter.
func WithID(fn func(c *gin.Context, a Actor, id uuid.UUID)) gin.HandlerFunc {
	return Act(func(c *gin.Context, a Actor) {
		if id, ok := kernel.ParamUUID(c, "id"); ok {
			fn(c, a, id)
		}
	})
}

// WithSub resolves :id and a second UUID path parameter.
func WithSub(name string, fn func(c *gin.Context, a Actor, id, sub uuid.UUID)) gin.HandlerFunc {
	return WithID(func(c *gin.Context, a Actor, id uuid.UUID) {
		if sub, ok := kernel.ParamUUID(c, name); ok {
			fn(c, a, id, sub)
		}
	})
}

// Body decodes the JSON body into a T and runs fn, or answers 400.
func Body[T any](c *gin.Context, fn func(in T)) {
	var in T
	if kernel.Bind(c, &in) {
		fn(in)
	}
}
