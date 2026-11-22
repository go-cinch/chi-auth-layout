package rds

import (
	"reflect"
	"testing"
	"time"
)

func TestStreamCommandsUseOnlyNamespacedKeys(t *testing.T) {
	hook := &captureCommands{}
	client := testClient(t, "test:worker", hook)
	s := NewStream(client, "audit:custom", "writers")
	ctx := t.Context()
	if err := s.Add(ctx, map[string]any{"event": "data"}); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(ctx, "instance"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Claim(ctx, "instance", "0-0", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, "instance", "42-0", true); err != nil {
		t.Fatal(err)
	}
	key := "test:worker:audit:custom"
	want := [][]any{
		{"xadd", key, "*", "event", "data"},
		{"xgroup", "create", key, "writers", "0", "mkstream"},
		{"xreadgroup", "group", "writers", "instance", "count", int64(1), "block", int64(1000), "streams", key, ">"},
		{"xautoclaim", key, "writers", "instance", int64(60000), "0-0", "count", int64(1)},
		{"eval", finishStreamEntry, 2, key, key + ":failed", "writers", "instance", "42-0", "failed"},
	}
	if !reflect.DeepEqual(hook.commands, want) {
		t.Fatalf("stream commands = %#v", hook.commands)
	}
}
