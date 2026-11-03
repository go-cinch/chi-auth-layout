package rds

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type counterClient interface {
	Eval(context.Context, string, []string, ...interface{}) *redis.Cmd
}

const incrementWindowScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`

// IncrementWindow atomically increments a counter and starts its TTL only on
// the first attempt. Pass a relative key and the prefix-aware Redis client.
// Callers own the window validation, attempt threshold and reset policy.
func IncrementWindow(ctx context.Context, client counterClient, key string, window time.Duration) (int64, error) {
	return client.Eval(ctx, incrementWindowScript, []string{key}, window.Milliseconds()).Int64()
}
