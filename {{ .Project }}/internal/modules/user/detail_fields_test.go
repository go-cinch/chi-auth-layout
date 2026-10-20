package user

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/report"
{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ end }}
)

func TestDetailSelectionSkipsAssociations(t *testing.T) {
	m, mock := newTestModule(t)
	mock.ExpectQuery("SELECT id FROM t_user WHERE id").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	value, err := m.GetSelected(t.Context(), 7, report.Fields{"id"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) != `{"id":7}` {
		t.Fatal(string(encoded), err)
	}
	for _, fields := range []report.Fields{ {}, {"unknown"}, {"id", "id"}} {
		if _, err := m.GetSelected(t.Context(), 7, fields); err == nil {
			t.Fatal("accepted invalid selector", fields)
		}
	}
	if _, err := m.GetSelected(t.Context(), 0, report.Fields{"id"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id FROM t_user WHERE id").WillReturnError(sql.ErrNoRows)
	if _, err := m.GetSelected(t.Context(), 7, report.Fields{"id"}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id FROM t_user WHERE id").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.GetSelected(t.Context(), 7, report.Fields{"id"}); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestDetailHTTPSelectorsBeforeSQL(t *testing.T) {
	m, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" }}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else }}
	handler := m.HTTP()
{{ end }}
	for _, q := range []string{"f=", "f=unknown", "f=id,id", "f=id&f=id", "broken=%ZZ"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/7?"+q, nil))
		if w.Code != 400 {
			t.Fatal(q, w.Code, w.Body.String())
		}
	}
	mock.ExpectQuery("SELECT id FROM t_user WHERE id").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/7?f=id", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"id":7}` {
		t.Fatal(w.Code, w.Body.String())
	}
}
