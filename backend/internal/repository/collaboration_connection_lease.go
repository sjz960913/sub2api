package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	collaborationUserConnectionPrefix   = "collaboration:connections:user:"
	collaborationDeviceConnectionPrefix = "collaboration:connections:device:"
)

var collaborationAcquireConnectionScript = redis.NewScript(`
local now = tonumber(ARGV[2])
local expires_at = tonumber(ARGV[3])
local user_limit = tonumber(ARGV[4])
local device_limit = tonumber(ARGV[5])
local key_ttl_ms = tonumber(ARGV[6])

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", now)
if #KEYS == 2 then
  redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", now)
end

if redis.call("ZSCORE", KEYS[1], ARGV[1]) then
  redis.call("ZADD", KEYS[1], expires_at, ARGV[1])
  redis.call("PEXPIRE", KEYS[1], key_ttl_ms)
  if #KEYS == 2 then
    redis.call("ZADD", KEYS[2], expires_at, ARGV[1])
    redis.call("PEXPIRE", KEYS[2], key_ttl_ms)
  end
  return 1
end

if redis.call("ZCARD", KEYS[1]) >= user_limit then
  return 0
end
if #KEYS == 2 and redis.call("ZCARD", KEYS[2]) >= device_limit then
  return 0
end

redis.call("ZADD", KEYS[1], expires_at, ARGV[1])
redis.call("PEXPIRE", KEYS[1], key_ttl_ms)
if #KEYS == 2 then
  redis.call("ZADD", KEYS[2], expires_at, ARGV[1])
  redis.call("PEXPIRE", KEYS[2], key_ttl_ms)
end
return 1
`)

var collaborationRenewConnectionScript = redis.NewScript(`
local now = tonumber(ARGV[2])
local expires_at = tonumber(ARGV[3])
local key_ttl_ms = tonumber(ARGV[4])
local user_score = redis.call("ZSCORE", KEYS[1], ARGV[1])
local device_score = nil
if #KEYS == 2 then
  device_score = redis.call("ZSCORE", KEYS[2], ARGV[1])
end

if not user_score or tonumber(user_score) <= now or (#KEYS == 2 and (not device_score or tonumber(device_score) <= now)) then
  redis.call("ZREM", KEYS[1], ARGV[1])
  if #KEYS == 2 then
    redis.call("ZREM", KEYS[2], ARGV[1])
  end
  return 0
end

redis.call("ZADD", KEYS[1], expires_at, ARGV[1])
redis.call("PEXPIRE", KEYS[1], key_ttl_ms)
if #KEYS == 2 then
  redis.call("ZADD", KEYS[2], expires_at, ARGV[1])
  redis.call("PEXPIRE", KEYS[2], key_ttl_ms)
end
return 1
`)

type collaborationConnectionLeaseStore struct {
	redis       *redis.Client
	ttl         time.Duration
	userLimit   int
	deviceLimit int
	now         func(context.Context) (time.Time, error)
}

func NewCollaborationConnectionLeaseStore(
	redisClient *redis.Client,
	cfg *config.Config,
) collaborationservice.ConnectionLeaseStore {
	ttl := 45 * time.Second
	userLimit := 5
	deviceLimit := 1
	if cfg != nil {
		if cfg.Collaboration.PresenceTTLSeconds > 0 {
			ttl = time.Duration(cfg.Collaboration.PresenceTTLSeconds) * time.Second
		}
		if cfg.Collaboration.MaxConnectionsPerUser > 0 {
			userLimit = cfg.Collaboration.MaxConnectionsPerUser
		}
		if cfg.Collaboration.MaxConnectionsPerDevice > 0 {
			deviceLimit = cfg.Collaboration.MaxConnectionsPerDevice
		}
	}
	return &collaborationConnectionLeaseStore{
		redis:       redisClient,
		ttl:         ttl,
		userLimit:   userLimit,
		deviceLimit: deviceLimit,
		now: func(ctx context.Context) (time.Time, error) {
			return redisClient.Time(ctx).Result()
		},
	}
}

func (s *collaborationConnectionLeaseStore) Acquire(
	ctx context.Context,
	lease collaborationservice.ConnectionLease,
) (bool, error) {
	if err := s.validate(lease); err != nil {
		return false, err
	}
	now, err := s.now(ctx)
	if err != nil {
		return false, err
	}
	now = now.UTC()
	keys := collaborationConnectionKeys(lease)
	result, err := collaborationAcquireConnectionScript.Run(ctx, s.redis, keys,
		lease.ConnectionID.String(),
		now.UnixMilli(),
		now.Add(s.ttl).UnixMilli(),
		s.userLimit,
		s.deviceLimit,
		(2 * s.ttl).Milliseconds(),
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (s *collaborationConnectionLeaseStore) Renew(
	ctx context.Context,
	lease collaborationservice.ConnectionLease,
) (bool, error) {
	if err := s.validate(lease); err != nil {
		return false, err
	}
	now, err := s.now(ctx)
	if err != nil {
		return false, err
	}
	now = now.UTC()
	result, err := collaborationRenewConnectionScript.Run(ctx, s.redis, collaborationConnectionKeys(lease),
		lease.ConnectionID.String(),
		now.UnixMilli(),
		now.Add(s.ttl).UnixMilli(),
		(2 * s.ttl).Milliseconds(),
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (s *collaborationConnectionLeaseStore) Release(
	ctx context.Context,
	lease collaborationservice.ConnectionLease,
) error {
	if err := s.validate(lease); err != nil {
		return err
	}
	pipe := s.redis.TxPipeline()
	for _, key := range collaborationConnectionKeys(lease) {
		pipe.ZRem(ctx, key, lease.ConnectionID.String())
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *collaborationConnectionLeaseStore) validate(
	lease collaborationservice.ConnectionLease,
) error {
	if s == nil || s.redis == nil || s.ttl <= 0 || s.userLimit <= 0 || s.deviceLimit <= 0 {
		return collaborationservice.ErrInvariantViolation
	}
	if lease.UserID <= 0 || lease.ConnectionID == uuid.Nil {
		return collaborationservice.ErrInvalidArgument
	}
	return nil
}

func collaborationConnectionKeys(lease collaborationservice.ConnectionLease) []string {
	keys := []string{fmt.Sprintf("%s%d", collaborationUserConnectionPrefix, lease.UserID)}
	if lease.DeviceID != uuid.Nil {
		keys = append(keys, collaborationDeviceConnectionPrefix+lease.DeviceID.String())
	}
	return keys
}
