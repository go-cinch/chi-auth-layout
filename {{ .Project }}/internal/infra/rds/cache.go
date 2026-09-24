package rds

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type namespaceCacheClient interface {
	Get(context.Context, string) *redis.StringCmd
	Incr(context.Context, string) *redis.IntCmd
	Set(context.Context, string, interface{}, time.Duration) *redis.StatusCmd
}

// NamespaceCache stores versioned values in one Redis namespace. Invalidation
// advances a non-expiring version; old values expire by TTL. A stale read stores
// under its original version and cannot repopulate a newer generation.
type NamespaceCache struct {
	client namespaceCacheClient
	prefix string
}

// NewRedisNamespaceCache owns a separate client so cache deadlines and retries
// do not change the behavior of session, challenge or idempotency storage.
func NewRedisNamespaceCache(dsn, keyPrefix, namespace string) (*NamespaceCache, func(), error) {
	options, err := redis.ParseURL(strings.TrimSpace(dsn))
	if err != nil {
		return nil, nil, fmt.Errorf("parse cache redis dsn: %w", err)
	}
	options.MaxRetries = -1 // The dictionary service owns the retry budget.
	options.ContextTimeoutEnabled = true
	options.DialTimeout = 200 * time.Millisecond
	options.ReadTimeout = 200 * time.Millisecond
	options.WriteTimeout = 200 * time.Millisecond
	options.PoolTimeout = 200 * time.Millisecond
	options.DialerRetries = 1 // go-redis counts total dial attempts here.
	client, err := newClient(options, keyPrefix)
	if err != nil {
		return nil, nil, err
	}
	cache, err := NewNamespaceCache(client, namespace)
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return cache, func() {
		if err := client.Close(); err != nil {
			slog.Error("close dictionary cache redis failed: " + err.Error())
		}
	}, nil
}

func NewNamespaceCache(client namespaceCacheClient, prefix string) (*NamespaceCache, error) {
	prefix = strings.TrimSpace(prefix)
	if client == nil {
		return nil, errors.New("namespace cache client is required")
	}
	if prefix == "" || strings.ContainsAny(prefix, "*?[]") {
		return nil, errors.New("namespace cache prefix is invalid")
	}
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	return &NamespaceCache{client: client, prefix: prefix}, nil
}

func (c *NamespaceCache) Load(ctx context.Context, key string) (string, bool, string, error) {
	version, err := c.version(ctx)
	if err != nil {
		return "", false, "", err
	}
	value, err := c.client.Get(ctx, c.valueKey(version, key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, version, nil
	}
	if err != nil {
		return "", false, "", fmt.Errorf("get namespace cache value: %w", err)
	}
	return value, true, version, nil
}

func (c *NamespaceCache) Store(ctx context.Context, key, value, version string, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("namespace cache ttl must be positive")
	}
	if err := c.client.Set(ctx, c.valueKey(version, key), value, ttl).Err(); err != nil {
		return fmt.Errorf("put namespace cache value: %w", err)
	}
	return nil
}

func (c *NamespaceCache) Invalidate(ctx context.Context) error {
	if err := c.client.Incr(ctx, c.prefix+"version").Err(); err != nil {
		return fmt.Errorf("advance namespace cache version: %w", err)
	}
	return nil
}

func (c *NamespaceCache) version(ctx context.Context) (string, error) {
	version, err := c.client.Get(ctx, c.prefix+"version").Result()
	if errors.Is(err, redis.Nil) {
		return "0", nil
	}
	if err != nil {
		return "", fmt.Errorf("get namespace cache version: %w", err)
	}
	return version, nil
}

func (c *NamespaceCache) valueKey(version, key string) string {
	return c.prefix + "value:" + version + ":" + key
}
