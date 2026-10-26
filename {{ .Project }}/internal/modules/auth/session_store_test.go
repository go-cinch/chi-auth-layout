package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	{{- if .Computed.enable_redis_final }}

	"github.com/redis/go-redis/v9"
	{{- end }}
)

func memoryStoreForTest(t *testing.T, capacity int, interval time.Duration) *memorySessionStore {
	t.Helper()
	store := newMemorySessionStore(capacity, interval)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestMemorySessionStoreLifecycle(t *testing.T) {
	store := memoryStoreForTest(t, 2, time.Hour)
	ctx := t.Context()
	expiredAt := time.Now().Add(time.Hour)
	if err := store.Create(ctx, "deployment:session", "original", expiredAt); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, "deployment:session", "replacement", expiredAt); !errors.Is(err, errSessionConflict) {
		t.Fatalf("duplicate create = %v", err)
	}
	if updated, err := store.Rotate(ctx, "deployment:session", "wrong", "replacement", expiredAt); updated || err != nil {
		t.Fatalf("mismatched rotate = %v, %v", updated, err)
	}
	if value, err := store.Get(ctx, "deployment:session"); err != nil || value != "original" {
		t.Fatalf("get original = %q, %v", value, err)
	}
	if updated, err := store.Rotate(ctx, "deployment:session", "original", "replacement", expiredAt); !updated || err != nil {
		t.Fatalf("rotate = %v, %v", updated, err)
	}
	if value, err := store.Get(ctx, "deployment:session"); err != nil || value != "replacement" {
		t.Fatalf("get replacement = %q, %v", value, err)
	}
	store.mu.Lock()
	storedDeadline := store.sessions["deployment:session"].expiredAt
	store.mu.Unlock()
	if !storedDeadline.Equal(expiredAt) {
		t.Fatalf("rotation changed absolute deadline: %v != %v", storedDeadline, expiredAt)
	}
	for range 2 {
		if err := store.Delete(ctx, "deployment:session"); err != nil {
			t.Fatal(err)
		}
	}
	if updated, err := store.Rotate(ctx, "deployment:session", "replacement", "resurrected", expiredAt); updated || err != nil {
		t.Fatalf("rotation after deletion = %v, %v", updated, err)
	}
	if _, err := store.Get(ctx, "deployment:session"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("deleted get = %v", err)
	}
}

func TestMemorySessionStoreConcurrentRotation(t *testing.T) {
	store := memoryStoreForTest(t, 1, time.Hour)
	expiredAt := time.Now().Add(time.Hour)
	if err := store.Create(t.Context(), "session", "original", expiredAt); err != nil {
		t.Fatal(err)
	}
	var successful atomic.Int32
	var group sync.WaitGroup
	for range 24 {
		group.Go(func() {
			updated, err := store.Rotate(t.Context(), "session", "original", "rotated", expiredAt)
			if err != nil {
				t.Errorf("rotate: %v", err)
			}
			if updated {
				successful.Add(1)
			}
		})
	}
	group.Wait()
	if successful.Load() != 1 {
		t.Fatalf("successful rotations = %d, want 1", successful.Load())
	}
}

func TestMemorySessionStoreExpirationAndCapacity(t *testing.T) {
	store := memoryStoreForTest(t, 1, time.Hour)
	ctx := t.Context()
	past := time.Now().Add(-time.Second)
	if err := store.Create(ctx, "expired", "value", past); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("create expired = %v", err)
	}
	deadline := time.Now().Add(20 * time.Millisecond)
	if err := store.Create(ctx, "short", "original", deadline); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, "other", "value", time.Now().Add(time.Hour)); !errors.Is(err, errSessionCapacity) {
		t.Fatalf("create over capacity = %v", err)
	}
	if updated, err := store.Rotate(ctx, "short", "original", "expired", past); updated || err != nil {
		t.Fatalf("rotate to expired deadline = %v, %v", updated, err)
	}
	time.Sleep(time.Until(deadline) + time.Millisecond)
	if updated, err := store.Rotate(ctx, "short", "original", "resurrected", time.Now().Add(time.Hour)); updated || err != nil {
		t.Fatalf("rotate expired record = %v, %v", updated, err)
	}
	if _, err := store.Get(ctx, "short"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("get expired record = %v", err)
	}
	// Expiration must also free capacity without the janitor or a read of that ID.
	deadline = time.Now().Add(20 * time.Millisecond)
	if err := store.Create(ctx, "unread", "value", deadline); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(deadline) + time.Millisecond)
	if err := store.Create(ctx, "other", "value", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("expired entry blocked capacity: %v", err)
	}
}

func TestMemorySessionStoreJanitor(t *testing.T) {
	store := memoryStoreForTest(t, 1, time.Millisecond)
	if err := store.Create(t.Context(), "unread", "value", time.Now().Add(10*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		remaining := len(store.sessions)
		store.mu.Unlock()
		if remaining == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("janitor retained expired session without any reads")
}

func TestMemorySessionStoreCancellationAndClose(t *testing.T) {
	store := NewMemorySessionStore()
	t.Cleanup(func() { _ = store.Close() })
	ctx := t.Context()
	expiredAt := time.Now().Add(time.Hour)
	if err := store.Create(ctx, "session", "original", expiredAt); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := store.Create(canceled, "other", "value", expiredAt); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled create = %v", err)
	}
	if _, err := store.Get(canceled, "session"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled get = %v", err)
	}
	if updated, err := store.Rotate(canceled, "session", "original", "replacement", expiredAt); updated || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled rotate = %v, %v", updated, err)
	}
	if err := store.Delete(canceled, "session"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled delete = %v", err)
	}
	if value, err := store.Get(ctx, "session"); value != "original" || err != nil {
		t.Fatalf("cancellation mutated existing record: %q, %v", value, err)
	}
	if _, err := store.Get(ctx, "other"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("canceled create wrote a record: %v", err)
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			if err := store.Close(); err != nil {
				t.Errorf("close: %v", err)
			}
		})
	}
	group.Wait()
	store.mu.Lock()
	remaining := len(store.sessions)
	store.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("close retained %d sessions", remaining)
	}
	select {
	case <-store.stopped:
	default:
		t.Fatal("close did not stop janitor")
	}
	if err := store.Create(ctx, "new", "value", expiredAt); !errors.Is(err, errSessionStoreClosed) {
		t.Fatalf("closed create = %v", err)
	}
	if _, err := store.Get(ctx, "session"); !errors.Is(err, errSessionStoreClosed) {
		t.Fatalf("closed get = %v", err)
	}
	if updated, err := store.Rotate(ctx, "session", "original", "replacement", expiredAt); updated || !errors.Is(err, errSessionStoreClosed) {
		t.Fatalf("closed rotate = %v, %v", updated, err)
	}
	if err := store.Delete(ctx, "session"); !errors.Is(err, errSessionStoreClosed) {
		t.Fatalf("closed delete = %v", err)
	}
}

{{ if .Computed.enable_redis_final -}}
// This stub supplies command results only. Real Lua semantics are covered by
// the optional Redis integration tests in internal/tests.
type sessionRedisStub struct {
	evalValue interface{}
	err       error
	getValue  string
	keys      []string
	args      []interface{}
	calls     int
}

func (s *sessionRedisStub) Eval(ctx context.Context, _ string, keys []string, args ...interface{}) *redis.Cmd {
	s.calls++
	s.keys, s.args = keys, args
	command := redis.NewCmd(ctx)
	command.SetVal(s.evalValue)
	command.SetErr(s.err)
	return command
}

func (s *sessionRedisStub) Get(ctx context.Context, key string) *redis.StringCmd {
	s.calls++
	s.keys = []string{key}
	command := redis.NewStringCmd(ctx)
	command.SetVal(s.getValue)
	command.SetErr(s.err)
	return command
}

func (s *sessionRedisStub) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	s.calls++
	s.keys = keys
	command := redis.NewIntCmd(ctx)
	command.SetErr(s.err)
	return command
}

func TestRedisSessionStoreCommandResults(t *testing.T) {
	client := &sessionRedisStub{evalValue: int64(1), getValue: "record"}
	store := NewRedisSessionStore(client)
	ctx := t.Context()
	expiredAt := time.Now().Add(time.Hour)
	if err := store.Create(ctx, "deployment:session", "record", expiredAt); err != nil {
		t.Fatal(err)
	}
	if len(client.keys) != 1 || client.keys[0] != "session:data:deployment:session" || len(client.args) != 2 || client.args[0] != "record" || client.args[1] != expiredAt.UnixMilli() {
		t.Fatalf("create arguments = %v, %v", client.keys, client.args)
	}
	for _, test := range []struct {
		result int64
		want   error
	}{
		{0, errSessionConflict}, {-1, ErrSessionNotFound},
	} {
		client.evalValue = test.result
		if err := store.Create(ctx, "session", "record", expiredAt); !errors.Is(err, test.want) {
			t.Fatalf("create result %d = %v", test.result, err)
		}
	}
	client.evalValue = int64(7)
	if err := store.Create(ctx, "session", "record", expiredAt); err == nil {
		t.Fatal("invalid create result was accepted")
	}
	for _, result := range []int64{0, 1} {
		client.evalValue = result
		updated, err := store.Rotate(ctx, "deployment:session", "before", "after", expiredAt)
		if err != nil || updated != (result == 1) {
			t.Fatalf("rotate result %d = %v, %v", result, updated, err)
		}
		if len(client.keys) != 1 || client.keys[0] != "session:data:deployment:session" || len(client.args) != 3 || client.args[0] != "before" || client.args[1] != "after" || client.args[2] != expiredAt.UnixMilli() {
			t.Fatalf("rotate arguments = %v, %v", client.keys, client.args)
		}
	}
	client.evalValue = int64(7)
	if updated, err := store.Rotate(ctx, "session", "before", "after", expiredAt); updated || err == nil {
		t.Fatalf("invalid rotate result = %v, %v", updated, err)
	}
	if value, err := store.Get(ctx, "deployment:session"); value != "record" || err != nil || client.keys[0] != "session:data:deployment:session" {
		t.Fatalf("get = %q, %v, keys %v", value, err, client.keys)
	}
	if err := store.Delete(ctx, "deployment:session"); err != nil || len(client.keys) != 1 || client.keys[0] != "session:data:deployment:session" {
		t.Fatalf("delete = %v, keys %v", err, client.keys)
	}
	client.err = redis.Nil
	if _, err := store.Get(ctx, "session"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing get = %v", err)
	}
}

func TestRedisSessionStoreErrorsAndCancellation(t *testing.T) {
	backendError := errors.New("redis unavailable")
	client := &sessionRedisStub{err: backendError}
	store := NewRedisSessionStore(client)
	expiredAt := time.Now().Add(time.Hour)
	for _, test := range []struct {
		name string
		call func(context.Context) error
	}{
		{"create", func(ctx context.Context) error { return store.Create(ctx, "session", "value", expiredAt) }},
		{"get", func(ctx context.Context) error { _, err := store.Get(ctx, "session"); return err }},
		{"rotate", func(ctx context.Context) error {
			_, err := store.Rotate(ctx, "session", "before", "after", expiredAt)
			return err
		}},
		{"delete", func(ctx context.Context) error { return store.Delete(ctx, "session") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(t.Context()); !errors.Is(err, backendError) {
				t.Fatalf("backend error = %v", err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			calls := client.calls
			if err := test.call(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation error = %v", err)
			}
			if client.calls != calls {
				t.Fatal("canceled operation reached redis")
			}
		})
	}
}

{{ end }}
