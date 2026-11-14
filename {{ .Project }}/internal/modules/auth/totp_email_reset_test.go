package auth

import (
	"database/sql"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestEmailResetRequiresTicketPasswordAndPurpose(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	attachTestEmail(t, m)
	const send = "/pub/totp/reset/email-code"
	const confirm = "/pub/totp/reset/confirm"
	begin := func() string {
		loginIdentity := identity
		loginIdentity.Methods = authn.Methods{Primary: authn.EmailCode}
		p, err := m.beginMFALogin(ctx, loginIdentity, false)
		if err != nil {
			t.Fatal(err)
		}
		return p.MFATicket
	}
	post := func(path, ticket string, body any, want int) {
		t.Helper()
		response := emailHTTPHeader(t, m, ctx, "POST", path, body, "X-MFA-Ticket", ticket)
		if response.Code != want {
			t.Fatalf("%s: got %d want %d", path, response.Code, want)
		}
	}
	credential := func(payload map[string]string) EncryptedLoginInput {
		challenge, err := m.credentials.challenge(ctx, loginCredentialType, 0)
		if err != nil {
			t.Fatal(err)
		}
		payload["challenge_id"] = challenge.ChallengeID
		return encryptedEmailInput(t, m, challenge.ChallengeID, loginCredentialType, payload)
	}
	pwd := func(value string) EncryptedLoginInput {
		return credential(map[string]string{"username": "owner", "method": "password", "password": value, "browser_uuid": emailTestUUID})
	}
	reset := func() EncryptedLoginInput {
		return credential(map[string]string{"username": "owner", "method": "email_code", "browser_uuid": emailTestUUID, "verification_id": "ticket-id", "verification_code": "123456"})
	}
	post(send, "invalid", nil, 400)
	ticket := begin()
	post(send, ticket, map[string]string{}, 400)
	post(send, ticket, reset(), 400)
	post(confirm, ticket, pwd("current"), 400)
	mock.ExpectQuery("SELECT u.password,u.email").WillReturnError(sql.ErrNoRows)
	post(send, ticket, pwd("current"), 503)
	mock.ExpectQuery("SELECT u.password,u.email").WillReturnRows(sqlmock.NewRows([]string{"password", "email"}).AddRow(passwordHash(t, "current"), "owner@example.test"))
	post(send, ticket, pwd("wrong"), 400)
	mock.ExpectQuery("SELECT u.password,u.email").WillReturnRows(sqlmock.NewRows([]string{"password", "email"}).AddRow(passwordHash(t, "current"), "owner@example.test"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT email FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("owner@example.test"))
	for range 3 {
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"at"}).AddRow(0))
	for range 3 {
		mock.ExpectExec("INSERT INTO t_auth_email_cooldown").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("UPDATE t_auth_email_code").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO t_auth_email_code").WillReturnResult(sqlmock.NewResult(0, 1))
expectEmailSeal(mock)
	mock.ExpectCommit()
	post(send, ticket, pwd("current"), 200)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT credential_version, status, login_count").WillReturnRows(sqlmock.NewRows([]string{"version", "status", "count"}).AddRow(identity.CredentialVersion, userStatusActive, 1))
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("owner@example.test"))
	mock.ExpectQuery("SELECT code_hash").WithArgs("ticket-id", emailTestUUID, "totp_reset", m.emailCodes.digest("reset-ticket", ticket), "owner@example.test", identity.UserID, identity.CredentialVersion).WillReturnError(sql.ErrNoRows)
	mock.ExpectCommit()
	post(confirm, ticket, reset(), 400)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
