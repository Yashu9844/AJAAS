package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/organization/events"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type failPublisher struct{ called int }

func (f *failPublisher) Publish(ctx context.Context, exchange, routingKey string, event interface{}) error {
	f.called++
	return errors.New("broker down")
}

func TestPublishOrOutbox_BrokerDown(t *testing.T) {
	fp := &failPublisher{}
	evt := events.NewEvent(events.TypeDepartmentCreated, events.RoutingKeyDepartmentCreated, uuid.New(), uuid.New(),
		events.DepartmentPayload{DepartmentID: uuid.New(), Name: "X"})

	// Nil tx (unit context): publish attempted, no panic, no block.
	publishOrOutbox(context.Background(), nil, fp, events.Exchange, evt)
	if fp.called != 1 {
		t.Fatalf("expected 1 publish attempt, got %d", fp.called)
	}

	// Dead database: outbox insert fails best-effort; the API path still returns (FR-E005).
	db, err := gorm.Open(postgres.Open("host=127.0.0.1 port=1 user=x password=x dbname=x connect_timeout=1"), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	done := make(chan struct{})
	go func() {
		publishOrOutbox(context.Background(), db, fp, events.Exchange, evt)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("publishOrOutbox blocked on dead database")
	}
}

func TestPublishOrOutbox_SuccessSkipsOutbox(t *testing.T) {
	evt := events.NewEvent(events.TypeTeamCreated, events.RoutingKeyTeamCreated, uuid.New(), uuid.New(),
		events.TeamPayload{TeamID: uuid.New(), DepartmentID: uuid.New(), Name: "T"})
	// Healthy publisher + nil tx: success path returns immediately.
	publishOrOutbox(context.Background(), nil, &stubPublisher{}, events.Exchange, evt)
}
