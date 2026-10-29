package aihooks

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"gorm.io/gorm"
)

// Service builds AI-facing views. It owns no tables and writes nothing.
type Service struct {
	db    *gorm.DB
	authz api.Authorizer
	tasks api.TaskResolver
	now   func() time.Time
}

// NewService builds the service.
func NewService(db *gorm.DB, authz api.Authorizer, tasks api.TaskResolver) *Service {
	return &Service{db: db, authz: authz, tasks: tasks, now: time.Now}
}

// Pack wraps a context payload with sizing hints so a runtime can budget its context window.
type Pack struct {
	Kind         string         `json:"kind"`
	ActingAs     uuid.UUID      `json:"acting_as"`
	GeneratedAt  time.Time      `json:"generated_at"`
	ApproxTokens int            `json:"approx_tokens"`
	Truncated    []string       `json:"truncated,omitempty"` // sections that were cut to their caps
	Data         map[string]any `json:"data"`
}

// seal fills the sizing hints (about four characters per token).
func (s *Service) seal(kind string, a api.Actor, data map[string]any, truncated []string) Pack {
	raw, _ := json.Marshal(data)
	return Pack{Kind: kind, ActingAs: a.UserID, GeneratedAt: s.now().UTC(), ApproxTokens: len(raw)/4 + 1, Truncated: truncated, Data: data}
}

// ToolsFor returns the catalogue. With a project id the list is limited to the tools the caller may use there, so an AI
// never even sees an action its user could not perform (the API would refuse it anyway).
func (s *Service) ToolsFor(ctx context.Context, a api.Actor, projectID *uuid.UUID) ([]Tool, error) {
	all := Catalogue()
	if projectID == nil {
		return all, nil
	}
	acc, err := s.authz.Access(ctx, nil, a, *projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Tool, 0, len(all))
	for _, t := range all {
		if t.Requires == "" || acc.Can(t.Requires) {
			out = append(out, t)
		}
	}
	return out, nil
}

// ReadableProjects lists the ids of the projects the caller can read, so an AI runtime can filter retrieval results
// (for example from its own vector index) to what this user is allowed to see before using them.
func (s *Service) ReadableProjects(ctx context.Context, a api.Actor) ([]uuid.UUID, error) {
	ids, err := s.authz.ReadableProjectIDs(ctx, nil, a)
	if err != nil {
		return nil, err
	}
	if len(ids) > maxReadable {
		ids = ids[:maxReadable]
	}
	return ids, nil
}

const maxReadable = 1000
