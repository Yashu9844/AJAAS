package services

import (
	"testing"
	"time"

	"github.com/jaas/jaas/internal/shared/config"
)

func TestTokenService_AccessToken(t *testing.T) {
	cfg := &config.JWTConfig{
		Secret:          "my_super_secret_signing_key_minimum_256_bits_for_safety",
		AccessTokenTTL:  15,
		RefreshTokenTTL: 7,
	}

	svc := NewTokenService(cfg)

	userID := "user-123"
	tenantID := "tenant-456"
	email := "user@example.com"
	roles := []string{"tenant_admin", "member"}
	sessionID := "session-789"

	tokenStr, expiresAt, err := svc.GenerateAccessToken(userID, tenantID, email, roles, sessionID)
	if err != nil {
		t.Fatalf("failed to generate access token: %v", err)
	}

	if tokenStr == "" {
		t.Fatal("generated token string is empty")
	}

	if expiresAt.Before(time.Now()) {
		t.Error("expiration time is in the past")
	}

	// Validate token
	claims, err := svc.ValidateAccessToken(tokenStr)
	if err != nil {
		t.Fatalf("failed to validate access token: %v", err)
	}

	if claims.Subject != userID {
		t.Errorf("expected Subject %q, got %q", userID, claims.Subject)
	}

	if claims.TenantID != tenantID {
		t.Errorf("expected TenantID %q, got %q", tenantID, claims.TenantID)
	}

	if claims.Email != email {
		t.Errorf("expected Email %q, got %q", email, claims.Email)
	}

	if len(claims.Roles) != 2 || claims.Roles[0] != "tenant_admin" {
		t.Errorf("expected Roles %v, got %v", roles, claims.Roles)
	}

	if claims.SessionID != sessionID {
		t.Errorf("expected SessionID %q, got %q", sessionID, claims.SessionID)
	}
}

func TestTokenService_OpaqueToken(t *testing.T) {
	cfg := &config.JWTConfig{
		Secret:          "secret",
		AccessTokenTTL:  15,
		RefreshTokenTTL: 7,
	}

	svc := NewTokenService(cfg)

	raw, hash, err := svc.GenerateOpaqueToken()
	if err != nil {
		t.Fatalf("failed to generate opaque token: %v", err)
	}

	if raw == "" || hash == "" {
		t.Error("raw token or hash is empty")
	}

	// Verify hashing function matches
	expectedHash := svc.HashOpaqueToken(raw)
	if expectedHash != hash {
		t.Errorf("expected hash %q, got %q", expectedHash, hash)
	}
}
