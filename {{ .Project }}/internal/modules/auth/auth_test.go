package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

const authTestKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newAuthTestModule(t *testing.T) (*Module, *authn.Manager, sqlmock.Sqlmock) {
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
	sessions, sessionErr := NewSessions(NewMemorySessionStore(), time.Hour, 30*24*time.Hour)
	if sessionErr != nil {
		t.Fatal(sessionErr)
	}
	authenticator, err := authn.New("test", authTestKey, time.Hour, sessions)
	if err != nil {
		t.Fatal(err)
	}
	credentials := newCredentialTestManager(t, time.Minute)
	captcha, captchaErr := NewPointCaptcha(NewMemoryPointCaptchaStore(), newPointCaptchaTestDictionary(), 5, 2*time.Minute, 300, 180, 22, nil)
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
	return New(&db.Store{DB: database}, authenticator, credentials, sessions, captcha, slider, passwordGuard, Switches{PasswordResetRequired: true}), authenticator, mock
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
		AddRow(int64(3), "readonly", "EXP78RGH", passwordHash(t, "cinch123"), status, wrong, int64(1))
}

func expectPasswordChangeLock(mock sqlmock.Sqlmock, locked bool) {
	mock.ExpectQuery("SELECT CASE").WithArgs(int64(3), "EXP78RGH", int64(6)).WillReturnRows(
		sqlmock.NewRows([]string{"locked"}).AddRow(locked),
	)
}

func authenticatedContext(t *testing.T, authenticator *authn.Manager, sessions *Sessions) context.Context {
	t.Helper()
	return authenticatedIdentityContext(t, authenticator, sessions, authn.Identity{CredentialVersion: 1, UserID: 3, Username: "readonly", Code: "EXP78RGH"})
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
	request := httptest.NewRequest("GET", "/auth/info", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request, err = authenticator.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	return request.Context()
}

func TestAuthSwitches(t *testing.T) {
	m, authenticator, mock := newAuthTestModule(t)
	m.switches.PasswordResetRequired = false
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at = \\$1, login_count = login_count \\+ 1").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(int64(1)))
	result, err := m.Login(t.Context(), LoginInput{Username: "readonly", Password: "cinch123", SliderProof: validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")})
	if err != nil || result.PasswordResetRequired {
		t.Fatalf("password-reset bypass login: %#v %v", result, err)
	}

	m.switches.ProtectSuper = true
	ctx := authenticatedIdentityContext(t, authenticator, m.sessions, authn.Identity{
		CredentialVersion: 1, UserID: SuperUserID, Username: "super", Code: "89HEK28Y",
	})
	if _, err := m.PasswordChangeChallenge(ctx); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("super password change challenge: %v", err)
	}
	if _, err := m.OpenPasswordChange(ctx, EncryptedPasswordChangeInput{}); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("open super password change: %v", err)
	}
	if err := m.ChangePassword(ctx, PasswordChangeInput{}); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("change super password: %v", err)
	}
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
	if _, err := m.Login(ctx, LoginInput{Username: "readonly", Password: "cinch123"}); !errors.Is(err, ErrPointCaptchaRequired) {
		t.Fatalf("captcha requirement: %v", err)
	}
	answer := storedPointCaptchaAnswer(t, m.captcha.store.(*memoryPointCaptchaStore), verification.Captcha.CaptchaID)
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive, 5))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(int64(2)))
	if result, err := m.Login(ctx, LoginInput{
		Username: "readonly", Password: "cinch123", CaptchaID: verification.Captcha.CaptchaID, CaptchaPoints: answer.Points,
	}); err != nil || result.AccessToken == "" {
		t.Fatalf("point captcha without slider: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusPending))
	if _, err := m.Login(ctx, LoginInput{Username: "readonly", Password: "cinch123"}); !errors.Is(err, ErrPendingApproval) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusLocked))
	mock.ExpectExec("UPDATE t_user SET status").WithArgs(userStatusActive, sqlmock.AnyArg(), int64(3), userStatusLocked, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	if _, err := m.Login(ctx, LoginInput{Username: "readonly", Password: "cinch123"}); !errors.Is(err, ErrLocked) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusLocked))
	mock.ExpectExec("UPDATE t_user SET status").WithArgs(userStatusActive, sqlmock.AnyArg(), int64(3), userStatusLocked, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(int64(2)))
	if result, err := m.Login(ctx, LoginInput{Username: "readonly", Password: "cinch123", SliderProof: validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")}); err != nil || result.AccessToken == "" {
		t.Fatalf("expired lock login: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Login(ctx, LoginInput{Username: " readonly ", Password: "cinch123", SliderProof: validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")}); err == nil {
		t.Fatal("update failure was ignored")
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(int64(2)))
	result, err := m.Login(ctx, LoginInput{Username: "readonly", Password: "cinch123", SliderProof: validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")})
	if err != nil || result.AccessToken == "" || result.ExpiredAt <= time.Now().UnixMilli() {
		t.Fatalf("login: %#v %v", result, err)
	}
	identity, err := authenticator.Parse(result.AccessToken)
	if err != nil || identity.UserID != 3 || identity.Username != "readonly" {
		t.Fatalf("identity: %#v %v", identity, err)
	}
}

func TestRegister(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	ctx := context.Background()
	for _, username := range []string{"tiny", "用户名称一", "1user", "-user", "_user", "user.name", "user name", " abcde ", "a" + strings.Repeat("b", 50)} {
		if err := m.Register(ctx, RegisterInput{Username: username, Password: "secret1"}); !errors.Is(err, ErrInvalidRegistration) {
			t.Fatalf("invalid username %q: %v", username, err)
		}
	}
	if err := m.Register(ctx, RegisterInput{Username: "new-user", Password: "short"}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("short password: %v", err)
	}

	mock.ExpectQuery("SELECT nextval").WillReturnError(sqlmock.ErrCancelled)
	if err := m.Register(ctx, RegisterInput{Username: "new-user", Password: "secret1"}); err == nil || errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("sequence error: %v", err)
	}

	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(36)))
	mock.ExpectExec("INSERT INTO t_user").WithArgs(
		int64(36), sqlmock.AnyArg(), sqlmock.AnyArg(), "new-user", sqlmock.AnyArg(), sqlmock.AnyArg(),
	).WillReturnError(&pq.Error{Code: "23505", Constraint: "uk_user_username"})
	if err := m.Register(ctx, RegisterInput{Username: "new-user", Password: "secret1"}); !errors.Is(err, ErrUsernameExists) {
		t.Fatalf("duplicate username: %v", err)
	}

	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(37)))
	mock.ExpectExec("INSERT INTO t_user").WithArgs(
		int64(37), sqlmock.AnyArg(), sqlmock.AnyArg(), "new-user", sqlmock.AnyArg(), sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO t_user").WithArgs(
		int64(37), sqlmock.AnyArg(), sqlmock.AnyArg(), "new-user", sqlmock.AnyArg(), sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := m.Register(ctx, RegisterInput{Username: "new-user", Password: "secret1"}); err != nil {
		t.Fatalf("register: %v", err)
	}
}

func TestUsernameAvailability(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	ctx := context.Background()
	for _, username := range []string{"tiny", "用户名称一", "1user", "-user", "_user", "user.name", "user name", " abcde ", "a" + strings.Repeat("b", 50)} {
		if _, err := m.UsernameAvailability(ctx, username); !errors.Is(err, ErrInvalidRegistration) {
			t.Fatalf("invalid username %q: %v", username, err)
		}
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("new-user").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	value, err := m.UsernameAvailability(ctx, "new-user")
	if err != nil || !value.Available {
		t.Fatalf("available username: %#v %v", value, err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("existing-user").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	value, err = m.UsernameAvailability(ctx, "existing-user")
	if err != nil || value.Available {
		t.Fatalf("existing username: %#v %v", value, err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("broken-user").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.UsernameAvailability(ctx, "broken-user"); err == nil {
		t.Fatal("database failure was ignored")
	}
}

func TestChangePassword(t *testing.T) {
	m, authenticator, mock := newAuthTestModule(t)
	if _, err := m.PasswordChangeChallenge(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unauthenticated challenge: %v", err)
	}
	if _, err := m.OpenPasswordChange(context.Background(), EncryptedPasswordChangeInput{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unauthenticated credential: %v", err)
	}
	if err := m.ChangePassword(context.Background(), PasswordChangeInput{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unauthenticated change: %v", err)
	}

	ctx := authenticatedContext(t, authenticator, m.sessions)
	challenge, err := m.PasswordChangeChallenge(ctx)
	if err != nil || challenge.ChallengeID == "" {
		t.Fatalf("password challenge: %#v %v", challenge, err)
	}
	encrypted := encryptedPasswordChangeInput(t, m.credentials, 3, "cinch123", "new-secret")
	opened, err := m.OpenPasswordChange(ctx, encrypted)
	if err != nil || opened.OldPassword != "cinch123" || opened.NewPassword != "new-secret" {
		t.Fatalf("open password change: %#v %v", opened, err)
	}

	for _, input := range []PasswordChangeInput{
		{},
		{OldPassword: "cinch123", NewPassword: "short"},
		{OldPassword: strings.Repeat("x", 73), NewPassword: "new-secret"},
		{OldPassword: "cinch123", NewPassword: strings.Repeat("x", 73)},
	} {
		if err := m.ChangePassword(ctx, input); !errors.Is(err, ErrInvalidPasswordChange) {
			t.Fatalf("invalid password input %#v: %v", input, err)
		}
	}

	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, "cinch123"), int64(1)),
	)
	mock.ExpectRollback()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "wrong-password", NewPassword: "new-secret"}); !errors.Is(err, ErrIncorrectPassword) {
		t.Fatalf("incorrect password: %v", err)
	}

	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, "cinch123"), int64(1)),
	)
	mock.ExpectRollback()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "cinch123", NewPassword: "cinch123"}); !errors.Is(err, ErrSamePassword) {
		t.Fatalf("same password: %v", err)
	}

	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, "cinch123"), int64(1)),
	)
	mock.ExpectExec("UPDATE t_user SET password").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), "EXP78RGH").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "cinch123", NewPassword: "new-secret"}); err != nil {
		t.Fatalf("change password: %v", err)
	}

	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnError(sqlmock.ErrCancelled)
	mock.ExpectRollback()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "cinch123", NewPassword: "another-secret"}); err == nil {
		t.Fatal("database failure was ignored")
	}
}

func TestChangePasswordRequiresCaptchaAfterThreeFailures(t *testing.T) {
	m, authenticator, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, authenticator, m.sessions)
	for attempt := 1; attempt <= 3; attempt++ {
		expectPasswordChangeLock(mock, false)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
			sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, "cinch123"), int64(1)),
		)
		mock.ExpectRollback()
		err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "wrong-password", NewPassword: "new-secret"})
		if !errors.Is(err, ErrIncorrectPassword) || requiresPasswordChangeCaptcha(err) != (attempt == 3) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	expectPasswordChangeLock(mock, false)
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "cinch123", NewPassword: "new-secret"}); !errors.Is(err, ErrPointCaptchaRequired) || !requiresPasswordChangeCaptcha(err) {
		t.Fatalf("missing captcha: %v", err)
	}
	challenge, err := m.NewPasswordChangeCaptcha(ctx)
	if err != nil {
		t.Fatal(err)
	}
	answer := storedPointCaptchaAnswer(t, m.captcha.store.(*memoryPointCaptchaStore), challenge.CaptchaID)
	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, "cinch123"), int64(1)),
	)
	mock.ExpectExec("UPDATE t_user SET password").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), "EXP78RGH").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := m.ChangePassword(ctx, PasswordChangeInput{
		OldPassword: "cinch123", NewPassword: "new-secret",
		CaptchaID: challenge.CaptchaID, CaptchaPoints: answer.Points,
	}); err != nil {
		t.Fatalf("captcha-protected password change: %v", err)
	}
	if required, err := m.passwordGuard.Required(ctx, 3); err != nil || required {
		t.Fatalf("failures were not cleared: %v, %v", required, err)
	}
}

func TestChangePasswordLocksAfterSixFailures(t *testing.T) {
	m, authenticator, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, authenticator, m.sessions)
	for attempt := 1; attempt <= 6; attempt++ {
		input := PasswordChangeInput{OldPassword: "wrong-password", NewPassword: "new-secret"}
		if attempt > 3 {
			challenge, err := m.NewPasswordChangeCaptcha(ctx)
			if err != nil {
				t.Fatal(err)
			}
			answer := storedPointCaptchaAnswer(t, m.captcha.store.(*memoryPointCaptchaStore), challenge.CaptchaID)
			input.CaptchaID = challenge.CaptchaID
			input.CaptchaPoints = answer.Points
		}
		expectPasswordChangeLock(mock, false)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
			sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, "cinch123"), int64(1)),
		)
		mock.ExpectRollback()
		if attempt == 6 {
			mock.ExpectExec("UPDATE t_user SET metadata = jsonb_set").WithArgs(int64(6), sqlmock.AnyArg(), int64(3), "EXP78RGH").WillReturnResult(sqlmock.NewResult(0, 1))
		}
		err := m.ChangePassword(ctx, input)
		if attempt < 6 && !errors.Is(err, ErrIncorrectPassword) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if attempt == 6 && !errors.Is(err, ErrPasswordChangeLocked) {
			t.Fatalf("lock attempt: %v", err)
		}
	}
	if count, err := m.passwordGuard.FailureCount(ctx, 3); err != nil || count != 0 {
		t.Fatalf("locked failure count = %d, %v", count, err)
	}
	expectPasswordChangeLock(mock, true)
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "cinch123", NewPassword: "new-secret"}); !errors.Is(err, ErrPasswordChangeLocked) {
		t.Fatalf("persisted lock was ignored: %v", err)
	}
}

func TestCurrentUser(t *testing.T) {
	m, authenticator, mock := newAuthTestModule(t)
	if _, err := m.Info(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	ctx := authenticatedContext(t, authenticator, m.sessions)
	mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").WillReturnError(sql.ErrNoRows)
	if _, err := m.Info(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Info(ctx); err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code", "role_id", "role_name", "role_word"}).
			AddRow(3, "readonly", "EXP78RGH", 2, "Reader", "reader"))
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"resource", "menu", "btn"}).
			AddRow("GET|/auth/info|/auth.v1.Auth/Info", "/dashboard/overview", "").
			AddRow("GET|/user|/auth.v1.User/ListUsers", "/system/user", "system.user.read"),
	)
	value, err := m.Info(ctx)
	if err != nil || value.Username != "readonly" || value.Role == nil || value.Role.Name != "Reader" || value.Role.Word != "reader" || len(value.Permission.Buttons) != 1 || value.Permission.Buttons[0] != "system.user.read" {
		t.Fatalf("info: %#v %v", value, err)
	}
	session, err := m.sessions.Issue(ctx, authn.Identity{CredentialVersion: 1, UserID: 3, Username: "readonly", Code: "EXP78RGH"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Logout(ctx, session.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if err := m.sessions.ValidateSession(ctx, session.SessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("session remained valid: %v", err)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count FROM t_user").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusLocked, int64(1), int64(1)))
	mock.ExpectExec("UPDATE t_user SET status").WithArgs(userStatusActive, sqlmock.AnyArg(), int64(3), userStatusLocked, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	if identity, err := m.refreshIdentity(ctx, authn.Identity{CredentialVersion: 1, UserID: 3, Username: "readonly", Code: "EXP78RGH"}); err != nil || identity.Username != "readonly" {
		t.Fatalf("refresh expired lock: %#v %v", identity, err)
	}
	if err := m.Logout(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordChangeRejectsStaleAuthenticatedRequest(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user.*FOR UPDATE").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, "cinch123"), int64(2)),
	)
	mock.ExpectRollback()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "cinch123", NewPassword: "new-secret"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("stale request: %v", err)
	}
	if count, err := m.passwordGuard.FailureCount(ctx, 3); err != nil || count != 0 {
		t.Fatalf("stale request counted as bad password: %d %v", count, err)
	}
}

func TestRefreshRejectsStaleCredential(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	for range 2 {
		session, err := m.sessions.Issue(t.Context(), authn.Identity{UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}, false)
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
