package tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/audit"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
)

type emailSink struct {
	mu                           sync.Mutex
	code                         string
	unlinkRecipient, unlinkToken string
	linkedRecipient, linkedBody string
	sent                         int
}

func (*emailSink) Ready() bool { return true }
func (s *emailSink) SendHTML(_ context.Context, recipient, subject string, body, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if code := regexp.MustCompile(`[0-9]{6}`).FindString(body); code != "" { s.code = code }
	if strings.Contains(subject, "邮箱已绑定") || strings.Contains(subject, "Email linked") { s.linkedRecipient, s.linkedBody = recipient, body }
	if _, fragment, ok := strings.Cut(body, "#token="); ok {
		s.unlinkRecipient = recipient
		s.unlinkToken, _, _ = strings.Cut(fragment, "\n")
	}
	s.sent++
	return nil
}
func (s *emailSink) latest() string { s.mu.Lock(); defer s.mu.Unlock(); return s.code }

func TestEmailIdentityAndSharedBudgets(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	localizer := testLocalization(t, store)
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	mustExec(`INSERT INTO t_user (id, username, code, password, status, login_count, email, email_verified_at) VALUES (101, 'emailtester', 'EMAIL101', $1, 1, 1, 'tester@example.com', 1), (102, 'unboundtester', 'EMAIL102', $1, 1, 1, NULL, NULL), (103, 'anotheruser', 'EMAIL103', $1, 1, 1, NULL, NULL)`, string(hash))
	limits, _ := pagination.New(100, 100)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, localizer)
	if err != nil {
		t.Fatal(err)
	}
	captcha, err := auth.NewPointCaptcha(auth.NewMemoryPointCaptchaStore(), dictionaries, 5, time.Minute, 300, 180, 22, false)
	if err != nil {
		t.Fatal(err)
	}
	guard, _ := auth.NewPasswordChangeGuard(auth.NewMemoryPasswordFailureStore(), captcha, 3, 6, time.Minute)
	slider, _ := auth.NewSliderCaptcha(auth.NewMemoryPointCaptchaStore(), auth.SliderCaptchaConfig{TTL: time.Minute, MinimumDuration: 100 * time.Millisecond})
	memory := auth.NewMemorySessionStore()
	t.Cleanup(func() { _ = memory.Close() })
	key := strings.Repeat("k", 64)
	sessions, _ := auth.NewSessions(memory, auth.SessionConfig{Issuer: "email-test", Key: key, SessionLifetime: time.Hour, RememberLifetime: 24 * time.Hour, RetryWindow: time.Second})
	manager, _ := authn.New("email-test", key, time.Hour, auth.NewIdentityValidator(store, sessions))
	module := auth.New(store, manager, nil, sessions, captcha, slider, guard, localizer, nil, &auth.Verification{})
	sink := &emailSink{}
	cfg := auth.EmailConfig{UnlinkURL: "https://pc.example.test/auth/unbind-email", RequireBinding: true, CodeTTL: time.Minute, ResendInterval: time.Minute, SendLimit: 3, MaxAttempts: 3}
	codes, err := newTemplateEmailCodes(store, sink, cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	module.SetEmailCodes(codes)

    stopWorkers, err := codes.StartWorkers(t.Context())
    if err != nil { t.Fatal(err) }
    t.Cleanup(stopWorkers)
	waitDelivered := func(id string) {
		t.Helper()
		deadline := time.Now().Add(5*time.Second)
		for time.Now().Before(deadline) {
			var delivered bool
			if err := store.DB.QueryRowContext(ctx, `SELECT delivered FROM t_auth_email_code WHERE verification_id=$1`, id).Scan(&delivered); err != nil { t.Fatal(err) }
			if delivered { return }
			time.Sleep(5*time.Millisecond)
		}
		t.Fatal("SMTP worker did not deliver ticket")
	}

	passwordLogin := func(identifier string) *auth.LoginResult {
		t.Helper()
		input, err := sliderLoginInput(ctx, slider, identifier, "secret", false)
		if err != nil {
			t.Fatal(err)
		}
		result, err := module.Login(ctx, input)
		if err != nil {
			t.Fatal(identifier, err)
		}
		return result
	}
	authenticate := func(token string) context.Context {
		t.Helper()
		req := httptest.NewRequest("GET", "/info", nil).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer "+token)
		verified, err := manager.AuthenticateRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		return verified.Context()
	}
	passwordLogin("emailtester")
	passwordLogin("TESTER@EXAMPLE.COM")
	for _, identifier := range []string{"emailtester", "tester@example.com"} {
		mustExec(`DELETE FROM t_auth_email_cooldown`)
		uuid := "a6eefc2b-1e8d-4446-9f69-3f2bd62d8a75"
		state, err := codes.Send(ctx, uuid, "192.0.2.1", "login", identifier, "tester@example.com", 101, 1)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := codes.Status(ctx, uuid, "192.0.2.1", "login", identifier, 0)
		if err != nil || restored.RetryAt != state.RetryAt || restored.VerificationID != state.VerificationID {
			t.Fatal("refresh lost server countdown", restored, err)
		}
		anotherUUID := "b6eefc2b-1e8d-4446-9f69-3f2bd62d8a75"
		other, err := codes.Status(ctx, anotherUUID, "192.0.2.1", "login", identifier, 0)
		if err != nil || other.RetryAt != state.RetryAt || other.VerificationID != "" {
			t.Fatal("IP cooldown or UUID isolation failed", other, err)
		}
		if _, err := codes.Send(ctx, anotherUUID, "192.0.2.1", "login", identifier, "tester@example.com", 101, 1); !errors.Is(err, auth.ErrEmailCooldown) {
			t.Fatal("new browser bypassed IP cooldown", err)
		}
		if _, err := codes.Send(ctx, uuid, "192.0.2.2", "login", identifier, "tester@example.com", 101, 1); !errors.Is(err, auth.ErrEmailCooldown) {
			t.Fatal("new IP bypassed UUID cooldown", err)
		}
		waitDelivered(state.VerificationID)
		input := auth.LoginInput{Method: "email_code", Username: identifier, BrowserUUID: uuid, VerificationID: state.VerificationID, VerificationCode: sink.latest()}
		wrongContext := input
		wrongContext.Username = "emailtester"
		if identifier == "emailtester" {
			wrongContext.Username = "tester@example.com"
		}
		if _, err := module.Login(ctx, wrongContext); !errors.Is(err, auth.ErrEmailInvalid) {
			t.Fatal("code crossed identifier context", err)
		}
		results := make(chan error, 2)
		for range 2 {
			go func() { _, err := module.Login(ctx, input); results <- err }()
		}
		successes := 0
		for range 2 {
			err := <-results
			if err == nil {
				successes++
			} else if !errors.Is(err, auth.ErrEmailInvalid) {
				t.Fatal(err)
			}
		}
		if successes != 1 {
			t.Fatal("code consumed more than once", successes)
		}
	}
	unbound := passwordLogin("unboundtester")
	authCtx := authenticate(unbound.AccessToken)
	mustExec(`UPDATE t_user SET action=(SELECT code FROM t_action WHERE scope='admin' AND config->>'permission_scope'='*') WHERE id=102`)
	manager.SetHTTPSecurityPolicy(module.CheckHTTPSecurity)
	actions := action.New(store, limits, localizer)
	users := user.New(store, limits, nil, guard, actions, role.New(store, limits, actions))
	httpConfig := &config.Config{}
	httpConfig.Auth.Authorization.Enabled = true
	httpConfig.Idempotency.TTL = time.Minute
	auditModule := audit.New(store, limits, config.DefaultAudit())
	handler, err := server.NewRouter(httpConfig, manager, idempotency.NewMemoryStore(), module, users, auditModule)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/user", "/permission"} {
		request := httptest.NewRequest("GET", path, nil)
		request.Header.Set("Authorization", "Bearer "+unbound.AccessToken)
		request.Header.Set("X-Original-Method", "GET")
		request.Header.Set("X-Original-URI", "/user")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var failure struct {
			ErrorCode string `json:"error_code"`
		}
		_ = json.Unmarshal(response.Body.Bytes(), &failure)
		if response.Code != 403 || failure.ErrorCode != "AUTH_EMAIL_BINDING_REQUIRED" {
			t.Fatal("HTTP or gateway binding restriction missing", path, response.Code, failure.ErrorCode)
		}
	}

	info, err := module.InfoSelected(authCtx, "admin", report.Fields{"email", "email_verified_at", "email_binding_required"})
	if err != nil || !info.EmailBindingRequired || info.Email != "" || info.EmailVerifiedAt != nil {
		t.Fatal("info binding requirement missing", info, err)
	}
	request := httptest.NewRequest("GET", "/user", nil).WithContext(authCtx)
	if err := module.CheckHTTPSecurity(request); !errors.Is(err, auth.ErrEmailBindingRequired) {
		t.Fatal("mandatory binding bypassed", err)
	}
	mustExec(`DELETE FROM t_auth_email_cooldown`)
	state, err := codes.Send(authCtx, "c6eefc2b-1e8d-4446-9f69-3f2bd62d8a75", "192.0.2.3", "bind", "bound@example.com", "bound@example.com", 102, 1)
	if err != nil {
		t.Fatal(err)
	}
	waitDelivered(state.VerificationID)
	bound, err := module.BindEmail(authCtx, auth.EmailBindInput{Email: "BOUND@example.com", BrowserUUID: "c6eefc2b-1e8d-4446-9f69-3f2bd62d8a75", VerificationID: state.VerificationID, VerificationCode: sink.latest()})
	if err != nil {
		t.Fatal(err)
	}

	var noticeID, noticeRecipient string
	if err := store.DB.QueryRowContext(ctx, `SELECT verification_id,recipient FROM t_auth_email_code WHERE purpose='email_linked' AND user_id=102`).Scan(&noticeID,&noticeRecipient); err != nil { t.Fatal(err) }
	if noticeRecipient != "bound@example.com" { t.Fatal("notice sent to an address other than the newly bound mailbox") }
	waitDelivered(noticeID)
	sink.mu.Lock()
	linkedRecipient, linkedBody := sink.linkedRecipient, sink.linkedBody
	sink.mu.Unlock()
	if linkedRecipient != noticeRecipient || !strings.Contains(linkedBody,"un***") || strings.Contains(linkedBody,"unboundtester") { t.Fatal("binding notice recipient or account masking incorrect") }
	if _, err := module.BindEmail(authCtx, auth.EmailBindInput{Email:"bound@example.com", BrowserUUID:"c6eefc2b-1e8d-4446-9f69-3f2bd62d8a75", VerificationID:state.VerificationID, VerificationCode:sink.latest()}); err == nil { t.Fatal("replayed binding succeeded") }
	var noticeCount int
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_auth_email_code WHERE purpose='email_linked' AND user_id=102`).Scan(&noticeCount); err != nil || noticeCount != 1 { t.Fatal("duplicate binding notice",err) }

	t.Run("notice failure rolls back binding and consumed proof",func(t *testing.T) {
		rollbackSession := passwordLogin("anotheruser")
		rollbackContext := authenticate(rollbackSession.AccessToken)
		mustExec(`DELETE FROM t_auth_email_cooldown`)
		proof, err := codes.Send(rollbackContext,"e6eefc2b-1e8d-4446-9f69-3f2bd62d8a75","192.0.2.44","bind","rollback@example.test","rollback@example.test",103,1)
		if err != nil { t.Fatal(err) }
		waitDelivered(proof.VerificationID)
		verification := sink.latest()
		mustExec(`CREATE FUNCTION reject_email_linked_notice() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.purpose='email_linked' THEN RAISE EXCEPTION 'notice storage unavailable'; END IF; RETURN NEW; END $$;
		CREATE TRIGGER reject_email_linked_notice BEFORE INSERT ON t_auth_email_code FOR EACH ROW EXECUTE FUNCTION reject_email_linked_notice();`)
		defer mustExec(`DROP TRIGGER reject_email_linked_notice ON t_auth_email_code; DROP FUNCTION reject_email_linked_notice();`)
		_, err = module.BindEmail(rollbackContext,auth.EmailBindInput{Email:"rollback@example.test",BrowserUUID:"e6eefc2b-1e8d-4446-9f69-3f2bd62d8a75",VerificationID:proof.VerificationID,VerificationCode:verification})
		if err == nil { t.Fatal("binding committed without its notification") }
		var address string
		var version int
		var consumed bool
		if err := store.DB.QueryRowContext(ctx,`SELECT COALESCE(email,''),credential_version FROM t_user WHERE id=103`).Scan(&address,&version); err != nil || address!="" || version!=1 { t.Fatal("binding was not rolled back",err) }
		if err := store.DB.QueryRowContext(ctx,`SELECT consumed FROM t_auth_email_code WHERE verification_id=$1`,proof.VerificationID).Scan(&consumed); err != nil || consumed { t.Fatal("failed binding consumed verification proof",err) }
	})
	info, err = module.InfoSelected(authenticate(bound.AccessToken), "admin", report.Fields{"email", "email_verified_at", "email_binding_required"})
	if err != nil || info.EmailBindingRequired || info.Email != "bound@example.com" || info.EmailVerifiedAt == nil || *info.EmailVerifiedAt <= 0 {
		t.Fatal("binding did not update info", info, err)
	}
	req := httptest.NewRequest("GET", "/info", nil)
	req.Header.Set("Authorization", "Bearer "+unbound.AccessToken)
	if _, err := manager.AuthenticateRequest(req); err == nil {
		t.Fatal("old session survived email credential version change")
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET email = 'bound@example.com' WHERE id = 103`); err == nil {
		t.Fatal("duplicate email accepted")
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET username = 'bound@example.com' WHERE id = 103`); err == nil {
		t.Fatal("cross-user login alias collision accepted")
	}
	t.Run("unlink targets the signed-in owner", func(t *testing.T) {
		mustExec(`DELETE FROM t_auth_email_cooldown`)
		post := func(path, body, accessToken string) *httptest.ResponseRecorder {
			t.Helper()
			req := httptest.NewRequest("POST", path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if accessToken != "" {
				req.Header.Set("Authorization", "Bearer "+accessToken)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			return response
		}
		attempt := httptest.NewRequest("POST", "/email/unbind", strings.NewReader("{}"))
		attempt.Header.Set("Authorization", "Bearer "+bound.AccessToken)
		attempt.Header.Set("Content-Type", "application/json")
		attempt.Header.Set("X-TOTP-Code", "123456")
		failed := httptest.NewRecorder()
		handler.ServeHTTP(failed, attempt)
		if failed.Code != 400 {
			t.Fatal("unexpected password-only account result", failed.Code)
		}
        sent := post("/email/unbind/request", `{"browser_uuid":"c6eefc2b-1e8d-4446-9f69-3f2bd62d8a75"}`, bound.AccessToken)
		var status auth.EmailCodeStatus
		if sent.Code != 200 || json.Unmarshal(sent.Body.Bytes(), &status) != nil || status.VerificationID == "" {
			t.Fatal("request unlink", sent.Code)
		}
		waitDelivered(status.VerificationID)
		sink.mu.Lock()
		recipient, token := sink.unlinkRecipient, sink.unlinkToken
		sink.mu.Unlock()
		if recipient != "bound@example.com" || token == "" {
			t.Fatal("confirmation did not reach the session owner's mailbox")
		}
		confirmation, err := json.Marshal(map[string]string{"token": token})
		if err != nil {
			t.Fatal(err)
		}
		if result := post("/pub/email/unbind", string(confirmation), ""); result.Code != 200 {
			t.Fatal("confirm unlink", result.Code)
		}
		if result := post("/pub/email/unbind", string(confirmation), ""); result.Code != 200 {
			t.Fatal("reopen confirmation", result.Code)
		}
		if result := post("/pub/email/unbind", `{"token":"invalid"}`, ""); result.Code != 409 {
			t.Fatal("invalid confirmation", result.Code)
		}
		for _, tc := range []struct {
			operation           string
			status              int
			actor               int64
			resource, errorCode string
		}{
			{"request_email_unlink", 200, 102, "102", ""},
			{"unlink_email", 400, 102, "102", "AUTH_INCORRECT_PASSWORD"},
			{"confirm_email_unlink", 200, 0, "102", ""},
			{"view_email_unlink_result", 200, 0, "102", ""},
			{"confirm_email_unlink", 409, 0, "", "AUTH_UNLINK_INVALID"},
		} {
			var count int
			err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM t_audit_event WHERE kind='operation' AND module='auth' AND resource_type='user' AND operation=$1 AND status=$2 AND COALESCE(user_id,0)=$3 AND resource_id=$4 AND error_code=$5 AND request_params='null'::jsonb AND response_params='null'::jsonb AND COALESCE(header->>'X-Mfa-Code','[redacted]')='[redacted]' AND COALESCE(header->>'Authorization','[redacted]')='[redacted]'`, tc.operation, tc.status, tc.actor, tc.resource, tc.errorCode).Scan(&count)
			if err != nil || count != 1 {
				t.Fatalf("audit %s/%d: rows=%d err=%v", tc.operation, tc.status, count, err)
			}
		}
		var ownerUnbound, otherStillBound bool
		err = store.DB.QueryRowContext(ctx, `SELECT (SELECT email IS NULL AND email_verified_at IS NULL FROM t_user WHERE id=102),(SELECT email='tester@example.com' AND email_verified_at IS NOT NULL FROM t_user WHERE id=101)`).Scan(&ownerUnbound, &otherStillBound)
		if err != nil || !ownerUnbound || !otherStillBound {
			t.Fatal("unlink changed the wrong binding", err)
		}
	})
	cfg.RequireBinding = false
	optional, err := newTemplateEmailCodes(store, sink, cfg, key)
	if err != nil {
		t.Fatal(err)
	}
	module.SetEmailCodes(optional)
	another := passwordLogin("anotheruser")
	// No-menu users still need self /info to reach the personal-page fallback.
	request = httptest.NewRequest("GET", "/info", nil)
	request.Header.Set("Authorization", "Bearer "+another.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var self auth.UserInfo
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &self) != nil || len(self.Permission.Menus) != 0 {
		t.Fatal("self info unavailable without menus", response.Code)
	}
	request = httptest.NewRequest("GET", "/user", nil)
	request.Header.Set("Authorization", "Bearer "+another.AccessToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("self info granted business permission", response.Code)
	}

	info, err = module.InfoSelected(authenticate(another.AccessToken), "admin", report.Fields{"email_binding_required"})
	if err != nil || info.EmailBindingRequired {
		t.Fatal("optional binding became mandatory", info, err)
	}
	// The preference belongs to this identity across sessions, even without menus.
	mustExec(`UPDATE t_user SET metadata = '{"existing":42}' WHERE id = 103`)
	request = httptest.NewRequest("PATCH", "/info", strings.NewReader(`{"metadata":{"email_binding_skipped":true}}`))
	request.Header.Set("Authorization", "Bearer "+another.AccessToken)
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 { t.Fatalf("save preference: %d %s", response.Code, response.Body.String()) }
	var preserved bool
	if err := store.DB.QueryRowContext(ctx, `SELECT metadata @> '{"existing":42,"email_binding_skipped":true}'::jsonb FROM t_user WHERE id = 103`).Scan(&preserved); err != nil || !preserved {
		t.Fatal("preference update replaced unrelated metadata", err)
	}
    if err := module.UpdateLanguage(authenticate(another.AccessToken), "zh-CN"); err != nil { t.Fatal(err) }
    if err := store.DB.QueryRowContext(ctx, `SELECT metadata @> '{"existing":42,"email_binding_skipped":true,"language":"zh-CN"}'::jsonb FROM t_user WHERE id=103`).Scan(&preserved); err != nil || !preserved { t.Fatal("language replaced metadata", err) }
    if err := module.UpdateTimezone(authenticate(another.AccessToken), "America/New_York"); err != nil { t.Fatal(err) }
    if err := store.DB.QueryRowContext(ctx, `SELECT metadata @> '{"existing":42,"email_binding_skipped":true,"language":"zh-CN","timezone":"America/New_York"}'::jsonb FROM t_user WHERE id=103`).Scan(&preserved); err != nil || !preserved { t.Fatal("timezone replaced metadata", err) }
	verifyUIPreferencePersistence(t, authenticate(another.AccessToken), store, module, dictionaries)
	later := authenticate(passwordLogin("anotheruser").AccessToken)
	module.SetEmailCodes(codes)
	info, err = module.InfoSelected(later, "admin", report.Fields{"metadata", "email_binding_required"})
	if err != nil || info.Metadata == nil || info.Metadata.Language != "zh-CN" || info.Metadata.Timezone != "America/New_York" || !info.Metadata.EmailBindingSkipped || !info.EmailBindingRequired {
		t.Fatal("saved preference or mandatory override missing", err)
	}
	module.SetEmailCodes(optional)
	// Exercise real binding against a concurrent resend foreign-key check and
	// administrator rename. These operations must not form a row/advisory lock cycle.
	var schema string
	if err := store.DB.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	waitForLock := func(pattern string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			var waiting bool
			err := store.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE application_name = $1 AND wait_event_type = 'Lock' AND query LIKE $2)`, schema, pattern).Scan(&waiting)
			if err != nil {
				t.Fatal(err)
			}
			if waiting {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("concurrent operation did not reach its expected database lock")
	}
	for index, operation := range []string{"resend", "rename"} {
		id := int64(104 + index)
		username := "concurrent-" + operation
		address := operation + "@concurrent.example.com"
		mustExec(`INSERT INTO t_user (id, username, code, password, status, login_count) VALUES ($1,$2,$3,$4,1,1)`, id, username, fmt.Sprintf("RACE%04d", id), string(hash))
		session := passwordLogin(username)
		bindingContext := authenticate(session.AccessToken)
		mustExec(`DELETE FROM t_auth_email_cooldown`)
		uuid := "d6eefc2b-1e8d-4446-9f69-3f2bd62d8a75"
		state, err := codes.Send(ctx, uuid, "192.0.2.9", "bind", address, address, id, 1)
		if err != nil {
			t.Fatal(err)
		}
		waitDelivered(state.VerificationID)
		verification := sink.latest()
		blocking, err := store.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer blocking.Rollback()
		var locked string
		if err := blocking.QueryRowContext(ctx, `SELECT verification_id FROM t_auth_email_code WHERE verification_id=$1 FOR UPDATE`, state.VerificationID).Scan(&locked); err != nil {
			_ = blocking.Rollback()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			_, err := module.BindEmail(bindingContext, auth.EmailBindInput{Email: address, BrowserUUID: uuid, VerificationID: state.VerificationID, VerificationCode: verification})
			done <- err
		}()
		waitForLock("SELECT code_hash, attempts%")
		if operation == "resend" {
			short, cancel := context.WithTimeout(ctx, time.Second)
			_, err = blocking.ExecContext(short, `INSERT INTO t_auth_email_code (verification_id,browser_uuid,client_ip,purpose,identifier,user_id,recipient,credential_version,code_hash,expired_at) VALUES ('concurrent-resend',$1,'192.0.2.9','bind',$2,$3,$2,1,'unused',1)`, uuid, address, id)
			cancel()
			_ = blocking.Rollback()
			bindErr := <-done
			if err != nil || bindErr != nil {
				t.Fatalf("binding/resend lock cycle: resend=%v bind=%v", err, bindErr)
			}
		} else {
			renamed := make(chan error, 1)
			go func() {
				_, err := store.DB.ExecContext(ctx, `UPDATE t_user SET username = $1 WHERE id = $2`, username+"-renamed", id)
				renamed <- err
			}()
			waitForLock("UPDATE t_user SET username =%")
			_ = blocking.Rollback()
			bindErr, renameErr := <-done, <-renamed
			if bindErr != nil || renameErr != nil {
				t.Fatalf("binding/rename lock cycle: bind=%v rename=%v", bindErr, renameErr)
			}
		}
	}

}

func TestConcurrentUsernameAndEmailAliases(t *testing.T) {
	store := permissionDatabase(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for index := range 2 {
		go func(index int) {
			<-start
			var err error
			if index == 0 {
				_, err = store.DB.ExecContext(t.Context(), `INSERT INTO t_user (id, username, code, password, status) VALUES (301, 'collision@example.com', 'COLL0301', 'hash', 1)`)
			} else {
				_, err = store.DB.ExecContext(t.Context(), `INSERT INTO t_user (id, username, code, password, status, email, email_verified_at) VALUES (302, 'collision-user', 'COLL0302', 'hash', 1, 'collision@example.com', 1)`)
			}
			results <- err
		}(index)
	}
	close(start)
	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent alias writes: %d succeeded, want exactly one", successes)
	}
	var matches int
	if err := store.DB.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM t_user WHERE username = 'collision@example.com' OR email = 'collision@example.com'`).Scan(&matches); err != nil || matches != 1 {
		t.Fatal("ambiguous alias committed", matches, err)
	}
}
