package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestCollaborationEventBusCrossInstanceDeliveryAndSequence(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	firstClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	secondClient := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = firstClient.Close()
		_ = secondClient.Close()
	})
	firstBus := NewCollaborationEventBus(firstClient)
	secondBus := NewCollaborationEventBus(secondClient)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	subscription, err := secondBus.SubscribeUser(ctx, 42)
	if err != nil {
		t.Fatalf("SubscribeUser() error = %v", err)
	}
	t.Cleanup(func() { _ = subscription.Close() })
	requestID := uuid.NewString()

	first, err := firstBus.PublishUser(ctx, 42, "command.started", &requestID, map[string]any{"command_id": requestID})
	if err != nil {
		t.Fatalf("PublishUser(first) error = %v", err)
	}
	second, err := firstBus.PublishUser(ctx, 42, "command.completed", &requestID, map[string]any{"command_id": requestID})
	if err != nil {
		t.Fatalf("PublishUser(second) error = %v", err)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("sequences = %d/%d, want 1/2", first.Sequence, second.Sequence)
	}

	for _, wantType := range []string{"command.started", "command.completed"} {
		select {
		case event := <-subscription.Events():
			if event.Type != wantType || event.RequestID == nil || *event.RequestID != requestID {
				t.Fatalf("event = %#v, want type %s", event, wantType)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s", wantType)
		}
	}
}

func TestCollaborationEventBusSeparatesDeviceChannels(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	bus := NewCollaborationEventBus(client)
	deviceID := uuid.New()
	otherDeviceID := uuid.New()
	subscription, err := bus.SubscribeDevice(context.Background(), deviceID)
	if err != nil {
		t.Fatalf("SubscribeDevice() error = %v", err)
	}
	t.Cleanup(func() { _ = subscription.Close() })

	if _, err := bus.PublishDevice(context.Background(), 42, otherDeviceID, "command.dispatched", nil, nil); err != nil {
		t.Fatalf("PublishDevice(other) error = %v", err)
	}
	if _, err := bus.PublishDevice(context.Background(), 42, deviceID, "command.dispatched", nil, nil); err != nil {
		t.Fatalf("PublishDevice(target) error = %v", err)
	}
	select {
	case event := <-subscription.Events():
		if event.Type != "command.dispatched" {
			t.Fatalf("event type = %s", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for device event")
	}
}
