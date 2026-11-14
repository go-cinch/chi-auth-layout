package rds

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/redact"

	"github.com/redis/go-redis/v9"
)

// Client accepts relative keys and applies the configured environment/service
// prefix to every key-bearing command. The raw client is deliberately private.
type Client struct {
	client *redis.Client
	prefix string
}

func New(ctx context.Context, cfg *config.Config) (*Client, func(), error) {
	logger := slog.Default()
	dsn := strings.TrimSpace(cfg.Redis.DSN)
	if dsn == "" {
		return nil, nil, errors.New("redis dsn is required")
	}
	options, err := redis.ParseURL(dsn)
	if err != nil {
		// Parser errors can contain credentials. Do not retain the cause: startup
		// logging unwraps errors before reporting them.
		return nil, nil, errors.New("parse redis dsn: invalid url")
	}
	if err := cfg.ValidateServer(); err != nil {
		return nil, nil, err
	}
	client, err := newClient(options, cfg.RedisPrefix())
	if err != nil {
		return nil, nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, nil, redisConnectionError(err, dsn)
	}
	cleanup := func() {
		if err := client.Close(); err != nil {
			logger.Error("close redis failed: "+err.Error(), slog.String("operation", "redis_close"), slog.String("outcome", "failure"), slog.Any("error", err))
		}
	}
	loggedDSN := redact.DSN(dsn)
	logger.InfoContext(ctx, "redis initialized: "+loggedDSN, slog.String("operation", "redis_init"), slog.String("outcome", "success"), slog.String("dsn", loggedDSN))
	return client, cleanup, nil
}

func newClient(options *redis.Options, prefix string) (*Client, error) {
	prefix = strings.TrimSuffix(strings.TrimSpace(prefix), ":")
	parts := strings.Split(prefix, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, errors.New("redis.prefix must be environment:service with an optional trailing colon")
	}
	for _, part := range parts {
		for _, character := range part {
			if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
				(character >= '0' && character <= '9') || strings.ContainsRune("-_.", character)) {
				return nil, errors.New("redis.prefix components may contain only letters, digits, hyphens, underscores and dots")
			}
		}
	}
	// Apply request deadlines to socket I/O for every client, including sessions
	// and idempotency. Keep caller-owned options unchanged.
	configured := *options
	configured.ContextTimeoutEnabled = true
	return &Client{client: redis.NewClient(&configured), prefix: prefix + ":"}, nil
}

func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.StatusCmd {
	return c.client.Set(ctx, c.prefix+key, value, ttl)
}

func (c *Client) Get(ctx context.Context, key string) *redis.StringCmd {
	return c.client.Get(ctx, c.prefix+key)
}

func (c *Client) GetDel(ctx context.Context, key string) *redis.StringCmd {
	return c.client.GetDel(ctx, c.prefix+key)
}

func (c *Client) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return c.client.Del(ctx, c.prefixedKeys(keys)...)
}

func (c *Client) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.BoolCmd {
	return c.client.SetNX(ctx, c.prefix+key, value, ttl)
}

func (c *Client) Incr(ctx context.Context, key string) *redis.IntCmd {
	return c.client.Incr(ctx, c.prefix+key)
}

// Eval prefixes KEYS only. Scripts must access keys through KEYS, never through
// hardcoded names or ARGV. Arguments and returned values are not rewritten.
func (c *Client) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	return c.client.Eval(ctx, script, c.prefixedKeys(keys), args...)
}

func (c *Client) prefixedKeys(keys []string) []string {
	result := make([]string, len(keys))
	for index, key := range keys {
		result[index] = c.prefix + key
	}
	return result
}

func (c *Client) Ping(ctx context.Context) *redis.StatusCmd {
	return c.client.Ping(ctx)
}

func (c *Client) Close() error {
	return c.client.Close()
}

func redisConnectionError(err error, dsn string) error {
	if redis.IsAuthError(err) {
		return fmt.Errorf("%v; dsn: %s", err, redact.DSN(dsn))
	}
	return fmt.Errorf("ping redis: %w", err)
}
