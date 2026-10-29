package webhooks

import (
	"context"
	"time"
)

// Loop periods of the background worker.
const (
	runInterval = 2 * time.Second
	purgeEvery  = time.Hour
)

// Run dispatches new events and sends due deliveries until ctx is cancelled. Errors are reported to onErr and never stop
// the loop; they carry no URLs or secrets.
func (s *Service) Run(ctx context.Context, onErr func(error)) {
	tick := time.NewTicker(runInterval)
	purge := time.NewTicker(purgeEvery)
	defer tick.Stop()
	defer purge.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.RunOnce(ctx, onErr)
		case <-purge.C:
			if err := s.Purge(ctx); err != nil {
				onErr(err)
			}
		}
	}
}

// RunOnce is one dispatch pass followed by one send pass.
func (s *Service) RunOnce(ctx context.Context, onErr func(error)) {
	if _, err := s.DispatchOnce(ctx); err != nil {
		onErr(err)
	}
	if _, err := s.SendOnce(ctx); err != nil {
		onErr(err)
	}
}
