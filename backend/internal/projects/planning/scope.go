package planning

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/api"
	"github.com/jaas/jaas/internal/projects/kernel"
	sharedErrors "github.com/jaas/jaas/internal/shared/errors"
)

// taskProjects lists the projects whose tasks the caller can read, for cross-project queries (search, My work, my
// calendar, the iCalendar feed). Every project role in M0 must hold project:read and task:read, so the readable projects
// are the task-readable ones.
func (s *Service) taskProjects(ctx context.Context, a api.Actor) ([]uuid.UUID, error) {
	return s.authz.ReadableProjectIDs(ctx, s.db, a)
}

func isNotFound(err error) bool {
	var app *sharedErrors.AppError
	return errors.As(err, &app) && app.StatusCode == http.StatusNotFound
}

// Window is an inclusive range of calendar days (UTC).
type Window struct {
	From kernel.Date
	To   kernel.Date
}

// ParseWindow reads from/to as YYYY-MM-DD and bounds the span (VW-03: at most 62 days; the Gantt allows more).
func ParseWindow(from, to string, maxDays int) (Window, error) {
	f, err := kernel.ParseDate(from)
	if err != nil {
		return Window{}, kernel.Invalid("from", "must be YYYY-MM-DD")
	}
	t, err := kernel.ParseDate(to)
	if err != nil {
		return Window{}, kernel.Invalid("to", "must be YYYY-MM-DD")
	}
	if t.Before(f) {
		return Window{}, kernel.Invalid("to", "must not be before from")
	}
	if days := int(t.Sub(f.Time).Hours()/24) + 1; days > maxDays {
		return Window{}, kernel.Invalid("to", "the window must not exceed "+itoa(maxDays)+" days")
	}
	return Window{From: f, To: t}, nil
}

// bounds returns the half-open instant range [start, end) covering the window days in UTC.
func (w Window) bounds() (time.Time, time.Time) {
	return w.From.Time, w.To.Time.AddDate(0, 0, 1)
}
