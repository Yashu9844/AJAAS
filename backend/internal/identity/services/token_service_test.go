package services

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

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

func TestEdDSASigningJWKSAndLegacyWindow(t *testing.T) {
	cfg := &config.JWTConfig{Secret: strings.Repeat("s", 40), AccessTokenTTL: 15}
	ts := NewTokenService(cfg)
	tok, _, err := ts.GenerateAccessToken("u", "t", "e", nil, "sid")
	if err != nil {
		t.Fatal(err)
	}
	if claims, err := ts.ValidateAccessToken(tok); err != nil || claims.Subject != "u" {
		t.Fatalf("EdDSA round trip: %v", err)
	}
	set, ok := KeySetOf(ts)
	if !ok || len(set.Keys) != 1 || set.Keys[0].Alg != "EdDSA" || set.Keys[0].Kty != "OKP" || set.Keys[0].Kid == "" {
		t.Fatalf("jwks: %+v", set)
	}
	if _, ok := KeySetOf(&MockTokenService{}); ok {
		t.Fatal("mocks expose no key set")
	}
	// another key id is rejected
	other := NewTokenService(&config.JWTConfig{Secret: strings.Repeat("o", 40), AccessTokenTTL: 15})
	foreign, _, _ := other.GenerateAccessToken("u", "t", "e", nil, "sid")
	if _, err := ts.ValidateAccessToken(foreign); err == nil {
		t.Fatal("foreign key id must fail")
	}
	// legacy HS256 accepted only while the window is open
	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, &UserClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "u", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}})
	hs, _ := legacy.SignedString([]byte(cfg.Secret))
	if _, err := ts.ValidateAccessToken(hs); err == nil {
		t.Fatal("HS256 refused when the legacy window is closed")
	}
	cfg.AcceptLegacyHS256 = true
	if _, err := ts.ValidateAccessToken(hs); err != nil {
		t.Fatalf("HS256 accepted during the window: %v", err)
	}
	seed := base64.StdEncoding.EncodeToString(make([]byte, 32))
	if _, err := ParseSigningKey(seed); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseSigningKey("c2hvcnQ="); err == nil {
		t.Fatal("short seed rejected")
	}
	if _, err := ParseSigningKey("!!!"); err == nil {
		t.Fatal("non-base64 rejected")
	}
	explicit := NewTokenService(&config.JWTConfig{SigningKey: seed, AccessTokenTTL: 15})
	if tok, _, err := explicit.GenerateAccessToken("u", "t", "e", nil, "s"); err != nil || tok == "" {
		t.Fatal("explicit key signs")
	}
}
