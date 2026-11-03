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

func TestMFALoginFailureRetainsTicketAuditIdentity(t *testing.T) {
	for _, scenario := range []string{"invalid_code", "rate_limited", "missing_code", "invalid_ticket"} {
		t.Run(scenario, func(t *testing.T) {
			m, mock, ctx, identity, state := newMFATest(t)
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
				expectMFALock(mock, identity.CredentialVersion)
				expectMFAFactor(t, m, mock, secret, -1)
				mock.ExpectRollback()
			case "rate_limited":
				state.attempts = map[string]int64{mfaAttemptKey(identity): m.mfa.config.RateLimitMaxAttempts}
				wantStatus = http.StatusTooManyRequests
			case "missing_code":
				code, wantStatus = "", http.StatusForbidden
			case "invalid_ticket":
				ticket = strings.Repeat("x", 43)
			}
			auditCtx, auditState := auditctx.WithState(t.Context())
			request := httptest.NewRequest(http.MethodPost, "/pub/login", strings.NewReader(`{}`)).WithContext(auditCtx)
			request.Header.Set(authn.MFATicketHeader, ticket)
			request.Header.Set(authn.MFACodeHeader, code)
			response := httptest.NewRecorder()
			mfaTestHTTP(m).ServeHTTP(response, request)
			if response.Code != wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, wantStatus)
			}
			event := auditState.Snapshot()
			if scenario == "invalid_ticket" {
				if event.UserID != 0 || event.Username != "" {
					t.Fatal("unverified ticket supplied an audit identity")
				}
			} else if event.UserID != identity.UserID || event.Username != identity.Username {
				t.Fatal("MFA continuation failure lost its verified account")
			}
		})
	}
}

func mfaTestHTTP(m *Module) http.Handler {
{{ if eq .Computed.http_router_final "gin" }}
    r:=gin.New();m.HTTP(r.Group(""));return r
{{ else }}
    return m.HTTP()
{{ end }}
}

func TestMFAHTTPBoundary(t *testing.T) {
    m,mock,ctx,_,state:=newMFATest(t)
    handler:=mfaTestHTTP(m)
    call:=func(method,path,body string)*httptest.ResponseRecorder {
        t.Helper();r:=httptest.NewRequest(method,path,strings.NewReader(body)).WithContext(ctx)
        out:=httptest.NewRecorder();handler.ServeHTTP(out,r);return out
    }
    expectMFAEnabled(mock,false)
    response:=call("GET","/mfa","")
    if response.Code!=200 || response.Header().Get("Cache-Control")!="no-store" {t.Fatal(response.Code)}
    response=call("POST","/mfa/challenge","")
    if response.Code!=200 {t.Fatal(response.Code)}
    var challenge CredentialChallenge
    if err:=json.Unmarshal(response.Body.Bytes(),&challenge);err!=nil {t.Fatal(err)}
    options:=(&jose.EncrypterOptions{}).WithType(jose.ContentType(mfaSetupCredentialType))
    encrypter,err:=jose.NewEncrypter(credentialContentAlgorithm,jose.Recipient{Algorithm:credentialKeyAlgorithm,Key:&m.credentials.privateKey.PublicKey,KeyID:m.credentials.keyID},options)
    if err!=nil {t.Fatal(err)}
    plaintext,_:=json.Marshal(registrationPasswordCredential{ChallengeID:challenge.ChallengeID,Password:"pass"})
    encrypted,err:=encrypter.Encrypt(plaintext);if err!=nil {t.Fatal(err)}
    credential,err:=encrypted.CompactSerialize();if err!=nil {t.Fatal(err)}
    body,_:=json.Marshal(EncryptedLoginInput{ChallengeID:challenge.ChallengeID,Credential:credential})
    mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t,"pass")))
    expectMFAEnabled(mock,false)
    response=call("POST","/mfa/setup",string(body))
    if response.Code!=200 {t.Fatal(response.Code,response.Body.String())}
    if response=call("POST","/mfa/setup",string(body));response.Code!=400 {t.Fatal("reusable enrollment challenge")}
    for _,path:=range []string{"/mfa/setup","/mfa/confirm","/mfa/test"} {
        for _,bad:=range []string{"invalid",`{"unknown":true}`,`{} {}`} {
            if response=call("POST",path,bad);response.Code!=400 {t.Fatalf("%s malformed body accepted",path)}
        }
    }
    state.attempts=map[string]int64{"3:1":10}
    for _,path:=range []string{"/mfa/confirm","/mfa/test"} {
        if response=call("POST",path,`{"verification_code":"123456"}`);response.Code!=429 {t.Fatal(path,response.Code)}
        required := response.Header().Get(authn.MFARequiredHeader) == "true"
        if required != (path == "/mfa/confirm") { t.Fatal("rate-limit MFA header differs from endpoint contract", path) }
        if response.Header().Get(authn.MFAEnrollmentRequiredHeader) != "" { t.Fatal("rate limit requested enrollment", path) }
    }
    for _,path:=range []string{"/pub/login/mfa","/mfa/verify"} {
        if response=call("POST",path,`{}`);response.Code!=404 {t.Fatal("retired verification route remains",path,response.Code)}
    }
    if response=call("POST","/mfa/reset/bad","");response.Code!=400 {t.Fatal(response.Code)}
}

func TestMFATestHTTPDoesNotRequestSensitiveVerification(t *testing.T) {
    m, mock, ctx, identity, state := newMFATest(t)
    call := func(body string) *httptest.ResponseRecorder {
        out := httptest.NewRecorder()
        mfaTestHTTP(m).ServeHTTP(out, httptest.NewRequest("POST", "/mfa/test", strings.NewReader(body)).WithContext(ctx))
        if out.Header().Get("Cache-Control") != "no-store" { t.Fatal("test response is cacheable") }
        if out.Header().Get(authn.MFARequiredHeader) != "" { t.Fatal("test triggered sensitive verification") }
        return out
    }
    for _, body := range []string{"invalid", `{"unexpected":true}`, `{} {}`} {
        if out := call(body); out.Code != 400 { t.Fatal("malformed test payload accepted", out.Code) }
    }
    if len(state.attempts) != 0 { t.Fatal("malformed JSON reached authenticator verification") }
    secret, _ := newTOTPSecret()
    step := time.Now().Unix()/30
    code, _ := totpCode(secret, step)
    expectMFALock(mock, 1)
    expectMFAFactor(t, m, mock, secret, -1)
    mock.ExpectExec("UPDATE t_user_totp SET last_used_step").WithArgs(step, sqlmock.AnyArg(), identity.UserID).WillReturnResult(sqlmock.NewResult(0, 1))
    mock.ExpectCommit()
    out := call(`{"verification_code":"`+code+`"}`)
    if out.Code != 200 { t.Fatal("test failed", out.Code, out.Body.String()) }
    var result map[string]any
    if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil || len(result) != 1 || result["verified"] != true { t.Fatal("test response leaked session or grant", err) }
    expectMFALock(mock, 1)
    expectMFAFactor(t, m, mock, secret, step)
    mock.ExpectRollback()
    out = call(`{"verification_code":"`+code+`"}`)
    if out.Code != 400 || !strings.Contains(out.Body.String(), "AUTH_MFA_INVALID") { t.Fatal("replay error lost standard contract", out.Code) }
    expectMFALock(mock, 1)
    expectMFAFactor(t, m, mock, secret, step)
    mock.ExpectRollback()
    if out := call(`{}`); out.Code != 400 { t.Fatal("empty code accepted", out.Code) }
    if state.attempts[mfaAttemptKey(identity)] != 3 { t.Fatal("success, replay and empty-code attempts were not all counted") }
    state.attempts[mfaAttemptKey(identity)] = m.mfa.config.RateLimitMaxAttempts
    out = call(`{"verification_code":"123456"}`)
    if out.Code != 429 || !strings.Contains(out.Body.String(), "AUTH_MFA_RATE_LIMITED") { t.Fatal("rate limit error lost standard contract", out.Code, out.Body.String()) }
    out = httptest.NewRecorder()
    mfaTestHTTP(m).ServeHTTP(out, httptest.NewRequest("POST", "/mfa/test", strings.NewReader(`{"verification_code":"123456"}`)))
    if out.Code != 401 { t.Fatal("anonymous authenticator test accepted", out.Code) }
    if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestMFAErrorResponses(t *testing.T) {
    for _,tc:=range []struct{err error;status int}{
        {ErrMFAUserNotFound,404},{ErrUnauthorized,401},{ErrMFARequired,403},{ErrMFAEnrollmentRequired,403},{apperror.Forbidden,403},
        {ErrMFARateLimited,429},{ErrMFAUnavailable,503},{ErrMFAAlreadyEnabled,409},
        {ErrMFAInvalid,400},{ErrIncorrectPassword,400},{ErrInvalidCredential,400},{ErrInvalid,400},
        {errors.New("storage failure"),500},
    } {
        out:=httptest.NewRecorder();writeMFAError(out,httptest.NewRequest("POST","/pub/login",nil),tc.err)
        if out.Code!=tc.status {t.Fatal(out.Code)}
        if strings.Contains(out.Body.String(),"storage failure") {t.Fatal("internal error leaked")}
    }
}

func TestMFAResetPagePermissionAndCredentialInvalidation(t *testing.T) {
    m,mock,ctx,_,_:=newMFATest(t)
    mock.ExpectQuery("WITH assigned AS").WillReturnRows(permissionRows())
    if err:=m.ResetUserMFA(ctx,7);!errors.Is(err,apperror.Forbidden) {t.Fatal("missing page grant accepted",err)}
    mock.ExpectQuery("WITH assigned AS").WillReturnRows(permissionRows().AddRow("admin:read:system:user",userReadPermissionConfig))
    mock.ExpectBegin()
    mock.ExpectQuery("UPDATE t_user SET credential_version").WithArgs(sqlmock.AnyArg(),int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
    mock.ExpectExec("DELETE FROM t_user_recovery_code").WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0,10))
    mock.ExpectExec("DELETE FROM t_user_totp").WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0,1));mock.ExpectCommit()
    if err:=m.ResetUserMFA(ctx,7);err!=nil {t.Fatal(err)}
    // The reset endpoint follows the same page permission even without user-edit authority.
    mock.ExpectQuery("WITH assigned AS").WillReturnRows(permissionRows().AddRow("admin:read:system:user",userReadPermissionConfig))
    if allowed,err:=m.AuthorizeHTTP(ctx,"POST","/mfa/reset/7");err!=nil || !allowed {t.Fatal(allowed,err)}
}

func TestMFASetupCredentialPurposeAndUserBinding(t *testing.T) {
    credentials:=newCredentialTestManager(t,time.Minute)
    challenge,err:=credentials.MFASetupChallenge(t.Context(),3);if err!=nil {t.Fatal(err)}
    wrong:=encryptLoginCredential(t,credentials,loginCredential{ChallengeID:challenge.ChallengeID,Username:"user",Password:"secret"},true)
    if _,err:=credentials.OpenMFASetup(t.Context(),3,EncryptedLoginInput{ChallengeID:challenge.ChallengeID,Credential:wrong});err==nil {t.Fatal("login credential accepted for MFA enrollment")}
}

func TestMFAResetMissingUser(t *testing.T) {
    m,mock,ctx,_,_:=newMFATest(t)
    mock.ExpectQuery("WITH assigned AS").WillReturnRows(permissionRows().AddRow("admin:read:system:user",userReadPermissionConfig))
    mock.ExpectBegin()
    mock.ExpectQuery("UPDATE t_user SET credential_version").WillReturnError(sql.ErrNoRows)
    mock.ExpectRollback()
    if err:=m.ResetUserMFA(ctx,999999);!errors.Is(err,ErrMFAUserNotFound) {t.Fatal(err)}
}

func TestMFAHTTPPasswordFreeSetupRequiresLoginGrant(t *testing.T) {
    m, mock, ctx, identity, _ := newMFATest(t)
    call := func(body string) *httptest.ResponseRecorder {
        out := httptest.NewRecorder()
        mfaTestHTTP(m).ServeHTTP(out, httptest.NewRequest("POST", "/mfa/setup", strings.NewReader(body)).WithContext(ctx))
        return out
    }
    mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t,"pass")))
    if out := call(`{}`); out.Code != 400 { t.Fatal("missing password accepted without login grant", out.Code) }
    if err := m.grantMFAEnrollment(ctx, identity, identity.SessionID); err != nil { t.Fatal(err) }
    mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t,"pass")))
    expectMFAEnabled(mock,false)
    if out := call(`{}`); out.Code != 200 || !strings.Contains(out.Body.String(),"otpauth://") || out.Header().Get("Cache-Control") != "no-store" { t.Fatal("login enrollment failed", out.Code) }
    // Supplying incomplete encrypted input cannot fall back to the grant.
    if out := call(`{"challenge_id":"invalid"}`); out.Code != 400 { t.Fatal(out.Code) }
    if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestMFAConfirmPasswordResetHeaderOnly(t *testing.T) {
	m, mock, _, identity, _ := newMFATest(t)
	manager, err := authn.New("test", authTestKey, time.Hour, NewIdentityValidator(m.store, m.sessions))
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count").WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow(identity.Username, identity.Code, userStatusActive, identity.CredentialVersion, int64(0)))
	request := httptest.NewRequest("POST", "/mfa/setup", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request, err = manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx := request.Context()
	mock.ExpectQuery("SELECT password FROM").WillReturnRows(sqlmock.NewRows([]string{"password"}).AddRow(passwordHash(t, "pass")))
	expectMFAEnabled(mock, false)
	setup, err := m.BeginMFAEnrollment(ctx, "pass")
	if err != nil {
		t.Fatal(err)
	}
	code, err := totpCode(setup.Secret, time.Now().Unix()/30)
	if err != nil {
		t.Fatal(err)
	}
	expectMFALock(mock, 1)
	expectMFAEnabled(mock, false)
	mock.ExpectExec("INSERT INTO t_user_totp").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("DELETE FROM t_user_recovery_code").WillReturnResult(sqlmock.NewResult(0, 0))
	for range 10 {
		mock.ExpectExec("INSERT INTO t_user_recovery_code").WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectExec("UPDATE t_user SET credential_version").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	body, _ := json.Marshal(MFAVerificationInput{VerificationCode: code})
	out := httptest.NewRecorder()
	mfaTestHTTP(m).ServeHTTP(out, httptest.NewRequest("POST", "/mfa/confirm", strings.NewReader(string(body))).WithContext(ctx))
	if out.Code != 200 || out.Header().Get(authn.PasswordResetRequiredHeader) != "true" || out.Header().Get(authn.MFAEnrollmentRequiredHeader) != "" {
		t.Fatal("MFA confirmation lost the remaining password reset requirement", out.Code)
	}
	assertNoPasswordResetPayload(t, out.Body.Bytes())
	var payload map[string]any
	if err := json.Unmarshal(out.Body.Bytes(), &payload); err != nil || len(payload) != 4 || payload["access_token"] == "" || payload["recovery_codes"] == nil {
		t.Fatal("MFA confirmation changed session or recovery-code payload", err)
	}
}
