package dictionary

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

func TestDeleteFailureLogIncludesCount(t *testing.T) {
	m, _, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" }}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else }}
	handler := m.HTTP()
{{ end }}
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	mock.ExpectExec("DELETE FROM t_dictionary").WillReturnError(sqlmock.ErrCancelled)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/42,43", nil))
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil { t.Fatal(err) }
	if w.Code != 500 || entry["count"] != float64(2) || entry["operation"] != "delete_dictionaries" || entry["outcome"] != "failure" || !strings.Contains(entry["msg"].(string), "delete 2 dictionaries failed") || entry["error"] == nil {
		t.Fatalf("failure log: %s", output.String())
	}
}

func TestDictionaryHTTP(t *testing.T) {
	m, cache, mock := newTestModule(t)
	if m.Name() != "/dictionary" || !m.Idempotent() {
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
	if body := request(http.MethodGet, "/pub/i18n", "", http.StatusOK); !strings.Contains(body, `"default_locale"`) || !strings.Contains(body, `"locales"`) || strings.Contains(body, `"messages"`) {
		t.Fatalf("public localization response: %s", body)
	}
	request(http.MethodGet, "/pub/i18n?unknown=1", "", http.StatusBadRequest)
	for _, query := range []string{"f=", "f=unknown", "f=locales,locales", "f=locales&f=messages", "f=messages,frontend_messages", "f=Locales", "f=locales,"} {
		request(http.MethodGet, "/pub/i18n?"+query, "", http.StatusBadRequest)
	}
	localizationBody := request(http.MethodGet, "/pub/i18n?f=scopes", "", http.StatusOK)
	if !strings.Contains(localizationBody, `"scopes"`) || strings.Contains(localizationBody, `"messages"`) || strings.Contains(localizationBody, `"locales"`) {
		t.Fatal(localizationBody)
	}
	localizationBody = request(http.MethodGet, "/pub/i18n?f=frontend_messages", "", http.StatusOK)
	if !strings.Contains(localizationBody, `"messages"`) || strings.Contains(localizationBody, `"backend"`) {
		t.Fatal(localizationBody)
	}

	request(http.MethodGet, "/bad", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT .* FROM t_dictionary WHERE id").WithArgs(int64(1)).WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/1", "", http.StatusInternalServerError)
	expectDictionaryGet(mock, 1, "TEST", "Test", `[1]`, true)
	request(http.MethodGet, "/1", "", http.StatusOK)
	request(http.MethodPost, "/", `{}`, http.StatusBadRequest)
	// Every committed write must still respond 200 when cache invalidation fails.
	m.cache = &faultValueCache{ValueCache: cache, invalidate: func(context.Context) error { return errors.New("redis unavailable") }}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_dictionary").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	expectDictionaryGet(mock, 3, "TEST_VALUES", "Test Values", `[1]`, true)
	mock.ExpectCommit()
	request(http.MethodPost, "/", `{"key":"TEST_VALUES","name":"Test Values","value":[1]}`, http.StatusOK)
	request(http.MethodPatch, "/bad", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{}`, http.StatusBadRequest)
	m.protectCaptchaDictionaries = true
	name := "Updated"
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_dictionary SET name").WithArgs(name, sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectDictionaryGet(mock, 1, "TEST", name, `[1]`, true)
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"name":"Updated"}`, http.StatusOK)
	for _, body := range []string{`{"key":"OTHER_KEY"}`, `{"value":["changed"]}`, `{"enabled":false}`} {
		disabledBody := request(http.MethodPatch, "/1", body, http.StatusForbidden)
		if !strings.Contains(disabledBody, `"error_code":"SYSTEM_FEATURE_DISABLED"`) {
			t.Fatalf("disabled captcha dictionary update: %s", disabledBody)
		}
	}
	request(http.MethodDelete, "/bad", "", http.StatusBadRequest)
	disabledBody := request(http.MethodDelete, "/1,2", "", http.StatusForbidden)
	if !strings.Contains(disabledBody, `"error_code":"SYSTEM_FEATURE_DISABLED"`) {
		t.Fatalf("disabled captcha dictionary deletion: %s", disabledBody)
	}
	m.protectCaptchaDictionaries = false
	request(http.MethodGet, "/?enabled=x", "", http.StatusBadRequest)
	request(http.MethodGet, "/?enabled=true&enabled=false", "", http.StatusBadRequest)
	request(http.MethodGet, "/?p=x", "", http.StatusBadRequest)
	request(http.MethodGet, "/?s=x", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT COUNT.*FROM t_dictionary").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	body := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(body, `"t":0`) {
		t.Fatalf("list body: %s", body)
	}
	mock.ExpectExec("DELETE FROM t_dictionary").WillReturnResult(sqlmock.NewResult(0, 1))
	request(http.MethodDelete, "/1", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_dictionary").WillReturnResult(sqlmock.NewResult(0, 0))
	request(http.MethodDelete, "/99", "", http.StatusNotFound)
}

func TestDictionaryHTTPVersionConflict(t *testing.T) {
	m, _, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT dictionary_key, value FROM t_dictionary").WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"dictionary_key", "value"}).AddRow("TEST_VALUES", []byte(`{}`)))
	mock.ExpectQuery("SELECT value = .* FOR UPDATE").WithArgs(int64(3), "{}").WillReturnRows(sqlmock.NewRows([]string{"matches"}).AddRow(false))
	mock.ExpectRollback()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPatch, "/3", strings.NewReader("{\"value\":{\"new\":true},\"expected_value\":{}}")))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "DICTIONARY_VERSION_CONFLICT") {
		t.Fatalf("version conflict: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDictionaryHTTPHitHeader(t *testing.T) {
	m, cache, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	request := func(path string, status int, header string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Cache-Hit", "1") // Never trust a client's claimed hit.
		handler.ServeHTTP(recorder, req)
		if recorder.Code != status || recorder.Header().Get("X-Cache-Hit") != header {
			t.Fatalf("%s: status=%d hit=%q body=%s", path, recorder.Code, recorder.Header().Get("X-Cache-Hit"), recorder.Body.String())
		}
	}
	expectDictionaryGet(mock, 1, "TEST", "Test", `[1]`, true)
	request("/1", http.StatusOK, "")
	request("/1", http.StatusOK, "1")
	if err := cache.Invalidate(t.Context()); err != nil {
		t.Fatal(err)
	}
	expectDictionaryGet(mock, 1, "TEST", "Updated", `[2]`, true)
	request("/1", http.StatusOK, "")
	request("/1", http.StatusOK, "1")
	if err := cache.Invalidate(t.Context()); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM t_dictionary WHERE id").WillReturnError(sql.ErrNoRows)
	request("/1", http.StatusNotFound, "")
	request("/bad", http.StatusBadRequest, "")
	m.cache = &faultValueCache{load: func(context.Context, string) (string, bool, string, error) {
		return "", false, "", errors.New("redis unavailable")
	}}
	expectDictionaryGet(mock, 1, "TEST", "Test", `[1]`, true)
	request("/1", http.StatusOK, "")
	mock.ExpectQuery("SELECT .* FROM t_dictionary WHERE id").WillReturnError(errors.New("database unavailable"))
	request("/1", http.StatusInternalServerError, "")
	mock.ExpectQuery("SELECT COUNT.*FROM t_dictionary").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request("/", http.StatusOK, "")
}
