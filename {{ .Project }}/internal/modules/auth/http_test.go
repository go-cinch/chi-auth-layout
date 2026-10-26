package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
	rolemodule "{{ .Computed.module_name_final }}/internal/modules/role"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ end }}
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

func validSliderProof(t *testing.T, slider *SliderCaptcha, purpose, username string) string {
	t.Helper()
	challenge, err := slider.IssueSliderChallenge(t.Context(), purpose, username)
	if err != nil {
		t.Fatal(err)
	}
	result, err := slider.VerifySlider(t.Context(), SliderCaptchaVerification{
		CaptchaID: challenge.CaptchaID,
		Username:  username,
		Purpose:   purpose,
		Duration:  int((100 * time.Millisecond) / time.Millisecond),
		Distance:  200,
		Width:     200,
		Tracks: []SliderCaptchaTrack{
			{X: 0, T: 0},
			{X: 100, T: 50},
			{X: 200, T: 100},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.Proof
}

func TestPermissionTargetFromRequest(t *testing.T) {
	request := httptest.NewRequest("GET", "/permission", nil)
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
			{"/permission", "GET", "/order/pub/item?x=1", 200},
			{"/permission", "GET", "/order/pub", 200},
			{"/permission", "HEAD", "/order/1", 200},
			{"/permission", "OPTIONS", "/order/1", 200},
			{"/permission", "GET", "/order/1", 401},
			{"/permission", "GET", "/order/public", 401},
			{"/permission", "GET", "/order/pub/../private", 401},
			{"/permission", "GET", "/order/pub/%2e%2e/private", 401},
			{"/permission", "GET", "/bad%zz/pub/item", 401},
			{"/permission", "HEAD", "", 401},
			{"/permission", "OPTIONS", "//host/path", 401},
			{"/permission", "", "/order/pub/item", 401},
			{"/info", "HEAD", "/order/1", 401},
			{"/info", "GET", "/order/pub/item", 401},
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
	localizer := localization.New()
	m, authenticator, mock := newAuthTestModule(t)
	localizer = m.catalogSource.(*localization.Module)
	if m.Name() != "" {
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

	request(http.MethodGet, "/info", "", "", http.StatusUnauthorized)
	request(http.MethodGet, "/permission", "", "", http.StatusUnauthorized)
	request(http.MethodPost, "/challenge", `{"purpose":"password_change"}`, "", http.StatusUnauthorized)
	request(http.MethodPost, "/captcha", `{}`, "", http.StatusUnauthorized)
	request(http.MethodPost, "/captcha/verify", `{}`, "", http.StatusUnauthorized)
	request(http.MethodPatch, "/change/pwd", `{}`, "", http.StatusUnauthorized)
	request(http.MethodPost, "/pub/challenge", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/challenge", `{} {}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/challenge", `{"purpose":"login","extra":true}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/challenge", `{"purpose":"password_change"}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/refresh", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/captcha/verify", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/refresh", `{}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/refresh", `{"refresh_token":"token","extra":true}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/logout", `{}`, "", http.StatusBadRequest)
	request(http.MethodGet, "/pub/register/username", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/pub/register/username?username=one&username=two", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/pub/register/username?broken=%zz", "", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("new-user").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	availabilityResponse := request(http.MethodGet, "/pub/register/username?username=new-user", "", "", http.StatusOK)
	var availability UsernameAvailability
	if err := json.Unmarshal(availabilityResponse.Body.Bytes(), &availability); err != nil || !availability.Available {
		t.Fatalf("availability response: %s %v", availabilityResponse.Body.String(), err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("existing-user").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	availabilityResponse = request(http.MethodGet, "/pub/register/username?username=existing-user", "", "", http.StatusOK)
	if err := json.Unmarshal(availabilityResponse.Body.Bytes(), &availability); err != nil || availability.Available {
		t.Fatalf("existing username response: %s %v", availabilityResponse.Body.String(), err)
	}
	registrationChallengeResponse := request(http.MethodPost, "/pub/challenge", `{"purpose":"register"}`, "", http.StatusOK)
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
	request(http.MethodPost, "/pub/register", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/register", `{} {}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/register", `{"username":"new-user","password":"secret1","extra":true}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/register", `{}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/register", `{"challenge_id":"missing","credential":"invalid"}`, "", http.StatusBadRequest)
	registerBody := func(username, password string) string {
		t.Helper()
		input := encryptedRegisterInputWithProof(t, m.credentials, username, password, validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeRegister, username))
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	missingRegisterProof, err := json.Marshal(encryptedRegisterInput(t, m.credentials, "new-user", "secret1"))
	if err != nil {
		t.Fatal(err)
	}
	request(http.MethodPost, "/pub/register", string(missingRegisterProof), "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/register", registerBody("new-user", "   "), "", http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(36)))
	mock.ExpectExec("INSERT INTO t_user").WillReturnError(&pq.Error{Code: "23505", Constraint: "uk_user_username"})
	mock.ExpectRollback()
	request(http.MethodPost, "/pub/register", registerBody("new-user", "secret1"), "", http.StatusConflict)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(37)))
	mock.ExpectExec("INSERT INTO t_user").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	request(http.MethodPost, "/pub/register", registerBody("new-user", "secret1"), "", http.StatusOK)
	challengeResponse := request(http.MethodPost, "/pub/challenge", `{"purpose":"login"}`, "", http.StatusOK)
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
	request(http.MethodGet, "/pub/login/verification", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/pub/login/verification?username=one&username=two", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/pub/login/verification?broken=%zz", "", "", http.StatusBadRequest)
	request(http.MethodGet, "/pub/login/verification?username=", "", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT wrong FROM t_user").WithArgs("missing").WillReturnError(sql.ErrNoRows)
	verificationResponse := request(http.MethodGet, "/pub/login/verification?username=missing", "", "", http.StatusOK)
	var verification LoginVerificationResult
	if err := json.Unmarshal(verificationResponse.Body.Bytes(), &verification); err != nil || verification.CaptchaRequired || verification.Captcha != nil {
		t.Fatalf("missing login verification response: %s %v", verificationResponse.Body.String(), err)
	}
	mock.ExpectQuery("SELECT wrong FROM t_user").WithArgs("readonly").WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(int64(5)))
	verificationResponse = request(http.MethodGet, "/pub/login/verification?username=readonly", "", "", http.StatusOK)
	if err := json.Unmarshal(verificationResponse.Body.Bytes(), &verification); err != nil || !verification.CaptchaRequired || verification.Captcha == nil {
		t.Fatalf("point login verification response: %s %v", verificationResponse.Body.String(), err)
	}
	if verificationResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("verification cache control = %q", verificationResponse.Header().Get("Cache-Control"))
	}
	request(http.MethodPost, "/pub/login", `{`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/login", `{} {}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/login", `{"challenge_id":"id","credential":"value","extra":true}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/login", `{}`, "", http.StatusBadRequest)
	request(http.MethodPost, "/pub/login", `{"challenge_id":"missing","credential":"invalid"}`, "", http.StatusBadRequest)

	loginBody := func(username, password string, rememberMe ...bool) string {
		t.Helper()
		input := encryptedLoginInputWithProof(t, m.credentials, username, password, validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, username), rememberMe...)
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	missingLoginProof, err := json.Marshal(encryptedLoginInput(t, m.credentials, "readonly", authTestPassword))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	request(http.MethodPost, "/pub/login", string(missingLoginProof), "", http.StatusBadRequest)

	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET wrong").WithArgs(sqlmock.AnyArg(), int64(3)).WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(int64(1)))
	request(http.MethodPost, "/pub/login", loginBody("readonly", "wrong"), "", http.StatusUnauthorized)
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive, 4))
	mock.ExpectQuery("UPDATE t_user SET wrong").WithArgs(sqlmock.AnyArg(), int64(3)).WillReturnRows(sqlmock.NewRows([]string{"wrong"}).AddRow(int64(5)))
	captchaRecorder := request(http.MethodPost, "/pub/login", loginBody("readonly", "wrong"), "", http.StatusUnauthorized)
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
	refreshedCaptcha := request(http.MethodPost, "/pub/captcha", string(refreshBody), "", http.StatusOK)
	var refreshedChallenge PointCaptchaChallenge
	if err := json.Unmarshal(refreshedCaptcha.Body.Bytes(), &refreshedChallenge); err != nil || refreshedChallenge.CaptchaID == captchaFailure.Captcha.CaptchaID {
		t.Fatalf("refreshed captcha: %s %v", refreshedCaptcha.Body.String(), err)
	}
	request(http.MethodPost, "/pub/captcha", string(refreshBody), "", http.StatusBadRequest)
	refreshedAnswer := storedPointCaptchaAnswer(t, m.captcha.store.(*memoryPointCaptchaStore), refreshedChallenge.CaptchaID)
	wrongOrder := append([]CaptchaPoint(nil), refreshedAnswer.Points...)
	wrongOrder[0], wrongOrder[1] = wrongOrder[1], wrongOrder[0]
	verifyBody, err := json.Marshal(VerifyLoginPointCaptchaInput{
		Username: "readonly", CaptchaID: refreshedChallenge.CaptchaID, CaptchaPoints: wrongOrder,
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongOrderRecorder := request(http.MethodPost, "/pub/captcha/verify", string(verifyBody), "", http.StatusOK)
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
	verifiedRecorder := request(http.MethodPost, "/pub/captcha/verify", string(verifyBody), "", http.StatusOK)
	captchaVerification = PointCaptchaVerificationResult{}
	if err := json.Unmarshal(verifiedRecorder.Body.Bytes(), &captchaVerification); err != nil || !captchaVerification.Verified || captchaVerification.Captcha != nil {
		t.Fatalf("correct-order verification: %s %v", verifiedRecorder.Body.String(), err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive, 5))
	invalidCaptchaBody, err := json.Marshal(encryptedLoginInput(t, m.credentials, "readonly", authTestPassword))
	if err != nil {
		t.Fatal(err)
	}
	invalidCaptchaRecorder := request(http.MethodPost, "/pub/login", string(invalidCaptchaBody), "", http.StatusUnauthorized)
	var invalidCaptchaFailure LoginFailureResponse
	if err := json.Unmarshal(invalidCaptchaRecorder.Body.Bytes(), &invalidCaptchaFailure); err != nil || invalidCaptchaFailure.Msg != ErrPointCaptchaRequired.Error() || !invalidCaptchaFailure.CaptchaRequired || invalidCaptchaFailure.Captcha == nil {
		t.Fatalf("invalid captcha failure: %s %v", invalidCaptchaRecorder.Body.String(), err)
	}
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusPending))
	request(http.MethodPost, "/pub/login", loginBody("readonly", authTestPassword), "", http.StatusForbidden)
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusLocked))
	mock.ExpectExec("UPDATE t_user SET status").WithArgs(userStatusActive, sqlmock.AnyArg(), int64(3), userStatusLocked, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	request(http.MethodPost, "/pub/login", loginBody("readonly", authTestPassword), "", http.StatusForbidden)
	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodPost, "/pub/login", loginBody("readonly", authTestPassword), "", http.StatusInternalServerError)

	mock.ExpectQuery("SELECT id, username, code, password, status, wrong").WithArgs("readonly").WillReturnRows(loginRows(t, userStatusActive))
	mock.ExpectQuery("UPDATE t_user SET last_logged_in_at").WithArgs(sqlmock.AnyArg(), int64(3), int64(1), userStatusActive).WillReturnRows(sqlmock.NewRows([]string{"login_count"}).AddRow(int64(2)))
	recorder := request(http.MethodPost, "/pub/login", loginBody("readonly", authTestPassword, true), "", http.StatusOK)
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
		req := httptest.NewRequest(http.MethodGet, "/permission", nil)
		req.Header.Set("Authorization", "Bearer "+login.AccessToken)
		req.Header.Set(permissionHeaderMethod, targetMethod)
		req.Header.Set(permissionHeaderPath, targetPath)
		handler.ServeHTTP(recorder, req)
		if recorder.Code != want {
			t.Fatalf("permission %s %s: got %d body %s", targetMethod, targetPath, recorder.Code, recorder.Body.String())
		}
		return recorder
	}
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(
		permissionRows().AddRow("admin:read:system:info", permissionConfig(localizer, "GET", "/info")),
	)
	permissionRecorder := permissionRequest(http.MethodGet, "/info", http.StatusOK)
	if code := permissionRecorder.Header().Get(permissionResponseCode); code != "EXP78RGH" {
		t.Fatalf("permission code = %q", code)
	}
	if value := permissionRecorder.Header().Get("X-Username"); value != "" {
		t.Fatalf("permission response exposed username: %q", value)
	}
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(
		permissionRows().AddRow("admin:read:system:info", permissionConfig(localizer, "GET", "/info")),
	)
	permissionRequest(http.MethodGet, "/missing", http.StatusForbidden)
	request(http.MethodPost, "/challenge", `{`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPost, "/challenge", `{} {}`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPost, "/challenge", `{"purpose":"login"}`, login.AccessToken, http.StatusBadRequest)
	passwordChallengeRecorder := request(http.MethodPost, "/challenge", `{"purpose":"password_change"}`, login.AccessToken, http.StatusOK)
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
	request(http.MethodPatch, "/change/pwd", `{`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPatch, "/change/pwd", `{} {}`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPatch, "/change/pwd", `{"old_password":"`+authTestPassword+`","new_password":"new-secret"}`, login.AccessToken, http.StatusBadRequest)
	request(http.MethodPatch, "/change/pwd", `{"challenge_id":"missing","credential":"invalid"}`, login.AccessToken, http.StatusBadRequest)
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
			sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, authTestPassword), int64(1)),
		)
		mock.ExpectRollback()
		failureRecorder := request(http.MethodPatch, "/change/pwd", passwordChangeBody("wrong-password", "new-secret"), login.AccessToken, http.StatusBadRequest)
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
	refreshedPasswordCaptcha := request(http.MethodPost, "/captcha", string(captchaRefreshBody), login.AccessToken, http.StatusOK)
	var refreshedPasswordChallenge PointCaptchaChallenge
	if err := json.Unmarshal(refreshedPasswordCaptcha.Body.Bytes(), &refreshedPasswordChallenge); err != nil || refreshedPasswordChallenge.CaptchaID == passwordFailure.Captcha.CaptchaID {
		t.Fatalf("refreshed password captcha: %s %v", refreshedPasswordCaptcha.Body.String(), err)
	}
	expectPasswordChangeLock(mock, false)
	missingCaptchaRecorder := request(http.MethodPatch, "/change/pwd", passwordChangeBody(authTestPassword, "new-secret"), login.AccessToken, http.StatusBadRequest)
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
	passwordVerifyRecorder := request(http.MethodPost, "/captcha/verify", string(passwordVerifyBody), login.AccessToken, http.StatusOK)
	var passwordVerification PointCaptchaVerificationResult
	if err := json.Unmarshal(passwordVerifyRecorder.Body.Bytes(), &passwordVerification); err != nil || !passwordVerification.Verified {
		t.Fatalf("password captcha verification: %s %v", passwordVerifyRecorder.Body.String(), err)
	}
	expectPasswordChangeLock(mock, false)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT password, credential_version FROM t_user").WithArgs(int64(3), "EXP78RGH").WillReturnRows(
		sqlmock.NewRows([]string{"password", "credential_version"}).AddRow(passwordHash(t, authTestPassword), int64(1)),
	)
	mock.ExpectExec("UPDATE t_user SET password").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), "EXP78RGH").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	passwordChangeRecorder := request(http.MethodPatch, "/change/pwd", passwordChangeBody(authTestPassword, "new-secret", PasswordChangeInput{
		CaptchaID: passwordFailure.Captcha.CaptchaID, CaptchaPoints: answer.Points,
	}), login.AccessToken, http.StatusOK)
	if passwordChangeRecorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("password change cache control = %q", passwordChangeRecorder.Header().Get("Cache-Control"))
	}
	expectPasswordChangeLock(mock, true)
	lockedPasswordRecorder := request(http.MethodPatch, "/change/pwd", passwordChangeBody(authTestPassword, "another-secret"), login.AccessToken, http.StatusForbidden)
	if err := json.Unmarshal(lockedPasswordRecorder.Body.Bytes(), &passwordFailure); err != nil || passwordFailure.Msg != ErrPasswordChangeLocked.Error() || passwordFailure.CaptchaRequired {
		t.Fatalf("locked password change: %s %v", lockedPasswordRecorder.Body.String(), err)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count FROM t_user").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(1), int64(1)))
	sessionRefreshBody, err := json.Marshal(RefreshTokenInput{RefreshToken: oldRefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	refreshRecorder := request(http.MethodPost, "/pub/refresh", string(sessionRefreshBody), "", http.StatusOK)
	var refreshed LoginResult
	if err := json.Unmarshal(refreshRecorder.Body.Bytes(), &refreshed); err != nil || refreshed.AccessToken == "" || refreshed.RefreshToken == "" || refreshed.RefreshToken == oldRefreshToken {
		t.Fatalf("refresh response: %s %v", refreshRecorder.Body.String(), err)
	}
	if cookies := refreshRecorder.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("refresh unexpectedly set cookies: %#v", cookies)
	}
	mock.ExpectQuery("SELECT username, code, status, credential_version, login_count FROM t_user").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", userStatusActive, int64(1), int64(1)))
	retryRecorder := request(http.MethodPost, "/pub/refresh", string(sessionRefreshBody), "", http.StatusOK)
	var retried LoginResult
	if err := json.Unmarshal(retryRecorder.Body.Bytes(), &retried); err != nil || retried.RefreshToken != refreshed.RefreshToken {
		t.Fatalf("refresh retry did not return the committed token: %s %v", retryRecorder.Body.String(), err)
	}

	mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code", "role_id", "role_name", "role_word"}).
			AddRow(3, "readonly", "EXP78RGH", 2, "Reader", "reader"))
	mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(
		permissionRows().AddRow("admin:read:system:user", userReadPermissionConfig),
	)
	infoRecorder := request(http.MethodGet, "/info", "", refreshed.AccessToken, http.StatusOK)
	var info UserInfo
	if err := json.Unmarshal(infoRecorder.Body.Bytes(), &info); err != nil || info.Role == nil || info.Role.Name != "Reader" || info.Role.Word != "reader" {
		t.Fatalf("info response: %s %v", infoRecorder.Body.String(), err)
	}
	request(http.MethodGet, "/codes", "", refreshed.AccessToken, http.StatusNotFound)
	logoutBody, err := json.Marshal(RefreshTokenInput{RefreshToken: refreshed.RefreshToken})
	if err != nil {
		t.Fatal(err)
	}
	logoutRecorder := request(http.MethodPost, "/pub/logout", string(logoutBody), "", http.StatusOK)
	if cookies := logoutRecorder.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("logout unexpectedly set cookies: %#v", cookies)
	}
	request(http.MethodGet, "/info", "", refreshed.AccessToken, http.StatusUnauthorized)
	request(http.MethodGet, "/info", "", "invalid", http.StatusUnauthorized)
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
		body, err := json.Marshal(encryptedLoginInputWithProof(t, m.credentials, "readonly", authTestPassword, validSliderProof(t, m.sliderCaptcha, sliderCaptchaPurposeLogin, "readonly")))
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/pub/login", strings.NewReader(string(body)))
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
	req := httptest.NewRequest("GET", "/info", nil)
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

func TestInfoUsesHeaderScopeAndIgnoresLegacyQuery(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
{{ if eq .Computed.http_router_final "gin" }}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else }}
	handler := m.HTTP()
{{ end }}
	for _, test := range []struct {
		query, header, want string
		status              int
	}{
		{"", "", "admin", 200}, {"", "mini", "mini", 200},
		{"?scope=mini", "admin", "admin", 200}, {"?scope=admin", "mini", "mini", 200},
		{"?scope=", "", "admin", 200}, {"?scope=admin&scope=mini", "admin", "admin", 200},
		{"?scope=Admin", "admin", "admin", 200}, {"?scope=%zz", "admin", "", 400},
	} {
		request := httptest.NewRequest(http.MethodGet, "/info"+test.query, nil).WithContext(ctx)
		if test.header != "" {
			request.Header.Set(authn.PermissionScopeHeader, test.header)
		}
		scoped, err := authn.ScopeRequest(request)
		if err != nil {
			t.Fatal(err)
		}
		if test.status == 200 {
			mock.ExpectQuery("SELECT u.id, u.username, u.code").WithArgs(int64(3), "EXP78RGH").WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code", "role_id", "role_name", "role_word"}).AddRow(3, "readonly", "EXP78RGH", nil, nil, nil))
			mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), test.want).WillReturnRows(permissionRows())
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, scoped)
		if response.Code != test.status {
			t.Fatalf("query=%s header=%s status=%d: %s", test.query, test.header, response.Code, response.Body.String())
		}
		if test.status == 200 {
			var value UserInfo
			if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
				t.Fatal(err)
			}
			if value.Scope != test.want {
				t.Fatalf("scope=%s want=%s", value.Scope, test.want)
			}
		}
	}
}

func TestConfiguredAuthHTTPPath(t *testing.T) {
	for _, prefix := range []string{"", "/auth", "/identity/v2"} {
		t.Run(prefix, func(t *testing.T) {
			m, manager, mock := newAuthTestModule(t)
			raw, err := json.Marshal(m.catalogSource.Current())
			if err != nil {
				t.Fatal(err)
			}
			canonical, err := localization.New(AuthHTTPPaths("")).CanonicalCatalog(raw)
			if err != nil {
				t.Fatal(err)
			}
			catalog := localization.New(AuthHTTPPaths(prefix))
			if err := catalog.Install(canonical); err != nil {
				t.Fatal(err)
			}
			m.httpPath, m.catalogSource = prefix, catalog
			cfg := &config.Config{}
			cfg.Auth.Path = prefix
			cfg.Auth.Authorization.Enabled = true
			cfg.Idempotency.TTL = time.Minute
			actions := actionmodule.New(m.store, pagination.Limits{MaxP: 100, MaxS: 100}, catalog)
			roles := rolemodule.New(m.store, pagination.Limits{MaxP: 100, MaxS: 100}, actions)
			handler, err := server.NewRouter(cfg, manager, idempotency.NewMemoryStore(), roles, m)
			if err != nil {
				t.Fatal(err)
			}
			request := func(method, path, body, token string, want int) {
				t.Helper()
				r := httptest.NewRequest(method, path, strings.NewReader(body))
				if token != "" {
					r.Header.Set("Authorization", "Bearer "+token)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, r)
				if response.Code != want {
					t.Fatalf("%s %s: got %d want %d: %s", method, path, response.Code, want, response.Body.String())
				}
			}
			request("POST", prefix+"/pub/challenge", `{"purpose":"login"}`, "", 200)
			request("GET", prefix+"/info", "", "", 401)
			request("GET", "/role", "", "", 401)
			request("GET", "/missing", "", "", 404)
			if prefix == "" {
				request("GET", "/auth/info", "", "", 404)
			} else {
				request("GET", "/info", "", "", 404)
			}
			identity := authn.Identity{UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}
			session, err := m.sessions.Issue(t.Context(), identity, false)
			if err != nil {
				t.Fatal(err)
			}
			identity.SessionID = session.SessionID
			token, _, err := manager.Issue(identity)
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(permissionRows().AddRow("admin:update:account:profile", `{"bindings":[{"resource_key":"admin:account:profile","operation":"update","feature_keys":["admin:account:profile:modify"]}],"ui":[],"endpoints":[]}`))
			request("POST", prefix+"/challenge", `{"purpose":"password_change"}`, token, 200)
			r := httptest.NewRequest("GET", prefix+"/permission", nil)
			r.Header.Set(permissionHeaderMethod, "GET")
			r.Header.Set(permissionHeaderPath, "/order/pub/item")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, r)
			if response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
		})
	}
}

func TestConfiguredAuthPathPreservesResetRestriction(t *testing.T) {
	for _, prefix := range []string{"", "/auth"} {
		t.Run(prefix, func(t *testing.T) {
			m, _, mock := newAuthTestModule(t)
			m.httpPath = prefix
			manager, err := authn.New("test", authTestKey, time.Hour, NewIdentityValidator(m.store, m.sessions))
			if err != nil {
				t.Fatal(err)
			}
			identity := authn.Identity{UserID: 3, Username: "readonly", Code: "EXP78RGH", CredentialVersion: 1}
			session, err := m.sessions.Issue(t.Context(), identity, false)
			if err != nil {
				t.Fatal(err)
			}
			identity.SessionID = session.SessionID
			token, _, err := manager.Issue(identity)
			if err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{}
			cfg.Auth.Path = prefix
			handler, err := server.NewRouter(cfg, manager, nil, m)
			if err != nil {
				t.Fatal(err)
			}
			for _, test := range []struct {
				method, path, body string
				want               int
			}{
				{"GET", "/info", "", 403},
				{"PATCH", "/change/pwd", "{}", 403},
				{"POST", "/challenge", `{"purpose":"password_change"}`, 403},
				{"POST", "/challenge", `{"purpose":"password_reset"}`, 200},
				{"PATCH", "/reset/pwd", "{}", 400},
			} {
				mock.ExpectQuery("SELECT username, code, status, credential_version, login_count").WithArgs(int64(3), "EXP78RGH").WillReturnRows(sqlmock.NewRows([]string{"username", "code", "status", "credential_version", "login_count"}).AddRow("readonly", "EXP78RGH", 1, 1, 0))
				r := httptest.NewRequest(test.method, prefix+test.path, strings.NewReader(test.body))
				r.Header.Set("Authorization", "Bearer "+token)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, r)
				if response.Code != test.want {
					t.Fatalf("%s %s: %d want %d: %s", test.method, prefix+test.path, response.Code, test.want, response.Body.String())
				}
			}
		})
	}
}

func TestAuthHTTPPathMappingsCoverRegisteredRoutes(t *testing.T) {
	for _, prefix := range []string{"", "/auth"} {
		m, manager, _ := newAuthTestModule(t)
		m.httpPath = prefix
		if _, err := server.NewRouter(&config.Config{}, manager, nil, m); err != nil {
			t.Fatal(err)
		}
		mapped := map[string]bool{}
		for _, actual := range AuthHTTPPaths(prefix) {
			mapped[actual] = true
		}
		registered := map[string]bool{}
		for _, route := range m.permissionRoutes {
			if !mapped[route.Path] {
				t.Fatalf("registered auth route %s has no configured-path mapping", route.Path)
			}
			registered[route.Path] = true
		}
		for path := range mapped {
			if !registered[path] {
				t.Fatalf("mapped path %s is not registered", path)
			}
		}
	}
}
