package role

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

func TestRoleHTTP(t *testing.T) {
	m, mock := newTestModule(t)
	if m.Name() != "/role" { t.Fatal(m.Name()) }
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
	mock.ExpectQuery("SELECT .* FROM t_role WHERE id").WithArgs(int64(1)).WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/1", "", http.StatusInternalServerError)
	expectRoleGet(mock, 1, "Admin", "admin")
	request(http.MethodGet, "/1", "", http.StatusOK)
	request(http.MethodPost, "/", `{}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_role").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	expectRoleGet(mock, 3, "Operator", "operator")
	mock.ExpectCommit()
	request(http.MethodPost, "/", `{"name":"Operator","word":"operator","action_codes":[]}`, http.StatusOK)
	request(http.MethodPatch, "/bad", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_role SET name").WillReturnResult(sqlmock.NewResult(0, 1))
	expectRoleGet(mock, 1, "Updated", "admin")
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"name":"Updated"}`, http.StatusOK)
	request(http.MethodDelete, "/bad", "", http.StatusBadRequest)
	m.protectSuper = true
	disabledBody := request(http.MethodDelete, "/1", "", http.StatusForbidden)
	if !strings.Contains(disabledBody, `"error_code":"SYSTEM_FEATURE_DISABLED"`) { t.Fatalf("disabled Admin deletion: %s", disabledBody) }
	m.protectSuper = false
	request(http.MethodGet, "/?p=x", "", http.StatusBadRequest)
	request(http.MethodGet, "/?s=1&s=2", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	listBody := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(listBody, `"t":0`) || strings.Contains(listBody, `"total"`) { t.Fatalf("list response: %s", listBody) }
	mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request(http.MethodGet, "/?p=1&s=2", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_role").WillReturnResult(sqlmock.NewResult(0, 1))
	request(http.MethodDelete, "/1", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_role").WillReturnResult(sqlmock.NewResult(0, 0))
	request(http.MethodDelete, "/99", "", http.StatusNotFound)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}))
	mock.ExpectRollback()
	request(http.MethodPost, "/", `{"name":"Missing","word":"missing","action_codes":["MISSING1"]}`, http.StatusNotFound)
	mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/", "", http.StatusInternalServerError)
}
