package auth

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

// Test-only store. Production Verification state has no memory fallback.
type testVerificationStore struct {
	mu             sync.Mutex
	values         map[string]string
	ttls           map[string]time.Duration
	attempts       map[string]int64
	attemptExpires map[string]time.Time
	resetAttempts  []string
	fail           error
}

func (s *testVerificationStore) Put(_ context.Context, key, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	s.values[key] = value
	if s.ttls == nil {
		s.ttls = map[string]time.Duration{}
	}
	s.ttls[key] = ttl
	return nil
}

func (s *testVerificationStore) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return "", s.fail
	}
	value, ok := s.values[key]
	if !ok {
		return "", ErrVerificationInvalid
	}
	return value, nil
}

func (s *testVerificationStore) Take(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return "", s.fail
	}
	value, ok := s.values[key]
	if !ok {
		return "", ErrVerificationInvalid
	}
	delete(s.values, key)
	return value, nil
}

func (s *testVerificationStore) Attempt(_ context.Context, key string, window time.Duration, maxAttempts int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return false, s.fail
	}
	if s.attempts == nil {
		s.attempts = map[string]int64{}
	}
	if s.attemptExpires == nil {
		s.attemptExpires = map[string]time.Time{}
	}
	if s.ttls == nil {
		s.ttls = map[string]time.Duration{}
	}
	if expired, exists := s.attemptExpires[key]; exists && !expired.After(time.Now()) {
		delete(s.attempts, key)
	}
	s.attempts[key]++
	if s.attempts[key] == 1 {
		s.attemptExpires[key] = time.Now().Add(window)
		s.ttls["attempt:"+key] = window
	}
	return s.attempts[key] <= maxAttempts, nil
}

func (s *testVerificationStore) ResetAttempts(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	delete(s.attempts, key)
	delete(s.attemptExpires, key)
	delete(s.ttls, "attempt:"+key)
	s.resetAttempts = append(s.resetAttempts, key)
	return nil
}

func newVerificationTest(t *testing.T) (*Module, sqlmock.Sqlmock, context.Context, authn.Identity, *testVerificationStore) {
	t.Helper()
	state := &testVerificationStore{values: map[string]string{}}
	m, manager, mock := newAuthTestModuleWithVerification(t, func(store *db.Store) *Verification {
		verification, err := NewVerification(store, state, VerificationConfig{TOTPEnabled: true, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: 30 * time.Second, TOTPEnrollmentTTL: 5 * time.Minute, SensitiveLifetime: time.Hour, RateLimitWindow: 5 * time.Minute, RateLimitMaxAttempts: 10, Issuer: "test", SigningKey: authTestKey})
		if err != nil {
			t.Fatal(err)
		}
		return verification
	})
	identity := authn.Identity{Methods: authn.Methods{Primary: authn.Password, SecondFactor: authn.TOTP}, UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}
	session, err := m.sessions.Issue(context.Background(), identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = session.SessionID
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/totp", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	request, err := manager.AuthenticateRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	return m, mock, request.Context(), identity, state
}

func expectTOTPBound(mock sqlmock.Sqlmock, enabled bool) {
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(enabled))
}

func expectVerificationLock(mock sqlmock.Sqlmock, version int64) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT credential_version, status").WithArgs(int64(3), "EXP78RGH").WillReturnRows(sqlmock.NewRows([]string{"version", "status"}).AddRow(version, 1))
}

func expectTOTPFactor(t *testing.T, m *Module, mock sqlmock.Sqlmock, secret string, step int64) {
	t.Helper()
	sealed, err := sealVerification(m.verification.cipher, secret, "totp:3")
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT secret_ciphertext, last_used_step").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"secret", "step", "period_seconds"}).AddRow(sealed, step, 30))
}

type delayedSensitiveStore struct {
	VerificationStateStore
	entered chan struct{}
	release chan struct{}
}

func (s *delayedSensitiveStore) Put(ctx context.Context, key, value string, ttl time.Duration) error {
	if strings.HasPrefix(key, "sensitive:") {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.VerificationStateStore.Put(ctx, key, value, ttl)
}

func TestVerificationConfiguration(t *testing.T) {
	m, _, _, _, _ := newVerificationTest(t)
	if _, err := NewVerification(nil, nil, VerificationConfig{TOTPEnabled: true}); err == nil {
		t.Fatal("invalid config accepted")
	}
	file := filepath.Join(t.TempDir(), "key")
	cfg := VerificationConfig{TOTPEnabled: true, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: 30 * time.Second, TOTPEnrollmentTTL: 5 * time.Minute, SensitiveLifetime: time.Hour, RateLimitWindow: 5 * time.Minute, RateLimitMaxAttempts: 10, Issuer: "test", SigningKey: authTestKey, EncryptionKeyFile: file}
	if _, err := NewVerification(m.store, nil, cfg); err == nil {
		t.Fatal("missing key accepted")
	}
	if err := os.WriteFile(file, []byte("bad!"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewVerification(m.store, nil, cfg); err == nil {
		t.Fatal("invalid key accepted")
	}
	if err := os.WriteFile(file, []byte(strings.Repeat("A", 43)+"="), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewVerification(m.store, nil, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestVerificationRejectsInvalidTimingConfiguration(t *testing.T) {
	m, _, _, _, _ := newVerificationTest(t)
	for _, timing := range []struct{ period, ttl time.Duration }{
		{0, time.Minute}, {-time.Second, time.Minute}, {1500 * time.Millisecond, time.Minute},
		{30 * time.Second, 0}, {30 * time.Second, -time.Minute}, {30 * time.Second, time.Microsecond},
	} {
		_, err := NewVerification(m.store, nil, VerificationConfig{TOTPEnabled: true, Issuer: "test", SigningKey: authTestKey, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: timing.period, TOTPEnrollmentTTL: timing.ttl, SensitiveLifetime: time.Hour, RateLimitWindow: 5 * time.Minute, RateLimitMaxAttempts: 10})
		if err == nil {
			t.Fatal("invalid Verification timing configuration accepted", timing)
		}
	}
}

func TestVerificationRateLimitConfiguration(t *testing.T) {
	m, _, _, _, _ := newVerificationTest(t)
	for _, limit := range []struct {
		window time.Duration
		max    int64
	}{
		{0, 10}, {-time.Second, 10}, {time.Microsecond, 10}, {time.Minute, 0}, {time.Minute, -1},
	} {
		cfg := VerificationConfig{TOTPEnabled: true, Issuer: "test", SigningKey: authTestKey, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: time.Minute, TOTPEnrollmentTTL: 5 * time.Minute, SensitiveLifetime: time.Hour, RateLimitWindow: limit.window, RateLimitMaxAttempts: limit.max}
		if _, err := NewVerification(m.store, nil, cfg); err == nil {
			t.Fatal("invalid Verification rate limit accepted", limit)
		}
	}
	cfg := VerificationConfig{TOTPEnabled: true, Issuer: "test", SigningKey: authTestKey, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: time.Minute, TOTPEnrollmentTTL: 5 * time.Minute, SensitiveLifetime: time.Hour, RateLimitWindow: 1500 * time.Millisecond, RateLimitMaxAttempts: 3}
	if _, err := NewVerification(m.store, nil, cfg); err != nil {
		t.Fatal("valid custom Verification rate limit rejected", err)
	}
}

func TestVerificationAttemptsShareOnlyCredentialVersion(t *testing.T) {
	m, _, ctx, identity, state := newVerificationTest(t)
	m.verification.config.RateLimitWindow = 45 * time.Second
	m.verification.config.RateLimitMaxAttempts = 2
	if err := m.verification.attempt(ctx, identity); err != nil {
		t.Fatal(err)
	}
	otherSession := identity
	otherSession.SessionID = "another-session"
	if err := m.verification.attempt(ctx, otherSession); err != nil {
		t.Fatal(err)
	}
	if err := m.verification.attempt(ctx, identity); !errors.Is(err, ErrVerificationRateLimited) {
		t.Fatal("same-version sessions escaped shared limit", err)
	}
	if state.ttls["attempt:"+verificationAttemptKey(identity)] != 45*time.Second {
		t.Fatal("attempt window still uses login ticket lifetime")
	}
	next := identity
	next.CredentialVersion++
	if err := m.verification.attempt(ctx, next); err != nil {
		t.Fatal("new credential version inherited old limit", err)
	}
	if state.attempts[verificationAttemptKey(next)] != 1 {
		t.Fatal("new credential attempt did not begin at one")
	}
	invalid := identity
	invalid.CredentialVersion = 0
	if err := m.verification.attempt(ctx, invalid); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("invalid credential version received a counter", err)
	}
}

func TestSensitiveLifetimeConfiguration(t *testing.T) {
	m, _, _, _, _ := newVerificationTest(t)
	for _, lifetime := range []time.Duration{0, -time.Second, time.Microsecond} {
		cfg := VerificationConfig{TOTPEnabled: true, Issuer: "test", SigningKey: authTestKey, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: time.Minute, TOTPEnrollmentTTL: 5 * time.Minute, SensitiveLifetime: lifetime, RateLimitWindow: 5 * time.Minute, RateLimitMaxAttempts: 10}
		if _, err := NewVerification(m.store, nil, cfg); err == nil {
			t.Fatal("invalid sensitive-operation window accepted", lifetime)
		}
	}
}

// Testing may only spend the shared attempt budget. A grant must not even be
// read, including when a session already has a valid sensitive-operation proof.
type testVerificationAttemptOnlyStore struct {
	*testVerificationStore
	t *testing.T
}

type testVerificationIdentityResolver struct{ identity authn.Identity }

func (r testVerificationIdentityResolver) ValidateIdentity(context.Context, authn.Identity) error {
	return nil
}

func (r testVerificationIdentityResolver) ResolveIdentity(context.Context, authn.Identity) (authn.Identity, error) {
	return r.identity, nil
}

func verificationContextWithIdentity(t *testing.T, ctx context.Context, identity authn.Identity) context.Context {
	t.Helper()
	manager, err := authn.New("verification-context-test", authTestKey, time.Hour, testVerificationIdentityResolver{identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	manager.SetRestrictedHTTPPolicy(func(string, string) bool { return true })
	token, _, err := manager.Issue(authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID: 3, Username: "readonly", Code: "EXP78RGH", SessionID: "fixture-session", CredentialVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/totp/test", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+token)
	request, err := manager.AuthenticateRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	return request.Context()
}

func (s *testVerificationAttemptOnlyStore) Get(context.Context, string) (string, error) {
	s.t.Error("authenticator test read transient Verification state")
	return "", errors.New("unexpected transient state read")
}

func (s *testVerificationAttemptOnlyStore) Put(context.Context, string, string, time.Duration) error {
	s.t.Error("authenticator test wrote transient Verification state")
	return errors.New("unexpected transient state write")
}

func (s *testVerificationAttemptOnlyStore) Take(context.Context, string) (string, error) {
	s.t.Error("authenticator test consumed transient Verification state")
	return "", errors.New("unexpected transient state consume")
}

func (s *testVerificationAttemptOnlyStore) ResetAttempts(context.Context, string) error {
	s.t.Error("authenticator test reset its attempt budget")
	return errors.New("unexpected attempt reset")
}
