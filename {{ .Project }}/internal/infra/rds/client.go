package rds

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client accepts relative keys and applies the configured environment/service
// prefix to every key-bearing command. The raw client is deliberately private.
type Client struct {
	client *redis.Client
	prefix string
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
	return &Client{client: redis.NewClient(options), prefix: prefix + ":"}, nil
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
