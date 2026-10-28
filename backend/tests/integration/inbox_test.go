package integration

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects"
	"github.com/jaas/jaas/internal/shared/inbox"
	"gorm.io/gorm"
)

// A redelivered event is recognised, and a rolled-back handler leaves no claim behind.
func TestInboxClaimIsOncePerConsumer(t *testing.T) {
	tenant, ev := uuid.New(), uuid.New()
	claim := func(db *gorm.DB, consumer string) bool {
		ok, err := inbox.Claim(db, projects.InboxTable, consumer, tenant, ev, "identity.user.deactivated")
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !claim(testDB, "a") || claim(testDB, "a") {
		t.Fatal("same consumer and event must claim exactly once")
	}
	if !claim(testDB, "b") {
		t.Fatal("another consumer must claim the same event independently")
	}
	ev2 := uuid.New()
	_ = testDB.Transaction(func(tx *gorm.DB) error {
		if ok, _ := inbox.Claim(tx, projects.InboxTable, "a", tenant, ev2, "x"); !ok {
			t.Fatal("first claim in tx")
		}
		return gorm.ErrInvalidTransaction // roll back
	})
	if ok, _ := inbox.Claim(testDB, projects.InboxTable, "a", tenant, ev2, "x"); !ok {
		t.Fatal("a rolled-back claim must not block the redelivery")
	}
	if _, err := inbox.Claim(testDB, "bad table;", "a", tenant, ev, "x"); err == nil {
		t.Fatal("table names are validated")
	}
}
