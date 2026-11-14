package tests

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
)

func TestOptionalOnboardingPersistsAndReenforcesPolicy(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	if _, err := store.DB.ExecContext(ctx, `INSERT INTO t_user (id,username,code,password,status) VALUES (101,'onboarding-user','ONBOARD1','unchanged-password-hash',1)`); err != nil {
		t.Fatal(err)
	}
	memory := auth.NewMemorySessionStore()
	t.Cleanup(func() { _ = memory.Close() })
	key := strings.Repeat("k", 64)
	sessions, err := auth.NewSessions(memory, auth.SessionConfig{Issuer: "onboarding-test", Key: key, SessionLifetime: time.Hour, RememberLifetime: time.Hour, RetryWindow: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := authn.New("onboarding-test", key, time.Hour, auth.NewIdentityValidator(store, sessions))
	if err != nil {
		t.Fatal(err)
	}
	cfg := auth.VerificationConfig{TOTPEnabled: true, TOTPEnrollmentOptional: true, Issuer: "onboarding-test", SigningKey: key, LoginTicketTTL: time.Minute, TOTPPeriod: 30 * time.Second, TOTPEnrollmentTTL: time.Minute, SensitiveLifetime: time.Hour, RateLimitWindow: time.Minute, RateLimitMaxAttempts: 10}
	factor, err := auth.NewVerification(store, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	module := auth.New(store, manager, nil, sessions, nil, nil, nil, nil, nil, factor)
	module.SetPasswordResetRequired(false)
	identity := authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID: 101, Username: "onboarding-user", Code: "ONBOARD1", CredentialVersion: 1}
	session, err := sessions.Issue(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = session.SessionID
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	current := func(path string) (*auth.StatusResult, error) {
		request := httptest.NewRequest("GET", path, nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		verified, err := manager.AuthenticateRequest(request)
		if err != nil {
			return nil, err
		}
		return module.Status(verified.Context(), auth.StatusQuery{Purpose: "onboarding", Fields: report.Fields{"requirements"}})
	}
	skip := func(step auth.OnboardingPreferences) error {
		request := httptest.NewRequest("PATCH", "/info", nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		verified, err := manager.AuthenticateRequest(request)
		if err != nil {
			return err
		}
		if err := module.CheckHTTPSecurity(verified); err != nil {
			return err
		}
		return module.SkipOnboarding(verified.Context(), step)
	}
	status, err := current("/status")
	if err != nil || !*status.Requirements.TOTPSkipAllowed || !*status.Requirements.PasswordResetSkipAllowed {
		t.Fatal(status, err)
	}
	if err := skip(auth.OnboardingPreferences{PasswordResetSkipped: true}); !errors.Is(err, auth.ErrTOTPEnrollmentRequired) {
		t.Fatal(err)
	}
	if err := skip(auth.OnboardingPreferences{TOTPEnrollmentSkipped: true}); err != nil {
		t.Fatal(err)
	}
	status, err = current("/status")
	if err != nil || *status.Requirements.TOTPEnrollmentRequired || !*status.Requirements.PasswordResetRequired {
		t.Fatal(status, err)
	}
	if err := skip(auth.OnboardingPreferences{PasswordResetSkipped: true}); err != nil {
		t.Fatal(err)
	}
	if err := skip(auth.OnboardingPreferences{PasswordResetSkipped: true}); err != nil {
		t.Fatal("retry", err)
	}
	status, err = current("/info")
	if err != nil || *status.Requirements.PasswordResetRequired || *status.Requirements.TOTPEnrollmentRequired {
		t.Fatal(status, err)
	}
	var count, version int64
	var hash string
	if err := store.DB.QueryRowContext(ctx, `SELECT login_count,credential_version,password FROM t_user WHERE id=101`).Scan(&count, &version, &hash); err != nil {
		t.Fatal(err)
	}
	if count != 1 || version != 1 || hash != "unchanged-password-hash" {
		t.Fatal("skip changed credentials or counted retries")
	}
	// A new login session and a refresh both read server-side preferences; no
	// browser storage or previous onboarding state is available to these requests.
	for range 2 {
		if err := sessions.Revoke(ctx, session.SessionID); err != nil {
			t.Fatal(err)
		}
		session, err = sessions.Issue(ctx, identity, false)
		if err != nil {
			t.Fatal(err)
		}
		identity.SessionID = session.SessionID
		token, _, err = manager.Issue(identity)
		if err != nil {
			t.Fatal(err)
		}
		status, err = current("/status")
		if err != nil || *status.Requirements.TOTPEnrollmentRequired || *status.Requirements.PasswordResetRequired {
			t.Fatal("new session repeated onboarding", status, err)
		}
		refreshed, err := module.Refresh(ctx, session.RefreshToken)
		if err != nil {
			t.Fatal(err)
		}
		token = refreshed.AccessToken
		request := httptest.NewRequest("GET", "/info?f=metadata", nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		verified, err := manager.AuthenticateRequest(request)
		if err != nil {
			t.Fatal(err)
		}
		info, err := module.InfoSelected(verified.Context(), "admin", report.Fields{"metadata"})
		if err != nil || info.Metadata == nil || !info.Metadata.TOTPEnrollmentSkipped || !info.Metadata.PasswordResetSkipped || info.Metadata.EmailBindingSkipped {
			t.Fatal("saved onboarding preferences were not loaded", info, err)
		}
		status, err = current("/status")
		if err != nil || *status.Requirements.TOTPEnrollmentRequired || *status.Requirements.PasswordResetRequired {
			t.Fatal("refresh repeated onboarding", status, err)
		}
	}

	cfg.TOTPEnrollmentOptional = false
	mandatoryFactor, err := auth.NewVerification(store, nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	module = auth.New(store, manager, nil, sessions, nil, nil, nil, nil, nil, mandatoryFactor)
	status, err = current("/status")
	if err != nil || !*status.Requirements.TOTPEnrollmentRequired || *status.Requirements.TOTPSkipAllowed {
		t.Fatal(status, err)
	}
	if err := skip(auth.OnboardingPreferences{TOTPEnrollmentSkipped: true}); !errors.Is(err, auth.ErrTOTPEnrollmentRequired) {
		t.Fatal(err)
	}
}
