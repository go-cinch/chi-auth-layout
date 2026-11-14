package auth

import (
    "{{ .Computed.module_name_final }}/internal/common/authn"
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/DATA-DOG/go-sqlmock"
    "github.com/go-jose/go-jose/v4"
{{ if eq .Computed.http_router_final "gin" }}
    "github.com/gin-gonic/gin"
{{ end }}
)

func emailHTTP(t *testing.T, m *Module, ctx context.Context, method, path string, body any, verificationCodes ...string) *httptest.ResponseRecorder {
    t.Helper()
    return emailHTTPHeader(t,m,ctx,method,path,body,"X-TOTP-Code",verificationCodes...)
}

func emailHTTPHeader(t *testing.T,m *Module,ctx context.Context,method,path string,body any,header string,values ...string)*httptest.ResponseRecorder {
    t.Helper()
    raw, _ := json.Marshal(body)
    req := httptest.NewRequest(method, path, strings.NewReader(string(raw))).WithContext(ctx)
    req.RemoteAddr = "192.0.2.1:1234"
    req.Header.Set("Content-Type", "application/json")
    for _,value := range values { req.Header.Add(header, value) }
    recorder := httptest.NewRecorder()
{{ if eq .Computed.http_router_final "gin" }}
    gin.SetMode(gin.TestMode)
    handler := gin.New()
    m.HTTP(handler.Group(""))
{{ else }}
    handler := m.HTTP()
{{ end }}
    handler.ServeHTTP(recorder, req)
    return recorder
}

func attachTestEmail(t *testing.T, m *Module) *testEmailSender {
    t.Helper()
    sender := &testEmailSender{}
    codes, err := NewEmailCodes(m.store, sender, emailTestConfig(), authTestKey)
    if err != nil { t.Fatal(err) }
    m.SetEmailCodes(codes)
    return sender
}

func expectEmailStatus(mock sqlmock.Sqlmock, deadline int64, pending bool) {
    mock.ExpectQuery("SELECT COALESCE\\(MAX").WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(deadline))
    rows := sqlmock.NewRows([]string{"id", "expired_at"})
    if pending { rows.AddRow("pending-id", deadline+60000) }
    mock.ExpectQuery("SELECT verification_id, expired_at").WillReturnRows(rows)
}

func expectEmailReservation(mock sqlmock.Sqlmock, recipient bool) {
    keys := 2; if recipient { keys++ }
    mock.ExpectBegin()
    for range keys { mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1)) }
    mock.ExpectQuery("SELECT COALESCE\\(MAX").WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(0))
    for range keys { mock.ExpectExec("INSERT INTO t_auth_email_cooldown").WillReturnResult(sqlmock.NewResult(0, 1)) }
    mock.ExpectExec("UPDATE t_auth_email_code").WillReturnResult(sqlmock.NewResult(0, 1))
    mock.ExpectExec("INSERT INTO t_auth_email_code").WillReturnResult(sqlmock.NewResult(0, 1))
expectEmailSeal(mock)
}
func expectEmailSend(mock sqlmock.Sqlmock, recipient bool) {
    expectEmailReservation(mock, recipient)
    mock.ExpectCommit()
}

func TestPublicSystemProperties(t *testing.T) {
    m, _, _ := newAuthTestModule(t)
    sender := attachTestEmail(t, m)
    for _, emailEnabled := range []bool{false, true} {
        sender.disabled = !emailEnabled
        for _, totpEnabled := range []bool{false, true} {
            m.verification.config.TOTPEnabled = totpEnabled
            response := emailHTTP(t, m, t.Context(), http.MethodGet, "/pub/opt", nil)
            var properties SystemProperties
            if err := json.Unmarshal(response.Body.Bytes(), &properties); err != nil { t.Fatal(err) }
            want := map[string]bool{"email_enabled": emailEnabled, "totp_enabled": totpEnabled}
            if response.Code != http.StatusOK || properties.EmailEnabled != emailEnabled || properties.TOTPEnabled != totpEnabled || len(properties.Locales) == 0 || properties.DefaultLocale == "" {
                t.Fatalf("unexpected public properties: status=%d, got=%v, want=%v", response.Code, properties, want)
            }
        }
    }
}

func TestEmailHTTPStatusAndValidation(t *testing.T) {
    m, manager, mock := newAuthTestModule(t)
    ctx := authenticatedContext(t, manager, m.sessions)
    if got := emailHTTP(t, m, ctx, "GET", "/pub/login/email-code/status?browser_uuid="+emailTestUUID+"&username=tester", nil).Code; got != 503 { t.Fatal(got) }
    attachTestEmail(t, m)
    for _, path := range []string{"/pub/login/email-code/status", "/pub/login/email-code/status?browser_uuid=invalid&username=x", "/pub/login/email-code/status?bad=%zz", "/email/code/status?browser_uuid="+emailTestUUID+"&email=invalid"} {
        if got := emailHTTP(t, m, ctx, "GET", path, nil).Code; got != 400 { t.Fatal(path, got) }
    }
    for _, path := range []string{"/pub/login/email-code/status?browser_uuid="+emailTestUUID+"&username=tester", "/email/code/status?browser_uuid="+emailTestUUID+"&email=tester@example.com"} {
        deadline := time.Now().Add(time.Minute).UnixMilli()
        expectEmailStatus(mock, deadline, true)
        response := emailHTTP(t, m, ctx, "GET", path, nil)
        var status EmailCodeStatus
        if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &status) != nil || status.RetryAt != deadline || status.VerificationID != "pending-id" { t.Fatal("status restoration failed", response.Code) }
        if response.Header().Get("Cache-Control") != "no-store" { t.Fatal("status response must not be cached") }
    }
    for _, path := range []string{"/pub/login/email-code", "/email/code", "/email/bind"} {
        if got := emailHTTP(t, m, ctx, "POST", path, map[string]string{"unknown":"field"}).Code; got != 400 { t.Fatal(path, got) }
    }
    mock.ExpectQuery("SELECT COALESCE\\(MAX").WillReturnError(errors.New("database unavailable"))
    if got := emailHTTP(t, m, ctx, "GET", "/pub/login/email-code/status?browser_uuid="+emailTestUUID+"&username=x", nil).Code; got != 500 { t.Fatal("database failure masked", got) }
}

func TestEmailHTTPSendAndResendUseOneBudget(t *testing.T) {
    for _, known := range []bool{false, true} {
        t.Run(map[bool]string{false:"unknown",true:"known"}[known], func(t *testing.T) {
            m, _, mock := newAuthTestModule(t)
            sender := attachTestEmail(t, m)
            expectEmailStatus(mock, 0, false)
            rows := sqlmock.NewRows([]string{"username"}); if known { rows.AddRow("readonly") }
            mock.ExpectQuery("SELECT username FROM t_user").WillReturnRows(rows)
            if known {
                mock.ExpectQuery("SELECT id, username").WillReturnRows(loginRows(t, userStatusActive))
                mock.ExpectQuery("SELECT COALESCE\\(email").WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("test@example.com"))
            }
            expectEmailSend(mock, known)
            input := EmailCodeInput{BrowserUUID: emailTestUUID, Username: "test@example.com", SliderProof: validSliderProof(t, m.sliderCaptcha, "email_login", "test@example.com")}
            response := emailHTTP(t, m, t.Context(), "POST", "/pub/login/email-code", input)
            var state EmailCodeStatus
            if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &state) != nil || state.VerificationID == "" || state.RetryAt <= state.ServerTime { t.Fatal("send failed", response.Code) }
            if sender.recipient != "" { t.Fatal("HTTP producer performed SMTP") }
            expectEmailStatus(mock, state.RetryAt, true)
            response = emailHTTP(t, m, t.Context(), "POST", "/pub/login/email-code", input)
            if response.Code != 429 || response.Header().Get("Retry-After") == "" { t.Fatal("same endpoint resend bypassed cooldown", response.Code) }
        })
    }
}

func TestEmailHTTPVerificationAndSMTPFailure(t *testing.T) {
    m, _, mock := newAuthTestModule(t)
    sender := attachTestEmail(t, m)
    input := EmailCodeInput{BrowserUUID: emailTestUUID, Username: "test@example.com"}
    expectEmailStatus(mock, 0, false)
    mock.ExpectQuery("SELECT username FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("readonly"))
    mock.ExpectQuery("SELECT id, username").WillReturnRows(loginRows(t, userStatusActive, 5))
    response := emailHTTP(t, m, t.Context(), "POST", "/pub/login/email-code", input)
    var failure LoginFailureResponse
    if response.Code != 400 || json.Unmarshal(response.Body.Bytes(), &failure) != nil || !failure.CaptchaRequired || failure.Captcha == nil { t.Fatal("point challenge not returned by sending endpoint", response.Code) }
    sender.err = errors.New("SMTP refused delivery")
    expectEmailSend(mock, true)
    state, err := m.emailCodes.Send(t.Context(), emailTestUUID, "192.0.2.1", "bind", "test@example.com", "test@example.com", 3, 1)
    if err != nil || state.VerificationID == "" || sender.body != "" { t.Fatal("HTTP producer waited for SMTP", err) }
    sender.disabled = true
    if _, err := m.emailCodes.Send(t.Context(), emailTestUUID, "192.0.2.1", "login", "readonly", "test@example.com", 3, 1); !errors.Is(err, ErrEmailUnavailable) { t.Fatal(err) }
}

func encryptedEmailBind(t *testing.T, m *Module, payload EmailBindInput) EncryptedLoginInput {
    t.Helper()
    challenge, err := m.credentials.EmailBindChallenge(t.Context(), 3)
    if err != nil { t.Fatal(err) }
    payload.ChallengeID = challenge.ChallengeID
    return encryptedEmailInput(t,m,challenge.ChallengeID,"email-bind+jwe",payload)
}

func encryptedEmailInput(t *testing.T, m *Module, id, typ string, payload any) EncryptedLoginInput {
    t.Helper()
    encrypter, err := jose.NewEncrypter(credentialContentAlgorithm, jose.Recipient{Algorithm: credentialKeyAlgorithm, Key: &m.credentials.privateKey.PublicKey, KeyID: m.credentials.keyID}, (&jose.EncrypterOptions{}).WithType(jose.ContentType(typ)))
    if err != nil { t.Fatal(err) }
    raw, _ := json.Marshal(payload)
    object, err := encrypter.Encrypt(raw); if err != nil { t.Fatal(err) }
    compact, err := object.CompactSerialize(); if err != nil { t.Fatal(err) }
    return EncryptedLoginInput{ChallengeID: id, Credential: compact}
}

func TestEmailHTTPBindingCommitsBeforeReturningSession(t *testing.T) {
    m, manager, mock := newAuthTestModule(t)
    attachTestEmail(t, m)
    ctx := authenticatedContext(t, manager, m.sessions)
    mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
    expectEmailSend(mock, true)
    response := emailHTTP(t, m, ctx, "POST", "/email/code", EmailCodeInput{Email:"test@example.com", BrowserUUID:emailTestUUID})
    if response.Code != 200 { t.Fatal("binding send", response.Code) }
    for _, code := range []string{"000000", "123456"} {
        encrypted := encryptedEmailBind(t, m, EmailBindInput{Email:"test@example.com", BrowserUUID:emailTestUUID, VerificationID:"verification", VerificationCode:code})
        mock.ExpectBegin()
        mock.ExpectQuery("SELECT COALESCE\\(email").WillReturnRows(sqlmock.NewRows([]string{"email","version","count","status"}).AddRow("",1,1,1))
        mock.ExpectQuery("SELECT code_hash, attempts").WillReturnRows(sqlmock.NewRows([]string{"hash","attempts","expires","delivered","consumed"}).AddRow(m.emailCodes.digest("verification","123456"),0,time.Now().Add(time.Minute).UnixMilli(),true,false))
        mock.ExpectExec("UPDATE t_auth_email_code").WillReturnResult(sqlmock.NewResult(0,1))
        if code == "123456" {
            mock.ExpectExec("UPDATE t_user SET email").WillReturnResult(sqlmock.NewResult(0,1))
            mock.ExpectExec("INSERT INTO t_auth_email_code").WillReturnResult(sqlmock.NewResult(0,1))
            expectEmailSeal(mock)
        }
        mock.ExpectCommit()
        response := emailHTTP(t, m, ctx, "POST", "/email/bind", encrypted)
        want := 200; if code == "000000" { want = 400 }
        if response.Code != want { t.Fatal("binding result", response.Code) }
        if code == "123456" {
            var result LoginResult
            if json.Unmarshal(response.Body.Bytes(), &result) != nil || result.AccessToken == "" { t.Fatal("replacement session missing") }
            identity, err := manager.Parse(result.AccessToken)
            if err != nil || identity.CredentialVersion != 2 || identity.Methods != (authn.Methods{Primary: authn.Password}) { t.Fatal("email binding changed login proof or lost credential rotation", identity.Methods, err) }
        }
        if replay := emailHTTP(t, m, ctx, "POST", "/email/bind", encrypted); replay.Code != 400 { t.Fatal("binding JWE replay accepted", replay.Code) }
    }
    mock.ExpectQuery("SELECT EXISTS").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
    if response := emailHTTP(t, m, ctx, "POST", "/email/code", EmailCodeInput{Email:"test@example.com", BrowserUUID:emailTestUUID}); response.Code != 409 { t.Fatal("occupied email not rejected", response.Code) }
    if response := emailHTTP(t, m, context.Background(), "POST", "/email/code", EmailCodeInput{}); response.Code != http.StatusUnauthorized { t.Fatal(response.Code) }
    mock.ExpectQuery("SELECT COALESCE\\(email").WillReturnError(sql.ErrNoRows)
    if err := m.populateEmailInfo(ctx, &UserInfo{}); err == nil { t.Fatal("missing user treated as unbound") }
}

func TestInfoEmailBindingPreference(t *testing.T) {
    m, manager, mock := newAuthTestModule(t)
    ctx := authenticatedContext(t, manager, m.sessions)
    input := map[string]any{"metadata": map[string]bool{"email_binding_skipped": true}}
    for _, invalid := range []any{map[string]any{"id": 9, "metadata": input["metadata"]}, map[string]any{"metadata": map[string]int{"password_change_failures": 0}}, map[string]any{}} {
        if got := emailHTTP(t, m, ctx, "PATCH", "/info", invalid).Code; got != 400 { t.Fatal(got) }
    }
    if got := emailHTTP(t, m, t.Context(), "PATCH", "/info", input).Code; got != 401 { t.Fatal(got) }
    mock.ExpectExec("UPDATE t_user.*SET metadata = jsonb_set.*WHERE id = .2 AND code = .3").
        WithArgs(sqlmock.AnyArg(), int64(3), "EXP78RGH").WillReturnResult(sqlmock.NewResult(0, 1))
    if got := emailHTTP(t, m, ctx, "PATCH", "/info", input).Code; got != 200 { t.Fatal(got) }
    for _, skipped := range []bool{false, true} {
        mock.ExpectQuery("SELECT u.id, COALESCE").WithArgs(int64(3), "EXP78RGH").
            WillReturnRows(sqlmock.NewRows([]string{"id", "language", "timezone", "skipped", "totp_skipped", "password_skipped"}).AddRow(3, "", "", skipped, false, false))
        info, err := m.InfoSelected(ctx, "admin", []string{"metadata"})
        if err != nil { t.Fatal(err) }
        raw, err := json.Marshal(info)
        want, _ := json.Marshal(map[string]any{"metadata": SelfMetadata{EmailBindingSkipped: skipped}})
        if err != nil || string(raw) != string(want) { t.Fatalf("unexpected self metadata: %s", raw) }
    }
    attachTestEmail(t, m)
    m.emailCodes.config.RequireBinding = true
    if got := emailHTTP(t, m, ctx, "PATCH", "/info", input).Code; got != 403 { t.Fatal(got) }
    if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestEmailHTTPRejectsMalformedLoginAddressBeforeSideEffects(t *testing.T) {
    for _, address := range []string{"", "super", "user@", "@example.com", "user@example", "user@example.", "user@@example.com", "user name@example.com", "Name <user@example.com>"} {
        t.Run(address, func(t *testing.T) {
            m, _, mock := newAuthTestModule(t)
            sender := attachTestEmail(t, m)
            // No SQL expectations: validation must precede status queries,
            // account lookup, captcha consumption and mail queue insertion.
            response := emailHTTP(t, m, t.Context(), "POST", "/pub/login/email-code", EmailCodeInput{BrowserUUID: emailTestUUID, Username: address})
            var body struct { ErrorCode string `json:"error_code"` }
            if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.ErrorCode != "AUTH_EMAIL_ADDRESS_INVALID" {
                t.Fatalf("invalid mailbox accepted: status=%d body=%s", response.Code, response.Body.String())
            }
            if sender.recipient != "" { t.Fatal("invalid mailbox triggered delivery") }
            if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
        })
    }
}
