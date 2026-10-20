package role

import (
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

func TestRoleHTTP(t *testing.T) {
	m, mock := newTestModule(t)
	if m.Name() != "/role" {
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
	request(http.MethodGet, "/?p=x", "", http.StatusBadRequest)
	request(http.MethodGet, "/?s=1&s=2", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	listBody := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(listBody, `"t":0`) || strings.Contains(listBody, `"total"`) {
		t.Fatalf("list response: %s", listBody)
	}
	mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request(http.MethodGet, "/?p=1&s=2", "", http.StatusOK)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT id FROM t_role .* FOR UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE t_user SET role_id = NULL, updated_at").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_role").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	request(http.MethodDelete, "/1", "", http.StatusOK)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT id FROM t_role .* FOR UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE t_user SET role_id = NULL, updated_at").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_role").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	request(http.MethodDelete, "/99", "", http.StatusNotFound)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "action_group", "word", "scope", "config"}))
	mock.ExpectRollback()
	request(http.MethodPost, "/", `{"name":"Missing","word":"missing","action_codes":["MISSING1"]}`, http.StatusNotFound)
	mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/", "", http.StatusInternalServerError)
}

func TestRoleExactGroupHTTP(t *testing.T) {
	m, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	for _, query := range []string{"group_exact=One&group_exact=Two", "group_exact=&group_exact=", "group_exact=%zz"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"error_code":"HTTP_INVALID_QUERY"`) {
			t.Fatalf("invalid group query %q: %d %s", query, recorder.Code, recorder.Body.String())
		}
	}
	for _, group := range []string{"", "Missing", "Finance, 100%_'"} {
		mock.ExpectQuery(`SELECT COUNT.*FROM t_role WHERE EXISTS .*a.action_group = \$1`).WithArgs(group).
			WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?group_exact="+url.QueryEscape(group), nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"items":[]`) || !strings.Contains(recorder.Body.String(), `"t":0`) {
			t.Fatalf("exact group %q: %d %s", group, recorder.Code, recorder.Body.String())
		}
	}
}

func TestRoleAllGroupsHTTP(t *testing.T) {
	m, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	invalid := []string{"", "null", `{}`, `"Finance"`, `["Finance",null]`, `[1]`, `[true]`, `[["Finance"]]`, `["Finance"] []`, `["Finance"`}
	for _, value := range invalid {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?groups_all="+url.QueryEscape(value), nil))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"error_code":"HTTP_INVALID_QUERY"`) {
			t.Fatalf("invalid all-groups value %q: %d %s", value, recorder.Code, recorder.Body.String())
		}
	}
	for _, query := range []string{"groups_all=%zz", "groups_all=%5B%5D&groups_all=%5B%5D", "groups_all=%5B%22A%22%5D&groups_all=%5B%22B%22%5D"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?"+query, nil))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid all-groups query %q: %d %s", query, recorder.Code, recorder.Body.String())
		}
	}
	cases := []struct {
		query string
		sql   string
		args  []driver.Value
	}{
		{"groups_all=" + url.QueryEscape(`[]`), `SELECT COUNT\(\*\) FROM t_role$`, nil},
		{"groups_all=" + url.QueryEscape(`["Finance, 100%_'","Operations","Audit","Finance, 100%_'"]`), `SELECT COUNT.*a.action_group = \$1\) AND EXISTS .*a.action_group = \$2\) AND EXISTS .*a.action_group = \$3\)$`, []driver.Value{"Finance, 100%_'", "Operations", "Audit"}},
		{"groups_all=" + url.QueryEscape(`[""," spaced ","SPACED"]`), `SELECT COUNT.*a.action_group = \$1\) AND EXISTS .*a.action_group = \$2\) AND EXISTS .*a.action_group = \$3\)$`, []driver.Value{"", " spaced ", "SPACED"}},
		{"group_exact=Legacy&has_permissions=true&groups_all=" + url.QueryEscape(`["Finance","Legacy"]`), `SELECT COUNT.* = \$1 AND EXISTS .*a.action_group = \$2\) AND EXISTS .*a.action_group = \$3\)$`, []driver.Value{true, "Legacy", "Finance"}},
	}
	for _, tc := range cases {
		mock.ExpectQuery(tc.sql).WithArgs(tc.args...).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"items":[]`) || !strings.Contains(recorder.Body.String(), `"t":0`) {
			t.Fatalf("all-groups query %q: %d %s", tc.query, recorder.Code, recorder.Body.String())
		}
	}
}

func TestRoleKeywordHTTP(t *testing.T) {
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
	for _, keyword := range []string{"  OpErAtOr  ", "100%_!", ""} {
		where := ""
		var args []driver.Value
		if keyword != "" {
			where = " WHERE (LOWER(name) LIKE LOWER($1) ESCAPE '!' OR LOWER(word) LIKE LOWER($1) ESCAPE '!')"
			pattern := "%OpErAtOr%"
			if keyword == "100%_!" {
				pattern = "%100!%!_!!%"
			}
			args = append(args, pattern)
		}
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM t_role" + where)).WithArgs(args...).
			WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?q="+url.QueryEscape(keyword), nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("keyword %q: %d %s", keyword, recorder.Code, recorder.Body.String())
		}
	}
}
