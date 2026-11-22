package auth

import (
    "encoding/json"
    "database/sql"
    "errors"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/DATA-DOG/go-sqlmock"
    "github.com/go-jose/go-jose/v4"
    "{{ .Computed.module_name_final }}/internal/common/apperror"
    "{{ .Computed.module_name_final }}/internal/common/auditctx"
    "{{ .Computed.module_name_final }}/internal/common/authn"
{{ if eq .Computed.http_router_final "gin" }}
    "github.com/gin-gonic/gin"
{{ end }}
)

func TestTOTPLoginFailureRetainsTicketAuditIdentity(t *testing.T) {
	for _, scenario := range []string{"invalid_code", "rate_limited", "missing_code", "invalid_ticket"} {
		t.Run(scenario, func(t *testing.T) {
			m, mock, ctx, identity, state := newVerificationTest(t)
			identity.SessionID = ""
			pending, err := m.beginMFALogin(ctx, identity, false)
			if err != nil {
				t.Fatal(err)
			}
			code, ticket, wantStatus := "invalid", pending.MFATicket, http.StatusBadRequest
			switch scenario {
			case "invalid_code":
				secret, err := newTOTPSecret()
				if err != nil {
					t.Fatal(err)
				}
				expectVerificationLock(mock, identity.CredentialVersion)
				expectTOTPFactor(t, m, mock, secret, -1)
				mock.ExpectRollback()
			case "rate_limited":
				state.attempts = map[string]int64{verificationAttemptKey(identity): m.verification.config.RateLimitMaxAttempts}
				wantStatus = http.StatusTooManyRequests
			case "missing_code":
				code, wantStatus = "", http.StatusForbidden
			case "invalid_ticket":
				ticket = strings.Repeat("x", 43)
			}
			auditCtx, auditState := auditctx.WithState(t.Context())
			request := httptest.NewRequest(http.MethodPost, "/pub/login", strings.NewReader(`{}`)).WithContext(auditCtx)
			request.Header.Set(authn.MFATicketHeader, ticket)
			request.Header.Set(authn.TOTPCodeHeader, code)
			response := httptest.NewRecorder()
			totpTestHTTP(m).ServeHTTP(response, request)
			if response.Code != wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, wantStatus)
			}
			event := auditState.Snapshot()
			if scenario == "invalid_ticket" {
				if event.UserID != 0 || event.Username != "" {
					t.Fatal("unverified ticket supplied an audit identity")
				}
			} else if event.UserID != identity.UserID || event.Username != identity.Username {
				t.Fatal("Verification continuation failure lost its verified account")
			}
		})
	}
}

func totpTestHTTP(m *Module) http.Handler {
{{ if eq .Computed.http_router_final "gin" }}
    r:=gin.New();m.HTTP(r.Group(""));return r
{{ else }}
    return m.HTTP()
{{ end }}
}

func TestTOTPHTTPBoundary(t *testing.T) {
    m,mock,ctx,_,state:=newVerificationTest(t)
    handler:=totpTestHTTP(m)
    call:=func(method,path,body string)*httptest.ResponseRecorder {
        t.Helper();r:=httptest.NewRequest(method,path,strings.NewReader(body)).WithContext(ctx)
        out:=httptest.NewRecorder();handler.ServeHTTP(out,r);return out
    }
    expectTOTPBound(mock,false)
    response:=call("GET","/totp","")
    if response.Code!=200 || response.Header().Get("Cache-Control")!="no-store" {t.Fatal(response.Code)}
    response=call("POST","/totp/challenge","")
    if response.Code!=200 {t.Fatal(response.Code)}
    var challenge CredentialChallenge
    if err:=json.Unmarshal(response.Body.Bytes(),&challenge);err!=nil {t.Fatal(err)}
    options:=(&jose.EncrypterOptions{}).WithType(jose.ContentType(totpSetupCredentialType))
    encrypter,err:=jose.NewEncrypter(credentialContentAlgorithm,jose.Recipient{Algorithm:credentialKeyAlgorithm,Key:&m.credentials.privateKey.PublicKey,KeyID:m.credentials.keyID},options)
    if err!=nil {t.Fatal(err)}
    plaintext,_:=json.Marshal(registrationPasswordCredential{ChallengeID:challenge.ChallengeID,Password:"pass"})
    encrypted,err:=encrypter.Encrypt(plaintext);if err!=nil {t.Fatal(err)}
    credential,err:=encrypted.CompactSerialize();if err!=nil {t.Fatal(err)}
    body,_:=json.Marshal(EncryptedLoginInput{ChallengeID:challenge.ChallengeID,Credential:credential})
    mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t,"pass")))
    expectTOTPBound(mock,false)
    response=call("POST","/totp/setup",string(body))
    if response.Code!=200 {t.Fatal(response.Code,response.Body.String())}
    if response=call("POST","/totp/setup",string(body));response.Code!=400 {t.Fatal("reusable enrollment challenge")}
    for _,path:=range []string{"/totp/setup","/totp/confirm","/totp/test"} {
        for _,bad:=range []string{"invalid",`{"unknown":true}`,`{} {}`} {
            if response=call("POST",path,bad);response.Code!=400 {t.Fatalf("%s malformed body accepted",path)}
        }
    }
    state.attempts=map[string]int64{"3:1":10}
    for _,path:=range []string{"/totp/confirm","/totp/test"} {
        if response=call("POST",path,`{"verification_code":"123456"}`);response.Code!=429 {t.Fatal(path,response.Code)}
        required := response.Header().Get(authn.VerificationRequiredHeader) == "true"
        if required != (path == "/totp/confirm") { t.Fatal("rate-limit Verification header differs from endpoint contract", path) }
        if response.Header().Get("X-MFA-Enrollment-Required") != "" { t.Fatal("rate limit requested enrollment", path) }
    }
    for _,path:=range []string{"/pub/login/totp","/totp/verify"} {
        if response=call("POST",path,`{}`);response.Code!=404 {t.Fatal("retired verification route remains",path,response.Code)}
    }
    if response=call("POST","/totp/reset/bad","");response.Code!=400 {t.Fatal(response.Code)}
}

func TestTOTPTestHTTPDoesNotRequestSensitiveVerification(t *testing.T) {
    m, mock, ctx, identity, state := newVerificationTest(t)
    call := func(body string) *httptest.ResponseRecorder {
        out := httptest.NewRecorder()
        totpTestHTTP(m).ServeHTTP(out, httptest.NewRequest("POST", "/totp/test", strings.NewReader(body)).WithContext(ctx))
        if out.Header().Get("Cache-Control") != "no-store" { t.Fatal("test response is cacheable") }
        if out.Header().Get(authn.VerificationRequiredHeader) != "" { t.Fatal("test triggered sensitive verification") }
        return out
    }
    for _, body := range []string{"invalid", `{"unexpected":true}`, `{} {}`} {
        if out := call(body); out.Code != 400 { t.Fatal("malformed test payload accepted", out.Code) }
    }
    if len(state.attempts) != 0 { t.Fatal("malformed JSON reached authenticator verification") }
    secret, _ := newTOTPSecret()
    step := time.Now().Unix()/30
    code, _ := totpCode(secret, step)
    expectVerificationLock(mock, 1)
    expectTOTPFactor(t, m, mock, secret, -1)
    mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WithArgs(step, sqlmock.AnyArg(), identity.UserID).WillReturnResult(sqlmock.NewResult(0, 1))
    mock.ExpectCommit()
    out := call(`{"verification_code":"`+code+`"}`)
    if out.Code != 200 { t.Fatal("test failed", out.Code, out.Body.String()) }
    var result map[string]any
    if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil || len(result) != 1 || result["verified"] != true { t.Fatal("test response leaked session or grant", err) }
    expectVerificationLock(mock, 1)
    expectTOTPFactor(t, m, mock, secret, step)
    mock.ExpectRollback()
    out = call(`{"verification_code":"`+code+`"}`)
    if out.Code != 400 || !strings.Contains(out.Body.String(), "AUTH_VERIFICATION_INVALID") { t.Fatal("replay error lost standard contract", out.Code) }
    expectVerificationLock(mock, 1)
    expectTOTPFactor(t, m, mock, secret, step)
    mock.ExpectRollback()
    if out := call(`{}`); out.Code != 400 { t.Fatal("empty code accepted", out.Code) }
    if state.attempts[verificationAttemptKey(identity)] != 3 { t.Fatal("success, replay and empty-code attempts were not all counted") }
    state.attempts[verificationAttemptKey(identity)] = m.verification.config.RateLimitMaxAttempts
    out = call(`{"verification_code":"123456"}`)
    if out.Code != 429 || !strings.Contains(out.Body.String(), "AUTH_VERIFICATION_RATE_LIMITED") { t.Fatal("rate limit error lost standard contract", out.Code, out.Body.String()) }
    out = httptest.NewRecorder()
    totpTestHTTP(m).ServeHTTP(out, httptest.NewRequest("POST", "/totp/test", strings.NewReader(`{"verification_code":"123456"}`)))
    if out.Code != 401 { t.Fatal("anonymous authenticator test accepted", out.Code) }
    if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestTOTPErrorResponses(t *testing.T) {
    for _,tc:=range []struct{err error;status int}{
        {ErrTOTPUserNotFound,404},{ErrUnauthorized,401},{ErrStepUpRequired,403},{ErrTOTPEnrollmentRequired,403},{apperror.Forbidden,403},
        {ErrVerificationRateLimited,429},{ErrVerificationUnavailable,503},{ErrTOTPAlreadyBound,409},
        {ErrVerificationInvalid,400},{ErrIncorrectPassword,400},{ErrInvalidCredential,400},{ErrInvalid,400},
        {errors.New("storage failure"),500},
    } {
        out:=httptest.NewRecorder();writeVerificationError(out,httptest.NewRequest("POST","/pub/login",nil),tc.err)
        if out.Code!=tc.status {t.Fatal(out.Code)}
        if strings.Contains(out.Body.String(),"storage failure") {t.Fatal("internal error leaked")}
    }
}

const resetTOTPGrant = `{"bindings":[{"resource_key":"admin:system:user","operation":"update","feature_keys":["admin:system:user:reset-totp"]}],"ui":[],"endpoints":[]}`

func expectResetPermission(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("WITH assigned AS").WillReturnRows(rows)
}

func TestTOTPResetButtonPermissionAndCredentialInvalidation(t *testing.T) {
	m, mock, ctx, _, _ := newVerificationTest(t)
	for _, grant := range []struct{ word, config string }{
		{"admin:read:system:user", userReadPermissionConfig},
		{"admin:update:system:user", userEditPermissionConfig},
	} {
		mock.ExpectQuery("WITH assigned AS").WillReturnRows(permissionRows().AddRow(grant.word, grant.config))
		if allowed, err := m.AuthorizeHTTP(ctx, "POST", "/totp/reset/7"); err != nil || allowed {
			t.Fatal(grant.word, allowed, err)
		}
		expectResetPermission(mock, permissionRows().AddRow(grant.word, grant.config))
		mock.ExpectRollback()
		if err := m.ResetUserTOTP(ctx, 7); !errors.Is(err, apperror.Forbidden) {
			t.Fatal(grant.word, err)
		}
	}

	mock.ExpectQuery("WITH assigned AS").WillReturnRows(permissionRows().AddRow("admin:update:system:user", resetTOTPGrant))
	if allowed, err := m.AuthorizeHTTP(ctx, "POST", "/totp/reset/7"); err != nil || !allowed {
		t.Fatal(allowed, err)
	}
	expectResetPermission(mock, permissionRows().AddRow("admin:update:system:user", resetTOTPGrant))
	mock.ExpectQuery("UPDATE t_user SET credential_version").WithArgs(sqlmock.AnyArg(), int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec("DELETE FROM t_user_recovery_code").WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 10))
	mock.ExpectExec("DELETE FROM t_user_totp").WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := m.ResetUserTOTP(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPSetupCredentialPurposeAndUserBinding(t *testing.T) {
    credentials:=newCredentialTestManager(t,time.Minute)
    challenge,err:=credentials.TOTPSetupChallenge(t.Context(),3);if err!=nil {t.Fatal(err)}
    wrong:=encryptLoginCredential(t,credentials,loginCredential{ChallengeID:challenge.ChallengeID,Username:"user",Password:"secret"},true)
    if _,err:=credentials.OpenTOTPSetup(t.Context(),3,EncryptedLoginInput{ChallengeID:challenge.ChallengeID,Credential:wrong});err==nil {t.Fatal("login credential accepted for Verification enrollment")}
}

func TestTOTPResetMissingUser(t *testing.T) {
    m,mock,ctx,_,_:=newVerificationTest(t)
    expectResetPermission(mock, permissionRows().AddRow("admin:update:system:user", resetTOTPGrant))
    mock.ExpectQuery("UPDATE t_user SET credential_version").WillReturnError(sql.ErrNoRows)
    mock.ExpectRollback()
    if err:=m.ResetUserTOTP(ctx,999999);!errors.Is(err,ErrTOTPUserNotFound) {t.Fatal(err)}
}

func TestTOTPHTTPPasswordFreeSetupRequiresLoginGrant(t *testing.T) {
    m, mock, ctx, identity, _ := newVerificationTest(t)
    call := func(body string) *httptest.ResponseRecorder {
        out := httptest.NewRecorder()
        totpTestHTTP(m).ServeHTTP(out, httptest.NewRequest("POST", "/totp/setup", strings.NewReader(body)).WithContext(ctx))
        return out
    }
    mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t,"pass")))
    if out := call(`{}`); out.Code != 400 { t.Fatal("missing password accepted without login grant", out.Code) }
    if err := m.grantTOTPEnrollment(ctx, identity, identity.SessionID); err != nil { t.Fatal(err) }
    mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t,"pass")))
    expectTOTPBound(mock,false)
    if out := call(`{}`); out.Code != 200 || !strings.Contains(out.Body.String(),"otpauth://") || out.Header().Get("Cache-Control") != "no-store" { t.Fatal("login enrollment failed", out.Code) }
    // Supplying incomplete encrypted input cannot fall back to the grant.
    if out := call(`{"challenge_id":"invalid"}`); out.Code != 400 { t.Fatal(out.Code) }
    if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestTOTPConfirmPasswordResetHeaderOnly(t *testing.T) {
	m, mock, _, identity, _ := newVerificationTest(t)
	manager, err := authn.New("test", authTestKey, time.Hour, NewIdentityValidator(m.store, m.sessions))
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count").WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow(identity.Username, identity.Code, userStatusActive, identity.CredentialVersion, int64(0)))
	request := httptest.NewRequest("POST", "/totp/setup", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request, err = manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx := request.Context()
	mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t, "pass")))
	expectTOTPBound(mock, false)
	setup, err := m.BeginTOTPEnrollment(ctx, "pass")
	if err != nil {
		t.Fatal(err)
	}
	code, err := totpCode(setup.Secret, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	expectVerificationLock(mock, 1)
	expectTOTPBound(mock, false)
	mock.ExpectExec("INSERT INTO t_user_totp").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM t_user_recovery_code").WillReturnResult(sqlmock.NewResult(0, 0))
	for range 10 {
		mock.ExpectExec("INSERT INTO t_user_recovery_code").WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectExec("UPDATE t_user SET credential_version").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	body, _ := json.Marshal(TOTPVerificationInput{VerificationCode: code})
	out := httptest.NewRecorder()
	totpTestHTTP(m).ServeHTTP(out, httptest.NewRequest("POST", "/totp/confirm", strings.NewReader(string(body))).WithContext(ctx))
	if out.Code != 200 || out.Header().Get("X-Password-Reset-Required") != "" || out.Header().Get("X-MFA-Enrollment-Required") != "" {
		t.Fatal("Verification confirmation lost the remaining password reset requirement", out.Code)
	}
	assertNoPasswordResetPayload(t, out.Body.Bytes())
	var payload map[string]any
	if err := json.Unmarshal(out.Body.Bytes(), &payload); err != nil || len(payload) != 4 || payload["access_token"] == "" || payload["recovery_codes"] == nil {
		t.Fatal("Verification confirmation changed session or recovery-code payload", err)
	}
}
