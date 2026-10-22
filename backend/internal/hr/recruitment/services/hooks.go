package services

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/hr/recruitment/api"
	"gorm.io/gorm"
)

// Hooks holds the hire hooks other contexts register (recruitment/api.HireHook).
type Hooks struct {
	mu    sync.RWMutex
	hooks []api.HireHook
}

// Add registers a hook.
func (h *Hooks) Add(fn api.HireHook) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hooks = append(h.hooks, fn)
}

// run calls every hook in order; nil Hooks is a no-op.
func (h *Hooks) run(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, fact api.HireFact) error {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	hooks := append([]api.HireHook(nil), h.hooks...)
	h.mu.RUnlock()
	for _, fn := range hooks {
		if err := fn(ctx, tx, tenantID, fact); err != nil {
			return err
		}
	}
	return nil
}
