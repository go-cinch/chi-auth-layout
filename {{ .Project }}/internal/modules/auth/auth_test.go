package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

const authTestKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// Keep the branded fixture below bcrypt's 72-byte limit, even for long names.
const authTestPassword = "{{ printf "%.64s" (fromJSON .Computed.brand).slug }}123"

func TestNewRequiresExplicitVerification(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("missing Verification dependency silently disabled enforcement")
		}
	}()
	New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func newAuthTestModule(t *testing.T) (*Module, *authn.Manager, sqlmock.Sqlmock) {
	t.Helper()
	return newAuthTestModuleWithVerification(t, func(*db.Store) *Verification { return &Verification{} })
}

func newAuthTestModuleWithVerification(t *testing.T, createVerification func(*db.Store) *Verification) (*Module, *authn.Manager, sqlmock.Sqlmock) {
	localizer := localization.New()
	installAuthCatalog(t, localizer)
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = database.Close()
	})
	memorySessions := NewMemorySessionStore()
	t.Cleanup(func() { _ = memorySessions.Close() })
	sessions, sessionErr := NewSessions(memorySessions, SessionConfig{
		Issuer:           "test",
		Key:              authTestKey,
		SessionLifetime:  time.Hour,
		RememberLifetime: 30 * 24 * time.Hour,
		RetryWindow:      10 * time.Second,
	})
	if sessionErr != nil {
		t.Fatal(sessionErr)
	}
	authenticator, err := authn.New("test", authTestKey, time.Hour, sessions)
	if err != nil {
		t.Fatal(err)
	}
	credentials := newCredentialTestManager(t, time.Minute)
	captcha, captchaErr := NewPointCaptcha(NewMemoryPointCaptchaStore(), newPointCaptchaTestDictionary(), 5, 2*time.Minute, 300, 180, 22, false)
	if captchaErr != nil {
		t.Fatal(captchaErr)
	}
	passwordGuard, guardErr := NewPasswordChangeGuard(NewMemoryPasswordFailureStore(), captcha, 3, 6, 10*time.Minute)
	if guardErr != nil {
		t.Fatal(guardErr)
	}
	slider, sliderErr := NewSliderCaptcha(NewMemoryPointCaptchaStore(), SliderCaptchaConfig{TTL: time.Minute, MinimumDuration: 100 * time.Millisecond})
	if sliderErr != nil {
		t.Fatal(sliderErr)
	}
	store := &db.Store{DB: database}
	return New(store, authenticator, credentials, sessions, captcha, slider, passwordGuard, localizer, defaultPermissionsStub{}, createVerification(store)), authenticator, mock
}

func passwordHash(t *testing.T, password string) string {
	t.Helper()
	value, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(value)
}

func loginRows(t *testing.T, status int16, wrongValues ...int64) *sqlmock.Rows {
	t.Helper()
	wrong := int64(0)
	if len(wrongValues) == 1 {
		wrong = wrongValues[0]
	}
	return sqlmock.NewRows([]string{"id", "username", "code", "password", "status", "wrong", "credential_version"}).
		AddRow(int64(3), "readonly", "EXP78RGH", passwordHash(t, authTestPassword), status, wrong, int64(1))
}

func expectPasswordChangeLock(mock sqlmock.Sqlmock, locked bool) {
	mock.ExpectQuery("SELECT CASE").WithArgs(int64(3), "EXP78RGH", int64(6)).WillReturnRows(
		sqlmock.NewRows([]string{"locked"}).AddRow(locked),
	)
}

func authenticatedContext(t *testing.T, authenticator *authn.Manager, sessions *Sessions) context.Context {
	t.Helper()
	return authenticatedIdentityContext(t, authenticator, sessions, authn.Identity{Methods: authn.Methods{Primary: authn.Password}, CredentialVersion: 1, UserID: 3, Username: "readonly", Code: "EXP78RGH"})
}

func authenticatedIdentityContext(t *testing.T, authenticator *authn.Manager, sessions *Sessions, identity authn.Identity) context.Context {
	t.Helper()
	session, err := sessions.Issue(t.Context(), identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = session.SessionID
	token, _, err := authenticator.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/info", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request, err = authenticator.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	return request.Context()
}

func TestLogin(t *testing.T) {
	m, authenticator, mock := newAuthTestModule(t)
	ctx := context.Background()
	if _, err := m.LoginVerification(ctx, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty login verification: %v", err)
	}
	mock.ExpectQuery("SELECT wrong FROM t_user").WithArgs("missing").WillReturnError(sql.ErrNoRows)
	verification, err := m.LoginVerification(ctx, "missing")
	if err != nil || verification.CaptchaRequired || verification.Captcha != nil {
		t.Fatalf("missing login verification: %#v %v", verification, err)
	}
	mock.ExpectQuery("SELECT wrong FROM t_user").WithArgs("readonly").WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(int64(4)))
	verification, err = m.LoginVerification(ctx, "readonly")
	if err != nil || verification.CaptchaRequired || verification.Captcha != nil {
		t.Fatalf("slider login verification: %#v %v", verification, err)
	}
	mock.ExpectQuery("SELECT wrong FROM t_user").WithArgs("readonly").WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(int64(5)))
	verification, err = m.LoginVerification(ctx, " readonly ")
	if err != nil || !verification.CaptchaRequired || verification.Captcha == nil || verification.Captcha.CaptchaID == "" {
		t.Fatalf("point login verification: %#v %v", verification, err)
	}
	if _, err := m.Login(ctx, LoginInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("missing").WillReturnError(sql.ErrNoRows)
	if _, err := m.Login(ctx, LoginInput{Username: "missing", Password: "secret"}); !errors.Is(err, ErrLoginFailed) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("broken").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Login(ctx, LoginInput{Username: "broken", Password: "secret"}); err == nil || errors.Is(err, ErrLoginFailed) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET wrong").WithArgs(sqlmock.AnyArg(), int64(3)).WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(int64(1)))
	if _, err := m.Login(ctx, LoginInput{Username: "readonly", Password: "wrong", SliderProof: validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")}); !errors.Is(err, ErrLoginFailed) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive, 5))
	if _, err := m.Login(ctx, LoginInput{Username: "readonly", Password: authTestPassword}); !errors.Is(err, ErrPointCaptchaRequired) {
		t.Fatalf("captcha requirement: %v", err)
	}
	answer := storedPointCaptchaAnswer(t, m.captcha.store.(*memoryPointCaptchaStore), verification.Captcha.CaptchaID)
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive, 5))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(int64(2)))
	if result, err := m.Login(ctx, LoginInput{
		Username: "readonly", Password: authTestPassword, CaptchaID: verification.Captcha.CaptchaID, CaptchaPoints: answer.Points,
	}); err != nil || result.AccessToken == "" {
		t.Fatalf("point captcha without slider: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusPending))
	if _, err := m.Login(ctx, LoginInput{Username: "readonly", Password: authTestPassword}); !errors.Is(err, ErrPendingApproval) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusLocked))
	mock.ExpectExec("UPDATE t_user SET status").WithArgs(userStatusActive, sqlmock.AnyArg(), int64(3), userStatusLocked, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	if _, err := m.Login(ctx, LoginInput{Username: "readonly", Password: authTestPassword}); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusLocked))
	mock.ExpectExec("UPDATE t_user SET status").WithArgs(userStatusActive, sqlmock.AnyArg(), int64(3), userStatusLocked, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(int64(2)))
	if result, err := m.Login(ctx, LoginInput{Username: "readonly", Password: authTestPassword, SliderProof: validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")}); err != nil || result.AccessToken == "" {
		t.Fatalf("expired lock login: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Login(ctx, LoginInput{Username: " readonly ", Password: authTestPassword, SliderProof: validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")}); err == nil {
		t.Fatal("update failure was ignored")
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(int64(2)))
	result, err := m.Login(ctx, LoginInput{Username: "readonly", Password: authTestPassword, SliderProof: validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")})
	if err != nil || result.AccessToken == "" || result.ExpiredAt <= time.Now().UnixMilli() {
		t.Fatalf("login: %#v %v", result, err)
	}
	identity, err := authenticator.Parse(result.AccessToken)
	if err != nil || identity.UserID != 3 || identity.Username != "readonly" || identity.Methods != (authn.Methods{Primary: authn.Password}) {
		t.Fatalf("identity: %#v %v", identity, err)
	}
}

func TestCurrentUser(t *testing.T) {
	m, authenticator, mock := newAuthTestModule(t)
	if _, err := m.Info(context.Background(), "admin"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	ctx := authenticatedContext(t, authenticator, m.sessions)
	mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").WillReturnError(sql.ErrNoRows)
	if _, err := m.Info(ctx, "admin"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Info(ctx, "admin"); err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code", "role_id", "role_name", "role_word"}).
			AddRow(3, "readonly", "EXP78RGH", 2, "Reader", "reader"))
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:read:system:user", userReadPermissionConfig))
	value, err := m.Info(ctx, "admin")
	if err != nil || value.Username != "readonly" || value.Role == nil || value.Role.Name != "Reader" || value.Role.Word != "reader" || len(value.Permission.Buttons) != 1 || value.Permission.Buttons[0] != "admin:btn:system:user:query" {
		t.Fatalf("info: %#v %v", value, err)
	}
	session, err := m.sessions.Issue(ctx, authn.Identity{Methods: authn.Methods{Primary: authn.Password}, CredentialVersion: 1, UserID: 3, Username: "readonly", Code: "EXP78RGH"}, false)
	if err != nil {
		t.Fatal(err)
	}
	auditCtx, state := auditctx.WithState(context.Background())
	if err := m.Logout(auditCtx, session.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if event := state.Snapshot(); event.UserID != 3 || event.Username != "readonly" {
		t.Fatal("logout did not attribute the verified session owner")
	}
	if err := m.sessions.ValidateSession(ctx, session.SessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("session remained valid: %v", err)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count FROM t_user").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusLocked, int64(1), int64(1)))
	mock.ExpectExec("UPDATE t_user SET status").WithArgs(userStatusActive, sqlmock.AnyArg(), int64(3), userStatusLocked, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	if identity, err := m.refreshIdentity(ctx, authn.Identity{Methods: authn.Methods{Primary: authn.Password}, CredentialVersion: 1, UserID: 3, Username: "readonly", Code: "EXP78RGH"}); err != nil || identity.Username != "readonly" {
		t.Fatalf("refresh expired lock: %#v %v", identity, err)
	}
	if err := m.Logout(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestLogoutClearsOnlyItsSensitiveGrant(t *testing.T) {
	for _, mode := range []string{"current", "rotated", "corrupt", "verification-disabled"} {
		t.Run(mode, func(t *testing.T) {
			m, _, ctx, identity, state := newVerificationTest(t)
			record, err := m.sessions.read(ctx, identity.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			token := m.sessions.token(record).RefreshToken
			other, err := m.sessions.Issue(ctx, identity, false)
			if err != nil {
				t.Fatal(err)
			}
			key := "sensitive:" + identity.SessionID
			otherKey := "sensitive:" + other.SessionID
			state.values[key], state.values[otherKey] = "current grant", "other grant"
			if _, err := state.Attempt(ctx, verificationAttemptKey(identity), time.Minute, 10); err != nil {
				t.Fatal(err)
			}
			deadline := state.attemptExpires[verificationAttemptKey(identity)]
			switch mode {
			case "rotated":
				refresh, err := m.sessions.ReadRefresh(ctx, token)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.sessions.Renew(ctx, refresh, identity); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if changed, err := m.sessions.store.Rotate(ctx, m.sessions.storageID(identity.SessionID), record.raw, "corrupt", time.UnixMilli(record.ExpiredAt)); err != nil || !changed {
					t.Fatal("could not prepare corrupt record", err)
				}
			case "verification-disabled":
				m.verification.config.TOTPEnabled = false
			}
			if err := m.Logout(ctx, token+"!"); err != nil {
				t.Fatal(err)
			}
			if state.values[key] != "current grant" {
				t.Fatal("unverified token cleared a grant")
			}
			for range 2 {
				if err := m.Logout(context.Background(), token); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := m.sessions.read(ctx, identity.SessionID); !errors.Is(err, ErrSessionNotFound) {
				t.Fatal("logout left the session active", err)
			}
			if _, exists := state.values[key]; exists {
				t.Fatal("logout left the sensitive grant")
			}
			if state.values[otherKey] != "other grant" {
				t.Fatal("logout cleared another session's grant")
			}
			if err := m.sessions.ValidateSession(ctx, other.SessionID); err != nil {
				t.Fatal("logout revoked another session", err)
			}
			if state.attempts[verificationAttemptKey(identity)] != 1 || state.attemptExpires[verificationAttemptKey(identity)] != deadline || len(state.resetAttempts) != 0 {
				t.Fatal("logout changed the shared Verification rate limit")
			}
		})
	}
}

type logoutFailSessionStore struct {
	SessionStore
	failure error
}

func (s *logoutFailSessionStore) Delete(ctx context.Context, key string) error {
	if s.failure != nil {
		return s.failure
	}
	return s.SessionStore.Delete(ctx, key)
}

func TestLogoutRetriesPartialCleanup(t *testing.T) {
	for _, failed := range []string{"session", "grant", "both"} {
		t.Run(failed, func(t *testing.T) {
			m, _, ctx, identity, state := newVerificationTest(t)
			record, err := m.sessions.read(ctx, identity.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			token := m.sessions.token(record).RefreshToken
			key := "sensitive:" + identity.SessionID
			state.values[key] = "grant"
			sessionFailure, grantFailure := errors.New("session delete unavailable"), errors.New("grant delete unavailable")
			store := &logoutFailSessionStore{SessionStore: m.sessions.store}
			m.sessions.store = store
			if failed != "grant" {
				store.failure = sessionFailure
			}
			if failed != "session" {
				state.fail = grantFailure
			}
			err = m.Logout(ctx, token)
			if failed != "grant" && !errors.Is(err, sessionFailure) {
				t.Fatal("lost session revocation failure", err)
			}
			if failed != "session" && !errors.Is(err, grantFailure) {
				t.Fatal("lost grant cleanup failure", err)
			}
			if failed == "grant" {
				if err := m.sessions.ValidateSession(ctx, identity.SessionID); !errors.Is(err, ErrSessionNotFound) {
					t.Fatal("grant error prevented session revocation", err)
				}
			}
			if failed == "session" {
				if _, exists := state.values[key]; exists {
					t.Fatal("session error prevented grant cleanup")
				}
			}
			store.failure, state.fail = nil, nil
			if err := m.Logout(ctx, token); err != nil {
				t.Fatal("cleanup retry failed", err)
			}
			if _, exists := state.values[key]; exists {
				t.Fatal("cleanup retry left grant")
			}
			if err := m.sessions.ValidateSession(ctx, identity.SessionID); !errors.Is(err, ErrSessionNotFound) {
				t.Fatal("cleanup retry left session", err)
			}
		})
	}
}

type changingInfoCatalog struct {
	before       localization.Catalog
	after        localization.Catalog
	selectCalls  int
	currentCalls int
	fields       []string
}

func (c *changingInfoCatalog) Select(fields []string) localization.Catalog {
	c.selectCalls++
	c.fields = append([]string{}, fields...)
	if c.selectCalls == 1 {
		return c.before
	}
	return c.after
}

func (c *changingInfoCatalog) Current() localization.Catalog {
	c.currentCalls++
	return c.after
}

func TestInfoUsesOnePermissionCatalogSnapshot(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	catalog := &changingInfoCatalog{before: m.catalogSource.Current(), after: m.catalogSource.Current()}
	resource := catalog.after.ResourceCatalog.Resources["admin:system:user"]
	entry := resource.Entries["admin:menu:system:user"]
	entry.Path = "/system/user-new"
	resource.Entries["admin:menu:system:user"] = entry
	catalog.after.ResourceCatalog.Resources["admin:system:user"] = resource
	m.catalogSource = catalog
	mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code", "role_id", "role_name", "role_word"}).
			AddRow(3, "readonly", "EXP78RGH", nil, nil, nil))
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").
		WillReturnRows(permissionRows().AddRow("admin:read:system:user", userReadPermissionConfig))

	info, err := m.Info(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Permission.Menus) != 1 || info.Permission.Menus[0].MenuPath != "/system/user" {
		t.Fatalf("unexpected menus: %+v", info.Permission.Menus)
	}
	if len(info.Permission.Navigation) != 1 || info.Permission.Navigation[0].MenuPath != info.Permission.Menus[0].MenuPath {
		t.Fatalf("navigation used a different catalog snapshot: menus=%+v navigation=%+v", info.Permission.Menus, info.Permission.Navigation)
	}
	if catalog.selectCalls != 1 || catalog.currentCalls != 0 || len(catalog.fields) != 1 || catalog.fields[0] != "resource_catalog" {
		t.Fatalf("unnecessary catalog data loaded: Select=%d Current=%d fields=%v", catalog.selectCalls, catalog.currentCalls, catalog.fields)
	}
}

func TestRefreshRejectsStaleCredential(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	for range 2 {
		session, err := m.sessions.Issue(t.Context(), authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}, false)
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectQuery("SELECT username, code, status, credential_version").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
			sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(2), int64(1)),
		)
		if _, err := m.Refresh(t.Context(), session.RefreshToken); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("refreshed stale credential: %v", err)
		}
		if err := m.sessions.ValidateSession(t.Context(), session.SessionID); !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("stale session not revoked: %v", err)
		}
	}
}

func TestLoginResolvesTOTPBindingOnce(t *testing.T) {
	for _, tc := range []struct {
		name string
		enabled, bound bool
	}{
		{"enrollment required", true, false},
		{"bound factor", true, true},
		{"Verification disabled", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock, _, _, _ := newVerificationTest(t)
			m.verification.config.TOTPEnabled = tc.enabled
			mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
			if tc.enabled { expectTOTPBound(mock, tc.bound) }
			if !tc.enabled || !tc.bound {
				mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(2))
			}
			// The mock permits one factor lookup for password login, and none
			// during session issuance or after the login counter update.
			result, err := m.Login(t.Context(), LoginInput{Username: "readonly", Password: authTestPassword, SliderProof: validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")})
			if err != nil { t.Fatal(err) }
			if tc.enabled && tc.bound {
				if !result.MFARequired || result.MFATicket == "" || result.AccessToken != "" { t.Fatal("bound login skipped Verification") }
				return
			}
			if result.AccessToken == "" || result.PasswordResetRequired || result.TOTPEnrollmentRequired != tc.enabled { t.Fatal("incorrect login restrictions") }
			identity, err := m.authenticator.Parse(result.AccessToken)
			if err != nil { t.Fatal(err) }
			allowed, err := m.canEnrollWithoutPassword(t.Context(), identity)
			if err != nil || allowed != tc.enabled { t.Fatal("incorrect enrollment grant", err) }
		})
	}
}

type defaultPermissionsStub struct{}

func (defaultPermissionsStub) DefaultUserCodes(_ context.Context, selected []string) ([]string, error) {
	return append([]string{}, selected...), nil
}

func TestRefreshTransientFailurePreservesSession(t *testing.T) {
	for _, scenario := range []string{"database", "rotation not committed", "rotation response lost"} {
		t.Run(scenario, func(t *testing.T) {
			m, manager, mock := newAuthTestModule(t)
			identity := authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}
			login, sessionID, err := m.issueLoginResult(t.Context(), identity, false)
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("temporary backend failure")
			query := mock.ExpectQuery("SELECT username, code, status, credential_version, login_count FROM t_user").WithArgs(int64(3), "EXP78RGH")
			if scenario == "database" {
				query.WillReturnError(failure)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(1), int64(1)))
				m.sessions.store = &refreshRotationFailureStore{SessionStore: m.sessions.store, failure: failure, afterCommit: scenario == "rotation response lost"}
			}
			if _, err := m.Refresh(t.Context(), login.RefreshToken); !errors.Is(err, failure) {
				t.Fatalf("first refresh = %v; want transient failure", err)
			}
			request := httptest.NewRequest("GET", "/info", nil)
			request.Header.Set("Authorization", "Bearer "+login.AccessToken)
			if _, err := manager.AuthenticateRequest(request); err != nil {
				t.Fatalf("transient failure invalidated access token: %v", err)
			}
			var committedToken string
			if scenario == "rotation response lost" {
				record, err := m.sessions.read(t.Context(), sessionID)
				if err != nil {
					t.Fatal(err)
				}
				committedToken = m.sessions.token(record).RefreshToken
			}
			mock.ExpectQuery("SELECT username, code, status, credential_version, login_count FROM t_user").WithArgs(int64(3), "EXP78RGH").
				WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(1), int64(1)))
			retried, err := m.Refresh(t.Context(), login.RefreshToken)
			if err != nil || retried.RefreshToken == login.RefreshToken {
				t.Fatalf("retry = %#v, %v", retried, err)
			}
			if committedToken != "" && retried.RefreshToken != committedToken {
				t.Fatal("response-loss retry did not recover the committed refresh token")
			}
		})
	}
}

type refreshRotationFailureStore struct {
	SessionStore
	failure     error
	afterCommit bool
}

func (s *refreshRotationFailureStore) Rotate(ctx context.Context, id, expected, value string, expiredAt time.Time) (bool, error) {
	if s.failure == nil {
		return s.SessionStore.Rotate(ctx, id, expected, value, expiredAt)
	}
	failure := s.failure
	s.failure = nil
	if s.afterCommit {
		if changed, err := s.SessionStore.Rotate(ctx, id, expected, value, expiredAt); err != nil || !changed {
			return changed, err
		}
	}
	return false, failure
}

func (c *changingInfoCatalog) PublicCatalog(...string) localization.PublicCatalog {
	panic("public catalog requested during authenticated permission lookup")
}
