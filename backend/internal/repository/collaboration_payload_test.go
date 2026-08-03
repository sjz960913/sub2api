package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestCollaborationPayloadStoreSeparatesTenantsAndExpires(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewCollaborationPayloadStore(client, &config.Config{Collaboration: config.CollaborationConfig{
		CommandPayloadTTLSeconds:  2,
		SessionSnapshotTTLSeconds: 3,
		ThreadSnapshotTTLSeconds:  4,
		MaxEventBytes:             1024,
	}})
	ctx := context.Background()
	commandID := uuid.New()
	syncID := uuid.New()

	if err := store.PutCommand(ctx, 42, commandID, "fix the login flow"); err != nil {
		t.Fatalf("PutCommand() error = %v", err)
	}
	if _, err := store.GetCommand(ctx, 7, commandID); !errors.Is(err, collaborationservice.ErrPayloadNotFound) {
		t.Fatalf("cross-tenant GetCommand() error = %v", err)
	}
	if err := store.PutSync(ctx, 42, syncID, collabdomain.SyncKindSessionList, []byte(`{"items":[]}`)); err != nil {
		t.Fatalf("PutSync() error = %v", err)
	}
	payload, err := store.GetSync(ctx, 42, syncID, collabdomain.SyncKindSessionList)
	if err != nil || string(payload) != `{"items":[]}` {
		t.Fatalf("GetSync() = %q, %v", payload, err)
	}

	server.FastForward(4 * time.Second)
	if _, err := store.GetCommand(ctx, 42, commandID); !errors.Is(err, collaborationservice.ErrPayloadNotFound) {
		t.Fatalf("expired GetCommand() error = %v", err)
	}
	if _, err := store.GetSync(ctx, 42, syncID, collabdomain.SyncKindSessionList); !errors.Is(err, collaborationservice.ErrPayloadNotFound) {
		t.Fatalf("expired GetSync() error = %v", err)
	}
}

func TestCollaborationPayloadStoreRejectsInvalidOrOversizedJSON(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewCollaborationPayloadStore(client, &config.Config{Collaboration: config.CollaborationConfig{
		MaxEventBytes: 8,
	}})

	for _, payload := range [][]byte{[]byte(`not-json`), []byte(`{"payload":"too large"}`)} {
		err := store.PutSync(context.Background(), 42, uuid.New(), collabdomain.SyncKindThreadSnapshot, payload)
		if !errors.Is(err, collaborationservice.ErrInvalidArgument) {
			t.Fatalf("PutSync(%q) error = %v", payload, err)
		}
	}
}
