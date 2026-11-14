package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestAccountRecoveryReplacesSensitiveProofWithEnrollmentSession(t *testing.T) {
	m, mock, _, old, _ := newVerificationTest(t)
	old.Methods = authn.Methods{Primary: authn.EmailCode, SecondFactor: authn.TOTP}
	ctx := authenticatedIdentityContext(t, m.authenticator, m.sessions, old)
	const endpoint = "/totp/recovery/confirm"
	expectRecoveryMutation(mock, 4)
	response := emailHTTPHeader(t, m, ctx, http.MethodPost, endpoint, accountResetProof{}, authn.RecoveryCodeHeader, "recovery-code")
	if response.Code != http.StatusOK || response.Header().Get("X-MFA-Enrollment-Required") != "" {
		t.Fatalf("account recovery did not require enrollment: HTTP %d", response.Code)
	}
	var result LoginResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	replacement, err := m.authenticator.Parse(result.AccessToken)
	want := authn.Methods{Primary: authn.EmailCode}
	if err != nil || replacement.Methods != want || replacement.CredentialVersion != old.CredentialVersion+1 {
		t.Fatalf("unexpected recovery credential: %+v, %v", replacement.Methods, err)
	}
	session, err := m.sessions.ReadRefresh(ctx, result.RefreshToken)
	if err != nil || session.Methods != want {
		t.Fatalf("recovery refresh record retained a lost factor: %+v, %v", session.Methods, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccountRecoveryRequiresLiveCallerSession(t *testing.T) {
	m, _, ctx, identity, _ := newVerificationTest(t)
	if err := m.sessions.Revoke(ctx, identity.SessionID); err != nil {
		t.Fatal(err)
	}
	for _, caller := range []context.Context{t.Context(), ctx} {
		response := emailHTTPHeader(t, m, caller, http.MethodPost, "/totp/recovery/confirm", accountResetProof{}, authn.RecoveryCodeHeader, "recovery-code")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("recovery accepted missing or revoked session: HTTP %d", response.Code)
		}
	}
}

func TestAccountRecoveryRejectsInvalidProofsBeforeAttempts(t *testing.T) {
	for _, test := range []struct {
		name  string
		proof accountResetProof
		codes []string
	}{
		{name: "missing proof"},
		{name: "empty recovery header", codes: []string{""}},
		{name: "duplicate recovery headers", codes: []string{"recovery-code", "recovery-code"}},
		{name: "blank recovery code", codes: []string{"   "}},
		{name: "mixed proofs", proof: accountResetProof{BrowserUUID: emailTestUUID}, codes: []string{"recovery-code"}},
		{name: "incomplete email proof", proof: accountResetProof{BrowserUUID: emailTestUUID, VerificationCode: "123456"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, mock, ctx, identity, state := newVerificationTest(t)
			response := emailHTTPHeader(t, m, ctx, http.MethodPost, "/totp/recovery/confirm", test.proof, authn.RecoveryCodeHeader, test.codes...)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid proof returned HTTP %d", response.Code)
			}
			if len(state.attempts) != 0 {
				t.Fatal("invalid proof consumed an Verification attempt")
			}
			if err := m.sessions.ValidateIdentity(ctx, identity); err != nil {
				t.Fatalf("invalid proof changed the caller session: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAccountRecoveryEmailUnavailable(t *testing.T) {
	m, mock, ctx, _, _ := newVerificationTest(t)
	response := emailHTTP(t, m, ctx, http.MethodPost, "/totp/recovery/email-code", SensitiveEmailRequest{BrowserUUID: emailTestUUID})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled email returned HTTP %d", response.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
