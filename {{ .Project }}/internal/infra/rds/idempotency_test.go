package rds

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type fakeSetNXClient struct {
	replayAllowed int64
	err           error
	expiration    time.Duration
	key           string
	value         any
}

func (f *fakeSetNXClient) SetNX(ctx context.Context, key string, value any, expiration time.Duration) *redis.BoolCmd {
	f.key, f.value, f.expiration = key, value, expiration
	command := redis.NewBoolCmd(ctx)
	command.SetVal(true)
	command.SetErr(f.err)
	return command
}

func TestIdempotencyStoreClaim(t *testing.T) {
	client := &fakeSetNXClient{}
	store := NewIdempotencyStore(client)
	claimed, err := store.Claim(t.Context(), "request-1", time.Hour)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %v, %v", claimed, err)
	}
	if client.key != "idempotency:request:request-1" || client.value != "1" || client.expiration != time.Hour {
		t.Fatalf("SetNX() = %q, %#v, %v", client.key, client.value, client.expiration)
	}

	client.err = errors.New("redis unavailable")
	if _, err := store.Claim(t.Context(), "request-2", time.Hour); err == nil {
		t.Fatal("redis error was ignored")
	}
}

func TestIdempotencyStoreValidation(t *testing.T) {
	if _, err := NewIdempotencyStore(nil).Claim(t.Context(), "request-1", time.Hour); err == nil {
		t.Fatal("nil client was accepted")
	}
	var nilStore *IdempotencyStore
	if _, err := nilStore.Claim(t.Context(), "request-1", time.Hour); err == nil {
		t.Fatal("nil store was accepted")
	}
	store := &IdempotencyStore{client: &fakeSetNXClient{}}
	if _, err := store.Claim(t.Context(), "", time.Hour); err == nil {
		t.Fatal("empty key was accepted")
	}
	if _, err := store.Claim(t.Context(), "request-1", 0); err == nil {
		t.Fatal("zero ttl was accepted")
	}
}

func (f *fakeSetNXClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	f.key = keys[0]
	f.value = args[0]
	cmd := redis.NewCmd(ctx)
	cmd.SetVal(f.replayAllowed)
	cmd.SetErr(f.err)
	return cmd
}

func TestAllowTransactional(t *testing.T) {
	client := &fakeSetNXClient{replayAllowed: 1}
	store := NewIdempotencyStore(client)
	ok, err := store.AllowTransactional(t.Context(), "example", time.Minute)
	if !ok || err != nil || client.key != idempotencyPrefix+"example" || client.value != int64(60000) {
		t.Fatalf("transactional reservation: %v %v %s %v", ok, err, client.key, client.value)
	}
	client.replayAllowed = 0
	if ok, err = store.AllowTransactional(t.Context(), "example", time.Minute); ok || err != nil {
		t.Fatalf("legacy reservation: %v %v", ok, err)
	}
}
