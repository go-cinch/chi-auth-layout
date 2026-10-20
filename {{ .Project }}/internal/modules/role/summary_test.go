package role

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

func newSummaryTestModule(t *testing.T) (*Module, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = database.Close()
	})
	return &Module{store: &db.Store{DB: database}}, mock
}

func TestManagementSummary(t *testing.T) {
	m, mock := newSummaryTestModule(t)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total", "with_permissions", "without_permissions", "groups"}).AddRow(int64(3), int64(1), int64(1), `[{"id":2,"name":"group","count":2}]`))
	value, err := m.Summary(t.Context())
	if err != nil || value.Total != 3 || len(value.Counts) != 2 || value.Counts["with_permissions"] != 1 || value.Trend == nil || value.Groups == nil {
		t.Fatalf("summary: %#v %v", value, err)
	}
	if len(value.Groups) != 1 || value.Groups[0].Name != "group" || value.Groups[0].Count != 2 {
		t.Fatalf("groups: %#v", value.Groups)
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Summary(t.Context()); !errors.Is(err, sqlmock.ErrCancelled) {
		t.Fatalf("query error: %v", err)
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow("invalid"))
	if _, err := m.Summary(t.Context()); err == nil {
		t.Fatal("accepted malformed SQL result")
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total", "with_permissions", "without_permissions", "groups"}).AddRow(int64(3), int64(1), int64(1), `invalid`))
	if _, err := m.Summary(t.Context()); err == nil {
		t.Fatal("accepted invalid group JSON")
	}
}

func TestManagementSummaryHTTP(t *testing.T) {
	m, mock := newSummaryTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	request := func(target string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != want {
			t.Fatalf("%s: %d %s", target, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	request("/summary?q=x", http.StatusBadRequest)
	for _, q := range []string{"f=", "f=total&f=counts", "f=unknown", "f=total,total"} {
		request("/summary?"+q, http.StatusBadRequest)
	}
	request("/summary?q=%zz", http.StatusBadRequest)
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total", "with_permissions", "without_permissions", "groups"}).AddRow(int64(0), int64(0), int64(0), `[]`))
	body := request("/summary", http.StatusOK)
	if !strings.Contains(body, `"total":0`) || !strings.Contains(body, `"groups":[]`) || !strings.Contains(body, `"trend":[]`) {
		t.Fatal(body)
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled)
	request("/summary", http.StatusInternalServerError)
}

func TestRolePermissionFilter(t *testing.T) {
	m, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	for _, q := range []string{"has_permissions=", "has_permissions=1", "has_permissions=False", "has_permissions=true&has_permissions=false"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/?"+q, nil))
		if w.Code != 400 {
			t.Fatal(q, w.Code)
		}
	}
	for _, value := range []bool{true, false} {
		mock.ExpectQuery("SELECT COUNT.*EXISTS.*t_action").WithArgs(value).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
		if _, err := m.List(t.Context(), ListRolesInput{HasPermissions: &value}); err != nil {
			t.Fatal(err)
		}
	}
	mock.ExpectQuery("SELECT COUNT.*EXISTS.*t_action").WithArgs(false).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/?has_permissions=false", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSelectedSummaryTotalOnly(t *testing.T) {
	m, mock := newSummaryTestModule(t)
	mock.ExpectQuery("^" + regexp.QuoteMeta("SELECT COUNT(*) FROM t_role") + "$").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	value, err := m.Summary(t.Context(), "total")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil || string(data) != `{"total":0}` {
		t.Fatal(string(data), err)
	}
	if _, err := m.Summary(t.Context(), "unknown"); err == nil {
		t.Fatal("accepted unknown field")
	}
}
