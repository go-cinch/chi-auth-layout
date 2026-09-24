package rds

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const idempotencyPrefix = "idempotent:"

type setNXClient interface {
	SetNX(context.Context, string, any, time.Duration) *redis.BoolCmd
}

type IdempotencyStore struct {
	client setNXClient
}

func NewIdempotencyStore(client setNXClient) *IdempotencyStore {
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
