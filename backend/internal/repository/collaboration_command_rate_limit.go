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
	collaborationCommandRateLimitPrefix = "collaboration:rate:commands:user:"
	collaborationCommandRateWindow      = time.Minute
)

var collaborationCommandRateLimitScript = redis.NewScript(`
local now = tonumber(ARGV[1])
local window_start = now - tonumber(ARGV[2])
local member = ARGV[3]
local limit = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", window_start)
if redis.call("ZSCORE", KEYS[1], member) then
  redis.call("PEXPIRE", KEYS[1], ttl_ms)
  return 1
end
if redis.call("ZCARD", KEYS[1]) >= limit then
  return 0
end

redis.call("ZADD", KEYS[1], now, member)
redis.call("PEXPIRE", KEYS[1], ttl_ms)
return 1
`)

type collaborationCommandRateLimiter struct {
	redis *redis.Client
	limit int
	now   func(context.Context) (time.Time, error)
}

func NewCollaborationCommandRateLimiter(
	redisClient *redis.Client,
	cfg *config.Config,
) collaborationservice.CommandRateLimiter {
	limit := 20
	if cfg != nil && cfg.Collaboration.MaxCommandsPerUserMinute > 0 {
		limit = cfg.Collaboration.MaxCommandsPerUserMinute
	}
	return &collaborationCommandRateLimiter{
		redis: redisClient,
		limit: limit,
		now: func(ctx context.Context) (time.Time, error) {
			return redisClient.Time(ctx).Result()
		},
	}
}

func (l *collaborationCommandRateLimiter) Allow(
	ctx context.Context,
	userID int64,
	idempotencyKey uuid.UUID,
) (bool, error) {
	if l == nil || l.redis == nil || l.limit <= 0 || userID <= 0 || idempotencyKey == uuid.Nil {
		return false, collaborationservice.ErrInvalidArgument
	}
	now, err := l.now(ctx)
	if err != nil {
		return false, err
	}
	result, err := collaborationCommandRateLimitScript.Run(
		ctx,
		l.redis,
		[]string{fmt.Sprintf("%s%d", collaborationCommandRateLimitPrefix, userID)},
		now.UTC().UnixMilli(),
		collaborationCommandRateWindow.Milliseconds(),
		idempotencyKey.String(),
		l.limit,
		(2 * collaborationCommandRateWindow).Milliseconds(),
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}
