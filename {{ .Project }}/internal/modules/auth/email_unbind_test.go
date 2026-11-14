package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestUnlinkProofUsesAccountHistory(t *testing.T) {
	const address = "owner@example.test"
	for _, tc := range []struct {
		name, method, address, password, code string
		want                                  error
	}{
		{"already unbound", "password", "", "pw", "", ErrUnlinkInvalid},
		{"factor required", "verification", address, "pw", "", ErrVerificationInvalid},
		{"previous factor", "enroll", address, "pw", "", ErrUnlinkFactor},
		{"password required", "password", address, "", "123456", ErrIncorrectPassword},
		{"wrong password", "password", address, "wrong", "", ErrIncorrectPassword},
		{"correct password", "password", address, "pw", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock, ctx, identity, _ := newVerificationTest(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT COALESCE").WithArgs(identity.UserID, identity.Code, identity.CredentialVersion, userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"email", "fallback"}).AddRow(tc.address, tc.method))
			if tc.address != "" && tc.method == "password" && tc.password != "" {
				mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t, "pw")))
			}
			if tc.want != nil {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec("UPDATE t_user SET email=NULL").WithArgs(identity.UserID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE t_auth_email_code SET consumed=TRUE").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err := m.verifyUnlinkProof(ctx, identity, EmailUnlinkProof{Password: tc.password, VerificationCode: tc.code})
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
		})
	}
}

func TestUnlinkLinkStateAndNotification(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		offset                         time.Duration
		version                        int64
		consumed, confirmed, delivered bool
		want                           error
	}{
		{"initial", time.Minute, 1, false, false, true, nil},
		{"replayed", time.Minute, 1, true, true, true, nil},
		{"superseded", time.Minute, 1, true, false, true, ErrUnlinkInvalid},
		{"expired", -time.Minute, 1, false, false, true, ErrUnlinkInvalid},
		{"credential changed", time.Minute, 2, false, false, true, ErrUnlinkInvalid},
		{"smtp pending", time.Minute, 1, false, false, false, ErrEmailUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock, ctx, _, _ := newVerificationTest(t)
			attachTestEmail(t, m)
			id, err := randomSessionValue()
			if err != nil {
				t.Fatal(err)
			}
			token := id + "." + m.emailCodes.digest("unlink-link", id)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT u.id").WillReturnRows(sqlmock.NewRows([]string{"id", "version", "email"}).AddRow(3, int64(1), "owner@example.test"))
			mock.ExpectQuery("SELECT recipient,credential_version").WillReturnRows(sqlmock.NewRows([]string{"email", "version", "expires", "delivered", "consumed", "confirmed"}).AddRow("owner@example.test", tc.version, time.Now().Add(tc.offset).UnixMilli(), tc.delivered, tc.consumed, tc.confirmed))
			if tc.want != nil {
				mock.ExpectRollback()
			} else {
				if !tc.confirmed {
					for _, query := range []string{"UPDATE t_user SET email=NULL", "UPDATE t_auth_email_code SET consumed=TRUE", "UPDATE t_auth_email_code SET unlink_confirmed", "INSERT INTO t_auth_email_code"} {
						mock.ExpectExec(query).WillReturnResult(sqlmock.NewResult(0, 1))
 if query == "INSERT INTO t_auth_email_code" { expectEmailSeal(mock) }
					}
				}
				mock.ExpectCommit()
			}
			r, err := m.confirmUnlinkToken(ctx, token)
			if !errors.Is(err, tc.want) || r.AlreadyConfirmed != tc.confirmed {
				t.Fatal(r, err)
			}
		})
	}
}

func TestUnlinkPublicAndCredentialBoundaries(t *testing.T) {
	m, _, ctx, _, _ := newVerificationTest(t)
	attachTestEmail(t, m)
	for _, input := range []any{map[string]string{"token": "invalid"}, map[string]string{"password": "pw"}} {
		res := emailHTTP(t, m, ctx, http.MethodPost, "/pub/email/unbind", input)
		if res.Code < 400 || res.Code >= 500 {
			t.Fatal(res.Code)
		}
	}
	for _, path := range []string{"/email/unbind", "/email/unbind/request"} {
		if r := emailHTTP(t, m, t.Context(), http.MethodPost, path, map[string]string{}); r.Code != 401 {
			t.Fatal(path, r.Code)
		}
	}
	r := emailHTTP(t, m, ctx, http.MethodPost, "/email/unbind", map[string]string{"password": "pw"})
	if r.Code != 400 {
		t.Fatal(r.Code)
	}
	for _, bad := range []string{"http://example.test/unbind", "https://user@host.test/unbind", "/relative", "https://host.test/#token=x"} {
		if validUnlinkURL(bad) {
			t.Fatal(bad)
		}
	}
	for _, good := range []string{"http://127.0.0.1:5666/auth/unbind-email", "https://host.test/#/auth/unbind-email"} {
		if !validUnlinkURL(good) {
			t.Fatal(good)
		}
	}
}

func TestProfileUnlinkStatusAndRequest(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	sender := attachTestEmail(t, m)
	m.emailCodes.config.UnlinkURL = "https://pc.example.test/#/auth/unbind-email"
	const address = "owner@example.test"
	path := "/email/unbind/status?browser_uuid=" + emailTestUUID
	row := func(email, method string) {
		mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"email", "fallback"}).AddRow(email, method))
	}
	row(address, "verification")
	mock.ExpectQuery("SELECT COALESCE\\(MAX").WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(123))
	mock.ExpectQuery("SELECT verification_id,expired_at").WillReturnRows(sqlmock.NewRows([]string{"id", "expired_at"}).AddRow("public-ticket-id", 456))
	r := emailHTTP(t, m, ctx, "GET", path, nil)
	var state EmailUnlinkStatus
	if err := json.Unmarshal(r.Body.Bytes(), &state); err != nil || r.Code != 200 || !state.Bound || !state.CanSend || state.Fallback != "verification" || state.VerificationID != "public-ticket-id" {
		t.Fatal(state, r.Code, err)
	}
	row("", "password")
	r = emailHTTP(t, m, ctx, "GET", path, nil)
	state = EmailUnlinkStatus{}
	if err := json.Unmarshal(r.Body.Bytes(), &state); err != nil || r.Code != 200 || state.Bound || state.VerificationID != "" {
		t.Fatal(state, r.Code, err)
	}
	sender.disabled = true
	row(address, "password")
	req := EmailUnlinkRequest{BrowserUUID: emailTestUUID}
	if r = emailHTTP(t, m, ctx, "POST", "/email/unbind/request", req); r.Code != 503 {
		t.Fatal(r.Code)
	}
	sender.disabled = false
	if r = emailHTTP(t, m, ctx, "POST", "/email/unbind/request", map[string]string{"browser_uuid": emailTestUUID, "email": "other@example.test"}); r.Code != 400 {
		t.Fatal(r.Code)
	}
	row("", "verification")
	if r = emailHTTP(t, m, ctx, "POST", "/email/unbind/request", req); r.Code != 409 {
		t.Fatal(r.Code)
	}
	row(address, "password")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT email FROM t_user").WithArgs(identity.UserID, identity.CredentialVersion, address, userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow(address))
	for range 3 {
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery("SELECT COALESCE\\(MAX").WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(0))
	for range 3 {
		mock.ExpectExec("INSERT INTO t_auth_email_cooldown").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	for _, q := range []string{"UPDATE t_auth_email_code", "INSERT INTO t_auth_email_code"} {
		mock.ExpectExec(q).WillReturnResult(sqlmock.NewResult(0, 1))
 if q == "INSERT INTO t_auth_email_code" { expectEmailSeal(mock) }
	}
	mock.ExpectCommit()
	r = emailHTTP(t, m, ctx, "POST", "/email/unbind/request", req)
	var issued EmailCodeStatus
	if err := json.Unmarshal(r.Body.Bytes(), &issued); err != nil || r.Code != 200 || !validSessionID(issued.VerificationID) {
		t.Fatal(err, r.Code)
	}
	m.emailCodes.config.Templates = testEmailTemplateIDs()
 m.emailCodes.SetEmailTemplates(testEmailTemplates{})
 _, body, _, err := m.emailCodes.deliveryMessage(t.Context(), issued.VerificationID, emailAttempt{purpose: "unlink", account: "tester"})
 if err != nil { t.Fatal(err) }
	if !strings.Contains(body, "/#/auth/unbind-email#token="+issued.VerificationID+".") || strings.Contains(r.Body.String(), m.emailCodes.digest("unlink-link", issued.VerificationID)) {
		t.Fatal("incorrect link or exposed email proof")
	}
}

func TestUnlinkTOTPHeaderTransport(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	const address = "owner@example.test"
	const path = "/email/unbind"
	body := map[string]string{}
	for _, tc := range []struct {
		name  string
		data  any
		codes []string
	}{
		{"missing code", body, nil},
		{"empty code", body, []string{""}},
		{"duplicate header", body, []string{"123456", "654321"}},
		{"malformed code", body, []string{"12345x"}},
		{"null body", nil, []string{"123456"}},
		{"caller-supplied address", map[string]string{"email": "other@example.test"}, []string{"123456"}},
		{"plaintext password", map[string]string{"password": "pw"}, nil},
		{"code in body", map[string]string{"verification_code": "123456"}, nil},
		{"mixed proofs", map[string]string{"credential": "unused", "challenge_id": "unused"}, []string{"123456"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if r := emailHTTP(t, m, ctx, "POST", path, tc.data, tc.codes...); r.Code != 400 {
				t.Fatal(r.Code)
			}
		})
	}
	secret, err := newTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	step := time.Now().Unix() / 30
	code, err := totpCode(secret, step)
	if err != nil {
		t.Fatal(err)
	}
	for _, used := range []bool{false, true} {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"email", "fallback"}).AddRow(address, "verification"))
		if used {
			expectTOTPFactor(t, m, mock, secret, step)
			mock.ExpectRollback()
		} else {
			expectTOTPFactor(t, m, mock, secret, -1)
			mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WithArgs(step, sqlmock.AnyArg(), int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE t_user SET email=NULL").WithArgs(identity.UserID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE t_auth_email_code SET consumed=TRUE").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		}
		r := emailHTTP(t, m, ctx, "POST", path, body, code)
		if !used && r.Code != 200 || used && r.Code != 400 {
			t.Fatal(used, r.Code)
		}
	}
}

func TestUnlinkPasswordChallengeTransport(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	attachTestEmail(t,m)
	const address = "owner@example.test"
	challenge, err := m.credentials.challenge(ctx, unlinkCredentialType, 3)
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]string{"challenge_id": challenge.ChallengeID, "verification_code": "123456"}
	send := func() int {
		input := encryptedEmailInput(t, m, challenge.ChallengeID, unlinkCredentialType, payload)
		return emailHTTP(t, m, ctx, "POST", "/email/unbind", input).Code
	}
	if status := send(); status != 400 {
		t.Fatalf("encrypted Verification code: %d", status)
	}
	delete(payload, "verification_code")
	payload["password"] = "pw"
	payload["email"] = "other@example.test"
	if status := send(); status != 400 {
		t.Fatalf("address in password credential: %d", status)
	}
	delete(payload, "email")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"email", "fallback"}).AddRow(address, "password"))
	mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t, "pw")))
	mock.ExpectExec("UPDATE t_user SET email=NULL").WithArgs(identity.UserID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE t_auth_email_code SET consumed=TRUE").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO t_auth_email_code").WithArgs(sqlmock.AnyArg(),identity.UserID,address,identity.CredentialVersion,sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0,1))
expectEmailSeal(mock)
	mock.ExpectCommit()
	if status := send(); status != 200 {
		t.Fatal(status)
	}
	if status := send(); status != 400 {
		t.Fatalf("reused password challenge: %d", status)
	}
}
