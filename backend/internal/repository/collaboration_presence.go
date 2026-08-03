package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collaborationdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const collaborationPresencePrefix = "collaboration:presence:device:"

type collaborationPresenceStore struct {
	redis *redis.Client
	ttl   time.Duration
}

type collaborationPresenceValue struct {
	DeviceID          uuid.UUID `json:"device_id"`
	UserID            int64     `json:"user_id"`
	Status            string    `json:"status"`
	AppServerStatus   string    `json:"app_server_status"`
	ActiveThreadCount int       `json:"active_thread_count"`
	LastSeenAt        time.Time `json:"last_seen_at"`
}

func NewCollaborationPresenceStore(
	redisClient *redis.Client,
	cfg *config.Config,
) collaborationservice.PresenceStore {
	ttlSeconds := 45
	if cfg != nil && cfg.Collaboration.PresenceTTLSeconds > 0 {
		ttlSeconds = cfg.Collaboration.PresenceTTLSeconds
	}
	return &collaborationPresenceStore{
		redis: redisClient,
		ttl:   time.Duration(ttlSeconds) * time.Second,
	}
}

func (s *collaborationPresenceStore) Touch(ctx context.Context, presence collaborationservice.DevicePresence) error {
	if s == nil || s.redis == nil || presence.DeviceID == uuid.Nil || presence.UserID <= 0 || !presence.Status.Valid() || presence.Status == collaborationdomain.DeviceStatusRevoked {
		return collaborationservice.ErrInvalidArgument
	}
	value, err := json.Marshal(collaborationPresenceValue{
		DeviceID:          presence.DeviceID,
		UserID:            presence.UserID,
		Status:            string(presence.Status),
		AppServerStatus:   presence.AppServerStatus,
		ActiveThreadCount: presence.ActiveThreadCount,
		LastSeenAt:        presence.LastSeenAt.UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal collaboration presence: %w", err)
	}
	return s.redis.Set(ctx, collaborationPresenceKey(presence.DeviceID), value, s.ttl).Err()
}

func (s *collaborationPresenceStore) GetMany(
	ctx context.Context,
	deviceIDs []uuid.UUID,
) (map[uuid.UUID]collaborationservice.DevicePresence, error) {
	result := make(map[uuid.UUID]collaborationservice.DevicePresence, len(deviceIDs))
	if len(deviceIDs) == 0 {
		return result, nil
	}
	if s == nil || s.redis == nil {
		return nil, collaborationservice.ErrInvariantViolation
	}
	keys := make([]string, 0, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		keys = append(keys, collaborationPresenceKey(deviceID))
	}
	values, err := s.redis.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	for index, raw := range values {
		text, ok := raw.(string)
		if !ok || text == "" {
			continue
		}
		var value collaborationPresenceValue
		if err := json.Unmarshal([]byte(text), &value); err != nil {
			continue
		}
		status := collaborationdomain.DeviceStatus(value.Status)
		if value.DeviceID != deviceIDs[index] || value.UserID <= 0 || !status.Valid() || status == collaborationdomain.DeviceStatusRevoked {
			continue
		}
		result[value.DeviceID] = collaborationservice.DevicePresence{
			DeviceID:          value.DeviceID,
			UserID:            value.UserID,
			Status:            status,
			AppServerStatus:   value.AppServerStatus,
			ActiveThreadCount: value.ActiveThreadCount,
			LastSeenAt:        value.LastSeenAt.UTC(),
		}
	}
	return result, nil
}

func (s *collaborationPresenceStore) Remove(ctx context.Context, deviceID uuid.UUID) error {
	if s == nil || s.redis == nil || deviceID == uuid.Nil {
		return collaborationservice.ErrInvalidArgument
	}
	return s.redis.Del(ctx, collaborationPresenceKey(deviceID)).Err()
}

func collaborationPresenceKey(deviceID uuid.UUID) string {
	return collaborationPresencePrefix + deviceID.String()
}
