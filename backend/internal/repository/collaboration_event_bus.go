package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	collaborationUserEventChannelPrefix   = "collaboration:events:user:"
	collaborationDeviceEventChannelPrefix = "collaboration:events:device:"
	collaborationUserSequencePrefix       = "collaboration:sequence:user:"
	collaborationEventBufferSize          = 64
)

type collaborationEventBus struct {
	redis *redis.Client
	now   func() time.Time
}

func NewCollaborationEventBus(redisClient *redis.Client) collaborationservice.EventBus {
	return &collaborationEventBus{redis: redisClient, now: time.Now}
}

func (b *collaborationEventBus) PublishUser(
	ctx context.Context,
	userID int64,
	eventType string,
	requestID *string,
	payload map[string]any,
) (collaborationservice.EventEnvelope, error) {
	if userID <= 0 {
		return collaborationservice.EventEnvelope{}, collaborationservice.ErrInvalidArgument
	}
	return b.publish(ctx, userID, collaborationUserEventChannel(userID), eventType, requestID, payload)
}

func (b *collaborationEventBus) PublishDevice(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	eventType string,
	requestID *string,
	payload map[string]any,
) (collaborationservice.EventEnvelope, error) {
	if userID <= 0 || deviceID == uuid.Nil {
		return collaborationservice.EventEnvelope{}, collaborationservice.ErrInvalidArgument
	}
	return b.publish(ctx, userID, collaborationDeviceEventChannel(deviceID), eventType, requestID, payload)
}

func (b *collaborationEventBus) publish(
	ctx context.Context,
	userID int64,
	channel string,
	eventType string,
	requestID *string,
	payload map[string]any,
) (collaborationservice.EventEnvelope, error) {
	if b == nil || b.redis == nil || !collaborationservice.ValidEventType(eventType) {
		return collaborationservice.EventEnvelope{}, collaborationservice.ErrInvalidArgument
	}
	sequence, err := b.redis.Incr(ctx, collaborationUserSequenceKey(userID)).Result()
	if err != nil {
		return collaborationservice.EventEnvelope{}, err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	event := collaborationservice.EventEnvelope{
		Version:    1,
		Type:       eventType,
		EventID:    uuid.NewString(),
		RequestID:  requestID,
		Sequence:   sequence,
		OccurredAt: b.now().UTC(),
		Payload:    payload,
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return collaborationservice.EventEnvelope{}, fmt.Errorf("marshal collaboration event: %w", err)
	}
	if err := b.redis.Publish(ctx, channel, encoded).Err(); err != nil {
		return collaborationservice.EventEnvelope{}, err
	}
	return event, nil
}

func (b *collaborationEventBus) SubscribeUser(
	ctx context.Context,
	userID int64,
) (collaborationservice.EventSubscription, error) {
	if userID <= 0 {
		return nil, collaborationservice.ErrInvalidArgument
	}
	return b.subscribe(ctx, collaborationUserEventChannel(userID))
}

func (b *collaborationEventBus) SubscribeDevice(
	ctx context.Context,
	deviceID uuid.UUID,
) (collaborationservice.EventSubscription, error) {
	if deviceID == uuid.Nil {
		return nil, collaborationservice.ErrInvalidArgument
	}
	return b.subscribe(ctx, collaborationDeviceEventChannel(deviceID))
}

func (b *collaborationEventBus) subscribe(
	ctx context.Context,
	channel string,
) (collaborationservice.EventSubscription, error) {
	if b == nil || b.redis == nil {
		return nil, collaborationservice.ErrInvariantViolation
	}
	subscriptionContext, cancel := context.WithCancel(ctx)
	pubsub := b.redis.Subscribe(subscriptionContext, channel)
	if _, err := pubsub.Receive(subscriptionContext); err != nil {
		cancel()
		_ = pubsub.Close()
		return nil, err
	}
	subscription := &collaborationEventSubscription{
		cancel: cancel,
		pubsub: pubsub,
		events: make(chan collaborationservice.EventEnvelope, collaborationEventBufferSize),
	}
	go subscription.run(subscriptionContext)
	return subscription, nil
}

type collaborationEventSubscription struct {
	cancel context.CancelFunc
	pubsub *redis.PubSub
	events chan collaborationservice.EventEnvelope
	once   sync.Once
}

func (s *collaborationEventSubscription) Events() <-chan collaborationservice.EventEnvelope {
	return s.events
}

func (s *collaborationEventSubscription) Close() error {
	var err error
	s.once.Do(func() {
		s.cancel()
		err = s.pubsub.Close()
	})
	return err
}

func (s *collaborationEventSubscription) run(ctx context.Context) {
	defer close(s.events)
	messages := s.pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-messages:
			if !ok {
				return
			}
			var event collaborationservice.EventEnvelope
			if err := json.Unmarshal([]byte(message.Payload), &event); err != nil || event.Version != 1 || !collaborationservice.ValidEventType(event.Type) {
				continue
			}
			select {
			case s.events <- event:
			case <-ctx.Done():
				return
			default:
				// Fail closed for a slow consumer. The WebSocket layer observes the
				// closed stream and clients rebuild authoritative state via REST.
				_ = s.Close()
				return
			}
		}
	}
}

func collaborationUserEventChannel(userID int64) string {
	return fmt.Sprintf("%s%d", collaborationUserEventChannelPrefix, userID)
}

func collaborationDeviceEventChannel(deviceID uuid.UUID) string {
	return collaborationDeviceEventChannelPrefix + deviceID.String()
}

func collaborationUserSequenceKey(userID int64) string {
	return fmt.Sprintf("%s%d", collaborationUserSequencePrefix, userID)
}
