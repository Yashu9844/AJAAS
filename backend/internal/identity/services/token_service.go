package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/shared/config"
)

// UserClaims defines the payload structures inside access token JWTs.
type UserClaims struct {
	jwt.RegisteredClaims
	TenantID  string   `json:"tid"`
	Email     string   `json:"email"`
	Roles     []string `json:"roles"`
	SessionID string   `json:"sid"`
}

// TokenService manages secure token lifecycle operations.
type TokenService interface {
	GenerateAccessToken(userID, tenantID, email string, roles []string, sessionID string) (string, time.Time, error)
	ValidateAccessToken(tokenStr string) (*UserClaims, error)
	GenerateOpaqueToken() (string, string, error) // Returns (rawToken, tokenHash, error)
	HashOpaqueToken(token string) string
}

type tokenService struct {
	cfg *config.JWTConfig
}

// NewTokenService creates a new TokenService.
func NewTokenService(cfg *config.JWTConfig) TokenService {
	return &tokenService{cfg: cfg}
}

func (s *tokenService) GenerateAccessToken(userID, tenantID, email string, roles []string, sessionID string) (string, time.Time, error) {
	now := time.Now()
	ttl := time.Duration(s.cfg.AccessTokenTTL) * time.Minute
	expiresAt := now.Add(ttl)

	claims := &UserClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    "jaas-identity",
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.New().String(),
		},
		TenantID:  tenantID,
		Email:     email,
		Roles:     roles,
		SessionID: sessionID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(s.cfg.Secret))
	if err != nil {
		return "", time.Time{}, err
	}

	return signed, expiresAt, nil
}

func (s *tokenService) ValidateAccessToken(tokenStr string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signature method is HMAC
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.Secret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*UserClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// GenerateOpaqueToken yields a cryptographically secure random token and its SHA-256 hash.
func (s *tokenService) GenerateOpaqueToken() (string, string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate secure random bytes: %w", err)
	}

	// base64 url-encoding safe for queries
	rawToken := base64.RawURLEncoding.EncodeToString(bytes)
	tokenHash := s.HashOpaqueToken(rawToken)

	return rawToken, tokenHash, nil
}

// HashOpaqueToken takes raw base64 string and computes SHA-256 hex string.
func (s *tokenService) HashOpaqueToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
