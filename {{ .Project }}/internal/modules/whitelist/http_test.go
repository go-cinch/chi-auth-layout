package whitelist

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

func TestWhitelistHTTP(t *testing.T) {
	m, mock := newTestModule(t)
	if m.Name() != "/whitelist" { t.Fatal(m.Name()) }
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
	mock.ExpectQuery("SELECT .* FROM t_whitelist WHERE id").WithArgs(int64(1)).WillReturnError(sqlmock.ErrCancelled)
	request(http.MethodGet, "/1", "", http.StatusInternalServerError)
	expectWhitelistGet(mock, 1, 0, "GET|/healthz")
	request(http.MethodGet, "/1", "", http.StatusOK)
	request(http.MethodPost, "/", `{}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_whitelist").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	expectWhitelistGet(mock, 3, 0, "GET|/readyz")
	mock.ExpectCommit()
	request(http.MethodPost, "/", `{"category":0,"resource":"GET|/readyz"}`, http.StatusOK)
	request(http.MethodPatch, "/bad", `{}`, http.StatusBadRequest)
	request(http.MethodPatch, "/1", `{}`, http.StatusBadRequest)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_whitelist SET resource").WillReturnResult(sqlmock.NewResult(0, 1))
	expectWhitelistGet(mock, 1, 0, "GET|/livez")
	mock.ExpectCommit()
	request(http.MethodPatch, "/1", `{"resource":"GET|/livez"}`, http.StatusOK)
	request(http.MethodDelete, "/bad", "", http.StatusBadRequest)
	request(http.MethodGet, "/?category=2", "", http.StatusBadRequest)
	request(http.MethodGet, "/?category=0&category=1", "", http.StatusBadRequest)
	request(http.MethodGet, "/?p=x", "", http.StatusBadRequest)
	request(http.MethodGet, "/?s=x", "", http.StatusBadRequest)
	mock.ExpectQuery("SELECT COUNT.*FROM t_whitelist").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	listBody := request(http.MethodGet, "/", "", http.StatusOK)
	if !strings.Contains(listBody, `"t":0`) || strings.Contains(listBody, `"total"`) { t.Fatalf("list response: %s", listBody) }
	mock.ExpectQuery("SELECT COUNT.*FROM t_whitelist WHERE").WithArgs(int16(0)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	request(http.MethodGet, "/?category=0&p=1&s=2", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_whitelist").WillReturnResult(sqlmock.NewResult(0, 1))
	request(http.MethodDelete, "/1", "", http.StatusOK)
	mock.ExpectExec("DELETE FROM t_whitelist").WillReturnResult(sqlmock.NewResult(0, 0))
	request(http.MethodDelete, "/99", "", http.StatusNotFound)
}
