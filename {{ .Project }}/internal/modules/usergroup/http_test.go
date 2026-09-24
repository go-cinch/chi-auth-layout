package usergroup

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

func TestUserGroupHTTP(t *testing.T) {
	m, mock := newTestModule(t)
	if m.Name() != "/user-group" { t.Fatal(m.Name()) }
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
	mock.ExpectQuery("SELECT .* FROM t_user_group WHERE id").WithArgs(int64(1)).WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/1", "", http.StatusInternalServerError)
	expectGroupGet(mock, 1, "Read Only", "readonly")
	request(http.MethodGet, "/1", "", http.StatusOK)
	request(http.MethodPost, "/", `{}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_user_group").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))
	mock.ExpectExec("DELETE FROM t_user_user_group_relation").WillReturnResult(sqlmock.NewResult(0, 0))
	expectGroupGet(mock, 4, "Auditors", "auditor")
	mock.ExpectCommit()
	request(http.MethodPost, "/", `{"name":"Auditors","word":"auditor","action_codes":[],"user_ids":[]}`, http.StatusOK)
	request(http.MethodPatch, "/bad", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user_group SET name").WillReturnResult(sqlmock.NewResult(0, 1))
	expectGroupGet(mock, 1, "Updated", "readonly")
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"name":"Updated"}`, http.StatusOK)
	request(http.MethodDelete, "/bad", "", http.StatusBadRequest)
	request(http.MethodGet, "/?p=wrong", "", http.StatusBadRequest)
	request(http.MethodGet, "/?s=1&s=2", "", http.StatusBadRequest)
	request(http.MethodGet, "/?broken=%zz", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	listBody := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(listBody, `"t":0`) || strings.Contains(listBody, `"total"`) { t.Fatalf("list response: %s", listBody) }
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request(http.MethodGet, "/?p=1&s=2", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_user_group").WillReturnResult(sqlmock.NewResult(0, 1))
	request(http.MethodDelete, "/1", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_user_group").WillReturnResult(sqlmock.NewResult(0, 0))
	request(http.MethodDelete, "/99", "", http.StatusNotFound)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE id").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()
	request(http.MethodPost, "/", `{"name":"Missing","word":"missing","user_ids":[99]}`, http.StatusNotFound)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user_group").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	request(http.MethodPatch, "/99", `{"name":"Missing"}`, http.StatusNotFound)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/", "", http.StatusInternalServerError)
}
