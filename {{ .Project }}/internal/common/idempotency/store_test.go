package idempotency

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryStoreClaim(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, time.September, 21, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	claimed, err := store.Claim(t.Context(), "request-1", time.Hour)
	if err != nil || !claimed {
		t.Fatalf("first claim = %v, %v", claimed, err)
	}
	claimed, err = store.Claim(t.Context(), "request-1", time.Hour)
	if err != nil || claimed {
		t.Fatalf("duplicate claim = %v, %v", claimed, err)
	}
	now = now.Add(time.Hour)
	claimed, err = store.Claim(t.Context(), "request-1", time.Hour)
	if err != nil || !claimed {
		t.Fatalf("expired claim = %v, %v", claimed, err)
	}
}

func TestMemoryStoreClaimValidation(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Claim(t.Context(), "", time.Hour); err == nil {
		t.Fatal("empty key was accepted")
	}
	if _, err := store.Claim(t.Context(), "request-1", 0); err == nil {
		t.Fatal("zero ttl was accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Claim(ctx, "request-1", time.Hour); err != context.Canceled {
		t.Fatalf("canceled context error = %v", err)
	}
}

func TestMemoryStoreClaimIsAtomic(t *testing.T) {
	store := NewMemoryStore()
	var claimed atomic.Int32
	var group sync.WaitGroup
	for range 32 {
		group.Add(1)
		go func() {
			defer group.Done()
			ok, err := store.Claim(t.Context(), "shared-request", time.Hour)
			if err != nil {
				t.Errorf("Claim() error = %v", err)
			}
			if ok {
				claimed.Add(1)
			}
		}()
	}
	group.Wait()
	if claimed.Load() != 1 {
		t.Fatalf("successful claims = %d", claimed.Load())
	}
}
