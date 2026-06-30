package models

import (
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestGORMModels_Parsing(t *testing.T) {
	models := []interface{}{
		&Tenant{},
		&TenantSettings{},
		&User{},
		&Role{},
		&Permission{},
		&UserRole{},
		&RolePermission{},
		&Session{},
		&RefreshToken{},
		&PasswordResetToken{},
		&MFAConfig{},
		&AuditLog{},
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

func TestGORMModels_AuditLog_NoSoftDelete(t *testing.T) {
	cacheStore := &sync.Map{}
	s, err := schema.Parse(&AuditLog{}, cacheStore, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("failed to parse AuditLog: %v", err)
	}

	for _, field := range s.Fields {
		if field.Name == "DeletedAt" || field.Name == "UpdatedAt" {
			t.Errorf("unexpected field %s in AuditLog (should be insert-only, no soft delete / updated_at)", field.Name)
		}
	}
}

func TestGORMModels_BaseHook(t *testing.T) {
	// Verify BeforeCreate hook is defined on BaseModel
	tenant := &Tenant{}
	err := tenant.BeforeCreate(nil)
	if err != nil {
		t.Fatalf("BeforeCreate failed: %v", err)
	}
	if tenant.ID.String() == "" {
		t.Error("BeforeCreate failed to generate UUID for Tenant")
	}
}
