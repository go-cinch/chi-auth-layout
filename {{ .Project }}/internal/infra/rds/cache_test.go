package rds

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type fakeNamespaceCacheClient struct {
	values  map[string]string
	version int64
	ttls    map[string]time.Duration
	getErr  error
	setErr  error
	incrErr error
}

func (f *fakeNamespaceCacheClient) Get(ctx context.Context, key string) *redis.StringCmd {
	command := redis.NewStringCmd(ctx)
	if f.getErr != nil {
		command.SetErr(f.getErr)
		return command
	}
	value, ok := f.values[key]
	if !ok {
		command.SetErr(redis.Nil)
	} else {
		command.SetVal(value)
	}
	return command
}

func (f *fakeNamespaceCacheClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) *redis.StatusCmd {
	if f.setErr != nil {
		return redis.NewStatusResult("", f.setErr)
	}
	if f.ttls == nil {
		f.ttls = make(map[string]time.Duration)
	}
	f.ttls[key] = ttl
	f.values[key] = value.(string)
	return redis.NewStatusResult("OK", nil)
}

func (f *fakeNamespaceCacheClient) Incr(ctx context.Context, _ string) *redis.IntCmd {
	if f.incrErr != nil {
		return redis.NewIntResult(0, f.incrErr)
	}
	f.version++
	f.values["dictionary:cache:version"] = strconv.FormatInt(f.version, 10)
	return redis.NewIntResult(f.version, nil)
}

func TestNamespaceCache(t *testing.T) {
	client := &fakeNamespaceCacheClient{values: make(map[string]string)}
	cache, err := NewNamespaceCache(client, "dictionary:cache")
	if err != nil {
		t.Fatal(err)
	}
	value, hit, version, err := cache.Load(t.Context(), "KEY")
	if err != nil || hit || value != "" || version != "0" {
		t.Fatalf("empty get = %q %v %q %v", value, hit, version, err)
	}
	if err := cache.Store(t.Context(), "KEY", `[1]`, version, time.Minute); err != nil {
		t.Fatal(err)
	}
	value, hit, version, err = cache.Load(t.Context(), "KEY")
	if err != nil || !hit || value != `[1]` || version != "0" {
		t.Fatalf("cached get = %q %v %q %v", value, hit, version, err)
	}
	if err := cache.Invalidate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if value, hit, version, err = cache.Load(t.Context(), "KEY"); err != nil || hit || version != "1" {
		t.Fatalf("invalidated get = %q %v %q %v", value, hit, version, err)
	}
}

func TestNewNamespaceCacheValidation(t *testing.T) {
	if _, err := NewNamespaceCache(nil, "cache"); err == nil {
		t.Fatal("nil client accepted")
	}
	client := &fakeNamespaceCacheClient{values: make(map[string]string)}
	if _, err := NewNamespaceCache(client, "*"); err == nil {
		t.Fatal("wildcard prefix accepted")
	}
	cache, _ := NewNamespaceCache(client, "cache")
	if err := cache.Store(t.Context(), "key", "value", "0", 0); err == nil {
		t.Fatal("invalid ttl accepted")
	}
}

func TestNamespaceCacheRetainsOldGenerationUntilTTL(t *testing.T) {
	client := &fakeNamespaceCacheClient{values: make(map[string]string)}
	cache, err := NewNamespaceCache(client, "dictionary:cache")
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Store(t.Context(), "KEY", `"old"`, "0", 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := cache.Invalidate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if client.values["dictionary:cache:value:0:KEY"] != `"old"` {
		t.Fatal("old value was deleted instead of left to TTL")
	}
	if client.ttls["dictionary:cache:value:0:KEY"] != 10*time.Minute {
		t.Fatal("value TTL missing")
	}
	if _, ok := client.ttls["dictionary:cache:version"]; ok {
		t.Fatal("version must not expire")
	}
	// An in-flight database read finishes after invalidation and refills v0.
	if err := cache.Store(t.Context(), "KEY", `"stale"`, "0", 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, hit, version, err := cache.Load(t.Context(), "KEY"); err != nil || hit || version != "1" {
		t.Fatalf("stale refill became visible: %v %s %v", hit, version, err)
	}
	if err := cache.Store(t.Context(), "KEY", `"new"`, "1", 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	if value, hit, _, err := cache.Load(t.Context(), "KEY"); err != nil || !hit || value != `"new"` {
		t.Fatalf("new generation read: %s %v %v", value, hit, err)
	}
}

func TestNamespaceCacheFailures(t *testing.T) {
	failure := errors.New("redis unavailable")
	client := &fakeNamespaceCacheClient{values: make(map[string]string), getErr: failure, setErr: failure, incrErr: failure}
	cache, err := NewNamespaceCache(client, "dictionary:cache")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := cache.Load(t.Context(), "KEY"); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := cache.Store(t.Context(), "KEY", `[]`, "0", time.Minute); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := cache.Invalidate(t.Context()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

func TestRedisNamespaceCacheClientDeadlines(t *testing.T) {
	if _, _, err := NewRedisNamespaceCache("redis://127.0.0.1:6379", "", "cache"); err == nil {
		t.Fatal("empty shared prefix accepted")
	}
	if _, _, err := NewRedisNamespaceCache("://bad", "test:auth:", "cache"); err == nil {
		t.Fatal("invalid DSN accepted")
	}
	if _, _, err := NewRedisNamespaceCache("redis://127.0.0.1:6379", "test:auth:", "*"); err == nil {
		t.Fatal("invalid prefix accepted")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		// Accept commands but never respond, including during initialization.
		_, _ = io.Copy(io.Discard, conn)
	}()
	cache, cleanup, err := NewRedisNamespaceCache("redis://"+listener.Addr().String(), "test:auth:", "dictionary:cache")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cleanup(); listener.Close(); <-done }()
	options := cache.client.(*Client).client.Options()
	if options.MaxRetries != 0 || !options.ContextTimeoutEnabled || options.DialerRetries != 1 {
		t.Fatal("cache client does not bound retries and timeouts")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := cache.Invalidate(ctx); err == nil {
		t.Fatal("unresponsive server accepted invalidation")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("redis ignored independent deadline")
	}
}
