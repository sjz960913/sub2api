//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	collabservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

func TestCollaborationCrossInstanceSyncAndCommandWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	user := createCollaborationTestUser(t, decimal.NewFromInt(1), decimal.Zero)
	repository := NewCollaborationRepository(integrationDB)
	device := registerCollaborationTestDevice(t, repository, user.ID)

	redisServer := miniredis.RunT(t)
	pcRedis := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	mobileRedis := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() {
		_ = pcRedis.Close()
		_ = mobileRedis.Close()
	})
	cfg := &config.Config{Collaboration: config.CollaborationConfig{
		ProtocolVersion: 1, PresenceTTLSeconds: 45,
		TaskFeeAmount: "0.100000", TaskFeeCurrency: "USD",
		CommandTTLSeconds: 300, SyncTTLSeconds: 10,
		MaxPromptBytes: 32 * 1024, MaxEventBytes: 1024 * 1024,
		MaxCommandsPerUserMinute: 20,
	}}
	pcBus := NewCollaborationEventBus(pcRedis)
	mobileBus := NewCollaborationEventBus(mobileRedis)
	pcPayloads := NewCollaborationPayloadStore(pcRedis, cfg)
	mobilePayloads := NewCollaborationPayloadStore(mobileRedis, cfg)
	pcService, err := collabservice.NewService(repository, cfg.Collaboration)
	if err != nil {
		t.Fatalf("NewService(pc) error = %v", err)
	}
	mobileService, err := collabservice.NewService(repository, cfg.Collaboration)
	if err != nil {
		t.Fatalf("NewService(mobile) error = %v", err)
	}
	pcService.SetPresenceStore(NewCollaborationPresenceStore(pcRedis, cfg))
	pcService.SetRealtime(pcBus, pcPayloads)
	mobileService.SetPresenceStore(NewCollaborationPresenceStore(mobileRedis, cfg))
	mobileService.SetRealtime(mobileBus, mobilePayloads)
	limiter := concreteCollaborationCommandRateLimiter(t, mobileRedis, cfg)
	limiter.now = func(context.Context) (time.Time, error) { return time.Now().UTC(), nil }
	mobileService.SetCommandRateLimiter(limiter)

	pcSubscription, err := pcBus.SubscribeDevice(ctx, device.ID)
	if err != nil {
		t.Fatalf("SubscribeDevice() error = %v", err)
	}
	t.Cleanup(func() { _ = pcSubscription.Close() })
	mobileSubscription, err := mobileBus.SubscribeUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("SubscribeUser() error = %v", err)
	}
	t.Cleanup(func() { _ = mobileSubscription.Close() })
	if _, err := pcService.RecordHeartbeat(ctx, user.ID, device.ID, "ready", 1); err != nil {
		t.Fatalf("RecordHeartbeat() error = %v", err)
	}

	syncResult, err := mobileService.RequestSync(ctx, user.ID, collabservice.RequestSyncInput{
		DeviceID: device.ID, IdempotencyKey: uuid.New(), Kind: collabdomain.SyncKindSessionList,
		Payload: map[string]any{"limit": 50, "archived": false},
	})
	if err != nil {
		t.Fatalf("RequestSync() error = %v", err)
	}
	requireCollaborationEvent(t, pcSubscription.Events(), "session_sync.requested")
	syncID := syncResult.Sync.ID.String()
	if err := pcService.HandleDeviceEvent(ctx, user.ID, device.ID, collabservice.EventEnvelope{
		Version: 1, Type: "session_sync.completed", EventID: uuid.NewString(),
		Payload: map[string]any{
			"sync_id": syncID, "snapshot_version": int64(1),
			"items": []any{map[string]any{
				"thread_id": "thread_123", "title": "Realtime workflow", "archived": false,
				"updated_at": time.Now().UTC().Format(time.RFC3339), "write_state": "writable_loaded",
			}},
		},
	}); err != nil {
		t.Fatalf("HandleDeviceEvent(sync completed) error = %v", err)
	}
	requireCollaborationEvent(t, mobileSubscription.Events(), "session_sync.completed")
	syncState, payload, err := mobileService.GetSyncResult(ctx, user.ID, syncResult.Sync.ID)
	if err != nil || syncState.Status != collabdomain.SyncStatusCompleted {
		t.Fatalf("GetSyncResult() = %#v, %s, %v", syncState, payload, err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		t.Fatalf("sync snapshot = %#v, %v", snapshot, err)
	}
	items, ok := snapshot["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("sync snapshot items = %#v", snapshot["items"])
	}

	commandResult, err := mobileService.DispatchCommand(ctx, user.ID, collabservice.SubmitCommandInput{
		DeviceID: device.ID, ThreadID: "thread_123", IdempotencyKey: uuid.New(), Prompt: "continue the task",
	})
	if err != nil {
		t.Fatalf("DispatchCommand() error = %v", err)
	}
	dispatched := requireCollaborationEvent(t, pcSubscription.Events(), "command.dispatched")
	dispatchedJSON, _ := json.Marshal(dispatched.Payload)
	if !strings.Contains(string(dispatchedJSON), "continue the task") {
		t.Fatalf("dispatched payload omitted prompt: %s", dispatchedJSON)
	}
	commandID := commandResult.Command.ID.String()
	if err := pcService.HandleDeviceEvent(ctx, user.ID, device.ID, collabservice.EventEnvelope{
		Version: 1, Type: "command.received", EventID: uuid.NewString(),
		Payload: map[string]any{"command_id": commandID},
	}); err != nil {
		t.Fatalf("HandleDeviceEvent(command received) error = %v", err)
	}
	requireCollaborationEvent(t, mobileSubscription.Events(), "command.received")
	if err := pcService.HandleDeviceEvent(ctx, user.ID, device.ID, collabservice.EventEnvelope{
		Version: 1, Type: "command.started", EventID: uuid.NewString(),
		Payload: map[string]any{"command_id": commandID, "turn_id": "turn_123"},
	}); err != nil {
		t.Fatalf("HandleDeviceEvent(command started) error = %v", err)
	}
	requireCollaborationEvent(t, mobileSubscription.Events(), "command.started")
	if err := pcService.HandleDeviceEvent(ctx, user.ID, device.ID, collabservice.EventEnvelope{
		Version: 1, Type: "command.completed", EventID: uuid.NewString(),
		Payload: map[string]any{"command_id": commandID},
	}); err != nil {
		t.Fatalf("HandleDeviceEvent(command completed) error = %v", err)
	}
	requireCollaborationEvent(t, mobileSubscription.Events(), "command.completed")
	commandState, err := mobileService.GetCommand(ctx, user.ID, commandResult.Command.ID)
	if err != nil || commandState.Status != collabdomain.CommandStatusCompleted || commandState.TurnID == nil || *commandState.TurnID != "turn_123" {
		t.Fatalf("GetCommand() = %#v, %v", commandState, err)
	}
	assertCollaborationCounts(t, user.ID, 1, 1)
	var balance string
	if err := integrationDB.QueryRow(`SELECT balance::text FROM users WHERE id = $1`, user.ID).Scan(&balance); err != nil {
		t.Fatalf("query user balance: %v", err)
	}
	if balance != "0.90000000" {
		t.Fatalf("balance = %s, want one backend charge", balance)
	}
}

func requireCollaborationEvent(
	t *testing.T,
	events <-chan collabservice.EventEnvelope,
	wantType string,
) collabservice.EventEnvelope {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatalf("event stream closed while waiting for %s", wantType)
		}
		if event.Type != wantType {
			t.Fatalf("event type = %s, want %s", event.Type, wantType)
		}
		return event
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", wantType)
		return collabservice.EventEnvelope{}
	}
}
