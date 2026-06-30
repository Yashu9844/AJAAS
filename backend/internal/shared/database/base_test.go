package database

import (
	"testing"

	"github.com/google/uuid"
)

func TestBaseModel_BeforeCreate(t *testing.T) {
	bm := &BaseModel{}

	// Call BeforeCreate hook with nil DB (since we only check ID generation logic)
	err := bm.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bm.ID == uuid.Nil {
		t.Error("expected ID to be set, got Nil UUID")
	}

	// Verify it does not overwrite an existing ID
	existingID := uuid.New()
	bm2 := &BaseModel{ID: existingID}
	err = bm2.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bm2.ID != existingID {
		t.Errorf("expected ID to remain %s, got %s", existingID, bm2.ID)
	}
}
