package auth

import (
    "context"
    "time"
{{- if .Computed.enable_redis_final }}
    "errors"
    "github.com/redis/go-redis/v9"
    "{{ .Computed.module_name_final }}/internal/infra/rds"
{{- end }}
)

// MFAStateStore stores transient MFA state centrally. Production never falls
// back to process memory: losing Redis must not bypass or widen verification.
type MFAStateStore interface {
    Put(context.Context, string, string, time.Duration) error
    Get(context.Context, string) (string, error)
    Take(context.Context, string) (string, error)
    Attempt(context.Context, string, time.Duration, int64) (bool, error)
    ResetAttempts(context.Context, string) error
}

{{ if .Computed.enable_redis_final -}}
type mfaRedisClient interface {
    Set(context.Context, string, interface{}, time.Duration) *redis.StatusCmd
    Get(context.Context, string) *redis.StringCmd
    GetDel(context.Context, string) *redis.StringCmd
    Eval(context.Context, string, []string, ...interface{}) *redis.Cmd
}

type redisMFAStore struct { client mfaRedisClient }

func NewRedisMFAStore(client mfaRedisClient) MFAStateStore { return &redisMFAStore{client: client} }

func (s *redisMFAStore) Put(ctx context.Context, key, value string, ttl time.Duration) error {
    return s.client.Set(ctx, "mfa:"+key, value, ttl).Err()
}

func (s *redisMFAStore) Get(ctx context.Context, key string) (string, error) {
    value, err := s.client.Get(ctx, "mfa:"+key).Result()
    if errors.Is(err, redis.Nil) { return "", ErrMFAInvalid }
    return value, err
}

func (s *redisMFAStore) Take(ctx context.Context, key string) (string, error) {
    value, err := s.client.GetDel(ctx, "mfa:"+key).Result()
    if errors.Is(err, redis.Nil) { return "", ErrMFAInvalid }
    return value, err
}

func (s *redisMFAStore) Attempt(ctx context.Context, key string, ttl time.Duration, maxAttempts int64) (bool, error) {
    if ttl < time.Millisecond || ttl%time.Millisecond != 0 || maxAttempts <= 0 { return false, errors.New("invalid MFA rate limit") }
    count, err := rds.IncrementWindow(ctx, s.client, "mfa:attempt:"+key, ttl)
    if err != nil { return false, err }
    return count <= maxAttempts, nil
}

func (s *redisMFAStore) ResetAttempts(ctx context.Context, key string) error {
    _, err := s.client.GetDel(ctx, "mfa:attempt:"+key).Result()
    if errors.Is(err, redis.Nil) { return nil }
    return err
}
{{ end -}}
