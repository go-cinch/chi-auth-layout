package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEmailOnlySensitiveTransport(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	m.verification.config.TOTPEnabled = false
	attachTestEmail(t, m)
	const address, code = "owner@example.test", "123456"
	binding := func() {
		mock.ExpectQuery("SELECT COALESCE").WithArgs(identity.UserID, identity.CredentialVersion, userStatusActive).
			WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(address))
	}
	req := httptest.NewRequest(http.MethodDelete, "/user/7", nil).WithContext(ctx)
	req.Header.Set("X-TOTP-Code", code)
	binding()
	if err := m.CheckHTTPSecurity(req); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal("email verification must reject a code sent as TOTP", err)
	}
	req.Header.Del("X-TOTP-Code")
	ticket, err := randomSessionValue()
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := m.credentials.challenge(ctx, sensitiveScope(identity), identity.UserID)
	if err != nil {
		t.Fatal(err)
	}
	id := challenge.ChallengeID
	payload := sensitiveCredential{ChallengeID: id, Method: "email", Code: code, BrowserUUID: emailTestUUID, VerificationID: ticket}
	credential := encryptedEmailInput(t, m, id, sensitiveCredentialType, payload)
	req.Header.Set("X-Step-Up-Challenge", id)
	req.Header.Set("X-Step-Up-Credential", credential.Credential)
	binding()
	mock.ExpectBegin()
	binding()
	mock.ExpectQuery("SELECT code_hash, attempts").WithArgs(ticket, emailTestUUID, "sensitive", identity.SessionID, address, identity.UserID, identity.CredentialVersion).
		WillReturnRows(sqlmock.NewRows([]string{"hash", "attempts", "expires", "delivered", "consumed"}).
			AddRow(m.emailCodes.digest(ticket, code), 0, time.Now().Add(time.Minute).UnixMilli(), true, false))
	mock.ExpectExec("UPDATE t_auth_email_code SET attempts").WithArgs(true, sqlmock.AnyArg(), ticket).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := m.CheckHTTPSecurity(req); err != nil {
		t.Fatal("encrypted email proof rejected", err)
	}
	if err := m.verifySensitiveCredential(ctx, id, credential.Credential); !errors.Is(err, ErrVerificationInvalid) {
		t.Fatal("email challenge reused", err)
	}
}
