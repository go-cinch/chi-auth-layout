package rds

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type counterHook struct {
	captureCommands
	err error
	ctx context.Context
}

func (h *counterHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		h.ctx = ctx
		h.commands = append(h.commands, append([]interface{}(nil), cmd.Args()...))
		if h.err != nil {
			return h.err
		}
		cmd.(*redis.Cmd).SetVal(int64(3))
		return nil
	}
}

func TestIncrementWindowIsolationAndErrors(t *testing.T) {
	for _, prefix := range []string{"dev:auth:", "prod:auth:", "dev:order:"} {
		t.Run(prefix, func(t *testing.T) {
			client, err := newClient(&redis.Options{Addr: "unused:6379"}, prefix)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			hook := &counterHook{}
			client.client.AddHook(hook)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			count, err := IncrementWindow(ctx, client, "attempt:user:1", 1500*time.Millisecond)
			if err != nil || count != 3 || hook.ctx != ctx {
				t.Fatal("counter result or context lost", count, err)
			}
			want := []interface{}{"eval", incrementWindowScript, 1, prefix + "attempt:user:1", int64(1500)}
			if !reflect.DeepEqual(hook.commands[0], want) {
				t.Fatalf("command = %#v", hook.commands[0])
			}
			hook.err = errors.New("counter unavailable")
			if _, err := IncrementWindow(ctx, client, "attempt:user:1", time.Minute); !errors.Is(err, hook.err) {
				t.Fatal("counter failure hidden", err)
			}
		})
	}
}
