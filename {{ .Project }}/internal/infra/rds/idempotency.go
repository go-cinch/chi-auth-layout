package rds

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const idempotencyPrefix = "idempotency:request:"

type idempotencyClient interface {
	Eval(context.Context, string, []string, ...interface{}) *redis.Cmd
	SetNX(context.Context, string, any, time.Duration) *redis.BoolCmd
}

type IdempotencyStore struct {
	client idempotencyClient
}

func NewIdempotencyStore(client idempotencyClient) *IdempotencyStore {
	return &IdempotencyStore{client: client}
}

func (s *IdempotencyStore) Claim(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if s == nil || s.client == nil {
		return false, errors.New("redis client is required")
	}
	if key == "" {
		return false, errors.New("idempotency key is required")
	}
	if ttl <= 0 {
		return false, errors.New("idempotency ttl must be positive")
	}
	return s.client.SetNX(ctx, idempotencyPrefix+key, "1", ttl).Result()
}

// Legacy writers store "1" before dispatch and cannot identify a committed
// message. Keep them mutually exclusive with transactional writers during rollout.
func (s *IdempotencyStore) AllowTransactional(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if s == nil || s.client == nil || key == "" || ttl <= 0 {
		return false, errors.New("transactional idempotency requires a client, key and positive ttl")
	}
	const reserve = `local owner = redis.call("GET", KEYS[1])
if owner then return owner == "transactional" and 1 or 0 end
redis.call("PSETEX", KEYS[1], ARGV[1], "transactional")
return 1`
	allowed, err := s.client.Eval(ctx, reserve, []string{idempotencyPrefix + key}, max(int64(1), ttl.Milliseconds())).Int()
	return allowed == 1, err
}
