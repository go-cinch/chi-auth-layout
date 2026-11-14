package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
)

func TestHTTPSecurityGuardEnforcedWithoutAuthorization(t *testing.T) {
	manager, err := authn.New("test", strings.Repeat("k", 64), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(authn.Identity{Methods: authn.Methods{Primary: authn.Password}, UserID: 1, Username: "operator", Code: "ABCDEFGH", CredentialVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	denied := apperror.New("AUTH_STEP_UP_REQUIRED", "verify")
	manager.SetHTTPSecurityPolicy(func(r *http.Request) error {
		if _, ok := authn.FromContext(r.Context()); !ok {
			t.Fatal("guard ran before authentication")
		}
		return denied
	})
	handler, err := NewRouter(&config.Config{}, manager, nil, protectedTestModule{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/private", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 403 || response.Header().Get(authn.VerificationRequiredHeader) != "true" || !strings.Contains(response.Body.String(), "AUTH_STEP_UP_REQUIRED") {
		t.Fatal("guard was bypassed with permissions disabled", response.Code)
	}
    if response.Header().Get("X-Password-Reset-Required") != "" || response.Header().Get("X-MFA-Enrollment-Required") != "" { t.Fatal("retired headers returned") }
	manager.SetHTTPSecurityPolicy(func(*http.Request) error { return nil })
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatal("verified request blocked", response.Code)
	}
}

func TestHTTPSecurityErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{apperror.New("AUTH_TOTP_ENROLLMENT_REQUIRED", "enroll"), 403},
		{apperror.New("AUTH_VERIFICATION_UNAVAILABLE", "unavailable"), 503},
		{apperror.New("AUTH_VERIFICATION_INVALID", "invalid"), 400},
		{apperror.New("AUTH_VERIFICATION_RATE_LIMITED", "rate limit"), 429},
		{apperror.New("AUTH_UNAUTHORIZED", "unauthorized"), 401},
		{errors.New("database unavailable"), 500},
	} {
		response := httptest.NewRecorder()
		WriteHTTPSecurityError(response, httptest.NewRequest("DELETE", "/user/1", nil), tc.err)
		if response.Code != tc.status {
			t.Fatal(response.Code)
		}
		if tc.status == 500 && strings.Contains(response.Body.String(), "database") {
			t.Fatal("internal detail leaked")
		}
	}
}
