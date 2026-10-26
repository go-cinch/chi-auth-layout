package auth

import (
	"context"
	"errors"
	"sync"
	"time"
	{{- if .Computed.enable_redis_final }}
	"fmt"

	"github.com/redis/go-redis/v9"
	{{- end }}
)

const (
	defaultSessionCapacity        = 10000
	defaultSessionCleanupInterval = time.Minute
)

var errSessionStoreClosed = errors.New("authentication session store is closed")

type storedMemorySession struct {
	value     string
	expiredAt time.Time
}

type memorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]storedMemorySession
	capacity int
	closed   bool
	done     chan struct{}
	stopped  chan struct{}
}

func NewMemorySessionStore() *memorySessionStore {
	return newMemorySessionStore(defaultSessionCapacity, defaultSessionCleanupInterval)
}

func newMemorySessionStore(capacity int, cleanupInterval time.Duration) *memorySessionStore {
	if capacity <= 0 || cleanupInterval <= 0 {
		panic("session capacity and cleanup interval must be positive")
	}
	s := &memorySessionStore{
		sessions: make(map[string]storedMemorySession),
		capacity: capacity,
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go s.cleanExpired(cleanupInterval)
	return s
}

func (s *memorySessionStore) Create(ctx context.Context, id, value string, expiredAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkLocked(ctx); err != nil {
		return err
	}
	now := time.Now()
	if !expiredAt.After(now) {
		return ErrSessionNotFound
	}
	if current, ok := s.sessions[id]; ok {
		if current.expiredAt.After(now) {
			return errSessionConflict
		}
		delete(s.sessions, id)
	}
	if len(s.sessions) >= s.capacity {
		s.removeExpiredLocked(now)
		if len(s.sessions) >= s.capacity {
			return errSessionCapacity
		}
	}
	s.sessions[id] = storedMemorySession{value: value, expiredAt: expiredAt}
	return nil
}

func (s *memorySessionStore) Get(ctx context.Context, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkLocked(ctx); err != nil {
		return "", err
	}
	current, ok := s.sessions[id]
	if !ok || !current.expiredAt.After(time.Now()) {
		delete(s.sessions, id)
		return "", ErrSessionNotFound
	}
	return current.value, nil
}

func (s *memorySessionStore) Rotate(ctx context.Context, id, expected, value string, expiredAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkLocked(ctx); err != nil {
		return false, err
	}
	now := time.Now()
	current, ok := s.sessions[id]
	if !ok || !current.expiredAt.After(now) {
		delete(s.sessions, id)
		return false, nil
	}
	if current.value != expected || !expiredAt.After(now) {
		return false, nil
	}
	s.sessions[id] = storedMemorySession{value: value, expiredAt: expiredAt}
	return true, nil
}

func (s *memorySessionStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.checkLocked(ctx); err != nil {
		return err
	}
	delete(s.sessions, id)
	return nil
}

func (s *memorySessionStore) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.sessions = nil
		close(s.done)
	}
	s.mu.Unlock()
	<-s.stopped
	return nil
}

func (s *memorySessionStore) checkLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.closed {
		return errSessionStoreClosed
	}
	return nil
}

func (s *memorySessionStore) cleanExpired(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer close(s.stopped)
	for {
		select {
		case <-s.done:
			return
		case now := <-ticker.C:
			s.mu.Lock()
			s.removeExpiredLocked(now)
			s.mu.Unlock()
		}
	}
}

func (s *memorySessionStore) removeExpiredLocked(now time.Time) {
	for id, current := range s.sessions {
		if !current.expiredAt.After(now) {
			delete(s.sessions, id)
		}
	}
}

{{ if .Computed.enable_redis_final -}}
type redisSessionClient interface {
	Eval(context.Context, string, []string, ...interface{}) *redis.Cmd
	Get(context.Context, string) *redis.StringCmd
	Del(context.Context, ...string) *redis.IntCmd
}

type redisSessionStore struct {
	client redisSessionClient
}

const createSessionScript = `
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
if tonumber(ARGV[2]) <= now_ms then
  return -1
end
local created = redis.call('SET', KEYS[1], ARGV[1], 'NX', 'PXAT', ARGV[2])
if created then
  return 1
end
return 0
`

const rotateSessionScript = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then
  return 0
end
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
if tonumber(ARGV[3]) <= now_ms then
  return 0
end
local updated = redis.call('SET', KEYS[1], ARGV[2], 'XX', 'PXAT', ARGV[3])
if updated then
  return 1
end
return 0
`

func NewRedisSessionStore(client redisSessionClient) SessionStore {
	return &redisSessionStore{client: client}
}

func (s *redisSessionStore) Create(ctx context.Context, id, value string, expiredAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	created, err := s.client.Eval(ctx, createSessionScript, []string{"session:data:" + id}, value, expiredAt.UnixMilli()).Int64()
	if err != nil {
		return err
	}
	switch created {
	case 1:
		return nil
	case 0:
		return errSessionConflict
	case -1:
		return ErrSessionNotFound
	default:
		return fmt.Errorf("unexpected session create result: %d", created)
	}
}

func (s *redisSessionStore) Get(ctx context.Context, id string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	value, err := s.client.Get(ctx, "session:data:"+id).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrSessionNotFound
	}
	return value, err
}

func (s *redisSessionStore) Rotate(ctx context.Context, id, expected, value string, expiredAt time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	updated, err := s.client.Eval(ctx, rotateSessionScript, []string{"session:data:" + id}, expected, value, expiredAt.UnixMilli()).Int64()
	if err != nil {
		return false, err
	}
	switch updated {
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected session rotate result: %d", updated)
	}
}

func (s *redisSessionStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.client.Del(ctx, "session:data:"+id).Err()
}

{{ end }}
