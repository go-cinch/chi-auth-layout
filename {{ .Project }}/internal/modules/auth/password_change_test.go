package auth

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

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
	encrypted := encryptedPasswordChangeInput(t, m.credentials, 3, authTestPassword, "new-secret")
	opened, err := m.OpenPasswordChange(ctx, encrypted)
	if err != nil || opened.OldPassword != authTestPassword || opened.NewPassword != "new-secret" {
		t.Fatalf("open password change: %#v %v", opened, err)
	}

	for _, input := range []PasswordChangeInput{
		{},
		{OldPassword: authTestPassword, NewPassword: "   "},
		{OldPassword: "   ", NewPassword: "new-secret"},
	} {
		if err := m.ChangePassword(ctx, input); !errors.Is(err, ErrInvalidPasswordChange) {
			t.Fatalf("invalid password input %#v: %v", input, err)
		}
	}

	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, authTestPassword), int64(1)),
	)
	mock.ExpectRollback()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "wrong-password", NewPassword: "new-secret"}); !errors.Is(err, ErrIncorrectPassword) {
		t.Fatalf("incorrect password: %v", err)
	}

	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, authTestPassword), int64(1)),
	)
	mock.ExpectRollback()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: authTestPassword, NewPassword: authTestPassword}); !errors.Is(err, ErrSamePassword) {
		t.Fatalf("same password: %v", err)
	}

	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, authTestPassword), int64(1)),
	)
	mock.ExpectExec("UPDATE t_user SET password").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), "EXP78RGH").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: authTestPassword, NewPassword: "x"}); err != nil {
		t.Fatalf("change password: %v", err)
	}

	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnError(sqlmock.ErrCancelled)
	mock.ExpectRollback()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: authTestPassword, NewPassword: "another-secret"}); err == nil {
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
			sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, authTestPassword), int64(1)),
		)
		mock.ExpectRollback()
		err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: "wrong-password", NewPassword: "new-secret"})
		if !errors.Is(err, ErrIncorrectPassword) || requiresPasswordChangeCaptcha(err) != (attempt == 3) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	expectPasswordChangeLock(mock, false)
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: authTestPassword, NewPassword: "new-secret"}); !errors.Is(err, ErrPointCaptchaRequired) || !requiresPasswordChangeCaptcha(err) {
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
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, authTestPassword), int64(1)),
	)
	mock.ExpectExec("UPDATE t_user SET password").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), "EXP78RGH").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := m.ChangePassword(ctx, PasswordChangeInput{
		OldPassword: authTestPassword, NewPassword: "new-secret",
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
			sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, authTestPassword), int64(1)),
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
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: authTestPassword, NewPassword: "new-secret"}); !errors.Is(err, ErrPasswordChangeLocked) {
		t.Fatalf("persisted lock was ignored: %v", err)
	}
}

func TestPasswordChangeRejectsStaleAuthenticatedRequest(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user.*FOR UPDATE").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, authTestPassword), int64(2)),
	)
	mock.ExpectRollback()
	if err := m.ChangePassword(ctx, PasswordChangeInput{OldPassword: authTestPassword, NewPassword: "new-secret"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("stale request: %v", err)
	}
	if count, err := m.passwordGuard.FailureCount(ctx, 3); err != nil || count != 0 {
		t.Fatalf("stale request counted as bad password: %d %v", count, err)
	}
}
