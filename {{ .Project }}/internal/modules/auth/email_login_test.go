package auth

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestEmailLoginRetainsMFAAndPasswordBoundEnrollment(t *testing.T) {
	for _, tc := range []struct {
		name           string
		enabled, bound bool
	}{
		{"disabled", false, false},
		{"unbound-factor", true, false},
		{"bound-factor", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock, _, _, _ := newVerificationTest(t)
			m.verification.config.TOTPEnabled = tc.enabled
			attachTestEmail(t, m)
			mock.ExpectQuery("SELECT username FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("readonly"))
			mock.ExpectQuery("SELECT id, username").WillReturnRows(loginRows(t, userStatusActive))
			mock.ExpectQuery("SELECT COALESCE\\(email").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("test@example.com"))
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT code_hash, attempts").WillReturnRows(sqlmock.NewRows([]string{"hash", "attempts", "expiry", "delivered", "consumed"}).AddRow(m.emailCodes.digest("verification", "123456"), 0, time.Now().Add(time.Minute).UnixMilli(), true, false))
			mock.ExpectExec("UPDATE t_auth_email_code").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if tc.enabled {
				expectTOTPBound(mock, tc.bound)
			}
			if !tc.bound {
				mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(1))
			}
			result, err := m.Login(t.Context(), LoginInput{Method: "email_code", Username: "test@example.com", BrowserUUID: emailTestUUID, VerificationID: "verification", VerificationCode: "123456"})
			if err != nil {
				t.Fatal(err)
			}
			if !tc.bound {
				identity, err := m.authenticator.Parse(result.AccessToken)
				if err != nil || identity.Methods != (authn.Methods{Primary: authn.EmailCode}) {
					t.Fatal("wrong authentication methods for email login", identity.Methods, err)
				}
			}
			if !tc.enabled {
				if result.AccessToken == "" || result.MFARequired || result.TOTPEnrollmentRequired || result.MFATicket != "" {
					t.Fatal("disabled Verification interrupted email-code login")
				}
			} else if tc.bound {
				if !result.MFARequired || result.MFATicket == "" || result.AccessToken != "" {
					t.Fatal("email code bypassed required Verification")
				}
				if _, err := m.RecoverTOTPLogin(t.Context(), result.MFATicket, "unused-code"); !errors.Is(err, ErrPasswordVerificationRequired) {
					t.Fatal("email-only ticket authorized recovery", err)
				}
				secret, err := newTOTPSecret()
				if err != nil {
					t.Fatal(err)
				}
				code, err := totpCode(secret, time.Now().Unix()/30)
				if err != nil {
					t.Fatal(err)
				}
				expectVerificationLock(mock, 1)
				expectTOTPFactor(t, m, mock, secret, -1)
				mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
				mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(1))
				completed, err := m.CompleteTOTPLogin(t.Context(), result.MFATicket, code)
				if err != nil {
					t.Fatal(err)
				}
				identity, err := m.authenticator.Parse(completed.AccessToken)
				if err != nil || identity.Methods != (authn.Methods{Primary: authn.EmailCode, SecondFactor: authn.TOTP}) {
					t.Fatal("email/TOTP continuation lost authentication methods", identity.Methods, err)
				}
			} else {
				if !result.TOTPEnrollmentRequired {
					t.Fatal("email login bypassed mandatory enrollment")
				}
				identity, err := m.authenticator.Parse(result.AccessToken)
				if err != nil {
					t.Fatal(err)
				}
				if allowed, err := m.canEnrollWithoutPassword(t.Context(), identity); err != nil || allowed {
					t.Fatal("email login granted password-only enrollment privilege", allowed, err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEmailLoginInvalidAndAmbiguousAliases(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	attachTestEmail(t, m)
	for _, count := range []int{0, 2} {
		rows := sqlmock.NewRows([]string{"username"})
		for i := 0; i < count; i++ {
			rows.AddRow("candidate")
		}
		mock.ExpectQuery("SELECT username FROM t_user").WillReturnRows(rows)
		if _, err := m.resolveLoginUsername(t.Context(), "test@example.com"); !errors.Is(err, ErrLoginFailed) {
			t.Fatal("ambiguous identifier was resolved", count, err)
		}
	}
	mock.ExpectQuery("SELECT username FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("readonly"))
	mock.ExpectQuery("SELECT id, username").WillReturnRows(loginRows(t, userStatusPending))
	if _, err := m.Login(t.Context(), LoginInput{Method: "email_code", Username: "readonly"}); !errors.Is(err, ErrEmailInvalid) {
		t.Fatal("inactive account accepted", err)
	}
	ctx := authenticatedContext(t, manager, m.sessions)
	m.emailCodes.config.RequireBinding = true
	mock.ExpectQuery("SELECT COALESCE\\(email").WillReturnRows(sqlmock.NewRows([]string{"email", "email_verified_at"}).AddRow("", nil))
	if err := m.checkEmailTarget(ctx, "GET", "/user"); !errors.Is(err, ErrEmailBindingRequired) {
		t.Fatal("business request bypassed binding", err)
	}
	if err := m.checkEmailTarget(ctx, "GET", "/info"); err != nil {
		t.Fatal("restricted info unavailable", err)
	}
	if err := m.checkEmailTarget(ctx, "GET", "/email/code/status"); err != nil {
		t.Fatal("restricted countdown unavailable", err)
	}
	if err := m.checkEmailTarget(context.Background(), "GET", "/user"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("anonymous protected call", err)
	}
	mock.ExpectQuery("SELECT COALESCE\\(email").WillReturnRows(sqlmock.NewRows([]string{"email", "email_verified_at"}).AddRow("test@example.com", 1))
	if err := m.checkEmailTarget(ctx, "GET", "/user"); err != nil {
		t.Fatal("bound account blocked", err)
	}
}

func TestEmailCodeLoginJWEIsOneUse(t *testing.T) {
	credentials := newCredentialTestManager(t, time.Minute)
	challenge, err := credentials.Challenge(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	payload := loginCredential{ChallengeID: challenge.ChallengeID, Method: "email_code", Username: "test@example.com", BrowserUUID: emailTestUUID, VerificationID: "verification", VerificationCode: "123456"}
	encrypted := EncryptedLoginInput{ChallengeID: challenge.ChallengeID, Credential: encryptLoginCredential(t, credentials, payload, true)}
	opened, err := credentials.Open(t.Context(), encrypted)
	if err != nil || opened.Method != "email_code" || opened.VerificationCode != "123456" || opened.Password != "" {
		t.Fatal("email JWE invalid", err)
	}
	if _, err := credentials.Open(t.Context(), encrypted); !errors.Is(err, ErrInvalidCredential) {
		t.Fatal("reused JWE accepted", err)
	}
}
