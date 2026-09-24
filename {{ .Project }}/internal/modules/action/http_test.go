package action

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

func TestActionHTTP(t *testing.T) {
	m, mock := newTestModule(t)
	if m.Name() != "/action" { t.Fatal(m.Name()) }
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
		if recorder.Code != want { t.Fatalf("%s %s: got %d body %s", method, target, recorder.Code, recorder.Body.String()) }
		return recorder.Body.String()
	}

	request(http.MethodGet, "/bad", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(int64(1)).WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/1", "", http.StatusInternalServerError)
	mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(int64(99)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}),
	)
	request(http.MethodGet, "/99", "", http.StatusNotFound)
	expectActionGet(mock, 1, "ABCDEFGH", "Demo")
	request(http.MethodGet, "/1", "", http.StatusOK)
	request(http.MethodPost, "/", `{}`, http.StatusBadRequest)
	request(http.MethodPost, "/", `{"name":"x","word":"x","unknown":1}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(24))
	mock.ExpectExec("INSERT INTO t_action").WillReturnResult(sqlmock.NewResult(0, 1))
	expectActionGet(mock, 24, "ABCDEFGH", "Demo")
	mock.ExpectCommit()
	request(http.MethodPost, "/", `{"name":"Demo","group":"Demo","word":"demo.read"}`, http.StatusOK)
	request(http.MethodPatch, "/1", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/bad", `{}`, http.StatusBadRequest)
	name := "Updated"
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_action SET name").WithArgs(name, sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectActionGet(mock, 1, "ABCDEFGH", name)
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"name":"Updated"}`, http.StatusOK)
	request(http.MethodDelete, "/bad", "", http.StatusBadRequest)
	m.protectSuper = true
	disabledBody := request(http.MethodPatch, "/1", `{"resource":"GET|/other"}`, http.StatusForbidden)
	if !strings.Contains(disabledBody, `"error_code":"SYSTEM_FEATURE_DISABLED"`) { t.Fatalf("disabled All Permissions update: %s", disabledBody) }
	disabledBody = request(http.MethodDelete, "/1", "", http.StatusForbidden)
	if !strings.Contains(disabledBody, `"error_code":"SYSTEM_FEATURE_DISABLED"`) { t.Fatalf("disabled All Permissions deletion: %s", disabledBody) }
	m.protectSuper = false
	request(http.MethodGet, "/?p=x", "", http.StatusBadRequest)
	request(http.MethodGet, "/?p=1&p=2", "", http.StatusBadRequest)
	request(http.MethodGet, "/?s=x", "", http.StatusBadRequest)
	request(http.MethodGet, "/?broken=%zz", "", http.StatusBadRequest)
	request(http.MethodGet, "/group?keyword=a&keyword=b", "", http.StatusBadRequest)
	request(http.MethodGet, "/group?keyword="+strings.Repeat("x", 51), "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT MIN\\(action_group\\).*LIKE.*LIMIT 20").WithArgs("%User%").WillReturnRows(
		sqlmock.NewRows([]string{"action_group"}).AddRow("User"),
	)
	request(http.MethodGet, "/group?keyword=User", "", http.StatusOK)
	mock.ExpectQuery("SELECT MIN\\(action_group\\)").WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/group", "", http.StatusInternalServerError)

	mock.ExpectQuery("SELECT COUNT.*FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	listBody := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(listBody, `"t":0`) || strings.Contains(listBody, `"total"`) { t.Fatalf("list response: %s", listBody) }
	mock.ExpectExec("DELETE FROM t_action").WillReturnResult(sqlmock.NewResult(0, 1))
	request(http.MethodDelete, "/1", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_action").WillReturnResult(sqlmock.NewResult(0, 0))
	request(http.MethodDelete, "/99", "", http.StatusNotFound)
}
