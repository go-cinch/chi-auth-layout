package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

func TestLoginRestrictionHeaders(t *testing.T) {
	for _, result := range []*LoginResult{nil, {}, {PasswordResetRequired: true}, {MFAEnrollmentRequired: true}, {PasswordResetRequired: true, MFAEnrollmentRequired: true}} {
		response := httptest.NewRecorder()
		writeLoginHeaders(response, result)
		reset, mfa := "", ""
		if result != nil && result.PasswordResetRequired {
			reset = "true"
		}
		if result != nil && result.MFAEnrollmentRequired {
			mfa = "true"
		}
		if response.Header().Get(authn.PasswordResetRequiredHeader) != reset || response.Header().Get(authn.MFARequiredHeader) != mfa || response.Header().Get(authn.MFAEnrollmentRequiredHeader) != mfa {
			t.Fatal("session restriction headers do not match the issued session")
		}
	}
}

func TestLoginAndRefreshPasswordResetHeaders(t *testing.T) {
	for _, loginCount := range []int64{0, 2} {
		m, manager, mock := newAuthTestModule(t)
		handler, err := server.NewRouter(&config.Config{}, manager, nil, m)
		if err != nil {
			t.Fatal(err)
		}
		input := encryptedLoginInputWithProof(t, m.credentials, "readonly", authTestPassword, validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly"))
		mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WillReturnRows(loginRows(t, userStatusActive))
		mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(loginCount))
		body, _ := json.Marshal(input)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/pub/login", strings.NewReader(string(body))))
		want := ""
		if loginCount == 0 {
			want = "true"
		}
		if response.Code != http.StatusOK || response.Header().Get(authn.PasswordResetRequiredHeader) != want {
			t.Fatal("login restriction header mismatch", response.Code)
		}
		var result LoginResult
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		assertNoPasswordResetPayload(t, response.Body.Bytes())
		mock.ExpectQuery("SELECT username, code, status, credential_version, login_count").WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(1), loginCount))
		body, _ = json.Marshal(RefreshTokenInput{RefreshToken: result.RefreshToken})
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/pub/refresh", strings.NewReader(string(body))))
		if response.Code != http.StatusOK || response.Header().Get(authn.PasswordResetRequiredHeader) != want {
			t.Fatal("refresh restriction header mismatch", response.Code)
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		assertNoPasswordResetPayload(t, response.Body.Bytes())
	}
}

func assertNoPasswordResetPayload(t *testing.T, body []byte) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["password_reset_required"]; exists {
		t.Fatal("password reset requirement must be response-header metadata only")
	}
}

func TestEnrollmentMetadataDoesNotChangeLoginJSON(t *testing.T) {
	result := &LoginResult{MFAEnrollmentRequired: true, PasswordResetRequired: true}
	out := httptest.NewRecorder()
	writeLoginHeaders(out, result)
	if out.Header().Get(authn.MFARequiredHeader) != "true" || out.Header().Get(authn.MFAEnrollmentRequiredHeader) != "true" || out.Header().Get(authn.PasswordResetRequiredHeader) != "true" {
		t.Fatal("enrollment headers missing")
	}
	raw, err := json.Marshal(result)
	if err != nil || strings.Contains(string(raw), "mfa_") {
		t.Fatal("MFA enrollment changed login JSON", err)
	}
	assertNoPasswordResetPayload(t, raw)
}
