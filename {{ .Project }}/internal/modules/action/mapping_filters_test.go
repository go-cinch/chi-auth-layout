package action

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

func TestMappingFilterUsesCurrentCatalogBeforePagination(t *testing.T) {
	m, mock := newTestModule(t)
	catalog := resourceMoveCatalog("admin:system:user")
	feature := catalog.ResourceCatalog.Buttons["admin:system:user:query"]
	feature.HTTP[0].Path = "/new_user"
	catalog.ResourceCatalog.Buttons["admin:system:user:query"] = feature
	raw, _ := json.Marshal(catalog)
	if err := m.catalogSource.(*localization.Module).Install(raw); err != nil {
		t.Fatal(err)
	}
	ordinary := `{"bindings":[{"resource_key":"admin:system:user","operation":"read","feature_keys":null}],"ui":[],"endpoints":[{"protocol":"http","method":"GET","path":"/old"}]}`
	all := `{"builtin":"all","ui":[],"endpoints":[]}`
	mock.ExpectQuery("SELECT word, config FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).
		AddRow("admin:read:system:user", ordinary).AddRow("admin:all:system:general", all))
	words := []string{"admin:all:system:general", "admin:read:system:user"}
	mock.ExpectQuery("SELECT COUNT.*word = ANY").WithArgs(pq.Array(words)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(2))
	mock.ExpectQuery("SELECT .* FROM t_action.*word = ANY").WithArgs(pq.Array(words), int32(1), int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "word", "config"}).AddRow(1, "admin:all:system:general", all))
	p, size := int32(2), int32(1)
	value, err := m.List(t.Context(), ListActionsInput{Resource: "/NEW_", Fields: report.Fields{"id", "resource"}, Page: &p, PageSize: &size})
	if err != nil {
		t.Fatal(err)
	}
	if value.Total != 2 || len(value.Items) != 1 || value.Items[0].Resource != "GET|/new_user" {
		t.Fatalf("%+v", value)
	}
	encoded, _ := json.Marshal(value.Items[0])
	if strings.Contains(string(encoded), `"menu"`) {
		t.Fatal(string(encoded))
	}
}

func TestMappingFiltersSkipCountWhenNoMatchAndPropagateFailures(t *testing.T) {
	m, mock := newTestModule(t)
	for _, filter := range []ListActionsInput{ {Resource: "/absent"}, {Menu: "/absent", Button: "/absent"}} {
		mock.ExpectQuery("SELECT word, config").WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow("admin:read:demo:record", `{"bindings":[],"ui":[],"endpoints":[]}`))
		value, err := m.List(t.Context(), filter)
		if err != nil || value.Total != 0 {
			t.Fatal(value, err)
		}
	}
	for _, test := range []struct {
		rows    *sqlmock.Rows
		failure error
	}{
		{failure: sqlmock.ErrCancelled},
		{rows: sqlmock.NewRows([]string{"word", "config"}).AddRow("admin:read:demo:record", `broken`)},
		{rows: sqlmock.NewRows([]string{"word", "config"}).AddRow("invalid", `{}`)},
		{rows: sqlmock.NewRows([]string{"word", "config"}).AddRow("admin:read:demo:record", `{"bindings":[],"ui":[],"endpoints":[]}`).RowError(0, sqlmock.ErrCancelled)},
	} {
		expect := mock.ExpectQuery("SELECT word, config")
		if test.failure != nil {
			expect.WillReturnError(test.failure)
		} else {
			expect.WillReturnRows(test.rows)
		}
		if _, err := m.List(t.Context(), ListActionsInput{Resource: "/user"}); err == nil {
			t.Fatal("lost mapping read failure")
		}
	}
	if !matchesMapping("GET|/literal_%", "missing, LITERAL_%") || matchesMapping("GET|/literal-ab", "literal_%") || !matchesMapping("", ",,") {
		t.Fatal("literal keyword matching")
	}
}
