package auth

import (
	"context"
	"database/sql"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"net/http/httptest"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestTOTPAuthenticatorOnlyAndInvalidation(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	secret, _ := newTOTPSecret()
	for _, code := range []string{"recovery-code", "12a456", "12345", "1234567"} {
		expectVerificationLock(mock, 1)
		expectTOTPFactor(t, m, mock, secret, -1)
		mock.ExpectRollback()
		if err := m.verification.consumeTOTP(ctx, identity, code); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("non-TOTP code accepted", err)
		}
	}
	expectVerificationLock(mock, 2)
	mock.ExpectRollback()
	if err := m.verification.consumeTOTP(ctx, identity, "123456"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale identity accepted", err)
	}
	expectVerificationLock(mock, 1)
	mock.ExpectQuery("SELECT secret_ciphertext").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	if err := m.verification.consumeTOTP(ctx, identity, "123456"); !errors.Is(err, ErrTOTPEnrollmentRequired) {
		t.Fatal(err)
	}
	state.attempts = map[string]int64{"3:1": 10}
	if err := m.verification.consumeTOTP(ctx, identity, "123456"); !errors.Is(err, ErrVerificationRateLimited) {
		t.Fatal("unbounded guesses", err)
	}
}

func TestTOTPDisabledSkipsEnforcement(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	m.verification.config.TOTPEnabled = false
	// No database/Redis access, even when the account has a stored factor.
	if required, err := m.totpEnrollmentRequired(ctx, identity); err != nil || required {
		t.Fatal(required, err)
	}
	request := httptest.NewRequest("DELETE", "/user/1", nil).WithContext(ctx)
	if err := m.CheckHTTPSecurity(request); err != nil {
		t.Fatal(err)
	}
	status, err := m.TOTPStatus(ctx)
	if err != nil || status.Enabled || status.EnrollmentRequired {
		t.Fatal(status, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPTestPreservesSensitiveGrantAndAudit(t *testing.T) {
	for _, granted := range []bool{false, true} {
		t.Run(map[bool]string{false: "without grant", true: "existing grant"}[granted], func(t *testing.T) {
			m, mock, ctx, identity, state := newVerificationTest(t)
			key := "sensitive:" + identity.SessionID
			if granted {
				now := time.Now().Add(-10 * time.Minute)
				proof := &SensitiveProof{UserID: identity.UserID, SessionID: identity.SessionID, CredentialVersion: identity.CredentialVersion, Fingerprint: "existing-verification", VerifiedAt: now.UnixMilli(), ExpiredAt: now.Add(time.Hour).UnixMilli()}
				if err := m.verification.put(ctx, key, proof, 50*time.Minute); err != nil {
					t.Fatal(err)
				}
			}
			before, ttl := state.values[key], state.ttls[key]
			m.verification.state = &testVerificationAttemptOnlyStore{testVerificationStore: state, t: t}
			auditCtx, auditState := auditctx.WithState(ctx)
			secret, _ := newTOTPSecret()
			step := time.Now().Unix() / 30
			code, _ := totpCode(secret, step)
			expectVerificationLock(mock, 1)
			expectTOTPFactor(t, m, mock, secret, -1)
			mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WithArgs(step, sqlmock.AnyArg(), identity.UserID).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			result, err := m.TestTOTP(auditCtx, " "+code+" ")
			if err != nil || result == nil || !result.Verified {
				t.Fatal("authenticator test failed", err)
			}
			if state.attempts[verificationAttemptKey(identity)] != 1 {
				t.Fatal("successful test did not spend an attempt")
			}
			// A failed replay must spend another attempt, preserving the same grant.
			expectVerificationLock(mock, 1)
			expectTOTPFactor(t, m, mock, secret, step)
			mock.ExpectRollback()
			if result, err := m.TestTOTP(auditCtx, code); result != nil || !errors.Is(err, ErrVerificationInvalid) {
				t.Fatal("tested code reused", err)
			}
			if state.attempts[verificationAttemptKey(identity)] != 2 {
				t.Fatal("failed test did not spend an attempt")
			}
			if state.values[key] != before || state.ttls[key] != ttl {
				t.Fatal("test changed or renewed the sensitive grant")
			}
			if event := auditState.Snapshot(); event.StepUpFingerprint != "" || event.StepUpVerifiedAt != 0 {
				t.Fatal("test created sensitive audit metadata")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTOTPTestRejectsUnauthorizedAndUnavailableState(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	for _, testCtx := range []context.Context{
		context.Background(),
		verificationContextWithIdentity(t, ctx, authn.Identity{UserID: identity.UserID, CredentialVersion: identity.CredentialVersion}),
	} {
		if _, err := m.TestTOTP(testCtx, "123456"); !errors.Is(err, ErrUnauthorized) {
			t.Fatal("unauthenticated test accepted", err)
		}
	}
	restricted := identity
	restricted.PasswordResetRequired = true
	if _, err := m.TestTOTP(verificationContextWithIdentity(t, ctx, restricted), "123456"); !errors.Is(err, authn.ErrPasswordResetRequired) {
		t.Fatal("password reset gate bypassed", err)
	}
	if len(state.attempts) != 0 {
		t.Fatal("rejected identity spent an attempt")
	}
	m.verification.config.TOTPEnabled = false
	if _, err := m.TestTOTP(ctx, "123456"); !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatal("disabled Verification accepted", err)
	}
	m.verification.config.TOTPEnabled = true
	m.verification.state = nil
	if _, err := m.TestTOTP(ctx, "123456"); !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatal("missing state failed open", err)
	}
	m.verification.state = state
	state.fail = errors.New("redis unavailable")
	if _, err := m.TestTOTP(ctx, "123456"); !errors.Is(err, state.fail) {
		t.Fatal("failed attempt store failed open", err)
	}
	state.fail = nil
	expectVerificationLock(mock, 2)
	mock.ExpectRollback()
	if _, err := m.TestTOTP(ctx, "123456"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale identity accepted", err)
	}
	expectVerificationLock(mock, 1)
	mock.ExpectQuery("SELECT secret_ciphertext").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	if _, err := m.TestTOTP(ctx, "123456"); !errors.Is(err, ErrTOTPEnrollmentRequired) {
		t.Fatal("unbound account accepted", err)
	}
	if state.attempts[verificationAttemptKey(identity)] != 2 {
		t.Fatal("validated failures bypassed shared attempt budget")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPTestSharesRateLimitAcrossSessionsAndFlows(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	m.verification.config.RateLimitMaxAttempts = 2
	secret, _ := newTOTPSecret()
	for _, code := range []string{"12345", "recovery-code"} {
		expectVerificationLock(mock, 1)
		expectTOTPFactor(t, m, mock, secret, -1)
		mock.ExpectRollback()
		if _, err := m.TestTOTP(ctx, code); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("non-TOTP test accepted", err)
		}
	}
	key := verificationAttemptKey(identity)
	deadline := state.attemptExpires[key]
	other := identity
	other.SessionID = "another-session"
	if _, err := m.TestTOTP(verificationContextWithIdentity(t, ctx, other), "123456"); !errors.Is(err, ErrVerificationRateLimited) {
		t.Fatal("other session escaped test limit", err)
	}
	if _, err := m.VerifySensitiveTOTP(ctx, "123456"); !errors.Is(err, ErrVerificationRateLimited) {
		t.Fatal("sensitive verification escaped test limit", err)
	}
	if _, err := m.BeginTOTPEnrollment(ctx, "password"); !errors.Is(err, ErrVerificationRateLimited) {
		t.Fatal("enrollment escaped test limit", err)
	}
	pending, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CompleteTOTPLogin(ctx, pending.MFATicket, "123456"); !errors.Is(err, ErrVerificationRateLimited) {
		t.Fatal("login escaped test limit", err)
	}
	if _, err := m.RecoverTOTPLogin(ctx, pending.MFATicket, "recovery-code"); !errors.Is(err, ErrVerificationRateLimited) {
		t.Fatal("recovery escaped test limit", err)
	}
	if state.attempts[key] != 7 || state.attemptExpires[key] != deadline {
		t.Fatal("attempts or fixed-window deadline changed incorrectly")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPVerificationUsesBindingPeriod(t *testing.T) {
	for _, tc := range []struct {
		name               string
		stored, configured int64
	}{
		{"existing_minute_after_default_changes", 60, 30},
		{"new_half_minute_after_default_changes", 30, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock, ctx, identity, _ := newVerificationTest(t)
			m.verification.config.TOTPPeriod = time.Duration(tc.configured) * time.Second
			secret, err := newTOTPSecret()
			if err != nil {
				t.Fatal(err)
			}
			sealed, err := sealVerification(m.verification.cipher, secret, "totp:3")
			if err != nil {
				t.Fatal(err)
			}
			step := time.Now().Unix() / tc.stored
			code, err := totpCode(secret, step)
			if err != nil {
				t.Fatal(err)
			}
			factor := func(lastStep int64) {
				mock.ExpectQuery("SELECT secret_ciphertext, last_used_step, period_seconds").WithArgs(identity.UserID).
					WillReturnRows(sqlmock.NewRows([]string{"secret", "step", "period_seconds"}).AddRow(sealed, lastStep, tc.stored))
			}
			expectVerificationLock(mock, 1)
			factor(step - 1)
			mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WithArgs(step, sqlmock.AnyArg(), identity.UserID).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if err := m.verification.consumeTOTP(ctx, identity, code); err != nil {
				t.Fatal("stored period was ignored", err)
			}
			expectVerificationLock(mock, 1)
			factor(step)
			mock.ExpectRollback()
			if err := m.verification.consumeTOTP(ctx, identity, code); !errors.Is(err, ErrVerificationInvalid) {
				t.Fatal("replay accepted after config change", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTOTPVerificationRejectsInvalidStoredPeriod(t *testing.T) {
	for _, seconds := range []int64{0, -1, 9223372037} {
		m, mock, ctx, identity, _ := newVerificationTest(t)
		secret, err := newTOTPSecret()
		if err != nil {
			t.Fatal(err)
		}
		sealed, err := sealVerification(m.verification.cipher, secret, "totp:3")
		if err != nil {
			t.Fatal(err)
		}
		expectVerificationLock(mock, 1)
		mock.ExpectQuery("SELECT secret_ciphertext, last_used_step, period_seconds").WithArgs(identity.UserID).
			WillReturnRows(sqlmock.NewRows([]string{"secret", "step", "period_seconds"}).AddRow(sealed, 0, seconds))
		mock.ExpectRollback()
		if err := m.verification.consumeTOTP(ctx, identity, "123456"); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("invalid binding period accepted", seconds, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTOTPDebugPreservesFactorChecksAndRateLimits(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	m.verification.config.TOTPDebug = true
	secret, _ := newTOTPSecret()
	step := time.Now().Unix() / 30
	for range 2 {
		expectVerificationLock(mock, 1)
		expectTOTPFactor(t, m, mock, secret, step)
		mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WithArgs(step, sqlmock.AnyArg(), identity.UserID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		if result, err := m.TestTOTP(ctx, "999999"); err != nil || !result.Verified {
			t.Fatal("debug code must be reusable", err)
		}
	}
	expectVerificationLock(mock, 1)
	expectTOTPFactor(t, m, mock, secret, step)
	mock.ExpectRollback()
	if _, err := m.TestTOTP(ctx, "000000"); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal("email code accepted as Verification", err)
	}
	expectVerificationLock(mock, 2)
	mock.ExpectRollback()
	if _, err := m.TestTOTP(ctx, "999999"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("stale identity accepted", err)
	}
	expectVerificationLock(mock, 1)
	mock.ExpectQuery("SELECT secret_ciphertext").WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	if _, err := m.TestTOTP(ctx, "999999"); !errors.Is(err, ErrTOTPEnrollmentRequired) {
		t.Fatal("missing factor accepted", err)
	}
	state.attempts = map[string]int64{"3:1": 10}
	if _, err := m.TestTOTP(ctx, "999999"); !errors.Is(err, ErrVerificationRateLimited) {
		t.Fatal("debug skipped rate limit", err)
	}
	m.verification.state = nil
	if _, err := m.TestTOTP(ctx, "999999"); !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatal("debug skipped state store", err)
	}
	m.verification.config.TOTPDebug = false
	// A counter consumed in normal mode remains unusable after disabling debug.
	code, _ := totpCode(secret, step)
	if _, err := m.verification.verifyTOTPCode(secret, code, time.Unix(step*30, 0), step+1, 30*time.Second); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal("TOTP replay accepted", err)
	}
	if _, err := m.verification.verifyTOTPCode("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", "999999", time.Unix(59, 0), -1, 30*time.Second); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal("debug code accepted with debug off", err)
	}
}
