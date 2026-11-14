package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

func TestPasswordChangeRequiresSensitiveVerificationUnderAuthPrefixes(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		t.Run(prefix, func(t *testing.T) {
			m, mock, ctx, identity, state := newVerificationTest(t)
			m.httpPath = prefix
			auditCtx, auditState := auditctx.WithState(ctx)
			r := httptest.NewRequest(http.MethodPatch, prefix+"/change/pwd", nil).WithContext(auditCtx)
			// A bound factor without a sensitive grant still requires verification.
			expectTOTPBound(mock, true)
			err := m.CheckHTTPSecurity(r)
			if !errors.Is(err, ErrStepUpRequired) {
				t.Fatal("password change bypassed sensitive verification", err)
			}
			out := httptest.NewRecorder()
			server.WriteHTTPSecurityError(out, r, err)
			if out.Code != http.StatusForbidden || out.Header().Get(authn.VerificationRequiredHeader) != "true" {
				t.Fatal("password change did not request the shared Verification dialog", out.Code)
			}
			secret, err := newTOTPSecret()
			if err != nil {
				t.Fatal(err)
			}
			code, err := totpCode(secret, time.Now().Unix()/30)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(authn.TOTPCodeHeader, code)
			r.Header.Set(authn.StepUpChallengeHeader, "mixed-proof")
			expectTOTPBound(mock, true)
			if err := m.CheckHTTPSecurity(r); !errors.Is(err, ErrVerificationInvalid) {
				t.Fatal("mixed authenticator and email proofs accepted", err)
			}
			r.Header.Del(authn.StepUpChallengeHeader)
			expectTOTPBound(mock, true)
			expectVerificationLock(mock, 1)
			expectTOTPFactor(t, m, mock, secret, -1)
			mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if err := m.CheckHTTPSecurity(r); err != nil {
				t.Fatal("same-endpoint Verification retry failed", err)
			}
			if event := auditState.Snapshot(); event.StepUpFingerprint == "" || event.StepUpVerifiedAt == 0 {
				t.Fatal("password change lacks sensitive verification audit metadata")
			}
			before := state.values["sensitive:"+identity.SessionID]
			r.Header.Del(authn.TOTPCodeHeader)
			expectTOTPBound(mock, true)
			if err := m.CheckHTTPSecurity(r); err != nil {
				t.Fatal("same-session grant was not reused", err)
			}
			if state.values["sensitive:"+identity.SessionID] != before || state.attempts[verificationAttemptKey(identity)] != 1 {
				t.Fatal("password change renewed the grant or consumed another code")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPasswordChangePolicyLeavesOnboardingAndChallengeUnchanged(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		m, mock, ctx, _, _ := newVerificationTest(t)
		m.httpPath = prefix
		for _, endpoint := range []struct{ method, path string }{
			{http.MethodPatch, "/reset/pwd"},
			{http.MethodPost, "/challenge"},
			{http.MethodGet, "/info"},
		} {
			expectTOTPBound(mock, true)
			r := httptest.NewRequest(endpoint.method, prefix+endpoint.path, nil).WithContext(ctx)
			if err := m.CheckHTTPSecurity(r); err != nil {
				t.Fatalf("%s %s unexpectedly requires step up: %v", endpoint.method, r.URL.Path, err)
			}
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAuthVerificationUsesOwnedRoutesUnderBusinessMounts(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		for _, bound := range []bool{false, true} {
			m, mock, ctx, _, state := newVerificationTest(t)
			m.httpPath = prefix
			for _, endpoint := range []struct{ method, path string }{
				{http.MethodGet, "/totp"},
				{http.MethodPost, "/totp/challenge"},
				{http.MethodPost, "/totp/setup"},
				{http.MethodPost, "/totp/confirm"},
				{http.MethodPost, "/logout"},
			} {
				expectTOTPBound(mock, bound)
				r := httptest.NewRequest(endpoint.method, prefix+endpoint.path, nil).WithContext(ctx)
				var want error
				if bound && (endpoint.path == "/totp/setup" || endpoint.path == "/totp/confirm") { want = ErrStepUpRequired }
				if err := m.CheckHTTPSecurity(r); !errors.Is(err, want) {
					t.Fatalf("bound=%t %s %s: %v, want %v", bound, endpoint.method, r.URL.Path, err, want)
				}
			}
			if len(state.values) != 0 || len(state.attempts) != 0 {
				t.Fatal("onboarding/logout created sensitive state or consumed a code")
			}
			// The gateway uses the same target policy. A required reset is allowed
			// only after binding; administrator factor reset remains sensitive.
			for _, endpoint := range []struct {
				method, path string
				want         error
			}{
				{http.MethodPatch, "/reset/pwd", nil},
				{http.MethodPost, "/challenge", nil},
				{http.MethodPost, "/totp/test", nil},
				{http.MethodPatch, "/change/pwd", ErrStepUpRequired},
				{http.MethodPost, "/totp/reset/123", ErrStepUpRequired},
			} {
				want := endpoint.want
				if !bound && endpoint.path != "/challenge" {
					want = ErrTOTPEnrollmentRequired
				}
				expectTOTPBound(mock, bound)
				r := httptest.NewRequest("GET", prefix+"/permission", nil).WithContext(ctx)
				if err := m.checkSensitiveRequest(r, endpoint.method, prefix+endpoint.path); !errors.Is(err, want) {
					t.Fatalf("bound=%t gateway %s %s: %v, want %v", bound, endpoint.method, prefix+endpoint.path, err, want)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAuthVerificationOwnershipDoesNotExemptForeignMethodsOrPaths(t *testing.T) {
	m, mock, ctx, _, _ := newVerificationTest(t)
	m.httpPath = "/user/auth"
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodDelete, "/user/auth/totp/test"},
		{http.MethodPost, "/user/auth/info"},
		{http.MethodPost, "/user/auth/totp/setup/other"},
		{http.MethodPost, "/user/auth/other"},
		{http.MethodPost, "/user/another/totp/setup"},
	} {
		expectTOTPBound(mock, true)
		if err := m.checkSensitiveTarget(ctx, endpoint.method, endpoint.path); !errors.Is(err, ErrStepUpRequired) {
			t.Fatalf("foreign route bypassed step-up: %s %s: %v", endpoint.method, endpoint.path, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSensitiveHTTPPolicy(t *testing.T) {
	for _, path := range []string{"/user", "/role", "/user-group", "/dictionary", "/action"} {
		for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
			if !SensitiveHTTP(method, path) || !SensitiveHTTP(method, path+"/123") {
				t.Errorf("%s %s is not protected", method, path)
			}
		}
		if SensitiveHTTP("GET", path) {
			t.Errorf("read requires step up: %s", path)
		}
	}
	for _, path := range []string{"/msg/inbox/1", "/msg/1", "/future-module/1"} {
		if !SensitiveHTTP("DELETE", path) {
			t.Errorf("delete bypass: %s", path)
		}
	}
	for _, path := range []string{"/user-other", "/role-other", "/pub/login", "/totp/verify", "/totp/test", "/msg"} {
		if SensitiveHTTP("POST", path) {
			t.Errorf("unrelated write protected: %s", path)
		}
	}
}

func TestTOTPTestPolicyNeverUsesSensitiveGrantUnderAuthPrefixes(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		t.Run(prefix, func(t *testing.T) {
			m, mock, ctx, _, state := newVerificationTest(t)
			m.httpPath = prefix
			m.verification.state = &testVerificationAttemptOnlyStore{testVerificationStore: state, t: t}
			auditCtx, auditState := auditctx.WithState(ctx)
			r := httptest.NewRequest(http.MethodPost, prefix+"/totp/test", nil).WithContext(auditCtx)
			// A caller-supplied step-up code must not authorize a testing request.
			r.Header.Set(authn.TOTPCodeHeader, "123456")
			expectTOTPBound(mock, true)
			if err := m.CheckHTTPSecurity(r); err != nil {
				t.Fatal("test endpoint treated as sensitive", err)
			}
			if len(state.attempts) != 0 {
				t.Fatal("security gate consumed the header code before the body test")
			}
			if event := auditState.Snapshot(); event.StepUpFingerprint != "" || event.StepUpVerifiedAt != 0 {
				t.Fatal("test gate added sensitive audit metadata")
			}
			expectTOTPBound(mock, false)
			if err := m.CheckHTTPSecurity(r); !errors.Is(err, ErrTOTPEnrollmentRequired) {
				t.Fatal("test bypassed enrollment restriction", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Only the owned, exact POST route is exempt from sensitive classification.
	m, mock, ctx, _, _ := newVerificationTest(t)
	m.httpPath = "/auth"
	expectTOTPBound(mock, true)
	if err := m.CheckHTTPSecurity(httptest.NewRequest(http.MethodPost, "/user/auth/totp/test", nil).WithContext(ctx)); !errors.Is(err, ErrStepUpRequired) {
		t.Fatal("foreign test-shaped business route bypassed sensitive verification", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFirstFactorSessionPolicyAfterEnablingTOTP(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	identity.Methods = authn.Methods{Primary: authn.EmailCode}
	ctx = verificationContextWithIdentity(t, ctx, identity)
	request := httptest.NewRequest(http.MethodGet, "/info", nil).WithContext(ctx)
	for _, bound := range []bool{false, true} {
		want := ErrTOTPEnrollmentRequired
		if bound {
			want = ErrUnauthorized
		}
		expectTOTPBound(mock, bound)
		if err := m.CheckHTTPSecurity(request); !errors.Is(err, want) {
			t.Fatalf("HTTP bound=%t: got %v, want %v", bound, err, want)
		}
		expectTOTPBound(mock, bound)
		_, allowed, err := m.CheckPermission(ctx, PermissionTarget{Resource: "/orders.Order/List"})
		if allowed || !errors.Is(err, want) {
			t.Fatalf("RPC bound=%t: allowed=%t, err=%v", bound, allowed, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshCannotUpgradeFirstFactorSession(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	identity.Methods = authn.Methods{Primary: authn.EmailCode}
	session, err := m.sessions.Issue(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count").WillReturnRows(
		sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).
			AddRow(identity.Username, identity.Code, userStatusActive, identity.CredentialVersion, 1),
	)
	expectTOTPBound(mock, true)
	if result, err := m.Refresh(ctx, session.RefreshToken); result != nil || !errors.Is(err, ErrUnauthorized) {
		t.Fatal("first-factor refresh authorized a bound account", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// Credential binding must be protected on both direct and gateway paths.
func TestCredentialBindingRequiresExistingFactor(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth"} {
		for _, endpoint := range []string{"/totp/setup", "/totp/confirm", "/email/bind"} {
			t.Run(prefix+endpoint, func(t *testing.T) {
				m, mock, ctx, identity, state := newVerificationTest(t)
				m.httpPath = prefix
				r := httptest.NewRequest(http.MethodPost, prefix+endpoint, nil).WithContext(ctx)
				expectTOTPBound(mock, true)
				if err := m.CheckHTTPSecurity(r); !errors.Is(err, ErrStepUpRequired) { t.Fatalf("direct request bypass: %v", err) }
				expectTOTPBound(mock, true)
				if err := m.checkSensitiveRequest(r, http.MethodPost, prefix+endpoint); !errors.Is(err, ErrStepUpRequired) { t.Fatalf("gateway request bypass: %v", err) }
				if _, err := m.grantSensitive(ctx, identity, ""); err != nil { t.Fatal(err) }
				before := state.values["sensitive:"+identity.SessionID]
				expectTOTPBound(mock, true)
				if err := m.CheckHTTPSecurity(r); err != nil { t.Fatalf("valid grant rejected: %v", err) }
				if state.values["sensitive:"+identity.SessionID] != before { t.Fatal("grant was extended") }
				if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
			})
		}
	}
}

func TestEnrollmentRequiresExistingEmailButAllowsFirstFactor(t *testing.T) {
	for _, endpoint := range []string{"/totp/setup", "/totp/confirm"} {
		for _, address := range []string{"", "owner@example.test"} {
			t.Run(endpoint+address, func(t *testing.T) {
				m, mock, ctx, identity, _ := newVerificationTest(t)
				attachTestEmail(t, m)
				binding := func() { mock.ExpectQuery("SELECT COALESCE").WithArgs(identity.UserID, identity.CredentialVersion, userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(address)) }
				r := httptest.NewRequest(http.MethodPost, endpoint, nil).WithContext(ctx)
				expectTOTPBound(mock, false)
				binding()
				var want error
				if address != "" { want = ErrStepUpRequired }
				if err := m.CheckHTTPSecurity(r); !errors.Is(err, want) { t.Fatalf("email=%q: %v, want %v", address, err, want) }
				if address != "" {
					if _, err := m.grantSensitive(ctx, identity, m.emailCodes.digest("sensitive-binding", address)); err != nil { t.Fatal(err) }
					expectTOTPBound(mock, false)
					binding()
					if err := m.CheckHTTPSecurity(r); err != nil { t.Fatal("verified mailbox did not allow enrollment", err) }
				}
				if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
			})
		}
	}
}

func TestMailDraftsDoNotRequireSensitiveVerification(t *testing.T) {
	for _, endpoint := range []struct {
		method, target string
		sensitive bool
	}{
		{http.MethodPost, "/mail-template", false},
		{http.MethodPost, "/mail-template/", false},
		{http.MethodPatch, "/mail-template/7", false},
		{http.MethodPatch, "/mail-template/7/", false},
		{http.MethodGet, "/mail-template/7", false},
		{http.MethodPost, "/mail-template/7/publish", true},
		{http.MethodPost, "/mail-template/7/test-send", true},
		{http.MethodDelete, "/mail-template/7", true},
		{http.MethodDelete, "/mail-template", true},
		{http.MethodPatch, "/mail-template/7/publish", true},
		{http.MethodPost, "/mail-template/7/other", true},
	} {
		t.Run(endpoint.method+endpoint.target, func(t *testing.T) {
			m, mock, ctx, _, state := newVerificationTest(t)
			expectTOTPBound(mock, true)
			r := httptest.NewRequest(endpoint.method, endpoint.target, nil).WithContext(ctx)
			var want error
			if endpoint.sensitive { want = ErrStepUpRequired }
			if err := m.CheckHTTPSecurity(r); !errors.Is(err, want) {
				t.Fatalf("direct request: got %v, want %v", err, want)
			}
			expectTOTPBound(mock, true)
			if err := m.checkSensitiveRequest(r, endpoint.method, endpoint.target); !errors.Is(err, want) {
				t.Fatalf("gateway target: got %v, want %v", err, want)
			}
			if len(state.values) != 0 || len(state.attempts) != 0 {
				t.Fatal("draft or unverified request created sensitive state")
			}
			if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
		})
	}
}
