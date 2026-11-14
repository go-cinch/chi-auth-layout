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
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

type recoveryFailureState struct {
	VerificationStateStore
	takeError  error
	grantError error
	resetError error
}

func (s recoveryFailureState) Take(ctx context.Context, key string) (string, error) {
	if strings.HasPrefix(key, loginTicketPrefix) && s.takeError != nil {
		return "", s.takeError
	}
	return s.VerificationStateStore.Take(ctx, key)
}

func (s recoveryFailureState) Put(ctx context.Context, key, value string, ttl time.Duration) error {
	if strings.HasPrefix(key, "enrollment-grant:") && s.grantError != nil {
		return s.grantError
	}
	return s.VerificationStateStore.Put(ctx, key, value, ttl)
}

func (s recoveryFailureState) ResetAttempts(ctx context.Context, key string) error {
	if s.resetError != nil {
		return s.resetError
	}
	return s.VerificationStateStore.ResetAttempts(ctx, key)
}

type recoverySessionFailure struct {
	SessionStore
	err error
}

func (s recoverySessionFailure) Create(context.Context, string, string, time.Time) error {
	return s.err
}

func expectRecoveryMutation(mock sqlmock.Sqlmock, loginCount int64) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT credential_version, status, login_count").WithArgs(int64(3), "EXP78RGH").WillReturnRows(sqlmock.NewRows([]string{"version", "status", "login_count"}).AddRow(int64(1), userStatusActive, loginCount))
	mock.ExpectQuery("SELECT recovery.id").WithArgs(int64(3), recoveryDigest("recovery-code")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec("DELETE FROM t_user_recovery_code").WithArgs(int64(3)).WillReturnResult(sqlmock.NewResult(0, 10))
	mock.ExpectExec("DELETE FROM t_user_totp").WithArgs(int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE t_user SET credential_version").WithArgs(sqlmock.AnyArg(), int64(3),true).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func TestRecoverTOTPLoginRestrictionsAndVersion(t *testing.T) {
	for _, loginCount := range []int64{0, 4} {
		m, mock, ctx, identity, state := newVerificationTest(t)
		pending, err := m.beginMFALogin(ctx, identity, true)
		if err != nil {
			t.Fatal(err)
		}
		expectRecoveryMutation(mock, loginCount)
		result, err := m.RecoverTOTPLogin(ctx, pending.MFATicket, "recovery-code")
		if err != nil {
			t.Fatal(err)
		}
		if !result.TOTPEnrollmentRequired || result.PasswordResetRequired != (loginCount == 0) {
			t.Fatal("recovery lost mandatory enrollment or current reset state")
		}
		next, err := m.authenticator.Parse(result.AccessToken)
		if err != nil || next.Methods != (authn.Methods{Primary: identity.Methods.Primary}) || next.CredentialVersion != identity.CredentialVersion+1 || next.SessionID == identity.SessionID {
			t.Fatal("recovery did not replace credential version and session", err)
		}
		if state.attempts[verificationAttemptKey(identity)] != 0 || state.attempts[verificationAttemptKey(next)] != 0 || len(state.resetAttempts) != 1 {
			t.Fatal("successful recovery did not clear the old counter and start a zero new counter")
		}
		// An old in-flight request can recreate only the old version's counter.
		if err := m.verification.attempt(ctx, identity); err != nil {
			t.Fatal(err)
		}
		if state.attempts[verificationAttemptKey(next)] != 0 {
			t.Fatal("old credential request polluted recovered counter")
		}
		session, err := m.sessions.ReadRefresh(ctx, result.RefreshToken)
		if err != nil || !session.RememberMe {
			t.Fatal("recovery lost remember lifetime", err)
		}
		if allowed, err := m.canEnrollWithoutPassword(ctx, next); err != nil || !allowed {
			t.Fatal("password-verified recovery cannot enroll", err)
		}
		if _, err := m.verification.proof(ctx, next); !errors.Is(err, ErrStepUpRequired) {
			t.Fatal("recovery granted sensitive access", err)
		}
		if _, err := m.RecoverTOTPLogin(ctx, pending.MFATicket, "recovery-code"); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("recovery ticket was reusable", err)
		}
	}
}

func TestRecoverTOTPLoginFailuresRollback(t *testing.T) {
	failure := errors.New("storage unavailable")
	for _, stage := range []string{"missing_user", "stale", "locked", "user_query", "invalid_code", "code_query", "ticket_take", "delete_codes", "delete_factor", "version", "session", "grant", "commit"} {
		t.Run(stage, func(t *testing.T) {
			m, mock, ctx, identity, state := newVerificationTest(t)
			memory := m.sessions.store.(*memorySessionStore)
			pending, err := m.beginMFALogin(ctx, identity, false)
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectBegin()
			query := mock.ExpectQuery("SELECT credential_version, status, login_count")
			switch stage {
			case "missing_user":
				query.WillReturnError(sql.ErrNoRows)
			case "user_query":
				query.WillReturnError(failure)
			default:
				version, status := int64(1), userStatusActive
				if stage == "stale" {
					version++
				}
				if stage == "locked" {
					status = userStatusLocked
				}
				query.WillReturnRows(sqlmock.NewRows([]string{"version", "status", "login_count"}).AddRow(version, status, 0))
			}
			if stage != "missing_user" && stage != "user_query" && stage != "stale" && stage != "locked" {
				query = mock.ExpectQuery("SELECT recovery.id")
				switch stage {
				case "invalid_code":
					query.WillReturnError(sql.ErrNoRows)
				case "code_query":
					query.WillReturnError(failure)
				default:
					query.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
				}
				if stage == "ticket_take" {
					m.verification.state = recoveryFailureState{VerificationStateStore: state, takeError: failure}
				}
				if stage != "invalid_code" && stage != "code_query" && stage != "ticket_take" {
					statements := []string{"DELETE FROM t_user_recovery_code", "DELETE FROM t_user_totp", "UPDATE t_user SET credential_version"}
					stages := []string{"delete_codes", "delete_factor", "version"}
					for i, statement := range statements {
						exec := mock.ExpectExec(statement)
						if stage == stages[i] {
							exec.WillReturnError(failure)
							break
						}
						exec.WillReturnResult(sqlmock.NewResult(0, 1))
					}
					if stage == "session" || stage == "grant" || stage == "commit" {
						if stage == "session" {
							m.sessions.store = recoverySessionFailure{SessionStore: memory, err: failure}
						}
						if stage == "grant" {
							m.verification.state = recoveryFailureState{VerificationStateStore: state, grantError: failure}
						}
					}
				}
			}
			if stage == "commit" {
				mock.ExpectCommit().WillReturnError(failure)
			} else {
				mock.ExpectRollback()
			}
			result, err := m.RecoverTOTPLogin(ctx, pending.MFATicket, "recovery-code")
			if err == nil || result != nil {
				t.Fatal("failed recovery returned usable session")
			}
			memory.mu.Lock()
			count := len(memory.sessions)
			memory.mu.Unlock()
			if count != 1 {
				t.Fatal("failed recovery left a provisional session", count)
			}
			if len(state.resetAttempts) != 0 || state.attempts[verificationAttemptKey(identity)] != 1 {
				t.Fatal("failed recovery reset or lost its counted attempt")
			}
			_, ticketRemains := state.values[loginTicketPrefix+pending.MFATicket]
			consumed := stage == "delete_codes" || stage == "delete_factor" || stage == "version" || stage == "session" || stage == "grant" || stage == "commit"
			if ticketRemains == consumed {
				t.Fatal("failed recovery violated one-use ticket semantics")
			}
			for key := range state.values {
				if strings.HasPrefix(key, "enrollment-grant:") {
					t.Fatal("failed recovery left provisional enrollment grant")
				}
			}
		})
	}
}

func TestRecoverTOTPLoginTicketAndLimit(t *testing.T) {
	m, _, ctx, identity, state := newVerificationTest(t)
	if _, err := m.RecoverTOTPLogin(ctx, "short", "recovery-code"); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal(err)
	}
	pending, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"", strings.Repeat("x", 129)} {
		if _, err := m.RecoverTOTPLogin(ctx, pending.MFATicket, code); !errors.Is(err, ErrVerificationInvalid) {
			t.Fatal("invalid recovery shape accepted", err)
		}
	}
	state.attempts = map[string]int64{"3:1": 10}
	if _, err := m.RecoverTOTPLogin(ctx, pending.MFATicket, "recovery-code"); !errors.Is(err, ErrVerificationRateLimited) {
		t.Fatal("unlimited recovery attempts", err)
	}
	if len(state.resetAttempts) != 0 || state.attempts[verificationAttemptKey(identity)] != 11 {
		t.Fatal("blocked recovery cleared its limit")
	}
	m.verification.config.TOTPEnabled = false
	if _, err := m.RecoverTOTPLogin(ctx, pending.MFATicket, "recovery-code"); !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatal("recovery enabled without Verification", err)
	}
}

func TestTOTPRecoveryHTTPUsesSeparateHeader(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	handler := totpTestHTTP(m)
	pending, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ ticket, code, recovery string }{
		{"", "", "recovery-code"}, {pending.MFATicket, "123456", "recovery-code"},
	} {
		r := httptest.NewRequest("POST", "/pub/login", strings.NewReader(`{}`))
		if test.ticket != "" {
			r.Header.Set(authn.MFATicketHeader, test.ticket)
		}
		if test.code != "" {
			r.Header.Set(authn.TOTPCodeHeader, test.code)
		}
		r.Header.Set(authn.RecoveryCodeHeader, test.recovery)
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, r)
		if out.Code != 400 {
			t.Fatal("ambiguous recovery envelope accepted", out.Code)
		}
	}
	for _, body := range []string{"", "null", "invalid", `{"code":"value"}`, `{} {}`} {
		r := httptest.NewRequest("POST", "/pub/login", strings.NewReader(body))
		r.Header.Set(authn.MFATicketHeader, pending.MFATicket)
		r.Header.Set(authn.RecoveryCodeHeader, "recovery-code")
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, r)
		if out.Code != 400 {
			t.Fatal("nonempty or invalid continuation body accepted", out.Code)
		}
	}
	// A recovery code in the ordinary verification header is rejected without a write.
	secret, _ := newTOTPSecret()
	expectVerificationLock(mock, 1)
	expectTOTPFactor(t, m, mock, secret, -1)
	mock.ExpectRollback()
	r := httptest.NewRequest("POST", "/pub/login", strings.NewReader(`{}`))
	r.Header.Set(authn.MFATicketHeader, pending.MFATicket)
	r.Header.Set(authn.TOTPCodeHeader, "recovery-code")
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, r)
	if out.Code != 400 {
		t.Fatal("recovery code passed as TOTP", out.Code)
	}
	// The same code and ticket can still start recovery through the explicit header.
	expectRecoveryMutation(mock, 0)
	r = httptest.NewRequest("POST", "/pub/login", strings.NewReader(`{}`))
	r.Header.Set(authn.MFATicketHeader, pending.MFATicket)
	r.Header.Set(authn.RecoveryCodeHeader, "recovery-code")
	out = httptest.NewRecorder()
	handler.ServeHTTP(out, r)
	if out.Code != 200 || out.Header().Get("X-MFA-Enrollment-Required") != "" || out.Header().Get(authn.VerificationRequiredHeader) != "" || out.Header().Get("X-Password-Reset-Required") != "" {
		t.Fatal("recovery did not return restricted session", out.Code)
	}
	assertNoPasswordResetPayload(t, out.Body.Bytes())
}

func TestRecoverTOTPCounterCleanupFailurePreservesSuccess(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	pending, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	m.verification.state = recoveryFailureState{VerificationStateStore: state, resetError: errors.New("Redis cleanup unavailable")}
	expectRecoveryMutation(mock, 0)
	result, err := m.RecoverTOTPLogin(ctx, pending.MFATicket, "recovery-code")
	if err != nil || result == nil || result.AccessToken == "" || !result.TOTPEnrollmentRequired {
		t.Fatal("committed recovery lost its token after cleanup failure", err)
	}
	next, err := m.authenticator.Parse(result.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if state.attempts[verificationAttemptKey(identity)] != 1 || state.attempts[verificationAttemptKey(next)] != 0 {
		t.Fatal("failed cleanup contaminated the new version")
	}
	if err := m.verification.attempt(ctx, next); err != nil {
		t.Fatal("recovered session could not start its fresh window", err)
	}
	if state.attempts[verificationAttemptKey(next)] != 1 {
		t.Fatal("first recovered attempt did not begin at one")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, err := newRecoveryCodes()
	if err != nil || len(codes) != 10 {
		t.Fatal("recovery code generation failed")
	}
	seen := map[string]bool{}
	for _, code := range codes {
		digest := recoveryDigest(code)
		if len(digest) != 64 || seen[digest] || digest == code {
			t.Fatal("invalid recovery digest")
		}
		seen[digest] = true
		if recoveryDigest(" "+strings.ToUpper(code)+" ") != digest {
			t.Fatal("recovery normalization mismatch")
		}
	}
}
