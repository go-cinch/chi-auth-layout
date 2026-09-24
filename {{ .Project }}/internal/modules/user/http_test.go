package user

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

type registrationCredentialsStub struct {
	input authmodule.EncryptedRegisterInput
}

func (s *registrationCredentialsStub) OpenRegistrationPassword(_ context.Context, input authmodule.EncryptedRegisterInput) (string, error) {
	s.input = input
	if input.Credential == "server-error" {
		return "", errors.New("challenge store unavailable")
	}
	if input.ChallengeID != "challenge" || input.Credential != "encrypted" {
		return "", authmodule.ErrInvalidRegistrationCredential
	}
	return "secret1", nil
}

func TestUserHTTP(t *testing.T) {
	m, mock := newTestModule(t)
	credentials := &registrationCredentialsStub{}
	m.registrationCredentials = credentials
	if m.Name() != "/user" { t.Fatal(m.Name()) }
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New(); m.HTTP(router.Group("")); handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	request := func(method, target, body string, want int) string {
		t.Helper(); recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))
		if recorder.Code != want { t.Fatalf("%s %s: got %d body %s", method, target, recorder.Code, recorder.Body.String()) }
		return recorder.Body.String()
	}
	request(http.MethodGet, "/bad", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT .* FROM t_user WHERE id").WithArgs(int64(1)).WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/1", "", http.StatusInternalServerError)
	mock.ExpectQuery("SELECT .* FROM t_user WHERE id").WithArgs(int64(99)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "role_id", "action", "username", "code", "last_logged_in_at", "status", "metadata", "wrong", "login_count"}),
	)
	request(http.MethodGet, "/99", "", http.StatusNotFound)
	expectUserGet(mock, 1, "super", "89HEK28Y")
	request(http.MethodGet, "/1", "", http.StatusOK)
	request(http.MethodPost, "/", `{}`, http.StatusBadRequest)
	request(http.MethodPost, "/", `{"username":"operator","password":"secret1"}`, http.StatusBadRequest)
	request(http.MethodPost, "/", `{"username":"operator","challenge_id":"challenge","credential":"encrypted","status":0}`, http.StatusBadRequest)
	request(http.MethodPost, "/", `{"username":"operator","challenge_id":"challenge","credential":"server-error"}`, http.StatusInternalServerError)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(6))
	mock.ExpectExec("INSERT INTO t_user").WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 6, "operator", "ABCDEFGH")
	mock.ExpectCommit()
	request(http.MethodPost, "/", `{"username":"operator","challenge_id":"challenge","credential":"encrypted","action_codes":[]}`, http.StatusOK)
	if credentials.input.ChallengeID != "challenge" || credentials.input.Credential != "encrypted" { t.Fatalf("credential input: %#v", credentials.input) }
	request(http.MethodPatch, "/bad", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{"password":"secret2"}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{"challenge_id":"challenge"}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{"challenge_id":"challenge","credential":"server-error"}`, http.StatusInternalServerError)
	request(http.MethodPatch, "/1", `{"status":2,"lockExpire":0}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{"status":2,"lock_expired_at":0}`, http.StatusBadRequest)
	m.switches.ProtectSuper = true
	disabledBody := request(http.MethodPatch, "/1", `{"challenge_id":"challenge","credential":"encrypted"}`, http.StatusForbidden)
	if !strings.Contains(disabledBody, `"error_code":"SYSTEM_FEATURE_DISABLED"`) { t.Fatalf("disabled password reset: %s", disabledBody) }
	disabledBody = request(http.MethodPatch, "/1", `{"role_id":2}`, http.StatusForbidden)
	if !strings.Contains(disabledBody, `"error_code":"SYSTEM_FEATURE_DISABLED"`) { t.Fatalf("disabled super role change: %s", disabledBody) }
	m.switches.ProtectSuper = false
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user SET password").WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 1, "super", "89HEK28Y")
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"challenge_id":"challenge","credential":"encrypted"}`, http.StatusOK)
	if credentials.input.ChallengeID != "challenge" || credentials.input.Credential != "encrypted" { t.Fatalf("update credential input: %#v", credentials.input) }
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user SET username").WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 1, "updated", "89HEK28Y")
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"username":"updated"}`, http.StatusOK)
	request(http.MethodDelete, "/bad", "", http.StatusBadRequest)
	m.switches.ProtectSuper = true
	disabledBody = request(http.MethodDelete, "/1", "", http.StatusForbidden)
	if !strings.Contains(disabledBody, `"error_code":"SYSTEM_FEATURE_DISABLED"`) { t.Fatalf("disabled super deletion: %s", disabledBody) }
	m.switches.ProtectSuper = false
	request(http.MethodGet, "/?status=wrong", "", http.StatusBadRequest)
	request(http.MethodGet, "/?status=1,wrong", "", http.StatusBadRequest)
	request(http.MethodGet, "/?status=1&status=2", "", http.StatusBadRequest)
	request(http.MethodGet, "/?status=3", "", http.StatusBadRequest)
	request(http.MethodGet, "/?p=x", "", http.StatusBadRequest)
	request(http.MethodGet, "/?s=x", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	listBody := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(listBody, `"t":0`) || strings.Contains(listBody, `"total"`) { t.Fatalf("list response: %s", listBody) }
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request(http.MethodGet, "/?p=1&s=2&status=1", "", http.StatusOK)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE.*END.* = ANY").WithArgs(sqlmock.AnyArg(), pq.Array([]int16{StatusPending, StatusLocked})).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request(http.MethodGet, "/?status=0,2", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_user").WillReturnResult(sqlmock.NewResult(0, 1))
	request(http.MethodDelete, "/1", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_user").WillReturnResult(sqlmock.NewResult(0, 0))
	request(http.MethodDelete, "/99", "", http.StatusNotFound)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/", "", http.StatusInternalServerError)
}
