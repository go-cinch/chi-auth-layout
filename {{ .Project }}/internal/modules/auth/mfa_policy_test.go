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

func TestPasswordChangeRequiresSensitiveMFAUnderAuthPrefixes(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		t.Run(prefix, func(t *testing.T) {
			m, mock, ctx, identity, state := newMFATest(t)
			m.httpPath = prefix
			auditCtx, auditState := auditctx.WithState(ctx)
			r := httptest.NewRequest(http.MethodPatch, prefix+"/change/pwd", nil).WithContext(auditCtx)
			// A bound factor without a sensitive grant still requires verification.
			expectMFAEnabled(mock, true)
			err := m.CheckHTTPSecurity(r)
			if !errors.Is(err, ErrMFARequired) {
				t.Fatal("password change bypassed sensitive verification", err)
			}
			out := httptest.NewRecorder()
			server.WriteHTTPSecurityError(out, r, err)
			if out.Code != http.StatusForbidden || out.Header().Get(authn.MFARequiredHeader) != "true" {
				t.Fatal("password change did not request the shared MFA dialog", out.Code)
			}
			secret, err := newTOTPSecret()
			if err != nil {
				t.Fatal(err)
			}
			code, err := totpCode(secret, time.Now().Unix()/30)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(authn.MFACodeHeader, code)
			expectMFAEnabled(mock, true)
			expectMFALock(mock, 1)
			expectMFAFactor(t, m, mock, secret, -1)
			mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if err := m.CheckHTTPSecurity(r); err != nil {
				t.Fatal("same-endpoint MFA retry failed", err)
			}
			if event := auditState.Snapshot(); event.MFAFingerprint == "" || event.MFAVerifiedAt == 0 {
				t.Fatal("password change lacks sensitive verification audit metadata")
			}
			before := state.values["sensitive:"+identity.SessionID]
			r.Header.Del(authn.MFACodeHeader)
			expectMFAEnabled(mock, true)
			if err := m.CheckHTTPSecurity(r); err != nil {
				t.Fatal("same-session grant was not reused", err)
			}
			if state.values["sensitive:"+identity.SessionID] != before || state.attempts[mfaAttemptKey(identity)] != 1 {
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
		m, mock, ctx, _, _ := newMFATest(t)
		m.httpPath = prefix
		for _, endpoint := range []struct{ method, path string }{
			{http.MethodPatch, "/reset/pwd"},
			{http.MethodPost, "/challenge"},
			{http.MethodGet, "/info"},
		} {
			expectMFAEnabled(mock, true)
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

func TestAuthMFAUsesOwnedRoutesUnderBusinessMounts(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		for _, bound := range []bool{false, true} {
			m, mock, ctx, _, state := newMFATest(t)
			m.httpPath = prefix
			for _, endpoint := range []struct{ method, path string }{
				{http.MethodGet, "/mfa"},
				{http.MethodPost, "/mfa/challenge"},
				{http.MethodPost, "/mfa/setup"},
				{http.MethodPost, "/mfa/confirm"},
				{http.MethodPost, "/logout"},
			} {
				expectMFAEnabled(mock, bound)
				r := httptest.NewRequest(endpoint.method, prefix+endpoint.path, nil).WithContext(ctx)
				if err := m.CheckHTTPSecurity(r); err != nil {
					t.Fatalf("bound=%t %s %s: %v", bound, endpoint.method, r.URL.Path, err)
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
				{http.MethodPost, "/mfa/test", nil},
				{http.MethodPatch, "/change/pwd", ErrMFARequired},
				{http.MethodPost, "/mfa/reset/123", ErrMFARequired},
			} {
				want := endpoint.want
				if !bound {
					want = ErrMFAEnrollmentRequired
				}
				expectMFAEnabled(mock, bound)
				r := httptest.NewRequest("GET", prefix+"/permission", nil).WithContext(ctx)
				if err := m.checkMFARequest(r, endpoint.method, prefix+endpoint.path); !errors.Is(err, want) {
					t.Fatalf("bound=%t gateway %s %s: %v, want %v", bound, endpoint.method, prefix+endpoint.path, err, want)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAuthMFAOwnershipDoesNotExemptForeignMethodsOrPaths(t *testing.T) {
	m, mock, ctx, _, _ := newMFATest(t)
	m.httpPath = "/user/auth"
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodDelete, "/user/auth/mfa/test"},
		{http.MethodPost, "/user/auth/info"},
		{http.MethodPost, "/user/auth/mfa/setup/other"},
		{http.MethodPost, "/user/auth/other"},
		{http.MethodPost, "/user/another/mfa/setup"},
	} {
		expectMFAEnabled(mock, true)
		if err := m.checkMFATarget(ctx, endpoint.method, endpoint.path); !errors.Is(err, ErrMFARequired) {
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
	for _, path := range []string{"/user-other", "/role-other", "/pub/login", "/mfa/verify", "/mfa/test", "/msg"} {
		if SensitiveHTTP("POST", path) {
			t.Errorf("unrelated write protected: %s", path)
		}
	}
}

func TestMFATestPolicyNeverUsesSensitiveGrantUnderAuthPrefixes(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/user/auth", "/dictionary/auth"} {
		t.Run(prefix, func(t *testing.T) {
			m, mock, ctx, _, state := newMFATest(t)
			m.httpPath = prefix
			m.mfa.state = &testMFAAttemptOnlyStore{testMFAStore: state, t: t}
			auditCtx, auditState := auditctx.WithState(ctx)
			r := httptest.NewRequest(http.MethodPost, prefix+"/mfa/test", nil).WithContext(auditCtx)
			// A caller-supplied step-up code must not authorize a testing request.
			r.Header.Set(authn.MFACodeHeader, "123456")
			expectMFAEnabled(mock, true)
			if err := m.CheckHTTPSecurity(r); err != nil {
				t.Fatal("test endpoint treated as sensitive", err)
			}
			if len(state.attempts) != 0 {
				t.Fatal("security gate consumed the header code before the body test")
			}
			if event := auditState.Snapshot(); event.MFAFingerprint != "" || event.MFAVerifiedAt != 0 {
				t.Fatal("test gate added sensitive audit metadata")
			}
			expectMFAEnabled(mock, false)
			if err := m.CheckHTTPSecurity(r); !errors.Is(err, ErrMFAEnrollmentRequired) {
				t.Fatal("test bypassed enrollment restriction", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Only the owned, exact POST route is exempt from sensitive classification.
	m, mock, ctx, _, _ := newMFATest(t)
	m.httpPath = "/auth"
	expectMFAEnabled(mock, true)
	if err := m.CheckHTTPSecurity(httptest.NewRequest(http.MethodPost, "/user/auth/mfa/test", nil).WithContext(ctx)); !errors.Is(err, ErrMFARequired) {
		t.Fatal("foreign test-shaped business route bypassed sensitive verification", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
