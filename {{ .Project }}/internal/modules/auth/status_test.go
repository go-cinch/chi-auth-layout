package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

func TestPublicStatusSelectionDoesNotIssueChallenges(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	// A status read must not touch the challenge service, even above threshold.
	m.captcha.store = nil
	m.captcha.dictionary = nil
	for _, failures := range []int{0, 5} {
		mock.ExpectQuery("SELECT wrong FROM t_user").WithArgs("owner").WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(failures))
		result, err := m.PublicStatus(t.Context(), StatusQuery{Purpose: "login", Fields: report.Fields{"requirements"}, Identifier: "owner"})
		if err != nil || result.Requirements == nil || *result.Requirements.CaptchaRequired != (failures == 5) || result.EmailCode != nil {
			t.Fatal(result, err)
		}
		raw, _ := json.Marshal(result)
		var fields map[string]any
		_ = json.Unmarshal(raw, &fields)
		if len(fields) != 3 {
			t.Fatalf("unselected results returned: %s", raw)
		}
	}
	attachTestEmail(t, m)
	expectEmailStatus(mock, 123, true)
	result, err := m.PublicStatus(t.Context(), StatusQuery{Purpose: "login", Fields: report.Fields{"email_code"}, BrowserUUID: emailTestUUID, Identifier: "unknown", IP: "192.0.2.1"})
	if err != nil || result.Requirements != nil || result.EmailCode.RetryAt != 123 || result.EmailCode.VerificationID != "pending-id" {
		t.Fatal(result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStatusEnrollmentRestrictions(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	q := StatusQuery{Purpose: "sensitive", Fields: report.Fields{"methods"}}
	if _, err := m.Status(t.Context(), q); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	expectTOTPBound(mock, false)
	value, err := m.Status(ctx, q)
	if err != nil || value.Methods == nil || value.Methods.TOTP || value.Methods.Email {
		t.Fatal(value, err)
	}
	identity.PasswordResetRequired = true
	ctx = verificationContextWithIdentity(t, ctx, identity)
	expectTOTPBound(mock, true)
	if _, err := m.Status(ctx, q); !errors.Is(err, authn.ErrPasswordResetRequired) {
		t.Fatal(err)
	}
	expectTOTPBound(mock, false)
	q = StatusQuery{Purpose: "totp_enrollment", Fields: report.Fields{"requirements"}}
	result, err := m.Status(ctx, q)
	if err != nil || !*result.Requirements.EnrollmentRequired || !*result.Requirements.PasswordRequired || result.Grant != nil {
		t.Fatal(result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStatusUnlinkSelectedDependencies(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	attachTestEmail(t, m)
	m.emailCodes.config.UnlinkURL = "https://example.test/unlink"
	q := StatusQuery{Purpose: "email_unbind", Fields: report.Fields{"methods"}}
	mock.ExpectQuery("SELECT COALESCE.*CASE WHEN EXISTS").WillReturnRows(sqlmock.NewRows([]string{"email", "fallback"}).AddRow("owner@example.test", "password"))
	result, err := m.Status(ctx, q)
	if err != nil || result.EmailCode != nil || !result.Methods.Email || result.Methods.Fallback != "password" {
		t.Fatal(result, err)
	}
	q.Fields, q.BrowserUUID = report.Fields{"email_code"}, emailTestUUID
	// Polling reads only the mailbox, never the factor/history fallback query.
	mock.ExpectQuery("SELECT COALESCE\\(CASE WHEN email_verified_at").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(""))
	result, err = m.Status(ctx, q)
	if err != nil || result.Methods != nil || !result.EmailCode.Completed {
		t.Fatal(result, err)
	}
	q.Fields = report.Fields{"methods", "email_code"}
	mock.ExpectQuery("SELECT COALESCE.*CASE WHEN EXISTS").WillReturnRows(sqlmock.NewRows([]string{"email", "fallback"}).AddRow("owner@example.test", "verification"))
	mock.ExpectQuery("SELECT COALESCE\\(MAX").WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(123))
	mock.ExpectQuery("SELECT verification_id,expired_at").WillReturnError(sql.ErrNoRows)
	result, err = m.Status(ctx, q)
	if err != nil || result.EmailCode.Completed || result.EmailCode.RetryAt != 123 || !result.Methods.TOTP {
		t.Fatal(result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStatusMethodsAndGrantAreIndependent(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	q := StatusQuery{Purpose: "sensitive", Fields: report.Fields{"methods"}}
	result, err := m.Status(ctx, q)
	if err != nil || result.Methods.TOTP || result.Methods.Email || result.Grant != nil {
		t.Fatal(result, err)
	}
	attachTestEmail(t, m)
	mock.ExpectQuery("SELECT COALESCE\\(CASE WHEN email_verified_at").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("owner@example.test"))
	result, err = m.Status(ctx, q)
	if err != nil || !result.Methods.Email || result.Methods.Address != "ow***@example.test" {
		t.Fatal(result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInfoTOTPBindingDoesNotDependOnEnforcementOrRedis(t *testing.T) {
	m, mock, ctx, _, state := newVerificationTest(t)
	m.verification.config.TOTPEnabled = false
	state.fail = errors.New("redis offline")
	mock.ExpectQuery("SELECT u.id, EXISTS").WillReturnRows(sqlmock.NewRows([]string{"id", "bound"}).AddRow(3, true))
	value, err := m.InfoSelected(ctx, "admin", report.Fields{"totp"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(value)
	if string(raw) != `{"totp":{"bound":true}}` {
		t.Fatal(string(raw))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStatusErrorsAndBindingSelection(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	q := StatusQuery{Purpose: "email_bind", Fields: report.Fields{"email_code"}, BrowserUUID: emailTestUUID, Identifier: "owner@example.test"}
	if _, err := m.Status(ctx, q); !errors.Is(err, ErrEmailUnavailable) {
		t.Fatal(err)
	}
	attachTestEmail(t, m)
	expectEmailStatus(mock, 0, false)
	result, err := m.Status(ctx, q)
	if err != nil || result.EmailCode.VerificationID != "" || result.EmailCode.RetryAt != 0 || result.Methods != nil {
		t.Fatal(result, err)
	}
	for _, q := range []StatusQuery{
		{}, {Purpose: "login", Fields: report.Fields{"methods"}}, {Purpose: "email_bind", Fields: report.Fields{"email_code"}, BrowserUUID: "invalid"}} {
		if _, err := m.Status(ctx, q); err == nil {
			t.Fatal(q)
		}
		if _, err := m.PublicStatus(context.Background(), q); err == nil {
			t.Fatal(q)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStatusGrantSelectionAndExpiry(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	state.fail = errors.New("redis unavailable")
	expectTOTPBound(mock, true)
	result, err := m.Status(ctx, StatusQuery{Purpose: "sensitive", Fields: report.Fields{"methods"}})
	if err != nil || !result.Methods.TOTP || result.Grant != nil {
		t.Fatal(result, err)
	}
	expectTOTPBound(mock, true)
	if _, err = m.Status(ctx, StatusQuery{Purpose: "sensitive", Fields: report.Fields{"grant"}}); err == nil {
		t.Fatal("grant read ignored Redis failure")
	}
	state.fail = nil
	proof, err := m.grantSensitive(ctx, identity, "")
	if err != nil {
		t.Fatal(err)
	}
	before := state.values["sensitive:"+identity.SessionID]
	expectTOTPBound(mock, true)
	result, err = m.Status(ctx, StatusQuery{Purpose: "sensitive", Fields: report.Fields{"grant"}})
	if err != nil || result.Methods != nil || result.Grant.ExpiredAt != proof.ExpiredAt || state.values["sensitive:"+identity.SessionID] != before {
		t.Fatal(result, err)
	}
	proof.VerifiedAt = time.Now().Add(-2 * time.Hour).UnixMilli()
	proof.ExpiredAt = proof.VerifiedAt + m.verification.config.SensitiveLifetime.Milliseconds()
	if err = m.verification.put(ctx, "sensitive:"+identity.SessionID, proof, time.Hour); err != nil {
		t.Fatal(err)
	}
	expectTOTPBound(mock, true)
	result, err = m.Status(ctx, StatusQuery{Purpose: "sensitive", Fields: report.Fields{"grant"}})
	if err != nil || result.Grant.ExpiredAt != 0 {
		t.Fatal(result, err)
	}
}

func TestStatusChallengePurposeGate(t *testing.T) {
	m, mock, ctx, _, _ := newVerificationTest(t)
	// Shared challenge issuance remains usable before mandatory enrollment.
	response := emailHTTP(t, m, ctx, "POST", "/challenge", map[string]string{"purpose": "totp_setup"})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, purpose := range []string{"password_reset", "password_change", "email_bind", "email_unbind"} {
		expectTOTPBound(mock, false)
		response = emailHTTP(t, m, ctx, "POST", "/challenge", map[string]string{"purpose": purpose})
		if response.Code != 403 {
			t.Fatal(purpose, response.Code, response.Body.String())
		}
	}
	expectTOTPBound(mock, false)
	response = emailHTTP(t, m, ctx, "POST", "/challenge", map[string]string{"purpose": "sensitive"})
	if response.Code != 200 {
		t.Fatal("enrollment must allow mailbox proof", response.Code)
	}

}

func TestPublicStatusAndExplicitCaptchaIssuance(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	mock.ExpectQuery("SELECT wrong FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(5))
	response := emailHTTP(t, m, t.Context(), "POST", "/pub/captcha", map[string]string{"username": "owner"})
	var result LoginVerificationResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != 200 || !result.CaptchaRequired || result.Captcha == nil {
		t.Fatal(response.Code, result, err)
	}
	response = emailHTTP(t, m, t.Context(), "POST", "/pub/captcha", map[string]string{"username": "owner", "captcha_id": "old"})
	if response.Code != 400 {
		t.Fatal(response.Code)
	}
}

func TestOnboardingStatusOwnsSessionRequirements(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	identity.PasswordResetRequired = true
	ctx = verificationContextWithIdentity(t, ctx, identity)
	expectTOTPBound(mock, false)
	expectPasswordResetOrigin(mock, true)
	response := emailHTTP(t, m, ctx, "GET", "/status?purpose=onboarding&f=requirements", nil)
	var result StatusResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != 200 {
		t.Fatal(response.Code, err, response.Body.String())
	}
	if !*result.Requirements.PasswordResetRequired || !*result.Requirements.TOTPEnrollmentRequired || *result.Requirements.EmailBindingRequired {
		t.Fatal(result.Requirements)
	}
	for _, name := range []string{"X-Password-Reset-Required", "X-MFA-Enrollment-Required", "X-Verification-Required"} {
		if response.Header().Get(name) != "" {
			t.Fatal("status query returned obsolete restriction header", name)
		}
	}
	attachTestEmail(t, m)
	m.emailCodes.config.RequireBinding = true
	expectTOTPBound(mock, false)
	mock.ExpectQuery("SELECT COALESCE\\(email, ''\\) FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(""))
	expectPasswordResetOrigin(mock, true)
	resultPtr, err := m.Status(ctx, StatusQuery{Purpose: "onboarding", Fields: report.Fields{"requirements"}})
	if err != nil || !*resultPtr.Requirements.EmailBindingRequired {
		t.Fatal(resultPtr, err)
	}
	if response := emailHTTP(t, m, ctx, "GET", "/pub/status?purpose=onboarding&f=requirements", nil); response.Code != 400 {
		t.Fatal("public endpoint exposed onboarding", response.Code)
	}
}

func TestLoginRecoveryStatusReadOnlyAndEmailEligibility(t *testing.T) {
	for _, test := range []struct {
		name, email string
		delivery    bool
	}{
		{"unbound", "", true}, {"bound", "owner@example.test", true}, {"delivery-disabled", "owner@example.test", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, mock, ctx, identity, state := newVerificationTest(t)
			if test.delivery {
				attachTestEmail(t, m)
			}
			pending, err := m.beginMFALogin(ctx, identity, false)
			if err != nil {
				t.Fatal(err)
			}
			key := loginTicketPrefix + pending.MFATicket
			before, ttl := state.values[key], state.ttls[key]
			q := StatusQuery{Purpose: "login_totp_recovery", Fields: report.Fields{"methods"}, MFATicket: pending.MFATicket}
			for range 2 {
				mock.ExpectQuery("SELECT COALESCE").WithArgs(identity.UserID, identity.CredentialVersion, userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(test.email))
				expectTOTPBound(mock, true)
				result, err := m.PublicStatus(ctx, q)
				wantEmail := test.delivery && test.email != ""
				if err != nil || result.Methods == nil || result.Methods.Email != wantEmail || !result.Methods.RecoveryCode || result.Methods.TOTP {
					t.Fatal(result, err)
				}
				if wantEmail && result.Methods.Address != "ow***@example.test" {
					t.Fatal(result.Methods)
				}
				if !wantEmail && result.Methods.Address != "" {
					t.Fatal("disclosed disabled mailbox")
				}
			}
			if state.values[key] != before || state.ttls[key] != ttl || len(state.attempts) != 0 {
				t.Fatal("status mutated ticket or attempt budget")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoginRecoveryStatusRejectsStaleTicketsAndAccounts(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	q := StatusQuery{Purpose: "login_totp_recovery", Fields: report.Fields{"methods"}}
	if _, err := m.PublicStatus(ctx, q); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal(err)
	}
	pending, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	q.MFATicket = pending.MFATicket
	mock.ExpectQuery("SELECT COALESCE").WillReturnError(sql.ErrNoRows)
	if _, err := m.PublicStatus(ctx, q); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(""))
	expectTOTPBound(mock, false)
	if _, err := m.PublicStatus(ctx, q); !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatal(err)
	}
	key := loginTicketPrefix + q.MFATicket
	if err := m.verification.put(ctx, key, mfaPending{Identity: identity, ExpiredAt: time.Now().Add(-time.Second).UnixMilli()}, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PublicStatus(ctx, q); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal(err)
	}
	delete(state.values, key)
	if _, err := m.PublicStatus(ctx, q); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMailboxCooldownIsReadOnlyAndIncludesRecipient(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	attachTestEmail(t, m)
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("owner@example.test"))
	keys := m.emailCodes.cooldownKeys(emailTestUUID, "192.0.2.1", "owner@example.test")
	mock.ExpectQuery("SELECT COALESCE\\(MAX").WithArgs(pq.Array(keys)).WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(61000))
	result, err := m.Status(ctx, StatusQuery{Purpose: "sensitive", Fields: report.Fields{"email_code"}, BrowserUUID: emailTestUUID, IP: "192.0.2.1"})
	if err != nil || result.EmailCode == nil || result.EmailCode.RetryAt != 61000 || result.EmailCode.VerificationID != "" || result.Methods != nil || result.Grant != nil {
		t.Fatal(result, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginRecoveryCooldownPreservesTicketAndAttempts(t *testing.T) {
	m, mock, ctx, identity, state := newVerificationTest(t)
	attachTestEmail(t, m)
	pending, err := m.beginMFALogin(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	key := loginTicketPrefix + pending.MFATicket
	before, ttl := state.values[key], state.ttls[key]
	mock.ExpectQuery("SELECT COALESCE").WithArgs(identity.UserID, identity.CredentialVersion, userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("owner@example.test"))
	expectTOTPBound(mock, true)
	mock.ExpectQuery("SELECT COALESCE\\(MAX").WithArgs(pq.Array(m.emailCodes.cooldownKeys(emailTestUUID, "192.0.2.1", "owner@example.test"))).WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(61000))
	result, err := m.PublicStatus(ctx, StatusQuery{Purpose: "login_totp_recovery", Fields: report.Fields{"email_code"}, BrowserUUID: emailTestUUID, IP: "192.0.2.1", MFATicket: pending.MFATicket})
	if err != nil || result.Methods != nil || result.EmailCode == nil || result.EmailCode.RetryAt != 61000 || result.EmailCode.VerificationID != "" {
		t.Fatal(result, err)
	}
	if state.values[key] != before || state.ttls[key] != ttl || len(state.attempts) != 0 {
		t.Fatal("cooldown query changed ticket or attempts")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyEmailFormsReadOnlyBrowserIPCooldown(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	attachTestEmail(t, m)
	for _, purpose := range []string{"login", "email_bind"} {
		mock.ExpectQuery("SELECT COALESCE\\(MAX").WithArgs(pq.Array(m.emailCodes.cooldownKeys(emailTestUUID, "192.0.2.1", ""))).WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(61000))
		q := StatusQuery{Purpose: purpose, Fields: report.Fields{"email_code"}, BrowserUUID: emailTestUUID, IP: "192.0.2.1"}
		var result *StatusResult
		var err error
		if purpose == "login" {
			result, err = m.PublicStatus(ctx, q)
		} else {
			result, err = m.Status(ctx, q)
		}
		if err != nil || result.EmailCode.RetryAt != 61000 || result.EmailCode.VerificationID != "" {
			t.Fatal(result, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
