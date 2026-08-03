package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collaborationdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestCollaborationPresenceStoreTouchReadAndExpire(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewCollaborationPresenceStore(client, &config.Config{Collaboration: config.CollaborationConfig{PresenceTTLSeconds: 45}})
	deviceID := uuid.New()
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	presence := collaborationservice.DevicePresence{
		DeviceID:          deviceID,
		UserID:            42,
		Status:            collaborationdomain.DeviceStatusOnline,
		AppServerStatus:   "ready",
		ActiveThreadCount: 1,
		LastSeenAt:        now,
	}
	if err := store.Touch(context.Background(), presence); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}

	items, err := store.GetMany(context.Background(), []uuid.UUID{uuid.New(), deviceID})
	if err != nil {
		t.Fatalf("GetMany() error = %v", err)
	}
	if got, ok := items[deviceID]; !ok || got != presence {
		t.Fatalf("GetMany() presence = %#v, found=%v, want %#v", got, ok, presence)
	}

	server.FastForward(46 * time.Second)
	items, err = store.GetMany(context.Background(), []uuid.UUID{deviceID})
	if err != nil {
		t.Fatalf("GetMany() after expiry error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expired presence remained: %#v", items)
	}
}

func TestCollaborationPresenceStoreRemove(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewCollaborationPresenceStore(client, &config.Config{})
	deviceID := uuid.New()
	if err := store.Touch(context.Background(), collaborationservice.DevicePresence{
		DeviceID:   deviceID,
		UserID:     42,
		Status:     collaborationdomain.DeviceStatusDegraded,
		LastSeenAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}
	if err := store.Remove(context.Background(), deviceID); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if server.Exists(collaborationPresenceKey(deviceID)) {
		t.Fatal("Remove() left presence key")
	}
}
