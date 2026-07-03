package services

import (
	"context"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/identity/models"
	"github.com/jaas/jaas/internal/shared/cache"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func newTestRedisClient(c *redis.Client) *cache.RedisClient {
	rc := &cache.RedisClient{}
	val := reflect.ValueOf(rc).Elem()
	field := val.FieldByName("client")
	ptr := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	ptr.Set(reflect.ValueOf(c))
	return rc
}

func TestSessionService_CreateSession(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	redisClient := newTestRedisClient(redis.NewClient(&redis.Options{Addr: "localhost:9999"}))

	svc := NewSessionService(sessionRepo, redisClient)
	ctx := context.Background()

	sessionRepo.CreateFunc = func(ctx context.Context, tx *gorm.DB, session *models.Session) error {
		session.ID = uuid.New()
		return nil
	}

	session, err := svc.CreateSession(ctx, nil, uuid.New(), uuid.New(), "127.0.0.1", "Chrome")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.ID == uuid.Nil {
		t.Error("expected session ID to be set")
	}
}

func TestSessionService_ValidateSession(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	redisClient := newTestRedisClient(redis.NewClient(&redis.Options{Addr: "localhost:9999"}))
	svc := NewSessionService(sessionRepo, redisClient)
	ctx := context.Background()
	sessionID := uuid.New()

	// GORM Success
	sessionRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Session, error) {
		return &models.Session{
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}, nil
	}

	valid, err := svc.ValidateSession(ctx, nil, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Error("expected session to be valid")
	}

	// GORM Revoked
	sessionRepo.FindByIDFunc = func(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Session, error) {
		now := time.Now()
		return &models.Session{
			ExpiresAt: time.Now().Add(1 * time.Hour),
			RevokedAt: &now,
		}, nil
	}

	valid, err = svc.ValidateSession(ctx, nil, sessionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if valid {
		t.Error("expected session to be invalid (revoked)")
	}
}

func TestSessionService_RevokeSession(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	redisClient := newTestRedisClient(redis.NewClient(&redis.Options{Addr: "localhost:9999"}))
	svc := NewSessionService(sessionRepo, redisClient)

	sessionRepo.RevokeByIDFunc = func(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
		return nil
	}

	err := svc.RevokeSession(context.Background(), nil, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSessionService_RevokeAllForUser(t *testing.T) {
	sessionRepo := &MockSessionRepository{}
	redisClient := newTestRedisClient(redis.NewClient(&redis.Options{Addr: "localhost:9999"}))
	svc := NewSessionService(sessionRepo, redisClient)

	sessionRepo.RevokeAllByUserIDFunc = func(ctx context.Context, tx *gorm.DB, tid, uid uuid.UUID) error {
		return nil
	}

	err := svc.RevokeAllForUser(context.Background(), nil, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
