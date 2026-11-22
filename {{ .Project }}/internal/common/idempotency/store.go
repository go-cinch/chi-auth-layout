package idempotency

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Store interface {
	Claim(context.Context, string, time.Duration) (bool, error)
	// AllowTransactional admits repeat transactional writers and rejects live legacy claims.
	AllowTransactional(context.Context, string, time.Duration) (bool, error)
}

type claim struct {
	expires       time.Time
	transactional bool
}

type MemoryStore struct {
	mu          sync.Mutex
	claims      map[string]claim
	nextCleanup time.Time
	now         func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{claims: make(map[string]claim), now: time.Now}
}

func (s *MemoryStore) Claim(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.claim(ctx, key, ttl, false)
}

func (s *MemoryStore) AllowTransactional(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.claim(ctx, key, ttl, true)
}

func (s *MemoryStore) claim(ctx context.Context, key string, ttl time.Duration, transactional bool) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if key == "" {
		return false, errors.New("idempotency key is required")
	}
	if ttl <= 0 {
		return false, errors.New("idempotency ttl must be positive")
	}

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nextCleanup.IsZero() || !now.Before(s.nextCleanup) {
		for storedKey, entry := range s.claims {
			if !now.Before(entry.expires) {
				delete(s.claims, storedKey)
			}
		}
		cleanupInterval := min(ttl, time.Minute)
		s.nextCleanup = now.Add(cleanupInterval)
	}
	if entry, exists := s.claims[key]; exists && now.Before(entry.expires) {
		return transactional && entry.transactional, nil
	}
	s.claims[key] = claim{expires: now.Add(ttl), transactional: transactional}
	return true, nil
}
