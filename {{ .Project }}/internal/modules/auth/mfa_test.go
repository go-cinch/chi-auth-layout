package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

// Test-only store. Production MFA state has no memory fallback.
type testMFAStore struct {
	mu             sync.Mutex
	values         map[string]string
	ttls           map[string]time.Duration
	attempts       map[string]int64
	attemptExpires map[string]time.Time
	resetAttempts  []string
	fail           error
}

func (s *testMFAStore) Put(_ context.Context, key, value string, ttl time.Duration) error {
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
func (s *testMFAStore) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return "", s.fail
	}
	value, ok := s.values[key]
	if !ok {
		return "", ErrMFAInvalid
	}
	return value, nil
}
func (s *testMFAStore) Take(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return "", s.fail
	}
	value, ok := s.values[key]
	if !ok {
		return "", ErrMFAInvalid
	}
	delete(s.values, key)
	return value, nil
}
func (s *testMFAStore) Attempt(_ context.Context, key string, window time.Duration, maxAttempts int64) (bool, error) {
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

func (s *testMFAStore) ResetAttempts(_ context.Context, key string) error {
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

func newMFATest(t *testing.T) (*Module, sqlmock.Sqlmock, context.Context, authn.Identity, *testMFAStore) {
	t.Helper()
	state := &testMFAStore{values: map[string]string{}}
	m, manager, mock := newAuthTestModuleWithMFA(t, func(store *db.Store) *MFA {
		mfa, err := NewMFA(store, state, MFAConfig{Enabled: true, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: 30 * time.Second, EnrollmentTTL: 5 * time.Minute, SensitiveLifetime: time.Hour, RateLimitWindow: 5 * time.Minute, RateLimitMaxAttempts: 10, Issuer: "test", SigningKey: authTestKey})
		if err != nil {
			t.Fatal(err)
		}
		return mfa
	})
	identity := authn.Identity{UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}
	session, err := m.sessions.Issue(context.Background(), identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = session.SessionID
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/mfa", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	request, err := manager.AuthenticateRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	return m, mock, request.Context(), identity, state
}

func expectMFAEnabled(mock sqlmock.Sqlmock, enabled bool) {
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(enabled))
}

func TestConfiguredMFALoginTicketLifetime(t *testing.T) {
	m, _, ctx, identity, state := newMFATest(t)
	m.mfa.config.LoginTicketTTL = 37500 * time.Millisecond
	started := time.Now()
	result, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	if state.ttls["login:"+result.MFATicket] != m.mfa.config.LoginTicketTTL || result.ExpiredAt < started.Add(m.mfa.config.LoginTicketTTL).UnixMilli() || result.ExpiredAt > time.Now().Add(m.mfa.config.LoginTicketTTL).UnixMilli() {
		t.Fatal("ticket deadline or Redis TTL ignored configuration")
	}
	for _, ttl := range []time.Duration{0, -time.Second, time.Nanosecond} {
		cfg := m.mfa.config
		cfg.LoginTicketTTL = ttl
		if _, err := NewMFA(m.store, state, cfg); err == nil {
			t.Fatal("invalid ticket TTL accepted", ttl)
		}
	}
}

func expectMFALock(mock sqlmock.Sqlmock, version int64) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT credential_version, status").WithArgs(int64(3), "EXP78RGH").WillReturnRows(sqlmock.NewRows([]string{"version", "status"}).AddRow(version, 1))
}

func expectMFAFactor(t *testing.T, m *Module, mock sqlmock.Sqlmock, secret string, step int64) {
	t.Helper()
	sealed, err := sealMFA(m.mfa.cipher, secret, "totp:3")
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT secret_ciphertext, last_used_step").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"secret", "step"}).AddRow(sealed, step))
}

type delayedSensitiveStore struct {
	MFAStateStore
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
	return s.MFAStateStore.Put(ctx, key, value, ttl)
}

func TestSensitiveVerificationCannotRestoreLoggedOutGrant(t *testing.T) {
	m, mock, ctx, identity, state := newMFATest(t)
	record, err := m.sessions.read(ctx, identity.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	refreshToken := m.sessions.token(record).RefreshToken
	secret, err := newTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	code, err := totpCode(secret, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	expectMFALock(mock, 1)
	expectMFAFactor(t, m, mock, secret, -1)
	mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	store := &delayedSensitiveStore{MFAStateStore: state, entered: make(chan struct{}), release: make(chan struct{})}
	m.mfa.state = store
	defer func() {
		select {
		case <-store.release:
		default:
			close(store.release)
		}
	}()
	auditCtx, auditState := auditctx.WithState(ctx)
	result := make(chan error, 1)
	go func() { _, err := m.VerifySensitiveMFA(auditCtx, code); result <- err }()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("verification did not reach grant write")
	}
	if err := m.Logout(context.Background(), refreshToken); err != nil {
		t.Fatal(err)
	}
	close(store.release)
	select {
	case err := <-result:
		if !errors.Is(err, ErrUnauthorized) {
			t.Fatal("late verification accepted a revoked session", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late verification did not return")
	}
	if _, err := state.Get(ctx, "sensitive:"+identity.SessionID); !errors.Is(err, ErrMFAInvalid) {
		t.Fatal("late verifier restored sensitive grant", err)
	}
	if event := auditState.Snapshot(); event.MFAFingerprint != "" || event.MFAVerifiedAt != 0 {
		t.Fatal("late verifier authorized sensitive audit metadata")
	}
	if state.attempts[mfaAttemptKey(identity)] != 1 {
		t.Fatal("logout cleared the consumed verification attempt")
	}
	if _, err := m.VerifySensitiveMFA(ctx, code); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("revoked session verified again", err)
	}
}

func TestMFASensitiveProofSessionIsolationAndFixedExpiry(t *testing.T) {
	m, mock, ctx, identity, state := newMFATest(t)
	if _, err := m.mfa.proof(ctx, identity); !errors.Is(err, ErrMFARequired) {
		t.Fatal(err)
	}
	secret, _ := newTOTPSecret()
	code, _ := totpCode(secret, time.Now().Unix()/30)
	expectMFALock(mock, 1)
	expectMFAFactor(t, m, mock, secret, -1)
	mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	auditCtx, auditState := auditctx.WithState(ctx)
	proof, err := m.VerifySensitiveMFA(auditCtx, code)
	if err != nil {
		t.Fatal(err)
	}
	if proof.ExpiredAt-proof.VerifiedAt != time.Hour.Milliseconds() || auditState.Snapshot().MFAFingerprint != proof.Fingerprint {
		t.Fatal("missing proof audit or wrong lifetime")
	}
	before := state.values["sensitive:"+identity.SessionID]
	for range 2 {
		expectMFAEnabled(mock, true)
		if err := m.checkMFATarget(auditCtx, "DELETE", "/user/7"); err != nil {
			t.Fatal(err)
		}
	}
	if state.values["sensitive:"+identity.SessionID] != before {
		t.Fatal("ordinary operation renewed proof")
	}
	other := identity
	other.SessionID = "different"
	if _, err := m.mfa.proof(ctx, other); !errors.Is(err, ErrMFARequired) {
		t.Fatal("proof leaked to another session")
	}
	other = identity
	other.CredentialVersion++
	if _, err := m.mfa.proof(ctx, other); !errors.Is(err, ErrMFARequired) {
		t.Fatal("proof survived credential reset")
	}
	proof.VerifiedAt = time.Now().Add(-2 * time.Hour).UnixMilli()
	proof.ExpiredAt = proof.VerifiedAt + time.Hour.Milliseconds()
	if err := m.mfa.put(ctx, "sensitive:"+identity.SessionID, proof, time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := m.mfa.proof(ctx, identity); !errors.Is(err, ErrMFARequired) {
		t.Fatal("expired proof accepted")
	}
	state.fail = errors.New("redis unavailable")
	if _, err := m.mfa.proof(ctx, identity); !errors.Is(err, state.fail) {
		t.Fatal("Redis error failed open")
	}
}

func TestMFALoginTicketDoesNotGrantSensitiveAccess(t *testing.T) {
	m, mock, ctx, identity, _ := newMFATest(t)
	identity.SessionID = ""
	pending, err := m.beginMFALogin(ctx, identity, false)
	if err != nil || !pending.MFARequired || pending.AccessToken != "" || pending.RefreshToken != "" {
		t.Fatal("login ticket leaked authenticated session")
	}
	secret, _ := newTOTPSecret()
	code, _ := totpCode(secret, time.Now().Unix()/30)
	expectMFALock(mock, 1)
	expectMFAFactor(t, m, mock, secret, -1)
	mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(2))
	result, err := m.CompleteMFALogin(ctx, pending.MFATicket, code)
	if err != nil || result.AccessToken == "" {
		t.Fatal("MFA login failed", err)
	}
	authenticated, err := m.authenticator.Parse(result.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.mfa.proof(ctx, authenticated); !errors.Is(err, ErrMFARequired) {
		t.Fatal("login created a sensitive grant")
	}
	if _, err := m.CompleteMFALogin(ctx, pending.MFATicket, code); !errors.Is(err, ErrMFAInvalid) {
		t.Fatal("consumed ticket accepted")
	}
	if _, err := m.CompleteMFALogin(ctx, "bad", code); !errors.Is(err, ErrMFAInvalid) {
		t.Fatal("short ticket accepted")
	}
}

func TestMFAAuthenticatorOnlyAndInvalidation(t *testing.T) {
	m, mock, ctx, identity, state := newMFATest(t)
	secret, _ := newTOTPSecret()
	for _, code := range []string{"recovery-code", "12a456", "12345", "1234567"} {
		expectMFALock(mock, 1)
		expectMFAFactor(t, m, mock, secret, -1)
		mock.ExpectRollback()
		if err := m.mfa.consume(ctx, identity, code); !errors.Is(err, ErrMFAInvalid) {
			t.Fatal("non-TOTP code accepted", err)
		}
	}
	expectMFALock(mock, 2)
	mock.ExpectRollback()
	if err := m.mfa.consume(ctx, identity, "123456"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale identity accepted", err)
	}
	expectMFALock(mock, 1)
	mock.ExpectQuery("SELECT secret_ciphertext").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	if err := m.mfa.consume(ctx, identity, "123456"); !errors.Is(err, ErrMFAEnrollmentRequired) {
		t.Fatal(err)
	}
	state.attempts = map[string]int64{"3:1": 10}
	if err := m.mfa.consume(ctx, identity, "123456"); !errors.Is(err, ErrMFARateLimited) {
		t.Fatal("unbounded guesses", err)
	}
}

func TestMFAEnrollmentConfirmsAndRotatesCredentials(t *testing.T) {
	m, mock, ctx, identity, state := newMFATest(t)
	m.mfa.config.AccountNamePrefix = "acme admin"
	m.mfa.config.TOTPPeriod = time.Minute
	m.mfa.config.EnrollmentTTL = 90 * time.Second
	started := time.Now()
	mock.ExpectQuery("SELECT password FROM").WithArgs(int64(3), int64(1)).WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t, "pass")))
	expectMFAEnabled(mock, false)
	setup, err := m.BeginMFAEnrollment(ctx, "pass")
	if err != nil {
		t.Fatal(err)
	}
	if setup.URI == "" || setup.Secret == "" {
		t.Fatal("missing enrollment material")
	}
	uri, err := url.Parse(setup.URI)
	if err != nil || uri.Path != "/test:acme admin - readonly" || uri.Query().Get("issuer") != "test" || uri.Query().Get("secret") != setup.Secret {
		t.Fatal("enrollment did not use the configured account name prefix")
	}
	if uri.Query().Get("period") != "60" || state.ttls["enrollment:"+identity.SessionID] != 90*time.Second || setup.ExpiredAt < started.Add(90*time.Second).UnixMilli() || setup.ExpiredAt > time.Now().Add(90*time.Second).UnixMilli() {
		t.Fatal("QR parameters, response expiry and storage TTL do not match configuration")
	}
	// A malformed code cannot commit a half-completed enrollment.
	if _, err := m.ConfirmMFAEnrollment(ctx, "invalid"); !errors.Is(err, ErrMFAInvalid) {
		t.Fatal(err)
	}
	code, _ := totpCode(setup.Secret, time.Now().Unix()/60)
	expectMFALock(mock, 1)
	expectMFAEnabled(mock, false)
	mock.ExpectExec("INSERT INTO t_user_totp").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM t_user_recovery_code").WillReturnResult(sqlmock.NewResult(0, 0))
	for range 10 {
		mock.ExpectExec("INSERT INTO t_user_recovery_code").WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectExec("UPDATE t_user SET credential_version").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	result, err := m.ConfirmMFAEnrollment(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RecoveryCodes) != 10 || result.MFAEnrollmentRequired {
		t.Fatal("incomplete enrollment result")
	}
	next, err := m.authenticator.Parse(result.AccessToken)
	if err != nil || next.CredentialVersion != 2 || next.SessionID == identity.SessionID {
		t.Fatal("binding did not replace session")
	}
	if _, err := m.mfa.proof(ctx, next); !errors.Is(err, ErrMFARequired) {
		t.Fatal("binding granted sensitive access")
	}
}

func TestMFAEnrollmentPolicyAndUnavailableStore(t *testing.T) {
	m, mock, ctx, identity, _ := newMFATest(t)
	expectMFAEnabled(mock, false)
	if err := m.checkMFATarget(ctx, "GET", "/info"); !errors.Is(err, ErrMFAEnrollmentRequired) {
		t.Fatal("mandatory setup bypass", err)
	}
	expectMFAEnabled(mock, false)
	if err := m.checkMFATarget(ctx, "POST", "/mfa/setup"); err != nil {
		t.Fatal("setup blocked", err)
	}
	expectMFAEnabled(mock, false)
	if _, err := m.MFAStatus(ctx); err != nil {
		t.Fatal(err)
	}
	expectMFAEnabled(mock, false)
	if err := m.checkMFATarget(ctx, "DELETE", "/msg/inbox/1"); !errors.Is(err, ErrMFAEnrollmentRequired) {
		t.Fatal("unbound deletion allowed", err)
	}
	expectMFAEnabled(mock, false)
	if err := m.checkMFATarget(ctx, "GET", "/dictionary"); !errors.Is(err, ErrMFAEnrollmentRequired) {
		t.Fatal(err)
	}
	m.mfa.state = nil
	if err := m.mfa.put(ctx, "a", identity, time.Minute); !errors.Is(err, ErrMFAUnavailable) {
		t.Fatal(err)
	}
	if _, err := m.mfa.proof(ctx, identity); !errors.Is(err, ErrMFAUnavailable) {
		t.Fatal(err)
	}
	if err := m.mfa.attempt(ctx, authn.Identity{UserID: 3, CredentialVersion: 1}); !errors.Is(err, ErrMFAUnavailable) {
		t.Fatal(err)
	}
}

func TestMFAConfiguration(t *testing.T) {
	m, _, _, _, _ := newMFATest(t)
	if _, err := NewMFA(nil, nil, MFAConfig{Enabled: true}); err == nil {
		t.Fatal("invalid config accepted")
	}
	file := filepath.Join(t.TempDir(), "key")
	cfg := MFAConfig{Enabled: true, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: 30 * time.Second, EnrollmentTTL: 5 * time.Minute, SensitiveLifetime: time.Hour, RateLimitWindow: 5 * time.Minute, RateLimitMaxAttempts: 10, Issuer: "test", SigningKey: authTestKey, EncryptionKeyFile: file}
	if _, err := NewMFA(m.store, nil, cfg); err == nil {
		t.Fatal("missing key accepted")
	}
	if err := os.WriteFile(file, []byte("bad!"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMFA(m.store, nil, cfg); err == nil {
		t.Fatal("invalid key accepted")
	}
	if err := os.WriteFile(file, []byte(strings.Repeat("A", 43)+"="), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMFA(m.store, nil, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestMFALoginEnrollmentGrantIsolation(t *testing.T) {
	m, mock, ctx, identity, state := newMFATest(t)
	expectPassword := func() {
		mock.ExpectQuery("SELECT password FROM").WithArgs(identity.UserID, identity.CredentialVersion).WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t, "pass")))
	}
	expectPassword()
	if _, err := m.BeginMFAEnrollment(ctx, ""); !errors.Is(err, ErrIncorrectPassword) {
		t.Fatal("ordinary session skipped password", err)
	}
	if err := m.grantMFAEnrollment(ctx, identity, identity.SessionID); err != nil {
		t.Fatal(err)
	}
	expectMFAEnabled(mock, false)
	status, err := m.MFAStatus(ctx)
	if err != nil || status.PasswordRequired {
		t.Fatal("login session still requires password", err)
	}
	expectPassword()
	expectMFAEnabled(mock, false)
	if _, err := m.BeginMFAEnrollment(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"other session", "other user", "old version", "disabled policy"} {
		t.Run(scenario, func(t *testing.T) {
			other := identity
			switch scenario {
			case "other session":
				other.SessionID = "another"
			case "other user":
				other.UserID++
			case "old version":
				other.CredentialVersion++
			case "disabled policy":
				m.mfa.config.Enabled = false
			}
			allowed, err := m.canEnrollWithoutPassword(ctx, other)
			m.mfa.config.Enabled = true
			if err != nil || allowed {
				t.Fatal("grant escaped its login session", err)
			}
		})
	}
	// Explicit passwords are always checked, including personal-center requests.
	expectPassword()
	if _, err := m.BeginMFAEnrollment(ctx, "wrong"); !errors.Is(err, ErrIncorrectPassword) {
		t.Fatal(err)
	}
	grant := mfaPending{Identity: identity, ExpiredAt: time.Now().Add(-time.Second).UnixMilli()}
	if err := m.mfa.put(ctx, "enrollment-grant:"+identity.SessionID, grant, time.Minute); err != nil {
		t.Fatal(err)
	}
	if allowed, err := m.canEnrollWithoutPassword(ctx, identity); err != nil || allowed {
		t.Fatal("expired grant accepted", err)
	}
	state.fail = errors.New("redis unavailable")
	if allowed, err := m.canEnrollWithoutPassword(ctx, identity); err == nil || allowed {
		t.Fatal("store failure bypassed password")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMFARejectsInvalidTimingConfiguration(t *testing.T) {
	m, _, _, _, _ := newMFATest(t)
	for _, timing := range []struct{ period, ttl time.Duration }{
		{0, time.Minute}, {-time.Second, time.Minute}, {1500 * time.Millisecond, time.Minute},
		{30 * time.Second, 0}, {30 * time.Second, -time.Minute}, {30 * time.Second, time.Microsecond},
	} {
		_, err := NewMFA(m.store, nil, MFAConfig{Enabled: true, Issuer: "test", SigningKey: authTestKey, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: timing.period, EnrollmentTTL: timing.ttl, SensitiveLifetime: time.Hour, RateLimitWindow: 5 * time.Minute, RateLimitMaxAttempts: 10})
		if err == nil {
			t.Fatal("invalid MFA timing configuration accepted", timing)
		}
	}
}

func TestMFADisabledSkipsEnforcement(t *testing.T) {
	m, mock, ctx, identity, _ := newMFATest(t)
	m.mfa.config.Enabled = false
	// No database/Redis access, even when the account has a stored factor.
	if required, err := m.mfaEnrollmentRequired(ctx, identity); err != nil || required {
		t.Fatal(required, err)
	}
	if err := m.checkMFATarget(ctx, "DELETE", "/user/1"); err != nil {
		t.Fatal(err)
	}
	status, err := m.MFAStatus(ctx)
	if err != nil || status.Enabled || status.EnrollmentRequired {
		t.Fatal(status, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMFARateLimitConfiguration(t *testing.T) {
	m, _, _, _, _ := newMFATest(t)
	for _, limit := range []struct {
		window time.Duration
		max    int64
	}{
		{0, 10}, {-time.Second, 10}, {time.Microsecond, 10}, {time.Minute, 0}, {time.Minute, -1},
	} {
		cfg := MFAConfig{Enabled: true, Issuer: "test", SigningKey: authTestKey, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: time.Minute, EnrollmentTTL: 5 * time.Minute, SensitiveLifetime: time.Hour, RateLimitWindow: limit.window, RateLimitMaxAttempts: limit.max}
		if _, err := NewMFA(m.store, nil, cfg); err == nil {
			t.Fatal("invalid MFA rate limit accepted", limit)
		}
	}
	cfg := MFAConfig{Enabled: true, Issuer: "test", SigningKey: authTestKey, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: time.Minute, EnrollmentTTL: 5 * time.Minute, SensitiveLifetime: time.Hour, RateLimitWindow: 1500 * time.Millisecond, RateLimitMaxAttempts: 3}
	if _, err := NewMFA(m.store, nil, cfg); err != nil {
		t.Fatal("valid custom MFA rate limit rejected", err)
	}
}

func TestMFAAttemptsShareOnlyCredentialVersion(t *testing.T) {
	m, _, ctx, identity, state := newMFATest(t)
	m.mfa.config.RateLimitWindow = 45 * time.Second
	m.mfa.config.RateLimitMaxAttempts = 2
	if err := m.mfa.attempt(ctx, identity); err != nil {
		t.Fatal(err)
	}
	otherSession := identity
	otherSession.SessionID = "another-session"
	if err := m.mfa.attempt(ctx, otherSession); err != nil {
		t.Fatal(err)
	}
	if err := m.mfa.attempt(ctx, identity); !errors.Is(err, ErrMFARateLimited) {
		t.Fatal("same-version sessions escaped shared limit", err)
	}
	if state.ttls["attempt:"+mfaAttemptKey(identity)] != 45*time.Second {
		t.Fatal("attempt window still uses login ticket lifetime")
	}
	next := identity
	next.CredentialVersion++
	if err := m.mfa.attempt(ctx, next); err != nil {
		t.Fatal("new credential version inherited old limit", err)
	}
	if state.attempts[mfaAttemptKey(next)] != 1 {
		t.Fatal("new credential attempt did not begin at one")
	}
	invalid := identity
	invalid.CredentialVersion = 0
	if err := m.mfa.attempt(ctx, invalid); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("invalid credential version received a counter", err)
	}
}

func TestMFASensitiveLifetimeConfiguration(t *testing.T) {
	m, _, _, _, _ := newMFATest(t)
	for _, lifetime := range []time.Duration{0, -time.Second, time.Microsecond} {
		cfg := MFAConfig{Enabled: true, Issuer: "test", SigningKey: authTestKey, LoginTicketTTL: 5 * time.Minute, TOTPPeriod: time.Minute, EnrollmentTTL: 5 * time.Minute, SensitiveLifetime: lifetime, RateLimitWindow: 5 * time.Minute, RateLimitMaxAttempts: 10}
		if _, err := NewMFA(m.store, nil, cfg); err == nil {
			t.Fatal("invalid sensitive-operation window accepted", lifetime)
		}
	}
}

func TestMFASensitiveCustomLifetimeAndRefresh(t *testing.T) {
	m, mock, ctx, identity, state := newMFATest(t)
	const lifetime = 90*time.Second + 250*time.Millisecond
	m.mfa.config.SensitiveLifetime = lifetime
	secret, _ := newTOTPSecret()
	code, _ := totpCode(secret, time.Now().Unix()/30)
	expectMFALock(mock, 1)
	expectMFAFactor(t, m, mock, secret, -1)
	mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	proof, err := m.VerifySensitiveMFA(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	key := "sensitive:" + identity.SessionID
	if proof.ExpiredAt-proof.VerifiedAt != lifetime.Milliseconds() || state.ttls[key] != lifetime {
		t.Fatal("custom proof deadline and Redis TTL diverged")
	}
	if _, err := m.mfa.proof(ctx, identity); err != nil {
		t.Fatal("custom proof lifetime rejected", err)
	}
	before := state.values[key]
	session, err := m.sessions.read(ctx, identity.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count").WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow(identity.Username, identity.Code, userStatusActive, identity.CredentialVersion, int64(1)))
	expectMFAEnabled(mock, true)
	if _, err := m.Refresh(ctx, m.sessions.token(session).RefreshToken); err != nil {
		t.Fatal(err)
	}
	if state.values[key] != before || state.ttls[key] != lifetime {
		t.Fatal("token refresh extended the sensitive window")
	}
	for _, boundary := range []string{"expired", "future", "old_default", "shortened"} {
		invalid := *proof
		switch boundary {
		case "expired":
			invalid.ExpiredAt = time.Now().UnixMilli()
			invalid.VerifiedAt = invalid.ExpiredAt - lifetime.Milliseconds()
		case "future":
			invalid.VerifiedAt = time.Now().Add(time.Second).UnixMilli()
			invalid.ExpiredAt = invalid.VerifiedAt + lifetime.Milliseconds()
		case "old_default":
			invalid.ExpiredAt = invalid.VerifiedAt + time.Hour.Milliseconds()
		case "shortened":
			invalid.ExpiredAt--
		}
		if err := m.mfa.put(ctx, key, &invalid, lifetime); err != nil {
			t.Fatal(err)
		}
		if _, err := m.mfa.proof(ctx, identity); !errors.Is(err, ErrMFARequired) {
			t.Fatal("invalid configured-window proof accepted", boundary, err)
		}
	}
	if err := m.mfa.put(ctx, key, proof, lifetime); err != nil {
		t.Fatal(err)
	}
	m.mfa.config.SensitiveLifetime = 2 * time.Second
	if _, err := m.mfa.proof(ctx, identity); !errors.Is(err, ErrMFARequired) {
		t.Fatal("old proof accepted after window configuration changed", err)
	}
}

// Testing may only spend the shared attempt budget. A grant must not even be
// read, including when a session already has a valid sensitive-operation proof.
type testMFAAttemptOnlyStore struct {
	*testMFAStore
	t *testing.T
}

type testMFAIdentityResolver struct{ identity authn.Identity }

func (r testMFAIdentityResolver) ValidateIdentity(context.Context, authn.Identity) error { return nil }

func (r testMFAIdentityResolver) ResolveIdentity(context.Context, authn.Identity) (authn.Identity, error) {
	return r.identity, nil
}

func mfaContextWithIdentity(t *testing.T, ctx context.Context, identity authn.Identity) context.Context {
	t.Helper()
	manager, err := authn.New("mfa-context-test", authTestKey, time.Hour, testMFAIdentityResolver{identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	manager.SetRestrictedHTTPPolicy(func(string, string) bool { return true })
	token, _, err := manager.Issue(authn.Identity{UserID: 3, Username: "readonly", Code: "EXP78RGH", SessionID: "fixture-session", CredentialVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/mfa/test", nil).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+token)
	request, err := manager.AuthenticateRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	return request.Context()
}

func (s *testMFAAttemptOnlyStore) Get(context.Context, string) (string, error) {
	s.t.Error("authenticator test read transient MFA state")
	return "", errors.New("unexpected transient state read")
}

func (s *testMFAAttemptOnlyStore) Put(context.Context, string, string, time.Duration) error {
	s.t.Error("authenticator test wrote transient MFA state")
	return errors.New("unexpected transient state write")
}

func (s *testMFAAttemptOnlyStore) Take(context.Context, string) (string, error) {
	s.t.Error("authenticator test consumed transient MFA state")
	return "", errors.New("unexpected transient state consume")
}

func (s *testMFAAttemptOnlyStore) ResetAttempts(context.Context, string) error {
	s.t.Error("authenticator test reset its attempt budget")
	return errors.New("unexpected attempt reset")
}

func TestMFATestPreservesSensitiveGrantAndAudit(t *testing.T) {
	for _, granted := range []bool{false, true} {
		t.Run(map[bool]string{false: "without grant", true: "existing grant"}[granted], func(t *testing.T) {
			m, mock, ctx, identity, state := newMFATest(t)
			key := "sensitive:" + identity.SessionID
			if granted {
				now := time.Now().Add(-10 * time.Minute)
				proof := &MFAProof{UserID: identity.UserID, SessionID: identity.SessionID, CredentialVersion: identity.CredentialVersion, Fingerprint: "existing-verification", VerifiedAt: now.UnixMilli(), ExpiredAt: now.Add(time.Hour).UnixMilli()}
				if err := m.mfa.put(ctx, key, proof, 50*time.Minute); err != nil {
					t.Fatal(err)
				}
			}
			before, ttl := state.values[key], state.ttls[key]
			m.mfa.state = &testMFAAttemptOnlyStore{testMFAStore: state, t: t}
			auditCtx, auditState := auditctx.WithState(ctx)
			secret, _ := newTOTPSecret()
			step := time.Now().Unix() / 30
			code, _ := totpCode(secret, step)
			expectMFALock(mock, 1)
			expectMFAFactor(t, m, mock, secret, -1)
			mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WithArgs(step, sqlmock.AnyArg(), identity.UserID).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			result, err := m.TestMFA(auditCtx, " "+code+" ")
			if err != nil || result == nil || !result.Verified {
				t.Fatal("authenticator test failed", err)
			}
			if state.attempts[mfaAttemptKey(identity)] != 1 {
				t.Fatal("successful test did not spend an attempt")
			}
			// A failed replay must spend another attempt, preserving the same grant.
			expectMFALock(mock, 1)
			expectMFAFactor(t, m, mock, secret, step)
			mock.ExpectRollback()
			if result, err := m.TestMFA(auditCtx, code); result != nil || !errors.Is(err, ErrMFAInvalid) {
				t.Fatal("tested code reused", err)
			}
			if state.attempts[mfaAttemptKey(identity)] != 2 {
				t.Fatal("failed test did not spend an attempt")
			}
			if state.values[key] != before || state.ttls[key] != ttl {
				t.Fatal("test changed or renewed the sensitive grant")
			}
			if event := auditState.Snapshot(); event.MFAFingerprint != "" || event.MFAVerifiedAt != 0 {
				t.Fatal("test created sensitive audit metadata")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMFATestRejectsUnauthorizedAndUnavailableState(t *testing.T) {
	m, mock, ctx, identity, state := newMFATest(t)
	for _, testCtx := range []context.Context{
		context.Background(),
		mfaContextWithIdentity(t, ctx, authn.Identity{UserID: identity.UserID, CredentialVersion: identity.CredentialVersion}),
	} {
		if _, err := m.TestMFA(testCtx, "123456"); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("unauthenticated test accepted", err)
		}
	}
	restricted := identity
	restricted.PasswordResetRequired = true
	if _, err := m.TestMFA(mfaContextWithIdentity(t, ctx, restricted), "123456"); !errors.Is(err, authn.ErrPasswordResetRequired) {
		t.Fatal("password reset gate bypassed", err)
	}
	if len(state.attempts) != 0 {
		t.Fatal("rejected identity spent an attempt")
	}
	m.mfa.config.Enabled = false
	if _, err := m.TestMFA(ctx, "123456"); !errors.Is(err, ErrMFAUnavailable) {
		t.Fatal("disabled MFA accepted", err)
	}
	m.mfa.config.Enabled = true
	m.mfa.state = nil
	if _, err := m.TestMFA(ctx, "123456"); !errors.Is(err, ErrMFAUnavailable) {
		t.Fatal("missing state failed open", err)
	}
	m.mfa.state = state
	state.fail = errors.New("redis unavailable")
	if _, err := m.TestMFA(ctx, "123456"); !errors.Is(err, state.fail) {
		t.Fatal("failed attempt store failed open", err)
	}
	state.fail = nil
	expectMFALock(mock, 2)
	mock.ExpectRollback()
	if _, err := m.TestMFA(ctx, "123456"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale identity accepted", err)
	}
	expectMFALock(mock, 1)
	mock.ExpectQuery("SELECT secret_ciphertext").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	if _, err := m.TestMFA(ctx, "123456"); !errors.Is(err, ErrMFAEnrollmentRequired) {
		t.Fatal("unbound account accepted", err)
	}
	if state.attempts[mfaAttemptKey(identity)] != 2 {
		t.Fatal("validated failures bypassed shared attempt budget")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMFATestSharesRateLimitAcrossSessionsAndFlows(t *testing.T) {
	m, mock, ctx, identity, state := newMFATest(t)
	m.mfa.config.RateLimitMaxAttempts = 2
	secret, _ := newTOTPSecret()
	for _, code := range []string{"12345", "recovery-code"} {
		expectMFALock(mock, 1)
		expectMFAFactor(t, m, mock, secret, -1)
		mock.ExpectRollback()
		if _, err := m.TestMFA(ctx, code); !errors.Is(err, ErrMFAInvalid) {
			t.Fatal("non-TOTP test accepted", err)
		}
	}
	key := mfaAttemptKey(identity)
	deadline := state.attemptExpires[key]
	other := identity
	other.SessionID = "another-session"
	if _, err := m.TestMFA(mfaContextWithIdentity(t, ctx, other), "123456"); !errors.Is(err, ErrMFARateLimited) {
		t.Fatal("other session escaped test limit", err)
	}
	if _, err := m.VerifySensitiveMFA(ctx, "123456"); !errors.Is(err, ErrMFARateLimited) {
		t.Fatal("sensitive verification escaped test limit", err)
	}
	if _, err := m.BeginMFAEnrollment(ctx, "password"); !errors.Is(err, ErrMFARateLimited) {
		t.Fatal("enrollment escaped test limit", err)
	}
	pending, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CompleteMFALogin(ctx, pending.MFATicket, "123456"); !errors.Is(err, ErrMFARateLimited) {
		t.Fatal("login escaped test limit", err)
	}
	if _, err := m.RecoverMFALogin(ctx, pending.MFATicket, "recovery-code"); !errors.Is(err, ErrMFARateLimited) {
		t.Fatal("recovery escaped test limit", err)
	}
	if state.attempts[key] != 7 || state.attemptExpires[key] != deadline {
		t.Fatal("attempts or fixed-window deadline changed incorrectly")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
