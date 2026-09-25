package rds

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type captureCommands struct {
	commands [][]interface{}
}

func (h *captureCommands) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *captureCommands) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, command redis.Cmder) error {
		h.commands = append(h.commands, append([]interface{}(nil), command.Args()...))
		return nil
	}
}

func (h *captureCommands) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func testClient(t *testing.T, prefix string, hook *captureCommands) *Client {
	t.Helper()
	client, err := newClient(&redis.Options{Addr: "unused:6379"}, prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	client.client.AddHook(hook)
	return client
}

func TestClientPrefixesEveryKeyCommand(t *testing.T) {
	hook := &captureCommands{}
	client := testClient(t, "dev:auth:", hook)
	ctx := t.Context()
	client.Set(ctx, "session:data:id", "session:value", time.Minute)
	client.Get(ctx, "session:data:id")
	client.GetDel(ctx, "session:refresh:digest")
	keys := []string{"session:data:id", "session:refresh:digest"}
	client.Del(ctx, keys...)
	client.SetNX(ctx, "idempotency:request:id", "1", time.Minute)
	client.Incr(ctx, "dictionary:cache:version")
	script := "return redis.call('GET', KEYS[1])"
	client.Eval(ctx, script, keys, "unchanged:argument")
	client.Ping(ctx)
	if !reflect.DeepEqual(keys, []string{"session:data:id", "session:refresh:digest"}) {
		t.Fatalf("caller keys mutated: %v", keys)
	}
	want := [][]interface{}{
		{"set", "dev:auth:session:data:id", "session:value", "ex", int64(60)},
		{"get", "dev:auth:session:data:id"},
		{"getdel", "dev:auth:session:refresh:digest"},
		{"del", "dev:auth:session:data:id", "dev:auth:session:refresh:digest"},
		{"set", "dev:auth:idempotency:request:id", "1", "ex", int64(60), "nx"},
		{"incr", "dev:auth:dictionary:cache:version"},
		{"eval", script, 2, "dev:auth:session:data:id", "dev:auth:session:refresh:digest", "unchanged:argument"},
		{"ping"},
	}
	if !reflect.DeepEqual(hook.commands, want) {
		t.Fatalf("commands = %#v, want %#v", hook.commands, want)
	}
}

func TestClientNamespaceIsolation(t *testing.T) {
	hook := &captureCommands{}
	for _, prefix := range []string{"dev:auth", "prod:auth:", "dev:order:"} {
		client := testClient(t, prefix, hook)
		client.Get(t.Context(), "session:data:same-id")
		client.Del(t.Context(), "session:data:same-id")
		cache, err := NewNamespaceCache(client, "dictionary:cache")
		if err != nil {
			t.Fatal(err)
		}
		if err := cache.Invalidate(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := cache.Store(t.Context(), "KEY", `[]`, "0", time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	for index, prefix := range []string{"dev:auth:", "prod:auth:", "dev:order:"} {
		for offset, key := range []string{"session:data:same-id", "session:data:same-id", "dictionary:cache:version", "dictionary:cache:value:0:KEY"} {
			if got := hook.commands[index*4+offset][1]; got != prefix+key {
				t.Fatalf("namespace %s command %d key = %v", prefix, offset, got)
			}
		}
	}
}

func TestClientRejectsInvalidPrefixes(t *testing.T) {
	for _, prefix := range []string{"", " ", "auth", ":auth", "dev:", "dev:auth::", "dev:auth:extra", "dev:*", "dev:a b", "dev:a?", "dev:{auth}", "dev:a\\b"} {
		if client, err := newClient(&redis.Options{}, prefix); err == nil {
			_ = client.Close()
			t.Errorf("accepted prefix %q", prefix)
		}
	}
	hook := &captureCommands{}
	client := testClient(t, " dev:auth-service.v2_1: ", hook)
	client.Get(t.Context(), "key")
	if got := hook.commands[0][1]; got != "dev:auth-service.v2_1:key" {
		t.Fatalf("normalized key = %v", got)
	}
}
