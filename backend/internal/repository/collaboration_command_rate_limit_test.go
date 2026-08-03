package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestCollaborationCommandRateLimiterIsCrossInstanceAndIdempotent(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	firstClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	secondClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = firstClient.Close()
		_ = secondClient.Close()
	})
	cfg := &config.Config{Collaboration: config.CollaborationConfig{MaxCommandsPerUserMinute: 2}}
	first := concreteCollaborationCommandRateLimiter(t, firstClient, cfg)
	second := concreteCollaborationCommandRateLimiter(t, secondClient, cfg)
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	first.now = func(context.Context) (time.Time, error) { return now, nil }
	second.now = func(context.Context) (time.Time, error) { return now, nil }
	ctx := context.Background()
	firstKey := uuid.New()
	secondKey := uuid.New()
	thirdKey := uuid.New()

	assertCommandRateLimit(t, first, ctx, 42, firstKey, true)
	assertCommandRateLimit(t, second, ctx, 42, firstKey, true)
	assertCommandRateLimit(t, second, ctx, 42, secondKey, true)
	assertCommandRateLimit(t, first, ctx, 42, thirdKey, false)
	assertCommandRateLimit(t, first, ctx, 43, thirdKey, true)

	now = now.Add(time.Minute + time.Millisecond)
	assertCommandRateLimit(t, second, ctx, 42, thirdKey, true)
}

func concreteCollaborationCommandRateLimiter(
	t *testing.T,
	client *redis.Client,
	cfg *config.Config,
) *collaborationCommandRateLimiter {
	t.Helper()
	limiter, ok := NewCollaborationCommandRateLimiter(client, cfg).(*collaborationCommandRateLimiter)
	if !ok {
		t.Fatal("NewCollaborationCommandRateLimiter() returned unexpected implementation")
	}
	return limiter
}

func assertCommandRateLimit(
	t *testing.T,
	limiter *collaborationCommandRateLimiter,
	ctx context.Context,
	userID int64,
	idempotencyKey uuid.UUID,
	want bool,
) {
	t.Helper()
	allowed, err := limiter.Allow(ctx, userID, idempotencyKey)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if allowed != want {
		t.Fatalf("Allow() = %t, want %t", allowed, want)
	}
}
