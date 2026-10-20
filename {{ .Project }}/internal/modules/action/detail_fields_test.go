package action

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

type detailCatalogSpy struct {
	CatalogSource
	selections [][]string
}

func (*detailCatalogSpy) Current() localization.Catalog {
	panic("selected details must not copy the complete catalog")
}

func (s *detailCatalogSpy) Select(fields []string) localization.Catalog {
	s.selections = append(s.selections, append([]string{}, fields...))
	return s.CatalogSource.Select(fields)
}

func TestDetailIdentitySelectionSkipsCatalog(t *testing.T) {
	m, mock := newTestModule(t)
	m.catalogSource = unneededCatalogSource{}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, created_at, updated_at, code, action_group, word, scope FROM t_action WHERE id = $1")).WithArgs(int64(4)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "action_group", "word", "scope"}).AddRow(4, 0, 0, "ABCDEFGH", "", "admin:read:demo:record", "admin"))
	value, err := m.GetSelected(t.Context(), 4, report.Fields{"scope", "word", "group", "code", "updated_at", "created_at", "id"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil || string(raw) != `{"code":"ABCDEFGH","created_at":0,"group":"","id":4,"scope":"admin","updated_at":0,"word":"admin:read:demo:record"}` {
		t.Fatalf("identity response: %s %v", raw, err)
	}
}

func TestDetailSelectionMatchesFullResponseAndLoadsOnlyDependencies(t *testing.T) {
	m, mock := newTestModule(t)
	expectActionGet(mock, 4, "ABCDEFGH", "Demo")
	full, err := m.GetSelected(t.Context(), 4, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(full)
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]json.RawMessage
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	spy := &detailCatalogSpy{CatalogSource: m.catalogSource}
	m.catalogSource = spy
	for _, field := range []string{"name", "names", "config", "resource", "menu", "button"} {
		t.Run(field, func(t *testing.T) {
			columns := "word, config - 'ui' - 'endpoints'"
			rows := sqlmock.NewRows([]string{"word", "config"}).AddRow("admin:read:demo:record", `{"bindings":[{"resource_key":"admin:demo:record","operation":"read","feature_keys":null}]}`)
			wantedCatalog := []string{"resource_catalog"}
			if field == "name" {
				wantedCatalog = []string{"frontend_messages", "default_locale"}
			}
			if field == "names" {
				columns = "word"
				rows = sqlmock.NewRows([]string{"word"}).AddRow("admin:read:demo:record")
				wantedCatalog = []string{"frontend_messages", "locales"}
			}
			mock.ExpectQuery(regexp.QuoteMeta("SELECT " + columns + " FROM t_action WHERE id = $1")).WithArgs(int64(4)).WillReturnRows(rows)
			spy.selections = nil
			value, err := m.GetSelected(t.Context(), 4, report.Fields{field})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(spy.selections, [][]string{wantedCatalog}) {
				t.Fatalf("catalog dependencies: %v, want %v", spy.selections, wantedCatalog)
			}
			actual, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var selected map[string]json.RawMessage
			if err := json.Unmarshal(actual, &selected); err != nil {
				t.Fatal(err)
			}
			if len(selected) != 1 || string(selected[field]) != string(expected[field]) {
				t.Fatalf("%s: selected %s, full %s", field, actual, raw)
			}
		})
	}
}

func TestDetailEmptyNamesRemainObject(t *testing.T) {
	// An empty selected map must not inherit the full response's omitempty.
	value := Action{selectedFields: report.Fields{"names"}, Names: map[string]string{}}
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) != `{"names":{}}` {
		t.Fatalf("empty names: %s %v", encoded, err)
	}
	value.Names = nil
	encoded, err = json.Marshal(value)
	if err != nil || string(encoded) != `{"names":{}}` {
		t.Fatalf("nil names: %s %v", encoded, err)
	}
}

func TestDetailInvalidSelectionDoesNotAccessDependencies(t *testing.T) {
	m := &Module{}
	for _, fields := range []report.Fields{ {}, {"unknown"}, {"id", "id"}} {
		if _, err := m.GetSelected(t.Context(), 4, fields); err == nil {
			t.Fatalf("accepted fields %v", fields)
		}
	}
	if _, err := m.GetSelected(t.Context(), 0, report.Fields{"id"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestDetailFieldsHTTP(t *testing.T) {
	m, mock := newTestModule(t)
{{ if eq .Computed.http_router_final "gin" -}}
	router := gin.New()
	m.HTTP(router.Group(""))
	handler := http.Handler(router)
{{ else -}}
	handler := m.HTTP()
{{ end -}}
	request := func(query string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/4?"+query, nil))
		if w.Code != want {
			t.Fatalf("%s: status %d, body %s", query, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	for _, query := range []string{"f=", "f=unknown", "f=id,id", "f=id,", "f=id&f=name", "f=Name", "broken=%zz"} {
		request(query, http.StatusBadRequest)
	}
	m.catalogSource = unneededCatalogSource{}
	query := regexp.QuoteMeta("SELECT id FROM t_action WHERE id = $1")
	mock.ExpectQuery(query).WithArgs(int64(4)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))
	if body := request("f=+id+", http.StatusOK); body != `{"id":4}` {
		t.Fatal(body)
	}
	mock.ExpectQuery(query).WithArgs(int64(4)).WillReturnError(sql.ErrNoRows)
	request("f=id", http.StatusNotFound)
	mock.ExpectQuery(query).WithArgs(int64(4)).WillReturnError(sqlmock.ErrCancelled)
	request("f=id", http.StatusInternalServerError)
}
