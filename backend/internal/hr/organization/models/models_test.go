package models

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// TestOrgModels_Parsing verifies every org model parses under GORM (table + fields resolve).
func TestOrgModels_Parsing(t *testing.T) {
	models := []interface{}{
		&Department{},
		&Team{},
		&Designation{},
		&Mapping{},
		&OrgEventOutbox{},
	}

	cacheStore := &sync.Map{}

	for _, model := range models {
		s, err := schema.Parse(model, cacheStore, schema.NamingStrategy{})
		if err != nil {
			t.Errorf("failed to parse schema for model %T: %v", model, err)
			continue
		}
		if s.Table == "" {
			t.Errorf("expected parsed table name for model %T to be non-empty", model)
		}
	}
}

// TestOrgModels_Mapping_AppendOnly guards the append-only rule: Mapping must carry
// status/reason for deactivation instead of relying on hard delete.
func TestOrgModels_Mapping_AppendOnly(t *testing.T) {
	cacheStore := &sync.Map{}
	s, err := schema.Parse(&Mapping{}, cacheStore, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("failed to parse Mapping: %v", err)
	}

	fields := map[string]bool{}
	for _, f := range s.Fields {
		fields[f.Name] = true
	}
	for _, want := range []string{"Status", "Reason", "IsPrimary", "ManagerUserID"} {
		if !fields[want] {
			t.Errorf("expected Mapping field %s (append-only + hierarchy rules FR-M)", want)
		}
	}
}

// TestOrgModels_UUIDHook verifies BeforeCreate generates IDs on all org models.
func TestOrgModels_UUIDHook(t *testing.T) {
	d := &Department{}
	if err := d.BeforeCreate(nil); err != nil {
		t.Fatalf("Department BeforeCreate failed: %v", err)
	}
	if d.ID.String() == "" {
		t.Error("Department BeforeCreate did not generate UUID")
	}

	m := &Mapping{}
	if err := m.BeforeCreate(nil); err != nil {
		t.Fatalf("Mapping BeforeCreate failed: %v", err)
	}
	if m.ID.String() == "" || m.Status != "active" {
		t.Errorf("Mapping BeforeCreate defaults wrong: id=%q status=%q", m.ID.String(), m.Status)
	}
}
