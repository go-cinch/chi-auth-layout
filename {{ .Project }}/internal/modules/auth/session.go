package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

var (
	ErrSessionNotFound = apperror.New("AUTH_SESSION_NOT_FOUND", "authentication session not found")
	errSessionConflict = errors.New("authentication session already exists")
	errSessionCapacity = errors.New("authentication session capacity reached")
)

// SessionStore operations are atomic. Rotate must compare the complete previous
// value and must never recreate a deleted or expired session. Deadlines are
// absolute: a retry must not extend a session's life.
type SessionStore interface {
	Create(context.Context, string, string, time.Time) error
	Get(context.Context, string) (string, error)
	Rotate(context.Context, string, string, string, time.Time) (bool, error)
	Delete(context.Context, string) error
}

type SessionConfig struct {
	Issuer           string
	Key              string
	SessionLifetime  time.Duration
	RememberLifetime time.Duration
	RetryWindow      time.Duration
}

type SessionToken struct {
	SessionID    string
	RefreshToken string
	ExpiredAt    time.Time
	RememberMe   bool
}

type refreshSession struct {
	SessionID         string `json:"session_id"`
	UserID            int64  `json:"user_id"`
	Username          string `json:"username"`
	Code              string `json:"code"`
	CredentialVersion int64  `json:"credential_version"`
	RememberMe        bool   `json:"remember_me"`
	Generation        int64  `json:"generation"`
	CreatedAt         int64  `json:"created_at"`
	ExpiredAt         int64  `json:"expired_at"`
	RotatedAt         int64  `json:"rotated_at"`
	Signature         string `json:"signature"`

	raw               string
	refreshGeneration int64
}

func (s refreshSession) identity() authn.Identity {
	return authn.Identity{UserID: s.UserID, Username: s.Username, Code: s.Code, SessionID: s.SessionID, CredentialVersion: s.CredentialVersion}
}

type Sessions struct {
	store            SessionStore
	key              []byte
	namespace        string
	sessionLifetime  time.Duration
	rememberLifetime time.Duration
	retryWindow      time.Duration
}

func NewSessions(store SessionStore, cfg SessionConfig) (*Sessions, error) {
	if store == nil {
		return nil, errors.New("authentication session store is required")
	}
	if strings.TrimSpace(cfg.Issuer) == "" || len([]byte(cfg.Key)) < 32 {
		return nil, errors.New("authentication sessions require an issuer and a signing key of at least 32 bytes")
	}
	if cfg.SessionLifetime < time.Millisecond || cfg.RememberLifetime < cfg.SessionLifetime {
		return nil, errors.New("remember lifetime must be at least the positive session lifetime")
	}
	if cfg.RetryWindow < time.Millisecond || cfg.RetryWindow > time.Minute || cfg.RetryWindow >= cfg.SessionLifetime {
		return nil, errors.New("refresh retry window must be between one millisecond and one minute and shorter than the session lifetime")
	}
	// Separate refresh/record MACs from JWT signing, and bind both to the issuer.
	mac := hmac.New(sha256.New, []byte(cfg.Key))
	_, _ = mac.Write([]byte("auth/session/v2\x00" + strings.TrimSpace(cfg.Issuer)))
	s := &Sessions{store: store, key: mac.Sum(nil), sessionLifetime: cfg.SessionLifetime, rememberLifetime: cfg.RememberLifetime, retryWindow: cfg.RetryWindow}
	s.namespace = s.sign("namespace", "session")
	return s, nil
}

func (s *Sessions) Issue(ctx context.Context, identity authn.Identity, rememberMe bool) (*SessionToken, error) {
	if identity.CredentialVersion <= 0 || identity.UserID <= 0 || strings.TrimSpace(identity.Username) == "" || strings.TrimSpace(identity.Code) == "" {
		return nil, ErrSessionNotFound
	}
	sessionID, err := randomSessionValue()
	if err != nil {
		return nil, fmt.Errorf("generate authentication session id: %w", err)
	}
	lifetime := s.sessionLifetime
	if rememberMe {
		lifetime = s.rememberLifetime
	}
	now := time.Now()
	record := refreshSession{
		SessionID: sessionID, UserID: identity.UserID, Username: strings.TrimSpace(identity.Username), Code: strings.TrimSpace(identity.Code),
		CredentialVersion: identity.CredentialVersion, RememberMe: rememberMe, Generation: 1,
		CreatedAt: now.UnixMilli(), ExpiredAt: now.Add(lifetime).UnixMilli(),
	}
	value, err := s.encodeRecord(record)
	if err != nil {
		return nil, err
	}
	if err := s.store.Create(ctx, s.storageID(sessionID), value, time.UnixMilli(record.ExpiredAt)); err != nil {
		return nil, fmt.Errorf("create authentication session: %w", err)
	}
	return s.token(record), nil
}

// ReadRefresh authenticates without consuming the token. Database or signing
// failures before Renew therefore leave the existing session retryable.
func (s *Sessions) ReadRefresh(ctx context.Context, token string) (refreshSession, error) {
	sessionID, generation, err := s.parseRefresh(token)
	if err != nil {
		return refreshSession{}, err
	}
	receivedAt := time.Now().UnixMilli()
	record, err := s.read(ctx, sessionID)
	if err != nil {
		return refreshSession{}, err
	}
	if err := s.validateRefresh(ctx, record, generation, receivedAt); err != nil {
		return refreshSession{}, err
	}
	record.refreshGeneration = generation
	return record, nil
}

func (s *Sessions) Renew(ctx context.Context, record refreshSession, identity authn.Identity) (*SessionToken, error) {
	identity.PasswordResetRequired = false
	if record.refreshGeneration <= 0 || record.identity() != identity {
		return nil, ErrSessionNotFound
	}
	// Reload on a CAS conflict so concurrent refreshes converge on one token.
	// A concurrent Delete always wins over any later attempted rotation.
	for range 3 {
		current, err := s.read(ctx, record.SessionID)
		if err != nil {
			return nil, err
		}
		if current.identity() != identity {
			return nil, ErrSessionNotFound
		}
		// Replay is classified when ReadRefresh accepts the request, before
		// potentially slow database work. Do not revoke a family merely because
		// an already accepted request crossed a window while in flight.
		if current.Generation != record.Generation {
			if current.Generation == record.Generation+1 && record.refreshGeneration == record.Generation {
				return s.token(current), nil
			}
			return nil, ErrSessionNotFound
		}
		if record.refreshGeneration == current.Generation-1 || (record.refreshGeneration == current.Generation && s.withinRetry(current, time.Now().UnixMilli())) {
			return s.token(current), nil
		}
		if record.refreshGeneration != current.Generation || current.Generation == math.MaxInt64 {
			return nil, ErrSessionNotFound
		}
		next := current
		next.Generation++
		next.RotatedAt = time.Now().UnixMilli()
		value, err := s.encodeRecord(next)
		if err != nil {
			return nil, err
		}
		updated, err := s.store.Rotate(ctx, s.storageID(record.SessionID), current.raw, value, time.UnixMilli(current.ExpiredAt))
		if err != nil {
			return nil, fmt.Errorf("rotate authentication session: %w", err)
		}
		if updated {
			return s.token(next), nil
		}
	}
	return nil, errors.New("authentication session changed during refresh; retry the request")
}

// Within a fixed retry window even the current token does not rotate again.
// This prevents a late duplicate from becoming two generations behind and being
// mistaken for replay. Neither retries nor rotations extend absolute expiry.
func (s *Sessions) validateRefresh(ctx context.Context, record refreshSession, generation, receivedAt int64) error {
	if s.withinRetry(record, receivedAt) && (generation == record.Generation || generation == record.Generation-1) {
		return nil
	}
	if generation == record.Generation && generation < math.MaxInt64 {
		return nil
	}
	// If the read crossed newer rotations, reject without revoking: the token
	// may have been current at arrival. Otherwise its MAC proves a stale replay.
	if generation < record.Generation && record.RotatedAt <= receivedAt {
		if err := s.Revoke(ctx, record.SessionID); err != nil {
			return err
		}
	}
	return ErrSessionNotFound
}

func (s *Sessions) withinRetry(record refreshSession, at int64) bool {
	return record.RotatedAt > 0 && at < record.RotatedAt+s.retryWindow.Milliseconds()
}

func (s *Sessions) ValidateSession(ctx context.Context, sessionID string) error {
	_, err := s.read(ctx, sessionID)
	return err
}

func (s *Sessions) ValidateIdentity(ctx context.Context, identity authn.Identity) error {
	_, err := s.authenticatedSession(ctx, identity)
	if errors.Is(err, ErrSessionNotFound) {
		return fmt.Errorf("%w: %w", authn.ErrUnauthorized, err)
	}
	return err
}

func (s *Sessions) authenticatedSession(ctx context.Context, identity authn.Identity) (refreshSession, error) {
	identity.PasswordResetRequired = false
	record, err := s.read(ctx, identity.SessionID)
	if err != nil {
		return refreshSession{}, err
	}
	if record.identity() != identity {
		return refreshSession{}, ErrSessionNotFound
	}
	return record, nil
}

func (s *Sessions) Revoke(ctx context.Context, sessionID string) error {
	if !validSessionID(sessionID) {
		return nil
	}
	if err := s.store.Delete(ctx, s.storageID(sessionID)); err != nil {
		return fmt.Errorf("revoke authentication session: %w", err)
	}
	return nil
}

func (s *Sessions) RevokeRefresh(ctx context.Context, token string) error {
	sessionID, _, err := s.parseRefresh(token)
	if errors.Is(err, ErrSessionNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// Any authenticated generation can revoke the family, including a token
	// used before rotation. Delete is a single retryable operation.
	return s.Revoke(ctx, sessionID)
}

func (s *Sessions) read(ctx context.Context, sessionID string) (refreshSession, error) {
	if !validSessionID(sessionID) {
		return refreshSession{}, ErrSessionNotFound
	}
	value, err := s.store.Get(ctx, s.storageID(sessionID))
	if err != nil {
		return refreshSession{}, err
	}
	record, err := s.decodeRecord(value)
	if err != nil || record.SessionID != sessionID {
		return refreshSession{}, ErrSessionNotFound
	}
	return record, nil
}

func (s *Sessions) encodeRecord(record refreshSession) (string, error) {
	record.Signature = ""
	value, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("encode authentication session: %w", err)
	}
	record.Signature = s.sign("record", string(value))
	value, err = json.Marshal(record)
	return string(value), err
}

func (s *Sessions) decodeRecord(value string) (refreshSession, error) {
	var record refreshSession
	if json.Unmarshal([]byte(value), &record) != nil || !validSessionID(record.SessionID) || record.UserID <= 0 || record.CredentialVersion <= 0 ||
		strings.TrimSpace(record.Username) == "" || strings.TrimSpace(record.Code) == "" || record.Generation <= 0 || record.CreatedAt <= 0 ||
		record.ExpiredAt <= record.CreatedAt || record.ExpiredAt <= time.Now().UnixMilli() || record.RotatedAt < 0 ||
		(record.Generation == 1 && record.RotatedAt != 0) || (record.Generation > 1 && (record.RotatedAt < record.CreatedAt || record.RotatedAt >= record.ExpiredAt)) {
		return refreshSession{}, ErrSessionNotFound
	}
	signature := record.Signature
	record.Signature = ""
	unsigned, err := json.Marshal(record)
	if err != nil || !hmac.Equal([]byte(signature), []byte(s.sign("record", string(unsigned)))) {
		return refreshSession{}, ErrSessionNotFound
	}
	record.Signature = signature
	record.raw = value
	return record, nil
}

func (s *Sessions) token(record refreshSession) *SessionToken {
	payload := "v2." + record.SessionID + "." + strconv.FormatInt(record.Generation, 10)
	return &SessionToken{SessionID: record.SessionID, RefreshToken: payload + "." + s.sign("refresh", payload), ExpiredAt: time.UnixMilli(record.ExpiredAt), RememberMe: record.RememberMe}
}

func (s *Sessions) parseRefresh(token string) (string, int64, error) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(token) > 128 || len(parts) != 4 || parts[0] != "v2" || !validSessionID(parts[1]) {
		return "", 0, ErrSessionNotFound
	}
	generation, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || generation <= 0 || strconv.FormatInt(generation, 10) != parts[2] ||
		!hmac.Equal([]byte(parts[3]), []byte(s.sign("refresh", strings.Join(parts[:3], ".")))) {
		return "", 0, ErrSessionNotFound
	}
	return parts[1], generation, nil
}

func (s *Sessions) sign(purpose, value string) string {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte(purpose + "\x00" + value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Sessions) storageID(sessionID string) string {
	return s.namespace + ":" + sessionID
}

func randomSessionValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validSessionID(value string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == value
}
