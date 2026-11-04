package actions

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/meetings/api"
	"github.com/jaas/jaas/internal/meetings/series"
	"github.com/jaas/jaas/internal/meetings/testdb"
)

func TestTaskEventsMarkSnapshotsStaleOnce(t *testing.T) {
	db := testdb.Open(t)
	svc := series.NewService(db, series.Ports{Users: &testdb.Users{}, Audit: &testdb.Audit{}, Resources: testdb.NewResources()})
	a := api.Actor{TenantID: uuid.New(), UserID: uuid.New(), Create: true}
	v, err := svc.Create(context.Background(), a, series.CreateInput{Title: "T", Start: "2030-01-01T09:00", TZ: "UTC", DurationMin: 30})
	if err != nil {
		t.Fatal(err)
	}
	task := uuid.New()
	if err := db.Exec(`INSERT INTO meeting_action_links (id, tenant_id, occurrence_id, task_id, created_by, snapshot_status) VALUES (?, ?, ?, ?, ?, 'todo')`,
		uuid.New(), a.TenantID, v.NextOccurrence.ID, task, a.UserID).Error; err != nil {
		t.Fatal(err)
	}
	s := NewService(db, svc, nil)
	e := TaskEvent{EventID: uuid.New(), EventType: "task.deleted", TenantID: a.TenantID, TaskID: task}
	for i := 0; i < 2; i++ { // a redelivery is skipped by the inbox
		if err := s.HandleTaskEvent(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	var status string
	db.Raw("SELECT snapshot_status FROM meeting_action_links WHERE task_id = ? AND stale_at IS NOT NULL", task).Scan(&status)
	if status != "deleted" {
		t.Fatalf("snapshot marked stale: %q", status)
	}
	var n int
	db.Raw("SELECT count(*) FROM meetings_events_inbox WHERE event_id = ?", e.EventID).Scan(&n)
	if n != 1 {
		t.Fatalf("inbox ledger: %d", n)
	}
	items, err := s.List(context.Background(), a, v.NextOccurrence.ID)
	if err != nil || len(items) != 1 || !items[0].Restricted || !items[0].Stale {
		t.Fatalf("without the tasks port every action is restricted: %+v %v", items, err)
	}
	if _, err := s.Add(context.Background(), a, v.NextOccurrence.ID, Input{Title: "x"}); err == nil {
		t.Fatal("no tasks port: 503")
	}
}
