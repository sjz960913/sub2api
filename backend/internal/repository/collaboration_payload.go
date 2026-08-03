package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	collaborationCommandPayloadPrefix  = "collaboration:command-payload:"
	collaborationSessionSnapshotPrefix = "collaboration:session-snapshot:"
	collaborationThreadSnapshotPrefix  = "collaboration:thread-snapshot:"
)

type collaborationPayloadStore struct {
	redis              *redis.Client
	commandTTL         time.Duration
	sessionSnapshotTTL time.Duration
	threadSnapshotTTL  time.Duration
	maxPayloadBytes    int64
}

func NewCollaborationPayloadStore(
	redisClient *redis.Client,
	cfg *config.Config,
) collaborationservice.PayloadStore {
	commandTTL := 5 * time.Minute
	sessionSnapshotTTL := 5 * time.Minute
	threadSnapshotTTL := 10 * time.Minute
	maxPayloadBytes := int64(1024 * 1024)
	if cfg != nil {
		if cfg.Collaboration.CommandPayloadTTLSeconds > 0 {
			commandTTL = time.Duration(cfg.Collaboration.CommandPayloadTTLSeconds) * time.Second
		}
		if cfg.Collaboration.SessionSnapshotTTLSeconds > 0 {
			sessionSnapshotTTL = time.Duration(cfg.Collaboration.SessionSnapshotTTLSeconds) * time.Second
		}
		if cfg.Collaboration.ThreadSnapshotTTLSeconds > 0 {
			threadSnapshotTTL = time.Duration(cfg.Collaboration.ThreadSnapshotTTLSeconds) * time.Second
		}
		if cfg.Collaboration.MaxEventBytes > 0 {
			maxPayloadBytes = cfg.Collaboration.MaxEventBytes
		}
	}
	return &collaborationPayloadStore{
		redis:              redisClient,
		commandTTL:         commandTTL,
		sessionSnapshotTTL: sessionSnapshotTTL,
		threadSnapshotTTL:  threadSnapshotTTL,
		maxPayloadBytes:    maxPayloadBytes,
	}
}

func (s *collaborationPayloadStore) PutCommand(
	ctx context.Context,
	userID int64,
	commandID uuid.UUID,
	prompt string,
) error {
	if err := s.validate(userID, commandID); err != nil {
		return err
	}
	if prompt == "" || int64(len(prompt)) > s.maxPayloadBytes {
		return collaborationservice.ErrInvalidArgument
	}
	return s.redis.Set(ctx, collaborationCommandPayloadKey(userID, commandID), prompt, s.commandTTL).Err()
}

func (s *collaborationPayloadStore) GetCommand(
	ctx context.Context,
	userID int64,
	commandID uuid.UUID,
) (string, error) {
	if err := s.validate(userID, commandID); err != nil {
		return "", err
	}
	value, err := s.redis.Get(ctx, collaborationCommandPayloadKey(userID, commandID)).Result()
	if err == redis.Nil {
		return "", collaborationservice.ErrPayloadNotFound
	}
	return value, err
}

func (s *collaborationPayloadStore) PutSync(
	ctx context.Context,
	userID int64,
	syncID uuid.UUID,
	kind collabdomain.SyncKind,
	payload []byte,
) error {
	key, ttl, err := s.syncKeyAndTTL(userID, syncID, kind)
	if err != nil {
		return err
	}
	if len(payload) == 0 || int64(len(payload)) > s.maxPayloadBytes || !json.Valid(payload) {
		return collaborationservice.ErrInvalidArgument
	}
	return s.redis.Set(ctx, key, payload, ttl).Err()
}

func (s *collaborationPayloadStore) GetSync(
	ctx context.Context,
	userID int64,
	syncID uuid.UUID,
	kind collabdomain.SyncKind,
) ([]byte, error) {
	key, _, err := s.syncKeyAndTTL(userID, syncID, kind)
	if err != nil {
		return nil, err
	}
	payload, err := s.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, collaborationservice.ErrPayloadNotFound
	}
	return payload, err
}

func (s *collaborationPayloadStore) DeleteSync(
	ctx context.Context,
	userID int64,
	syncID uuid.UUID,
	kind collabdomain.SyncKind,
) error {
	key, _, err := s.syncKeyAndTTL(userID, syncID, kind)
	if err != nil {
		return err
	}
	return s.redis.Del(ctx, key).Err()
}

func (s *collaborationPayloadStore) validate(userID int64, id uuid.UUID) error {
	if s == nil || s.redis == nil || s.commandTTL <= 0 || s.sessionSnapshotTTL <= 0 || s.threadSnapshotTTL <= 0 || s.maxPayloadBytes <= 0 {
		return collaborationservice.ErrInvariantViolation
	}
	if userID <= 0 || id == uuid.Nil {
		return collaborationservice.ErrInvalidArgument
	}
	return nil
}

func (s *collaborationPayloadStore) syncKeyAndTTL(
	userID int64,
	syncID uuid.UUID,
	kind collabdomain.SyncKind,
) (string, time.Duration, error) {
	if err := s.validate(userID, syncID); err != nil {
		return "", 0, err
	}
	switch kind {
	case collabdomain.SyncKindSessionList:
		return collaborationSyncPayloadKey(collaborationSessionSnapshotPrefix, userID, syncID), s.sessionSnapshotTTL, nil
	case collabdomain.SyncKindThreadSnapshot:
		return collaborationSyncPayloadKey(collaborationThreadSnapshotPrefix, userID, syncID), s.threadSnapshotTTL, nil
	default:
		return "", 0, collaborationservice.ErrInvalidArgument
	}
}

func collaborationCommandPayloadKey(userID int64, commandID uuid.UUID) string {
	return fmt.Sprintf("%s%d:%s", collaborationCommandPayloadPrefix, userID, commandID)
}

func collaborationSyncPayloadKey(prefix string, userID int64, syncID uuid.UUID) string {
	return fmt.Sprintf("%s%d:%s", prefix, userID, syncID)
}
