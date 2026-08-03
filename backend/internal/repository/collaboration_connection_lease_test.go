package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestCollaborationConnectionLeaseEnforcesUserAndDeviceLimits(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	otherClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		_ = otherClient.Close()
	})
	cfg := &config.Config{Collaboration: config.CollaborationConfig{
		PresenceTTLSeconds:      45,
		MaxConnectionsPerUser:   2,
		MaxConnectionsPerDevice: 1,
	}}
	store := concreteCollaborationConnectionLeaseStore(t, client, cfg)
	otherStore := concreteCollaborationConnectionLeaseStore(t, otherClient, cfg)
	ctx := context.Background()
	deviceID := uuid.New()
	first := collaborationservice.ConnectionLease{UserID: 42, DeviceID: deviceID, ConnectionID: uuid.New()}
	second := collaborationservice.ConnectionLease{UserID: 42, DeviceID: deviceID, ConnectionID: uuid.New()}
	mobile := collaborationservice.ConnectionLease{UserID: 42, ConnectionID: uuid.New()}
	third := collaborationservice.ConnectionLease{UserID: 42, DeviceID: uuid.New(), ConnectionID: uuid.New()}

	assertLeaseAcquire(t, store, ctx, first, true)
	assertLeaseAcquire(t, otherStore, ctx, second, false)
	assertLeaseAcquire(t, otherStore, ctx, mobile, true)
	assertLeaseAcquire(t, otherStore, ctx, third, false)
	if err := store.Release(ctx, first); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	assertLeaseAcquire(t, otherStore, ctx, third, true)
}

func TestCollaborationConnectionLeaseExpiresAndRenews(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := concreteCollaborationConnectionLeaseStore(t, client, &config.Config{Collaboration: config.CollaborationConfig{
		PresenceTTLSeconds:      1,
		MaxConnectionsPerUser:   1,
		MaxConnectionsPerDevice: 1,
	}})
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	store.now = func(context.Context) (time.Time, error) { return now, nil }
	ctx := context.Background()
	deviceID := uuid.New()
	first := collaborationservice.ConnectionLease{UserID: 42, DeviceID: deviceID, ConnectionID: uuid.New()}
	second := collaborationservice.ConnectionLease{UserID: 42, DeviceID: deviceID, ConnectionID: uuid.New()}

	assertLeaseAcquire(t, store, ctx, first, true)
	now = now.Add(500 * time.Millisecond)
	assertLeaseRenew(t, store, ctx, first, true)
	now = now.Add(1100 * time.Millisecond)
	assertLeaseRenew(t, store, ctx, first, false)
	assertLeaseAcquire(t, store, ctx, second, true)
}

func concreteCollaborationConnectionLeaseStore(
	t *testing.T,
	client *redis.Client,
	cfg *config.Config,
) *collaborationConnectionLeaseStore {
	t.Helper()
	store, ok := NewCollaborationConnectionLeaseStore(client, cfg).(*collaborationConnectionLeaseStore)
	if !ok {
		t.Fatal("NewCollaborationConnectionLeaseStore() returned unexpected implementation")
	}
	return store
}

func assertLeaseAcquire(
	t *testing.T,
	store *collaborationConnectionLeaseStore,
	ctx context.Context,
	lease collaborationservice.ConnectionLease,
	want bool,
) {
	t.Helper()
	acquired, err := store.Acquire(ctx, lease)
	assertLeaseResult(t, acquired, err, want)
}

func assertLeaseRenew(
	t *testing.T,
	store *collaborationConnectionLeaseStore,
	ctx context.Context,
	lease collaborationservice.ConnectionLease,
	want bool,
) {
	t.Helper()
	acquired, err := store.Renew(ctx, lease)
	assertLeaseResult(t, acquired, err, want)
}

func assertLeaseResult(t *testing.T, acquired bool, err error, want bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("lease operation error = %v", err)
	}
	if acquired != want {
		t.Fatalf("lease acquired = %t, want %t", acquired, want)
	}
}
