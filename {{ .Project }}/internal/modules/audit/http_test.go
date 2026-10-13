package audit

import (
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ else }}
	"github.com/go-chi/chi/v5"
{{ end }}
)

func TestHTTP(t *testing.T) {
	m, mock := setup(t)
	if m.Name() != "/audit" {
		t.Fatal(m.Name())
	}

{{ if eq .Computed.http_router_final "gin" }}
	router := gin.New()
	m.HTTP(router.Group("/audit"))
{{ else }}
	router := chi.NewRouter()
	router.Mount("/audit", m.HTTP())
{{ end }}
	handler := http.Handler(router)

	call := func(target string, status int) string {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", target, nil))
		if w.Code != status {
			t.Fatalf("%s = %d %s", target, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	for _, q := range []string{"p=", "p=1&p=2", "p=bad", "s=2147483648", "from=bad", "from=0", "to=-1", "result=bad", "time_zone=missing", "q=%ZZ", "weekday=0", "hour=8", "weekday=0&hour=24", "weekday=7&hour=0", "weekday=0&weekday=1&hour=8", "hour=no&weekday=0", "sort=invalid", "username=a&username=b"} {
		call("/audit/login?"+q, 400)
	}
	for _, route := range []string{"summary", "option", "export"} {
		call("/audit/login/"+route+"?p=bad", 400)
	}
	call("/audit/login/detail/abc", 400)
	call("/audit/login/detail/0", 400)
	call("/audit/login/missing", 404)
	mock.ExpectQuery("SELECT .* WHERE kind=").WillReturnError(sql.ErrNoRows)
	call("/audit/login/detail/1", 404)
	mock.ExpectQuery("SELECT .* WHERE kind=").WillReturnRows(row())
	call("/audit/login/detail/1", 200)
	mock.ExpectQuery("SELECT .* WHERE kind=").WillReturnError(sqlmock.ErrCancelled)
	call("/audit/login/detail/1", 500)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* ORDER BY").WillReturnRows(row())
	call("/audit/login?p=1&s=10", 200)
	mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled)
	call("/audit/login", 500)
	mock.ExpectQuery("SELECT DISTINCT").WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("alice"))
	call("/audit/login/option", 200)
	mock.ExpectQuery("SELECT DISTINCT").WillReturnError(sqlmock.ErrCancelled)
	call("/audit/login/option", 500)
	expectSummary(mock, -1)
	call("/audit/operation/summary", 200)
	expectSummary(mock, 0)
	call("/audit/operation/summary", 500)
	mock.ExpectQuery("SELECT .* LIMIT 10001").WillReturnRows(row())
	body := call("/audit/login/export", 200)
	if !strings.Contains(body, "alice") || strings.Contains(body, "password") {
		t.Fatal(body)
	}
	mock.ExpectQuery("SELECT .* LIMIT 10001").WillReturnError(sqlmock.ErrCancelled)
	call("/audit/login/export", 500)
	for _, s := range []string{"=SUM(1)", " +1", "-1", "@cmd"} {
		if !strings.HasPrefix(csvSafe(s), "'") {
			t.Fatal(s)
		}
	}
	if csvSafe("alice") != "alice" {
		t.Fatal("csv")
	}
}

func TestAuditTargets(t *testing.T) {
	m := &Module{}
	cases := []struct{ method, path, kind, operation, id string }{
		{"POST", "/auth/pub/login", "login", "login", ""},
		{"GET", "/auth/pub/login", "", "", ""},
		{"POST", "/auth/pub/register", "operation", "register", ""},
		{"PATCH", "/auth/change/pwd", "operation", "change_password", ""},
		{"PATCH", "/auth/reset/pwd", "operation", "reset_password", ""},
		{"POST", "/auth/logout", "operation", "logout", ""},
		{"POST", "/auth/pub/challenge", "", "", ""},
		{"POST", "/other", "", "", ""},
		{"POST", "/user", "operation", "create", ""},
		{"PATCH", "/user/2", "operation", "update", "2"},
		{"PUT", "/role/2", "operation", "update", "2"},
		{"DELETE", "/user/2,3", "operation", "delete", "2,3"},
		{"PATCH", "/msg/inbox/4", "operation", "read", "4"},
		{"DELETE", "/msg/inbox/4", "operation", "delete_personal", "4"},
		{"POST", "/msg/inbox/read-all", "operation", "read_all", ""},
		{"GET", "/audit/operation", "", "", ""},
	}
	for _, c := range cases {
		target := m.AuditTarget(c.method, c.path)
		if target.Kind != c.kind || target.Operation != c.operation || target.ResourceID != c.id {
			t.Fatalf("%s %s: %+v", c.method, c.path, target)
		}
	}
}
