package action

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

func nameCatalogFixture() localization.Catalog {
	catalog := actionCatalogFixture()
	catalog.Locales = append(catalog.Locales, localization.Language{Tag: "zh-CN", Name: "中文", Enabled: true})
	catalog.Messages = map[string]localization.Messages{
		"en-US": {Frontend: map[string]any{
			"actions":    map[string]any{"admin:custom:system:override": "Custom access"},
			"resources":  map[string]any{"admin:system:user": "User", "admin:demo:record": "Record"},
			"operations": map[string]any{"read": "Read", "update": "Update"},
		}},
		"zh-CN": {Frontend: map[string]any{
			"resources":  map[string]any{"admin:system:user": "用户"},
			"operations": map[string]any{"read": "查看", "update": "更新"},
		}},
	}
	return catalog
}

func TestActionNamesFollowActualBindings(t *testing.T) {
	catalog := nameCatalogFixture()
	read := ResourceBinding{ResourceKey: "admin:system:user", Operation: "read"}
	update := ResourceBinding{ResourceKey: "admin:system:user", Operation: "update"}
	for _, test := range []struct {
		name, word, locale, want string
		bindings                 []ResourceBinding
	}{
		{"canonical", "admin:read:system:user", "en-US", "Read User", []ResourceBinding{read}},
		{"renamed word", "admin:read:custom:users", "en-US", "Read User", []ResourceBinding{read}},
		{"different operation", "admin:read:system:user", "en-US", "Update User", []ResourceBinding{update}},
		{"different resource", "admin:read:demo:record", "en-US", "Read User", []ResourceBinding{read}},
		{"explicit override", "admin:custom:system:override", "zh-CN", "Custom access", []ResourceBinding{read}},
		{"localized", "admin:read:custom:users", "zh-CN", "查看用户", []ResourceBinding{read}},
		{"multiple bindings", "admin:custom:system:access", "en-US", "Update User / Read User", []ResourceBinding{update, read, update}},
		{"missing translation", "admin:read:custom:users", "en-US", "Read admin:unknown:record", []ResourceBinding{ {ResourceKey: "admin:unknown:record", Operation: "read"}}},
		{"missing operation", "admin:read:custom:users", "zh-CN", "export 用户", []ResourceBinding{ {ResourceKey: "admin:system:user", Operation: "export"}}},
		{"partial translation preserves other bindings", "admin:read:custom:users", "zh-CN", "查看用户 / 查看 admin:unknown:record", []ResourceBinding{read, {ResourceKey: "admin:unknown:record", Operation: "read"}}},
		{"no bindings", "admin:all:system:all", "en-US", "admin:all:system:all", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := Action{Word: test.word, Config: Config{Bindings: &test.bindings}}
			value.localizeName(catalog, test.locale)
			if value.Name != test.want {
				t.Fatalf("name=%q want=%q", value.Name, test.want)
			}
		})
	}
	legacy := Action{Word: "admin:read:custom:users", Config: Config{ResourceKey: read.ResourceKey, Operation: read.Operation}}
	legacy.localizeName(catalog, "en-US")
	if legacy.Name != "Read User" {
		t.Fatal(legacy.Name)
	}
}

func installNameCatalog(t *testing.T, m *Module) localization.Catalog {
	t.Helper()
	catalog := nameCatalogFixture()
	raw, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.catalogSource.(*localization.Module).Install(raw); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestBindingNamesAgreeAcrossReads(t *testing.T) {
	m, mock := newTestModule(t)
	catalog := installNameCatalog(t, m)
	word := "admin:read:custom:users"
	config, err := ResolveConfig(catalog, word, Config{ResourceKey: "admin:system:user", Operation: "update"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	rows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "action_group", "word", "scope", "config"}).AddRow(42, actionTestTime, actionTestTime, "NAME0001", "admin:system:user", word, "admin", string(raw))
	}
	ctx := i18n.WithLocale(t.Context(), "en-US")
	mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(int64(42)).WillReturnRows(rows())
	detail, err := m.Get(ctx, 42)
	if err != nil || detail.Name != "Update User" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array([]string{"NAME0001"})).WillReturnRows(rows())
	associations, err := m.Lookup(ctx, m.store.DB, []string{"NAME0001"})
	if err != nil || len(associations) != 1 || associations[0].Name != detail.Name {
		t.Fatalf("associations=%+v err=%v", associations, err)
	}

	// A sparse name read needs labels and binding identities, not the resource graph
	// or the stored endpoint/UI projections. An unexpected full catalog read panics.
	source := &narrowCatalogSource{source: m.catalogSource.(*localization.Module)}
	m.catalogSource = source
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT word, config - 'ui' - 'endpoints' FROM t_action")).WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow(word, `{"bindings":[{"resource_key":"admin:system:user","operation":"update"}]}`))
	list, err := m.List(ctx, ListActionsInput{Fields: report.Fields{"name"}})
	if err != nil || len(list.Items) != 1 || list.Items[0].Name != detail.Name {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if source.fields.Has("resource_catalog") {
		t.Fatal("name-only read loaded resource mappings")
	}
	encoded, err := json.Marshal(list.Items[0])
	if err != nil || string(encoded) != `{"name":"Update User"}` {
		t.Fatalf("projection=%s err=%v", encoded, err)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT code, word, action_group, config - 'ui' - 'endpoints' FROM t_action")).WillReturnRows(sqlmock.NewRows([]string{"code", "word", "action_group", "config"}).AddRow("NAME0001", word, "admin:system:user", string(raw)))
	summaries, err := m.LookupSummaries(ctx, m.store.DB, []string{"NAME0001"})
	if err != nil || summaries["NAME0001"].Name != detail.Name {
		t.Fatalf("summaries=%+v err=%v", summaries, err)
	}
}

func TestBindingNamesDriveKeywordAndNameFilters(t *testing.T) {
	for _, field := range []string{"q", "name"} {
		t.Run(field, func(t *testing.T) {
			m, mock := newTestModule(t)
			installNameCatalog(t, m)
			word := "admin:custom:alias:access"
			config := `{"bindings":[{"resource_key":"admin:system:user","operation":"update"}]}`
			input := ListActionsInput{Scope: "admin", Fields: report.Fields{"name"}}
			if field == "q" {
				input.Q = "Update User"
			} else {
				input.Name = "Update User"
			}
			mock.ExpectQuery(regexp.QuoteMeta("SELECT word, config - 'ui' - 'endpoints' FROM t_action WHERE scope = $1")).WithArgs("admin").WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow(word, config))
			args := []driver.Value{"%Update User%", pq.Array([]string{word})}
			if field == "name" {
				args = append(args, pq.Array([]string{word}))
			}
			args = append(args, "admin")
			mock.ExpectQuery("SELECT COUNT.*word = ANY").WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			mock.ExpectQuery("SELECT word, config - 'ui' - 'endpoints' FROM t_action WHERE.*word = ANY").WithArgs(append(args, int32(1), int64(0))...).WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow(word, config))
			value, err := m.List(i18n.WithLocale(t.Context(), "en-US"), input)
			if err != nil || value.Total != 1 || len(value.Items) != 1 || value.Items[0].Name != "Update User" {
				t.Fatalf("result=%+v err=%v", value, err)
			}
		})
	}
}

func TestNameFilterDatabaseFailures(t *testing.T) {
	for _, failure := range []string{"query", "scan", "rows"} {
		t.Run(failure, func(t *testing.T) {
			m, mock := newTestModule(t)
			expected := errors.New("name lookup failed")
			query := mock.ExpectQuery("SELECT word, config - 'ui' - 'endpoints' FROM t_action")
			switch failure {
			case "query":
				query.WillReturnError(expected)
			case "scan":
				query.WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow("word", "invalid JSON"))
			case "rows":
				query.WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow("word", `{}`).RowError(0, expected))
			}
			_, err := m.List(t.Context(), ListActionsInput{Q: "name"})
			if err == nil || (failure != "scan" && !errors.Is(err, expected)) || !strings.Contains(err.Error(), "load action filter names") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
