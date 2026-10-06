package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/shared/config"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
)

func TestAuditService_List(t *testing.T) {
	repo := &MockAuditLogRepository{}
	uid := uuid.New()
	rid := "res"
	repo.FindByTenantIDFunc = func(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.AuditLog, int64, error) {
		l := models.AuditLog{TenantID: tenantID, UserID: &uid, Action: "user.created", Resource: "user", ResourceID: &rid, CreatedAt: time.Now()}
		l.ID = uuid.New()
		return []models.AuditLog{l, {Action: "x", Resource: "y"}}, 41, nil
	}
	nop := zerolog.Nop()
	svc := NewAuditService(repo, &nop)
	res, err := svc.List(context.Background(), nil, uuid.New(), 2, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) != 2 || res.Data[0].UserID == nil || *res.Data[0].UserID != uid.String() || res.Data[1].UserID != nil {
		t.Fatalf("mapping wrong: %+v", res.Data)
	}
	if res.Meta.TotalItems != 41 || res.Meta.TotalPages != 3 || res.Meta.Page != 2 {
		t.Fatalf("meta wrong: %+v", res.Meta)
	}
	repo.FindByTenantIDFunc = func(context.Context, *gorm.DB, uuid.UUID, int, int) ([]models.AuditLog, int64, error) {
		return nil, 0, errors.New("db down")
	}
	if _, err := svc.List(context.Background(), nil, uuid.New(), 1, 20); err == nil {
		t.Fatal("repo failure must surface")
	}
}

func TestTokenService_RefreshTTLAndValidation(t *testing.T) {
	svc := NewTokenService(&config.JWTConfig{Secret: "0123456789abcdef0123456789abcdef", AccessTokenTTL: 15, RefreshTokenTTL: 3})
	if svc.RefreshTokenTTL() != 72*time.Hour {
		t.Fatalf("configured refresh TTL ignored: %v", svc.RefreshTokenTTL())
	}
	if NewTokenService(&config.JWTConfig{}).RefreshTokenTTL() != 7*24*time.Hour {
		t.Fatal("default refresh TTL must be 7 days")
	}
	tok, _, err := svc.GenerateAccessToken("u", "t", "e@x.test", []string{"r"}, "s")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateAccessToken(tok); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if _, err := svc.ValidateAccessToken(tok + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
	other := NewTokenService(&config.JWTConfig{Secret: "another-secret-another-secret-123456", AccessTokenTTL: 15})
	if _, err := other.ValidateAccessToken(tok); err == nil {
		t.Fatal("token signed with a different secret accepted")
	}
	expired := NewTokenService(&config.JWTConfig{Secret: "0123456789abcdef0123456789abcdef", AccessTokenTTL: -1})
	old, _, _ := expired.GenerateAccessToken("u", "t", "e", nil, "s")
	if _, err := svc.ValidateAccessToken(old); err == nil {
		t.Fatal("expired token accepted")
	}
	// alg=none must be refused
	if _, err := svc.ValidateAccessToken("eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ1In0."); err == nil {
		t.Fatal("alg=none token accepted")
	}
	raw1, h1, _ := svc.GenerateOpaqueToken()
	raw2, _, _ := svc.GenerateOpaqueToken()
	if raw1 == raw2 || svc.HashOpaqueToken(raw1) != h1 || len(h1) != 64 {
		t.Fatal("opaque token generation broken")
	}
}

type fakeConverger struct{ called bool }

func (f *fakeConverger) DeactivateUserMappings(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) error {
	f.called = true
	return nil
}

func TestUserDeactivationConvergerHook(t *testing.T) {
	fc := &fakeConverger{}
	SetUserDeactivationConverger(fc)
	defer SetUserDeactivationConverger(nil)
	e := newEnv(&faults{})
	if err := e.users().DeactivateUser(ctx, nil, e.tenantID, e.userID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if !fc.called {
		t.Fatal("deactivation must run the registered converger (FR-M005)")
	}
}

func TestSessionServiceTTL(t *testing.T) {
	repo := &MockSessionRepository{}
	var got *models.Session
	repo.CreateFunc = func(ctx context.Context, tx *gorm.DB, s *models.Session) error { got = s; return nil }
	s := NewSessionService(repo, nil, 45*time.Minute)
	if _, err := s.CreateSession(context.Background(), nil, uuid.New(), uuid.New(), "ip", "ua"); err != nil {
		t.Fatal(err)
	}
	if d := time.Until(got.ExpiresAt); d < 44*time.Minute || d > 46*time.Minute {
		t.Fatalf("session lifetime must follow the configured TTL, got %v", d)
	}
	repo.CreateFunc = func(context.Context, *gorm.DB, *models.Session) error { return errors.New("db") }
	if _, err := NewSessionService(repo, nil).CreateSession(context.Background(), nil, uuid.New(), uuid.New(), "", ""); err == nil {
		t.Fatal("create failure must surface")
	}
	repo.RevokeAllByUserIDFunc = func(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error { return errors.New("db") }
	if err := NewSessionService(repo, nil).RevokeAllForUser(context.Background(), nil, uuid.New(), uuid.New()); err == nil {
		t.Fatal("revoke-all failure must surface")
	}
}
