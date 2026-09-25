package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

{{- if .Computed.enable_redis_final }}
	"github.com/redis/go-redis/v9"
{{- end }}
)

type PasswordFailureStore interface {
	Count(context.Context, int64) (int64, error)
	Increment(context.Context, int64, time.Duration) (int64, error)
	Clear(context.Context, int64) error
}

type PasswordChangeGuard struct {
	store     PasswordFailureStore
	captcha   *PointCaptcha
	threshold int64
	lockThreshold int64
	ttl       time.Duration
}

func NewPasswordChangeGuard(store PasswordFailureStore, captcha *PointCaptcha, threshold, lockThreshold int64, ttl time.Duration) (*PasswordChangeGuard, error) {
	if store == nil {
		return nil, errors.New("password failure store is required")
	}
	if captcha == nil {
		return nil, errors.New("password change captcha is required")
	}
	if threshold < 1 {
		return nil, errors.New("password change threshold must be positive")
	}
	if lockThreshold <= threshold {
		return nil, errors.New("password change lock threshold must exceed the captcha threshold")
	}
	if ttl <= 0 || ttl > 24*time.Hour {
		return nil, errors.New("password failure ttl must be between 1ns and 24h")
	}
	return &PasswordChangeGuard{store: store, captcha: captcha, threshold: threshold, lockThreshold: lockThreshold, ttl: ttl}, nil
}

func (g *PasswordChangeGuard) Verify(ctx context.Context, userID int64, captchaID string, points []CaptchaPoint) (bool, error) {
	required, err := g.Required(ctx, userID)
	if err != nil || !required {
		return !required, err
	}
	return g.captcha.VerifyPasswordChange(ctx, passwordChangeCaptchaSubject(userID), captchaID, points)
}

func (g *PasswordChangeGuard) Check(ctx context.Context, userID int64, captchaID string, points []CaptchaPoint) (bool, error) {
	if userID <= 0 {
		return false, ErrUnauthorized
	}
	return g.captcha.CheckPasswordChange(ctx, passwordChangeCaptchaSubject(userID), captchaID, points)
}

func (g *PasswordChangeGuard) Required(ctx context.Context, userID int64) (bool, error) {
	count, err := g.FailureCount(ctx, userID)
	if err != nil {
		return false, err
	}
	return g.CaptchaRequired(count), nil
}

func (g *PasswordChangeGuard) FailureCount(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, ErrUnauthorized
	}
	count, err := g.store.Count(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("query password failures: %w", err)
	}
	return count, nil
}

func (g *PasswordChangeGuard) CaptchaRequired(count int64) bool {
	return count >= g.threshold
}

func (g *PasswordChangeGuard) LockRequired(count int64) bool {
	return count >= g.lockThreshold
}

func (g *PasswordChangeGuard) LockThreshold() int64 {
	return g.lockThreshold
}

func (g *PasswordChangeGuard) RecordFailure(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, ErrUnauthorized
	}
	count, err := g.store.Increment(ctx, userID, g.ttl)
	if err != nil {
		return 0, fmt.Errorf("record password failure: %w", err)
	}
	return count, nil
}

func (g *PasswordChangeGuard) Clear(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return ErrUnauthorized
	}
	if err := g.store.Clear(ctx, userID); err != nil {
		return fmt.Errorf("clear password failures: %w", err)
	}
	return nil
}

func (g *PasswordChangeGuard) NewChallenge(ctx context.Context, userID int64) (*PointCaptchaChallenge, error) {
	if userID <= 0 {
		return nil, ErrUnauthorized
	}
	return g.captcha.NewPasswordChangeChallenge(ctx, passwordChangeCaptchaSubject(userID))
}

func (g *PasswordChangeGuard) RefreshChallenge(ctx context.Context, captchaID string) (*PointCaptchaChallenge, error) {
	return g.captcha.RefreshPasswordChangeChallenge(ctx, captchaID)
}

func passwordChangeCaptchaSubject(userID int64) string {
	return "password-change:" + strconv.FormatInt(userID, 10)
}

type memoryPasswordFailure struct {
	count     int64
	expiredAt time.Time
}

type memoryPasswordFailureStore struct {
	mu     sync.Mutex
	values map[int64]memoryPasswordFailure
}

func NewMemoryPasswordFailureStore() PasswordFailureStore {
	return &memoryPasswordFailureStore{values: make(map[int64]memoryPasswordFailure)}
}

func (s *memoryPasswordFailureStore) Count(_ context.Context, userID int64) (int64, error) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[userID]
	if !ok || !value.expiredAt.After(now) {
		delete(s.values, userID)
		return 0, nil
	}
	return value.count, nil
}

func (s *memoryPasswordFailureStore) Increment(_ context.Context, userID int64, ttl time.Duration) (int64, error) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[userID]
	if !ok || !value.expiredAt.After(now) {
		value = memoryPasswordFailure{expiredAt: now.Add(ttl)}
	}
	value.count++
	s.values[userID] = value
	return value.count, nil
}

func (s *memoryPasswordFailureStore) Clear(_ context.Context, userID int64) error {
	s.mu.Lock()
	delete(s.values, userID)
	s.mu.Unlock()
	return nil
}

{{ if .Computed.enable_redis_final -}}
const incrementPasswordFailureScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return count
`

type redisPasswordFailureClient interface {
	Get(context.Context, string) *redis.StringCmd
	Eval(context.Context, string, []string, ...interface{}) *redis.Cmd
	Del(context.Context, ...string) *redis.IntCmd
}

type redisPasswordFailureStore struct {
	client redisPasswordFailureClient
}

func NewRedisPasswordFailureStore(client redisPasswordFailureClient) PasswordFailureStore {
	return &redisPasswordFailureStore{client: client}
}

func (s *redisPasswordFailureStore) Count(ctx context.Context, userID int64) (int64, error) {
	count, err := s.client.Get(ctx, passwordFailureKey(userID)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return count, err
}

func (s *redisPasswordFailureStore) Increment(ctx context.Context, userID int64, ttl time.Duration) (int64, error) {
	return s.client.Eval(ctx, incrementPasswordFailureScript, []string{passwordFailureKey(userID)}, ttl.Milliseconds()).Int64()
}

func (s *redisPasswordFailureStore) Clear(ctx context.Context, userID int64) error {
	return s.client.Del(ctx, passwordFailureKey(userID)).Err()
}

func passwordFailureKey(userID int64) string {
	return "password-change:failures:" + strconv.FormatInt(userID, 10)
}

{{ end -}}
