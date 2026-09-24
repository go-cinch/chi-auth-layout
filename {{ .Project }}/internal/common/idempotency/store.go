package idempotency

import (
	"context"
	"errors"
	"sync"
	"time"
)

type Store interface {
	Claim(context.Context, string, time.Duration) (bool, error)
}

type MemoryStore struct {
	mu          sync.Mutex
	expiredAt   map[string]time.Time
	nextCleanup time.Time
	now         func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{expiredAt: make(map[string]time.Time), now: time.Now}
}

func (s *MemoryStore) Claim(ctx context.Context, key string, ttl time.Duration) (bool, error) {
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
		for storedKey, expiration := range s.expiredAt {
			if !now.Before(expiration) {
				delete(s.expiredAt, storedKey)
			}
		}
		cleanupInterval := min(ttl, time.Minute)
		s.nextCleanup = now.Add(cleanupInterval)
	}
	if expiration, exists := s.expiredAt[key]; exists && now.Before(expiration) {
		return false, nil
	}
	s.expiredAt[key] = now.Add(ttl)
	return true, nil
}
