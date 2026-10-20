package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
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
	if m.Name() != "/user" {
		t.Fatal(m.Name())
	}
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	request := func(method, target, body string, want int) string {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))
		if recorder.Code != want {
			t.Fatalf("%s %s: got %d body %s", method, target, recorder.Code, recorder.Body.String())
		}
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
	expectDefaultUserPermissions(mock)
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(6))
	mock.ExpectExec("INSERT INTO t_user").WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 6, "operator", "ABCDEFGH")
	mock.ExpectCommit()
	request(http.MethodPost, "/", `{"username":"operator","challenge_id":"challenge","credential":"encrypted","action_codes":[]}`, http.StatusOK)
	if credentials.input.ChallengeID != "challenge" || credentials.input.Credential != "encrypted" {
		t.Fatalf("credential input: %#v", credentials.input)
	}
	request(http.MethodPatch, "/bad", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{"password":"secret2"}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{"challenge_id":"challenge"}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{"challenge_id":"challenge","credential":"server-error"}`, http.StatusInternalServerError)
	request(http.MethodPatch, "/1", `{"status":2,"lockExpire":0}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{"status":2,"lock_expired_at":0}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user SET password").WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 1, "super", "89HEK28Y")
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"challenge_id":"challenge","credential":"encrypted"}`, http.StatusOK)
	if credentials.input.ChallengeID != "challenge" || credentials.input.Credential != "encrypted" {
		t.Fatalf("update credential input: %#v", credentials.input)
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user SET username").WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 1, "updated", "89HEK28Y")
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"username":"updated"}`, http.StatusOK)
	request(http.MethodDelete, "/bad", "", http.StatusBadRequest)
	request(http.MethodGet, "/?status=wrong", "", http.StatusBadRequest)
	request(http.MethodGet, "/?status=1,wrong", "", http.StatusBadRequest)
	request(http.MethodGet, "/?status=1&status=2", "", http.StatusBadRequest)
	request(http.MethodGet, "/?status=3", "", http.StatusBadRequest)
	request(http.MethodGet, "/?p=x", "", http.StatusBadRequest)
	request(http.MethodGet, "/?s=x", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	listBody := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(listBody, `"t":0`) || strings.Contains(listBody, `"total"`) {
		t.Fatalf("list response: %s", listBody)
	}
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request(http.MethodGet, "/?p=1&s=2&status=1", "", http.StatusOK)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE.*END.* = ANY").WithArgs(sqlmock.AnyArg(), pq.Array([]int16{StatusPending, StatusLocked})).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request(http.MethodGet, "/?status=0,2", "", http.StatusOK)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT id FROM t_user .* FOR UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE t_msg SET sender_id = NULL, updated_at").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_user").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	request(http.MethodDelete, "/1", "", http.StatusOK)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT id FROM t_user .* FOR UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE t_msg SET sender_id = NULL, updated_at").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_user").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	request(http.MethodDelete, "/99", "", http.StatusNotFound)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/", "", http.StatusInternalServerError)
}

func TestUserLastLoggedInRangeHTTP(t *testing.T) {
	m, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	request := func(query string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
		if w.Code != want {
			t.Fatalf("%s: %d %s", query, w.Code, w.Body.String())
		}
	}
	for _, query := range []string{
		"last_logged_in_from=", "last_logged_in_to=", "last_logged_in_from=-1", "last_logged_in_to=-1",
		"last_logged_in_from=bad", "last_logged_in_to=1.5", "last_logged_in_from=9007199254740992", "last_logged_in_to=9223372036854775808",
		"last_logged_in_from=1&last_logged_in_from=2", "last_logged_in_to=1&last_logged_in_to=2",
		"last_logged_in_from=2&last_logged_in_to=2", "last_logged_in_from=3&last_logged_in_to=2",
	} {
		request(query, http.StatusBadRequest)
	}
	mock.ExpectQuery("SELECT COUNT.*last_logged_in_at > 0.*last_logged_in_at >=").WithArgs(int64(0)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request("last_logged_in_from=0", http.StatusOK)
	mock.ExpectQuery("SELECT COUNT.*last_logged_in_at > 0.*last_logged_in_at <").WithArgs(int64(9007199254740991)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request("last_logged_in_to=9007199254740991", http.StatusOK)
	mock.ExpectQuery("SELECT COUNT.*last_logged_in_at >=.*last_logged_in_at <").WithArgs(sqlmock.AnyArg(), StatusActive, int64(100), int64(200)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request("last_logged_in_from=100&last_logged_in_to=200&status=1", http.StatusOK)
}

func TestUserKeywordHTTP(t *testing.T) {
	m, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	for _, query := range []string{"q=one&q=two", "q=%zz"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid keyword %q: %d", query, recorder.Code)
		}
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM t_user WHERE (LOWER(username) LIKE LOWER($1) ESCAPE '!' OR LOWER(code) LIKE LOWER($1) ESCAPE '!')")).WithArgs("%Mixed!%!_!!%").
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?q=+Mixed%25_!+", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("keyword: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestUserHTTPFailureLogsAreQueryableWithoutCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, method, target, body, operation, message, logError string
		prepare                                                  func(sqlmock.Sqlmock)
		userID                                                   int64
		count                                                    int
	}{
		{
			name: "get", method: http.MethodGet, target: "/42?token=query-secret", operation: "get_user", userID: 42,
			message: "get user 42 failed: query user: database unavailable", logError: "query user: database unavailable",
			prepare: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT .* FROM t_user WHERE id").WithArgs(int64(42)).WillReturnError(errors.New("database unavailable"))
			},
		},
		{
			name: "delete", method: http.MethodDelete, target: "/42,43?token=query-secret", operation: "delete_users", count: 2,
			message: "delete 2 users failed: delete user: database unavailable", logError: "delete user: database unavailable",
			prepare: func(mock sqlmock.Sqlmock) {
				mock.ExpectBegin()
				mock.ExpectExec("SELECT id FROM t_user .* FOR UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("UPDATE t_msg SET sender_id = NULL, updated_at").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("DELETE FROM t_user").WithArgs(pq.Array([]int64{42, 43})).WillReturnError(errors.New("database unavailable"))
				mock.ExpectRollback()
			},
		},
		{
			name: "credential", method: http.MethodPost, target: "/?token=query-secret", operation: "decrypt_user_creation_credential",
			body:    `{"username":"private-username","challenge_id":"challenge-secret","credential":"server-error"}`,
			message: "decrypt user creation credential failed: challenge store unavailable", logError: "challenge store unavailable",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mock := newTestModule(t)
			m.registrationCredentials = &registrationCredentialsStub{}
			if tc.prepare != nil {
				tc.prepare(mock)
			}
{{ if eq .Computed.http_router_final "gin" -}}
			router := gin.New()
			m.HTTP(router.Group(""))
			handler := http.Handler(router)
{{ else -}}
			handler := m.HTTP()
{{ end -}}
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			defer slog.SetDefault(previous)
			request := httptest.NewRequest(tc.method, tc.target, strings.NewReader(tc.body))
			request.Header.Set("Authorization", "Bearer header-secret")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
			}
			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatalf("decode failure log: %v: %s", err, logs.String())
			}
			for key, want := range map[string]any{"operation": tc.operation, "outcome": "failure", "error": tc.logError, "msg": tc.message} {
				if entry[key] != want {
					t.Fatalf("%s = %#v, want %#v", key, entry[key], want)
				}
			}
			if tc.userID != 0 && entry["user_id"] != float64(tc.userID) {
				t.Fatalf("user_id is not numeric: %#v", entry["user_id"])
			}
			if tc.count != 0 {
				if entry["count"] != float64(tc.count) {
					t.Fatalf("count is not numeric: %#v", entry["count"])
				}
			}
			for _, secret := range []string{"query-secret", "header-secret", "private-username", "challenge-secret", "server-error"} {
				if strings.Contains(logs.String(), secret) {
					t.Fatalf("request secret %q leaked: %s", secret, logs.String())
				}
			}
		})
	}
}
