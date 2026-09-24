package auth

import (
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
{{- if .Computed.enable_redis_final }}
	"github.com/redis/go-redis/v9"
{{- end }}
)

var ErrSessionNotFound = apperror.New("AUTH_SESSION_NOT_FOUND", "authentication session not found")

type SessionStore interface {
	Put(context.Context, string, string, string, time.Duration) error
	TakeRefresh(context.Context, string) (string, error)
	Get(context.Context, string) (string, error)
	Delete(context.Context, string, string) error
}

type SessionToken struct {
	SessionID   string
	RefreshToken string
	ExpiredAt   time.Time
	RememberMe  bool
}

type refreshSession struct {
	SessionID     string `json:"session_id"`
	UserID        int64  `json:"user_id"`
	Username      string `json:"username"`
	Code          string `json:"code"`
	CredentialVersion int64 `json:"credential_version"`
	RefreshDigest string `json:"refresh_digest"`
	RememberMe    bool   `json:"remember_me"`
}

func (s refreshSession) identity() authn.Identity {
	return authn.Identity{UserID: s.UserID, Username: s.Username, Code: s.Code, SessionID: s.SessionID, CredentialVersion: s.CredentialVersion}
}

type Sessions struct {
	store              SessionStore
	sessionLifetime    time.Duration
	rememberLifetime   time.Duration
}

func NewSessions(store SessionStore, sessionLifetime, rememberLifetime time.Duration) (*Sessions, error) {
	if store == nil {
		return nil, errors.New("authentication session store is required")
	}
	if sessionLifetime <= 0 || rememberLifetime < sessionLifetime {
		return nil, errors.New("remember lifetime must be at least the positive session lifetime")
	}
	return &Sessions{
		store:            store,
		sessionLifetime:  sessionLifetime,
		rememberLifetime: rememberLifetime,
	}, nil
}

func (s *Sessions) Issue(ctx context.Context, identity authn.Identity, rememberMe bool) (*SessionToken, error) {
	if identity.CredentialVersion <= 0 || identity.UserID <= 0 || strings.TrimSpace(identity.Username) == "" || strings.TrimSpace(identity.Code) == "" {
		return nil, ErrSessionNotFound
	}
	sessionID, err := randomSessionValue()
	if err != nil {
		return nil, fmt.Errorf("generate authentication session id: %w", err)
	}
	record := refreshSession{
		SessionID:  sessionID,
		UserID:     identity.UserID,
		Username:   identity.Username,
		Code:       identity.Code,
		RememberMe: rememberMe,
		CredentialVersion: identity.CredentialVersion,
	}
	return s.save(ctx, record)
}

func (s *Sessions) ConsumeRefresh(ctx context.Context, token string) (refreshSession, error) {
	digest, err := refreshDigest(token)
	if err != nil {
		return refreshSession{}, ErrSessionNotFound
	}
	value, err := s.store.TakeRefresh(ctx, digest)
	if errors.Is(err, ErrSessionNotFound) {
		return refreshSession{}, ErrSessionNotFound
	}
	if err != nil {
		return refreshSession{}, fmt.Errorf("consume refresh token: %w", err)
	}
	var record refreshSession
	if json.Unmarshal([]byte(value), &record) != nil || record.SessionID == "" || record.UserID <= 0 || record.CredentialVersion <= 0 ||
		subtle.ConstantTimeCompare([]byte(record.RefreshDigest), []byte(digest)) != 1 {
		return refreshSession{}, ErrSessionNotFound
	}
	return record, nil
}

func (s *Sessions) Renew(ctx context.Context, record refreshSession, identity authn.Identity) (*SessionToken, error) {
	if record.SessionID == "" || identity.UserID != record.UserID || identity.Code != record.Code || identity.CredentialVersion <= 0 || identity.CredentialVersion != record.CredentialVersion {
		return nil, ErrSessionNotFound
	}
	record.Username = identity.Username
	return s.save(ctx, record)
}

func (s *Sessions) save(ctx context.Context, record refreshSession) (*SessionToken, error) {
	refreshToken, err := randomSessionValue()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	record.RefreshDigest, err = refreshDigest(refreshToken)
	if err != nil {
		return nil, err
	}
	lifetime := s.sessionLifetime
	if record.RememberMe {
		lifetime = s.rememberLifetime
	}
	value, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode authentication session: %w", err)
	}
	if err := s.store.Put(ctx, record.SessionID, record.RefreshDigest, string(value), lifetime); err != nil {
		return nil, fmt.Errorf("store authentication session: %w", err)
	}
	return &SessionToken{
		SessionID:    record.SessionID,
		RefreshToken: refreshToken,
		ExpiredAt:    time.Now().Add(lifetime),
		RememberMe:   record.RememberMe,
	}, nil
}

func (s *Sessions) ValidateSession(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return ErrSessionNotFound
	}
	if _, err := s.store.Get(ctx, sessionID); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("validate authentication session: %w", err)
	}
	return nil
}

// ValidateIdentity binds signed claims to the stored session before checking the user.
func (s *Sessions) ValidateIdentity(ctx context.Context, identity authn.Identity) error {
	if identity.SessionID == "" || identity.CredentialVersion <= 0 {
		return ErrSessionNotFound
	}
	value, err := s.store.Get(ctx, identity.SessionID)
	if err != nil {
		return err
	}
	var record refreshSession
	if json.Unmarshal([]byte(value), &record) != nil || record.identity() != identity {
		return ErrSessionNotFound
	}
	return nil
}

func (s *Sessions) Revoke(ctx context.Context, sessionID string) error {
	value, err := s.store.Get(ctx, sessionID)
	if errors.Is(err, ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read authentication session for revocation: %w", err)
	}
	var record refreshSession
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return fmt.Errorf("decode authentication session for revocation: %w", err)
	}
	if err := s.store.Delete(ctx, sessionID, record.RefreshDigest); err != nil {
		return fmt.Errorf("revoke authentication session: %w", err)
	}
	return nil
}

func (s *Sessions) RevokeRefresh(ctx context.Context, token string) error {
	record, err := s.ConsumeRefresh(ctx, token)
	if errors.Is(err, ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.Revoke(ctx, record.SessionID)
}

func randomSessionValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func refreshDigest(token string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || len(raw) != 32 {
		return "", ErrSessionNotFound
	}
	digest := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

type storedMemorySession struct {
	value         string
	refreshDigest string
	expiredAt     time.Time
}

type memorySessionStore struct {
	mu       sync.Mutex
	sessions map[string]storedMemorySession
	refresh  map[string]string
}

func NewMemorySessionStore() SessionStore {
	return &memorySessionStore{sessions: make(map[string]storedMemorySession), refresh: make(map[string]string)}
}

func (s *memorySessionStore) Put(_ context.Context, sessionID, digest, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.sessions[sessionID]; ok {
		delete(s.refresh, current.refreshDigest)
	}
	s.sessions[sessionID] = storedMemorySession{value: value, refreshDigest: digest, expiredAt: time.Now().Add(ttl)}
	s.refresh[digest] = sessionID
	return nil
}

func (s *memorySessionStore) TakeRefresh(_ context.Context, digest string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sessionID, ok := s.refresh[digest]
	delete(s.refresh, digest)
	if !ok {
		return "", ErrSessionNotFound
	}
	value, ok := s.sessions[sessionID]
	if !ok || !value.expiredAt.After(time.Now()) {
		delete(s.sessions, sessionID)
		return "", ErrSessionNotFound
	}
	return value.value, nil
}

func (s *memorySessionStore) Get(_ context.Context, sessionID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.sessions[sessionID]
	if !ok || !value.expiredAt.After(time.Now()) {
		if ok {
			delete(s.refresh, value.refreshDigest)
			delete(s.sessions, sessionID)
		}
		return "", ErrSessionNotFound
	}
	return value.value, nil
}

func (s *memorySessionStore) Delete(_ context.Context, sessionID, digest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value, ok := s.sessions[sessionID]; ok {
		delete(s.refresh, value.refreshDigest)
	}
	delete(s.refresh, digest)
	delete(s.sessions, sessionID)
	return nil
}

{{ if .Computed.enable_redis_final -}}
type redisSessionClient interface {
	Set(context.Context, string, interface{}, time.Duration) *redis.StatusCmd
	Get(context.Context, string) *redis.StringCmd
	GetDel(context.Context, string) *redis.StringCmd
	Del(context.Context, ...string) *redis.IntCmd
}

type redisSessionStore struct {
	client redisSessionClient
}

func NewRedisSessionStore(client redisSessionClient) SessionStore {
	return &redisSessionStore{client: client}
}

func (s *redisSessionStore) Put(ctx context.Context, sessionID, digest, value string, ttl time.Duration) error {
	refreshKey := "refresh:" + digest
	if err := s.client.Set(ctx, refreshKey, sessionID, ttl).Err(); err != nil {
		return err
	}
	if err := s.client.Set(ctx, "session:"+sessionID, value, ttl).Err(); err != nil {
		_ = s.client.Del(ctx, refreshKey).Err()
		return err
	}
	return nil
}

func (s *redisSessionStore) TakeRefresh(ctx context.Context, digest string) (string, error) {
	sessionID, err := s.client.GetDel(ctx, "refresh:"+digest).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrSessionNotFound
	}
	if err != nil {
		return "", err
	}
	return s.Get(ctx, sessionID)
}

func (s *redisSessionStore) Get(ctx context.Context, sessionID string) (string, error) {
	value, err := s.client.Get(ctx, "session:"+sessionID).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrSessionNotFound
	}
	return value, err
}

func (s *redisSessionStore) Delete(ctx context.Context, sessionID, digest string) error {
	return s.client.Del(ctx, "session:"+sessionID, "refresh:"+digest).Err()
}
{{ end -}}

func (s *Sessions) authenticatedSession(ctx context.Context, identity authn.Identity) (refreshSession, error) {
    identity.PasswordResetRequired = false
    value, err := s.store.Get(ctx, identity.SessionID)
    if err != nil { return refreshSession{}, err }
    var record refreshSession
    if json.Unmarshal([]byte(value), &record) != nil || record.identity() != identity {
        return refreshSession{}, ErrSessionNotFound
    }
    return record, nil
}
