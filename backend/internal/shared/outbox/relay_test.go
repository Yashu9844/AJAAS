package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type memStore struct {
	rows    []Row
	err     error
	failed  map[uuid.UUID]string
	marked  map[uuid.UUID]bool
	markErr error
}

func (m *memStore) Add(_ context.Context, _ *gorm.DB, msg Message) error {
	raw, _ := json.Marshal(msg.Envelope)
	m.rows = append(m.rows, Row{ID: uuid.New(), EventID: msg.EventID, Exchange: msg.Exchange, RoutingKey: msg.RoutingKey, Payload: string(raw)})
	return m.err
}
func (m *memStore) Pending(context.Context, *gorm.DB, int, int) ([]Row, error) { return m.rows, m.err }
func (m *memStore) MarkPublished(_ context.Context, _ *gorm.DB, id uuid.UUID, _ time.Time) error {
	m.marked[id] = true
	return m.markErr
}
func (m *memStore) RecordFailure(_ context.Context, _ *gorm.DB, id uuid.UUID, reason string) error {
	m.failed[id] = reason
	return nil
}

type pub struct {
	fail map[string]bool
	sent []string
}

func (p *pub) Publish(_ context.Context, exchange, key string, event interface{}) error {
	if p.fail[key] {
		return errors.New("broker down")
	}
	if _, ok := event.(json.RawMessage); !ok {
		return errors.New("payload must be the stored JSON")
	}
	p.sent = append(p.sent, exchange+"/"+key)
	return nil
}

func TestRelayPublishesAndRecordsFailures(t *testing.T) {
	s := &memStore{failed: map[uuid.UUID]string{}, marked: map[uuid.UUID]bool{}}
	_ = s.Add(context.Background(), nil, Message{EventID: uuid.New(), Exchange: "x", RoutingKey: "ok", Envelope: map[string]int{"a": 1}})
	_ = s.Add(context.Background(), nil, Message{EventID: uuid.New(), Exchange: "x", RoutingKey: "bad", Envelope: 1})
	p := &pub{fail: map[string]bool{"bad": true}}
	r := &Relay{Store: s, Publisher: p}
	n, err := r.RelayOnce(context.Background())
	if err != nil || n != 1 || len(p.sent) != 1 || p.sent[0] != "x/ok" {
		t.Fatalf("published %d %v %v", n, p.sent, err)
	}
	if len(s.failed) != 1 || len(s.marked) != 1 {
		t.Fatal("failure recorded, success marked")
	}
	s.markErr = errors.New("db")
	if n, _ := r.RelayOnce(context.Background()); n != 0 {
		t.Fatal("unmarked rows are not counted")
	}
	s.err = errors.New("db down")
	if _, err := r.RelayOnce(context.Background()); err == nil {
		t.Fatal("store errors surface")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.Run(ctx) // one pass, then returns
	if r.Interval != DefaultInterval || r.BatchSize != DefaultBatchSize || r.MaxAttempts != DefaultMaxAttempts {
		t.Fatal("defaults applied")
	}
}
