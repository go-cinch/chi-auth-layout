package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

func TestPermissionTargetFromRequest(t *testing.T) {
	request := httptest.NewRequest("GET", "/auth/permission", nil)
	request.Header.Set(permissionHeaderMethod, " patch ")
	request.Header.Set(permissionHeaderURI, "/gateway/user/7?verbose=true")
	request.Header.Set(permissionHeaderPath, " /user/7?verbose=true ")
	request.Header.Set(permissionHeaderResource, "auth.v1.Auth/Info")
	want := PermissionTarget{Method: "PATCH", Path: "/user/7", Resource: "/auth.v1.Auth/Info"}
	if target := PermissionTargetFromRequest(request); target != want {
		t.Fatalf("target = %#v, want %#v", target, want)
	}
	request.Header.Del(permissionHeaderPath)
	if target := PermissionTargetFromRequest(request); target.Path != "/gateway/user/7" {
		t.Fatalf("fallback target = %#v", target)
	}
}

func TestGatewayAnonymousTargets(t *testing.T) {
	m, authenticator, _ := newAuthTestModule(t)
	for _, enabled := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Auth.Authorization.Enabled = enabled
		handler, err := server.NewRouter(cfg, authenticator, nil, m)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			endpoint, method, target string
			status                   int
		}{
			{"/auth/permission", "GET", "/order/pub/item?x=1", 200},
			{"/auth/permission", "GET", "/order/pub", 200},
			{"/auth/permission", "HEAD", "/order/1", 200},
			{"/auth/permission", "OPTIONS", "/order/1", 200},
			{"/auth/permission", "GET", "/order/1", 401},
			{"/auth/permission", "GET", "/order/public", 401},
			{"/auth/permission", "GET", "/order/pub/../private", 401},
			{"/auth/permission", "GET", "/order/pub/%2e%2e/private", 401},
			{"/auth/permission", "GET", "/bad%zz/pub/item", 401},
			{"/auth/permission", "HEAD", "", 401},
			{"/auth/permission", "OPTIONS", "//host/path", 401},
			{"/auth/permission", "", "/order/pub/item", 401},
			{"/auth/info", "HEAD", "/order/1", 401},
			{"/auth/info", "GET", "/order/pub/item", 401},
		} {
			r := httptest.NewRequest(http.MethodGet, tc.endpoint, nil)
			r.Header.Set(permissionHeaderMethod, tc.method)
			r.Header.Set(permissionHeaderPath, tc.target)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("authorization=%v %s %s %s: %d", enabled, tc.endpoint, tc.method, tc.target, w.Code)
			}
			if w.Header().Get("X-Code") != "" || w.Header().Get("X-Username") != "" {
				t.Fatal("anonymous response contains identity")
			}
		}
	}
}

func TestAuthHTTP(t *testing.T) {
	m, authenticator, mock := newAuthTestModule(t)
	if m.Name() != "/auth" {
		t.Fatal(m.Name())
	}
	handler, err := server.NewRouter(&config.Config{}, authenticator, nil, m)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, target, body, token string, want int) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		handler.ServeHTTP(recorder, req)
		if recorder.Code != want {
			t.Fatalf("%s %s: got %d body %s", method, target, recorder.Code, recorder.Body.String())
		}
		return recorder
	}

	request(http.MethodGet, "/auth/info", "", "", http.StatusUnauthorized)
	request(http.MethodGet, "/auth/permission", "", "", http.StatusUnauthorized)
	request(http.MethodPost, "/auth/challenge", `{"purpose":"password_change"}`, "", http.StatusUnauthorized)
	request(http.MethodPost, "/auth/captcha", `{}`, "", http.StatusUnauthorized)
	request(http.MethodPost, "/auth/captcha/verify", `{}`, "", http.StatusUnauthorized)
	request(http.MethodPatch, "/auth/change/pwd", `{}`, "", http.StatusUnauthorized)
	request(http.MethodPost, "/auth/pub/challenge", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/challenge", `{} {}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/challenge", `{"purpose":"login","extra":true}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/challenge", `{"purpose":"password_change"}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/refresh", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/captcha/verify", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/refresh", `{}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/refresh", `{"refresh_token":"token","extra":true}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/logout", `{}`, "", http.StatusBadRequest)
	request(http.MethodGet, "/auth/pub/register/username", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/auth/pub/register/username?username=one&username=two", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/auth/pub/register/username?broken=%zz", "", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("new-user").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	availabilityResponse := request(http.MethodGet, "/auth/pub/register/username?username=new-user", "", "", http.StatusOK)
	var availability UsernameAvailability
	if err := json.Unmarshal(availabilityResponse.Body.Bytes(), &availability); err != nil || !availability.Available {
		t.Fatalf("availability response: %s %v", availabilityResponse.Body.String(), err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("existing-user").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	availabilityResponse = request(http.MethodGet, "/auth/pub/register/username?username=existing-user", "", "", http.StatusOK)
	if err := json.Unmarshal(availabilityResponse.Body.Bytes(), &availability); err != nil || availability.Available {
		t.Fatalf("existing username response: %s %v", availabilityResponse.Body.String(), err)
	}
	registrationChallengeResponse := request(http.MethodPost, "/auth/pub/challenge", `{"purpose":"register"}`, "", http.StatusOK)
	if registrationChallengeResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("registration challenge cache control = %q", registrationChallengeResponse.Header().Get("Cache-Control"))
	}
	var registrationChallenge CredentialChallenge
	if err := json.Unmarshal(registrationChallengeResponse.Body.Bytes(), &registrationChallenge); err != nil {
		t.Fatalf("registration challenge response: %s %v", registrationChallengeResponse.Body.String(), err)
	}
	if err := m.credentials.consumeChallenge(t.Context(), registrationChallenge.ChallengeID, registerCredentialType, 0, ErrInvalidRegistrationCredential); err != nil {
		t.Fatalf("registration challenge purpose: %v", err)
	}
	request(http.MethodPost, "/auth/pub/register", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/register", `{} {}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/register", `{"username":"new-user","password":"secret1","extra":true}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/register", `{}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/register", `{"challenge_id":"missing","credential":"invalid"}`, "", http.StatusBadRequest)
	registerBody := func(username, password string) string {
		t.Helper()
		input := encryptedRegisterInput(t, m.credentials, username, password)
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	request(http.MethodPost, "/auth/pub/register", registerBody("tiny", "secret1"), "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(36)))
	mock.ExpectExec("INSERT INTO t_user").WillReturnError(&pq.Error{Code: "23505", Constraint: "uk_user_username"})
	request(http.MethodPost, "/auth/pub/register", registerBody("new-user", "secret1"), "", http.StatusConflict)
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(37)))
	mock.ExpectExec("INSERT INTO t_user").WillReturnResult(sqlmock.NewResult(0, 1))
	request(http.MethodPost, "/auth/pub/register", registerBody("new-user", "secret1"), "", http.StatusOK)
	challengeResponse := request(http.MethodPost, "/auth/pub/challenge", `{"purpose":"login"}`, "", http.StatusOK)
	if challengeResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("challenge cache control = %q", challengeResponse.Header().Get("Cache-Control"))
	}
	var loginChallenge CredentialChallenge
	if err := json.Unmarshal(challengeResponse.Body.Bytes(), &loginChallenge); err != nil {
		t.Fatalf("login challenge response: %s %v", challengeResponse.Body.String(), err)
	}
	if err := m.credentials.consumeChallenge(t.Context(), loginChallenge.ChallengeID, loginCredentialType, 0, ErrInvalidCredential); err != nil {
		t.Fatalf("login challenge purpose: %v", err)
	}
	request(http.MethodGet, "/auth/pub/login/verification", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/auth/pub/login/verification?username=one&username=two", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/auth/pub/login/verification?broken=%zz", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/auth/pub/login/verification?username=", "", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT wrong FROM t_user").WithArgs("missing").WillReturnError(sql.ErrNoRows)
	verificationResponse := request(http.MethodGet, "/auth/pub/login/verification?username=missing", "", "", http.StatusOK)
	var verification LoginVerificationResult
	if err := json.Unmarshal(verificationResponse.Body.Bytes(), &verification); err != nil || verification.CaptchaRequired || verification.Captcha != nil {
		t.Fatalf("missing login verification response: %s %v", verificationResponse.Body.String(), err)
	}
	mock.ExpectQuery("SELECT wrong FROM t_user").WithArgs("readonly").WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(int64(5)))
	verificationResponse = request(http.MethodGet, "/auth/pub/login/verification?username=readonly", "", "", http.StatusOK)
	if err := json.Unmarshal(verificationResponse.Body.Bytes(), &verification); err != nil || !verification.CaptchaRequired || verification.Captcha == nil {
		t.Fatalf("point login verification response: %s %v", verificationResponse.Body.String(), err)
	}
	if verificationResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("verification cache control = %q", verificationResponse.Header().Get("Cache-Control"))
	}
	request(http.MethodPost, "/auth/pub/login", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/login", `{} {}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/login", `{"challenge_id":"id","credential":"value","extra":true}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/login", `{}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/auth/pub/login", `{"challenge_id":"missing","credential":"invalid"}`, "", http.StatusBadRequest)

	loginBody := func(username, password string, rememberMe ...bool) string {
		t.Helper()
		input := encryptedLoginInput(t, m.credentials, username, password, rememberMe...)
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET wrong").WithArgs(sqlmock.AnyArg(), int64(3)).WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(int64(1)))
	request(http.MethodPost, "/auth/pub/login", loginBody("readonly", "wrong"), "", http.StatusUnauthorized)
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive, 4))
	mock.ExpectQuery("UPDATE t_user SET wrong").WithArgs(sqlmock.AnyArg(), int64(3)).WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(int64(5)))
	captchaRecorder := request(http.MethodPost, "/auth/pub/login", loginBody("readonly", "wrong"), "", http.StatusUnauthorized)
	var captchaFailure struct {
		CaptchaRequired bool                  `json:"captcha_required"`
		Captcha         PointCaptchaChallenge `json:"captcha"`
	}
	if err := json.Unmarshal(captchaRecorder.Body.Bytes(), &captchaFailure); err != nil || !captchaFailure.CaptchaRequired || captchaFailure.Captcha.CaptchaID == "" {
		t.Fatalf("captcha login failure: %s %v", captchaRecorder.Body.String(), err)
	}
	var captchaFailureMessage struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(captchaRecorder.Body.Bytes(), &captchaFailureMessage); err != nil || captchaFailureMessage.Msg != ErrLoginFailed.Error() {
		t.Fatalf("captcha login failure message: %s %v", captchaRecorder.Body.String(), err)
	}
	refreshBody, err := json.Marshal(RefreshPointCaptchaInput{CaptchaID: captchaFailure.Captcha.CaptchaID})
	if err != nil {
		t.Fatal(err)
	}
	refreshedCaptcha := request(http.MethodPost, "/auth/pub/captcha", string(refreshBody), "", http.StatusOK)
	var refreshedChallenge PointCaptchaChallenge
	if err := json.Unmarshal(refreshedCaptcha.Body.Bytes(), &refreshedChallenge); err != nil || refreshedChallenge.CaptchaID == captchaFailure.Captcha.CaptchaID {
		t.Fatalf("refreshed captcha: %s %v", refreshedCaptcha.Body.String(), err)
	}
	request(http.MethodPost, "/auth/pub/captcha", string(refreshBody), "", http.StatusBadRequest)
	refreshedAnswer := storedPointCaptchaAnswer(t, m.captcha.store.(*memoryPointCaptchaStore), refreshedChallenge.CaptchaID)
	wrongOrder := append([]CaptchaPoint(nil), refreshedAnswer.Points...)
	wrongOrder[0], wrongOrder[1] = wrongOrder[1], wrongOrder[0]
	verifyBody, err := json.Marshal(VerifyLoginPointCaptchaInput{
		Username: "readonly", CaptchaID: refreshedChallenge.CaptchaID, CaptchaPoints: wrongOrder,
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongOrderRecorder := request(http.MethodPost, "/auth/pub/captcha/verify", string(verifyBody), "", http.StatusOK)
	var captchaVerification PointCaptchaVerificationResult
	if err := json.Unmarshal(wrongOrderRecorder.Body.Bytes(), &captchaVerification); err != nil || captchaVerification.Verified || captchaVerification.Captcha == nil {
		t.Fatalf("wrong-order verification: %s %v", wrongOrderRecorder.Body.String(), err)
	}
	verifiedAnswer := storedPointCaptchaAnswer(t, m.captcha.store.(*memoryPointCaptchaStore), captchaVerification.Captcha.CaptchaID)
	verifyBody, err = json.Marshal(VerifyLoginPointCaptchaInput{
		Username: "readonly", CaptchaID: captchaVerification.Captcha.CaptchaID, CaptchaPoints: verifiedAnswer.Points,
	})
	if err != nil {
		t.Fatal(err)
	}
	verifiedRecorder := request(http.MethodPost, "/auth/pub/captcha/verify", string(verifyBody), "", http.StatusOK)
	captchaVerification = PointCaptchaVerificationResult{}
	if err := json.Unmarshal(verifiedRecorder.Body.Bytes(), &captchaVerification); err != nil || !captchaVerification.Verified || captchaVerification.Captcha != nil {
		t.Fatalf("correct-order verification: %s %v", verifiedRecorder.Body.String(), err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive, 5))
	invalidCaptchaRecorder := request(http.MethodPost, "/auth/pub/login", loginBody("readonly", "cinch123"), "", http.StatusUnauthorized)
	var invalidCaptchaFailure LoginFailureResponse
	if err := json.Unmarshal(invalidCaptchaRecorder.Body.Bytes(), &invalidCaptchaFailure); err != nil || invalidCaptchaFailure.Msg != ErrPointCaptchaRequired.Error() || !invalidCaptchaFailure.CaptchaRequired || invalidCaptchaFailure.Captcha == nil {
		t.Fatalf("invalid captcha failure: %s %v", invalidCaptchaRecorder.Body.String(), err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusPending))
	request(http.MethodPost, "/auth/pub/login", loginBody("readonly", "cinch123"), "", http.StatusForbidden)
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusLocked))
	mock.ExpectExec("UPDATE t_user SET status").WithArgs(userStatusActive, sqlmock.AnyArg(), int64(3), userStatusLocked, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	request(http.MethodPost, "/auth/pub/login", loginBody("readonly", "cinch123"), "", http.StatusForbidden)
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodPost, "/auth/pub/login", loginBody("readonly", "cinch123"), "", http.StatusInternalServerError)

	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(int64(2)))
	recorder := request(http.MethodPost, "/auth/pub/login", loginBody("readonly", "cinch123", true), "", http.StatusOK)
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("login cache control = %q", recorder.Header().Get("Cache-Control"))
	}
	var login LoginResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &login); err != nil || login.AccessToken == "" || login.RefreshToken == "" {
		t.Fatalf("login response: %s %v", recorder.Body.String(), err)
	}
	if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("login unexpectedly set cookies: %#v", cookies)
	}
	oldRefreshToken := login.RefreshToken
	permissionRequest := func(targetMethod, targetPath string, want int) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/auth/permission", nil)
		req.Header.Set("Authorization", "Bearer "+login.AccessToken)
		req.Header.Set(permissionHeaderMethod, targetMethod)
		req.Header.Set(permissionHeaderPath, targetPath)
		handler.ServeHTTP(recorder, req)
		if recorder.Code != want {
			t.Fatalf("permission %s %s: got %d body %s", targetMethod, targetPath, recorder.Code, recorder.Body.String())
		}
		return recorder
	}
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"resource", "menu", "btn"}).
			AddRow("GET|/auth/info|/auth.v1.Auth/Info", "", ""),
	)
	permissionRecorder := permissionRequest(http.MethodGet, "/auth/info", http.StatusOK)
	if code := permissionRecorder.Header().Get(permissionResponseCode); code != "EXP78RGH" {
		t.Fatalf("permission code = %q", code)
	}
	if value := permissionRecorder.Header().Get("X-Username"); value != "" {
		t.Fatalf("permission response exposed username: %q", value)
	}
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"resource", "menu", "btn"}).
			AddRow("GET|/auth/info|/auth.v1.Auth/Info", "", ""),
	)
	permissionRequest(http.MethodGet, "/missing", http.StatusForbidden)
	request(http.MethodPost, "/auth/challenge", `{`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPost, "/auth/challenge", `{} {}`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPost, "/auth/challenge", `{"purpose":"login"}`, login.AccessToken, http.StatusBadRequest)
	passwordChallengeRecorder := request(http.MethodPost, "/auth/challenge", `{"purpose":"password_change"}`, login.AccessToken, http.StatusOK)
	if passwordChallengeRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("password challenge cache control = %q", passwordChallengeRecorder.Header().Get("Cache-Control"))
	}
	var passwordChallenge CredentialChallenge
	if err := json.Unmarshal(passwordChallengeRecorder.Body.Bytes(), &passwordChallenge); err != nil {
		t.Fatalf("password challenge response: %s %v", passwordChallengeRecorder.Body.String(), err)
	}
	if err := m.credentials.consumeChallenge(t.Context(), passwordChallenge.ChallengeID, passwordCredentialType, 3, ErrInvalidPasswordCredential); err != nil {
		t.Fatalf("password challenge purpose: %v", err)
	}
	request(http.MethodPatch, "/auth/change/pwd", `{`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPatch, "/auth/change/pwd", `{} {}`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPatch, "/auth/change/pwd", `{"old_password":"cinch123","new_password":"new-secret"}`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPatch, "/auth/change/pwd", `{"challenge_id":"missing","credential":"invalid"}`, login.AccessToken, http.StatusBadRequest)
	passwordChangeBody := func(oldPassword, newPassword string, captcha ...PasswordChangeInput) string {
		t.Helper()
		input := encryptedPasswordChangeInput(t, m.credentials, 3, oldPassword, newPassword, captcha...)
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	var passwordFailure PasswordChangeFailureResponse
	for attempt := 1; attempt <= 3; attempt++ {
		expectPasswordChangeLock(mock, false)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
			sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, "cinch123"), int64(1)),
		)
		mock.ExpectRollback()
		failureRecorder := request(http.MethodPatch, "/auth/change/pwd", passwordChangeBody("wrong-password", "new-secret"), login.AccessToken, http.StatusBadRequest)
		if attempt == 3 {
			if err := json.Unmarshal(failureRecorder.Body.Bytes(), &passwordFailure); err != nil || !passwordFailure.CaptchaRequired || passwordFailure.Captcha == nil || passwordFailure.Msg != ErrIncorrectPassword.Error() {
				t.Fatalf("password captcha failure: %s %v", failureRecorder.Body.String(), err)
			}
		}
	}
	captchaRefreshBody, err := json.Marshal(RefreshPointCaptchaInput{CaptchaID: passwordFailure.Captcha.CaptchaID})
	if err != nil {
		t.Fatal(err)
	}
	refreshedPasswordCaptcha := request(http.MethodPost, "/auth/captcha", string(captchaRefreshBody), login.AccessToken, http.StatusOK)
	var refreshedPasswordChallenge PointCaptchaChallenge
	if err := json.Unmarshal(refreshedPasswordCaptcha.Body.Bytes(), &refreshedPasswordChallenge); err != nil || refreshedPasswordChallenge.CaptchaID == passwordFailure.Captcha.CaptchaID {
		t.Fatalf("refreshed password captcha: %s %v", refreshedPasswordCaptcha.Body.String(), err)
	}
	expectPasswordChangeLock(mock, false)
	missingCaptchaRecorder := request(http.MethodPatch, "/auth/change/pwd", passwordChangeBody("cinch123", "new-secret"), login.AccessToken, http.StatusBadRequest)
	if err := json.Unmarshal(missingCaptchaRecorder.Body.Bytes(), &passwordFailure); err != nil || passwordFailure.Captcha == nil || passwordFailure.Msg != ErrPointCaptchaRequired.Error() {
		t.Fatalf("missing password captcha: %s %v", missingCaptchaRecorder.Body.String(), err)
	}
	answer := storedPointCaptchaAnswer(t, m.captcha.store.(*memoryPointCaptchaStore), passwordFailure.Captcha.CaptchaID)
	passwordVerifyBody, err := json.Marshal(VerifyPointCaptchaInput{
		CaptchaID: passwordFailure.Captcha.CaptchaID, CaptchaPoints: answer.Points,
	})
	if err != nil {
		t.Fatal(err)
	}
	passwordVerifyRecorder := request(http.MethodPost, "/auth/captcha/verify", string(passwordVerifyBody), login.AccessToken, http.StatusOK)
	var passwordVerification PointCaptchaVerificationResult
	if err := json.Unmarshal(passwordVerifyRecorder.Body.Bytes(), &passwordVerification); err != nil || !passwordVerification.Verified {
		t.Fatalf("password captcha verification: %s %v", passwordVerifyRecorder.Body.String(), err)
	}
	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, "cinch123"), int64(1)),
	)
	mock.ExpectExec("UPDATE t_user SET password").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), "EXP78RGH").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	passwordChangeRecorder := request(http.MethodPatch, "/auth/change/pwd", passwordChangeBody("cinch123", "new-secret", PasswordChangeInput{
		CaptchaID: passwordFailure.Captcha.CaptchaID, CaptchaPoints: answer.Points,
	}), login.AccessToken, http.StatusOK)
	if passwordChangeRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("password change cache control = %q", passwordChangeRecorder.Header().Get("Cache-Control"))
	}
	expectPasswordChangeLock(mock, true)
	lockedPasswordRecorder := request(http.MethodPatch, "/auth/change/pwd", passwordChangeBody("cinch123", "another-secret"), login.AccessToken, http.StatusForbidden)
	if err := json.Unmarshal(lockedPasswordRecorder.Body.Bytes(), &passwordFailure); err != nil || passwordFailure.Msg != ErrPasswordChangeLocked.Error() || passwordFailure.CaptchaRequired {
		t.Fatalf("locked password change: %s %v", lockedPasswordRecorder.Body.String(), err)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count FROM t_user").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(1), int64(1)))
	sessionRefreshBody, err := json.Marshal(RefreshTokenInput{RefreshToken: oldRefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	refreshRecorder := request(http.MethodPost, "/auth/pub/refresh", string(sessionRefreshBody), "", http.StatusOK)
	var refreshed LoginResult
	if err := json.Unmarshal(refreshRecorder.Body.Bytes(), &refreshed); err != nil || refreshed.AccessToken == "" || refreshed.RefreshToken == "" || refreshed.RefreshToken == oldRefreshToken {
		t.Fatalf("refresh response: %s %v", refreshRecorder.Body.String(), err)
	}
	if cookies := refreshRecorder.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("refresh unexpectedly set cookies: %#v", cookies)
	}
	request(http.MethodPost, "/auth/pub/refresh", string(sessionRefreshBody), "", http.StatusUnauthorized)

	mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code", "role_id", "role_name", "role_word"}).
			AddRow(3, "readonly", "EXP78RGH", 2, "Reader", "reader"))
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"resource", "menu", "btn"}).
			AddRow("GET|/auth/info|/auth.v1.Auth/Info", "/dashboard/overview", "system.dashboard.read"),
	)
	infoRecorder := request(http.MethodGet, "/auth/info", "", refreshed.AccessToken, http.StatusOK)
	var info UserInfo
	if err := json.Unmarshal(infoRecorder.Body.Bytes(), &info); err != nil || info.Role == nil || info.Role.Name != "Reader" || info.Role.Word != "reader" {
		t.Fatalf("info response: %s %v", infoRecorder.Body.String(), err)
	}
	request(http.MethodGet, "/auth/codes", "", refreshed.AccessToken, http.StatusNotFound)
	logoutBody, err := json.Marshal(RefreshTokenInput{RefreshToken: refreshed.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	logoutRecorder := request(http.MethodPost, "/auth/pub/logout", string(logoutBody), "", http.StatusOK)
	if cookies := logoutRecorder.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("logout unexpectedly set cookies: %#v", cookies)
	}
	request(http.MethodGet, "/auth/info", "", refreshed.AccessToken, http.StatusUnauthorized)
	request(http.MethodGet, "/auth/info", "", "invalid", http.StatusUnauthorized)
}

// Localizing errors must preserve the captcha challenge and the machine-readable code.
func TestLocalizedAuthFailures(t *testing.T) {
	m, authenticator, mock := newAuthTestModule(t)
	handler, err := server.NewRouter(&config.Config{}, authenticator, nil, m)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ locale, message string }{
		{"en-US", "invalid captcha"}, {"zh-CN", "验证码无效，请重新验证"},
	} {
		mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive, 5))
		body, err := json.Marshal(encryptedLoginInput(t, m.credentials, "readonly", "cinch123"))
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/auth/pub/login", strings.NewReader(string(body)))
		req.Header.Set("Accept-Language", tc.locale)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		var failure LoginFailureResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &failure); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != 401 || failure.Msg != tc.message || failure.ErrorCode != "AUTH_POINT_CAPTCHA_REQUIRED" || !failure.CaptchaRequired || failure.Captcha == nil || failure.Captcha.CaptchaID == "" {
			t.Fatalf("captcha response: %d %s", recorder.Code, recorder.Body.String())
		}
	}
	req := httptest.NewRequest("GET", "/auth/info", nil)
	req.Header.Set("Accept-Language", "zh-CN")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	var body server.ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 401 || body.ErrorCode != "HTTP_UNAUTHORIZED" || body.Msg != "登录状态无效或已过期，请重新登录" {
		t.Fatal(recorder.Body.String())
	}
}
