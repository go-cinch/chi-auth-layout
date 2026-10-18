package action

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var actionTestTime = time.Date(2026, 9, 18, 3, 0, 0, 123000000, time.UTC).UnixMilli()

func newTestModule(t *testing.T) (*Module, sqlmock.Sqlmock) {
	localizer := localization.New()
	installActionCatalog(t, localizer)
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
	limits, err := pagination.New(100, 100)
	if err != nil {
		t.Fatal(err)
	}
	module := New(&db.Store{DB: database}, limits, false, localizer)
	module.SetDefinitionCatalog(testDefinitionCatalog{localizer})
	return module, mock
}

func actionRows(id int64, code, name string) *sqlmock.Rows {
	return actionRowsWithGroup(id, code, name, "Demo")
}

func actionRowsWithGroup(id int64, code, name, group string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "action_group", "word", "scope", "config"}).AddRow(id, actionTestTime, actionTestTime, code, group, "admin:read:demo:record", "admin", `{"bindings":[{"resource_key":"admin:demo:record","operation":"read","feature_keys":null}],"feature_keys":null,"ui":[],"endpoints":[]}`)
}

func expectActionGet(mock sqlmock.Sqlmock, id int64, code, name string) {
	mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(id).WillReturnRows(actionRows(id, code, name))
}

func TestActionCRUD(t *testing.T) {
	localizer := localization.New()
	m, mock := newTestModule(t)
	localizer = m.catalogSource.(*localization.Module)
	ctx := context.Background()
	if _, err := m.Get(ctx, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(int64(99)).WillReturnError(sql.ErrNoRows)
	if _, err := m.Get(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	expectActionGet(mock, 1, "ABCDEFGH", "Demo")
	value, err := m.Get(ctx, 1)
	if err != nil || value.ID != 1 || value.CreatedAt != actionTestTime {
		t.Fatalf("get: %#v %v", value, err)
	}
	for _, input := range []CreateActionInput{
		{}, {Word: "demo.read"}, {Word: "admin:all:system:general", Config: Config{Builtin: "all", UI: []UIBinding{}, Endpoints: []Endpoint{}}}} {
		if _, err := m.Create(ctx, input); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	mock.ExpectBegin()
	expectRegisteredActionCatalog(t, mock, localizer)
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(24))
	for _, count := range []int64{0, 1} {
		mock.ExpectExec("INSERT INTO t_action").WithArgs(int64(24), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "admin:demo:record", "admin:read:demo:record", `{"bindings":[{"resource_key":"admin:demo:record","operation":"read","feature_keys":null}],"feature_keys":null,"ui":[],"endpoints":[]}`).WillReturnResult(sqlmock.NewResult(0, count))
	}
	expectActionGet(mock, 24, "ABCDEFGH", "Demo")
	mock.ExpectCommit()
	value, err = m.Create(ctx, CreateActionInput{Word: " admin:read:demo:record ", Config: Config{ResourceKey: "admin:demo:record", Operation: "read", UI: []UIBinding{}, Endpoints: []Endpoint{}}})
	if err != nil || value.Code != "ABCDEFGH" {
		t.Fatalf("create: %#v %v", value, err)
	}
	group := "admin:demo:record"
	if _, err := m.Update(ctx, 0, UpdateActionInput{Group: &group}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := m.Update(ctx, 1, UpdateActionInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	invalidWord := "demo.read"
	if _, err := m.Update(ctx, 1, UpdateActionInput{Word: &invalidWord}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	m.protectSuper = true
	if _, err := m.Update(ctx, AllPermissionsActionID, UpdateActionInput{Group: &group}); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatal(err)
	}
	if err := m.Delete(ctx, AllPermissionsActionID, 2); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatal(err)
	}
	m.protectSuper = false
	mock.ExpectBegin()
	expectActionGet(mock, 1, "ABCDEFGH", "Demo")
	expectRegisteredActionCatalog(t, mock, localizer)
	mock.ExpectExec("UPDATE t_action SET action_group").WithArgs(group, "admin:read:demo:record", `{"bindings":[{"resource_key":"admin:demo:record","operation":"read","feature_keys":null}],"feature_keys":null,"ui":[],"endpoints":[]}`, sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectActionGet(mock, 1, "ABCDEFGH", "Demo")
	mock.ExpectCommit()
	if _, err := m.Update(ctx, 1, UpdateActionInput{Group: &group}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(int64(99)).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	if _, err := m.Update(ctx, 99, UpdateActionInput{Group: &group}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := m.Delete(ctx); !errors.Is(err, ErrIDs) {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM t_action").WithArgs(pq.Array([]int64{1, 2})).WillReturnResult(sqlmock.NewResult(0, 2))
	if err := m.Delete(ctx, 1, 2, 1); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM t_action").WithArgs(pq.Array([]int64{99})).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := m.Delete(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestActionListAndLookup(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	mock.ExpectQuery("SELECT COUNT.*FROM t_action WHERE").WithArgs("%Demo%").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM t_action WHERE").WithArgs("%Demo%", int32(1), int64(0)).WillReturnRows(actionRows(1, "ABCDEFGH", "Demo"))
	result, err := m.List(ctx, ListActionsInput{Group: "Demo"})
	if err != nil || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("list: %#v %v", result, err)
	}
	groups := []string{"%Demo%", "%Admin%"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_action WHERE.*unnest").WithArgs(pq.Array(groups)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	result, err = m.List(ctx, ListActionsInput{Group: "Demo, Admin, demo"})
	if err != nil || result.Total != 0 {
		t.Fatalf("list by groups: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT word, config FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"word", "config"}))
	result, err = m.List(ctx, ListActionsInput{Resource: "GET|/user,POST|/role"})
	if err != nil || result.Total != 0 {
		t.Fatalf("list by resources: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT word, config FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"word", "config"}))
	result, err = m.List(ctx, ListActionsInput{Menu: "/system/user,/system/role", Button: "system.user.read,system.role.read"})
	if err != nil || result.Total != 0 {
		t.Fatalf("list by menus and buttons: %#v %v", result, err)
	}

	codes := []string{"ABCDEFGH", "IJKLMNOP"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_action WHERE code = ANY").WithArgs(pq.Array(codes)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(2))
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code = ANY").WithArgs(pq.Array(codes), int32(1), int64(0)).WillReturnRows(
		actionRows(1, "ABCDEFGH", "A").AddRow(2, actionTestTime, actionTestTime, "IJKLMNOP", "Demo", "admin:read:demo:second", "admin", `{"bindings":[{"resource_key":"admin:demo:record","operation":"read","feature_keys":null}],"feature_keys":null,"ui":[],"endpoints":[]}`),
	)
	result, err = m.List(ctx, ListActionsInput{Code: " ABCDEFGH, IJKLMNOP,ABCDEFGH "})
	if err != nil || result.Total != 2 || len(result.Items) != 2 {
		t.Fatalf("list by codes: %#v %v", result, err)
	}

	invalid := int32(0)
	result, err = m.List(ctx, ListActionsInput{Page: &invalid})
	if err != nil || len(result.Items) != 0 || result.Page != 0 {
		t.Fatalf("out of range: %#v %v", result, err)
	}

	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array([]string{"BBBBBBBB", "AAAAAAAA"})).WillReturnRows(
		actionRows(1, "AAAAAAAA", "A").AddRow(2, actionTestTime, actionTestTime, "BBBBBBBB", "Demo", "admin:read:demo:second", "admin", `{"bindings":[{"resource_key":"admin:demo:record","operation":"read","feature_keys":null}],"feature_keys":null,"ui":[],"endpoints":[]}`),
	)
	items, err := m.Lookup(ctx, m.store.DB, []string{"BBBBBBBB", "AAAAAAAA", "BBBBBBBB"})
	if err != nil || len(items) != 2 || items[0].Code != "BBBBBBBB" {
		t.Fatalf("lookup: %#v %v", items, err)
	}
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array([]string{"BBBBBBBB", "AAAAAAAA"})).WillReturnRows(
		actionRows(1, "AAAAAAAA", "A").AddRow(2, actionTestTime, actionTestTime, "BBBBBBBB", "Demo", "admin:read:demo:second", "admin", `{"bindings":[{"resource_key":"admin:demo:record","operation":"read","feature_keys":null}],"feature_keys":null,"ui":[],"endpoints":[]}`),
	)
	if _, err := m.ValidateCodes(ctx, m.store.DB, []string{"BBBBBBBB", "AAAAAAAA"}); err != nil {
		t.Fatal(err)
	}
	if got := m.SplitCodes(" A, B, A, "); len(got) != 2 || got[0] != "A" {
		t.Fatalf("codes: %#v", got)
	}
}

func TestActionListGroups(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()

	if _, err := m.ListGroups(ctx, strings.Repeat("x", 51)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT MIN\\(action_group\\).*GROUP BY.*LIMIT 20").WillReturnRows(
		sqlmock.NewRows([]string{"action_group"}).AddRow("Action").AddRow("User"),
	)
	result, err := m.ListGroups(ctx, "")
	if err != nil || len(result.Items) != 2 || result.Items[0] != "Action" {
		t.Fatalf("list groups: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT MIN\\(action_group\\).*LIKE.*GROUP BY.*LIMIT 20").WithArgs("%User%").WillReturnRows(
		sqlmock.NewRows([]string{"action_group"}).AddRow("User"),
	)
	result, err = m.ListGroups(ctx, " User ")
	if err != nil || len(result.Items) != 1 || result.Items[0] != "User" {
		t.Fatalf("search groups: %#v %v", result, err)
	}
	mock.ExpectQuery("SELECT MIN\\(action_group\\)").WillReturnError(sql.ErrConnDone)
	if _, err := m.ListGroups(ctx, ""); err == nil {
		t.Fatal("expected database error")
	}
}

func TestActionErrorsAndHelpers(t *testing.T) {
	if got := like("a%b_c!"); got != "%a!%b!_c!!%" {
		t.Fatal(got)
	}
	if _, err := normalizeIDs([]int64{0}); !errors.Is(err, ErrIDs) {
		t.Fatal(err)
	}
	if !errors.Is(mapWriteError(&pq.Error{Code: "23505"}), ErrConflict) {
		t.Fatal("unique error not mapped")
	}
	original := errors.New("database")
	if !errors.Is(mapWriteError(original), original) {
		t.Fatal("unexpected error mapping")
	}
}

func installActionTestLabels(t *testing.T, localizer *localization.Module) {
	t.Helper()
	if err := localizer.Install([]byte(`{"schema_version":2,"default_locale":"en-US","locales":[{"tag":"en-US","name":"English","enabled":true},{"tag":"zh-CN","name":"中文","enabled":true}],"messages":{"en-US":{"frontend":{"actions":{"admin:read:demo:record":"View message","admin:update:demo:record":"Send message"},"groups":{"admin:demo:record":"Messages"}}},"zh-CN":{"frontend":{"actions":{"admin:read:demo:record":"查看消息"},"groups":{"admin:demo:record":"消息"}}}}}`)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(localizer.Reset)
}

func TestActionLocalizedNameAndSearch(t *testing.T) {
	localizer := localization.New()
	m, mock := newTestModule(t)
	localizer = m.catalogSource.(*localization.Module)
	installActionTestLabels(t, localizer)
	ctx := i18n.WithLocale(t.Context(), "zh-CN")
	expectedWords := pq.Array([]string{"admin:read:demo:record"})
	mock.ExpectQuery("SELECT word, config - 'ui' - 'endpoints' FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow("admin:read:demo:record", `{}`).AddRow("admin:update:demo:record", `{}`))
	mock.ExpectQuery("SELECT COUNT.*word = ANY").WithArgs("%查看%", expectedWords).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM t_action WHERE.*word = ANY").WithArgs("%查看%", expectedWords, int32(1), int64(0)).WillReturnRows(actionRows(1, "ABCDEFGH", "ignored"))
	result, err := m.List(ctx, ListActionsInput{Q: "查看"})
	if err != nil || len(result.Items) != 1 || result.Items[0].Name != "查看消息" {
		t.Fatalf("localized list=%+v err=%v", result, err)
	}
	expectActionGet(mock, 1, "ABCDEFGH", "ignored")
	got, err := m.Get(ctx, 1)
	if err != nil || got.Name != "查看消息" {
		t.Fatalf("localized detail=%+v err=%v", got, err)
	}
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array([]string{"ABCDEFGH"})).WillReturnRows(actionRows(1, "ABCDEFGH", "ignored"))
	values, err := m.Lookup(ctx, m.store.DB, []string{"ABCDEFGH"})
	if err != nil || values[0].Name != "查看消息" {
		t.Fatalf("localized association=%+v err=%v", values, err)
	}
	mock.ExpectQuery("SELECT code, word, action_group, config - 'ui' - 'endpoints'").WithArgs(pq.Array([]string{"ABCDEFGH"})).WillReturnRows(sqlmock.NewRows([]string{"code", "word", "action_group", "config"}).AddRow("ABCDEFGH", "admin:read:demo:record", "admin:demo:record", `{}`))
	summaries, err := m.LookupSummaries(ctx, m.store.DB, []string{"ABCDEFGH"})
	if err != nil || summaries["ABCDEFGH"].Name != "查看消息" {
		t.Fatalf("localized summary=%+v err=%v", summaries, err)
	}
	// Name-only projections load binding identities without mapping expansion.
	mock.ExpectQuery("SELECT word, config - 'ui' - 'endpoints' FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow("admin:read:demo:record", `{}`).AddRow("admin:update:demo:record", `{}`))
	mock.ExpectQuery("SELECT COUNT.*word = ANY").WithArgs("%message%", pq.Array([]string{"admin:read:demo:record", "admin:update:demo:record"})).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT word, config - 'ui' - 'endpoints' FROM t_action WHERE").WithArgs("%message%", pq.Array([]string{"admin:read:demo:record", "admin:update:demo:record"}), int32(1), int64(0)).WillReturnRows(sqlmock.NewRows([]string{"word", "config"}).AddRow("admin:read:demo:record", `{}`))
	result, err = m.List(i18n.WithLocale(ctx, "en-US"), ListActionsInput{Q: "message", Fields: report.Fields{"name"}})
	if err != nil || result.Items[0].Name != "View message" {
		t.Fatalf("localized sparse list=%+v err=%v", result, err)
	}
}

func TestLocalizedGroupSearchAndNameFilters(t *testing.T) {
	localizer := localization.New()
	m, mock := newTestModule(t)
	localizer = m.catalogSource.(*localization.Module)
	installActionTestLabels(t, localizer)
	ctx := i18n.WithLocale(t.Context(), "zh-CN")
	mock.ExpectQuery("SELECT MIN.*action_group = ANY").WithArgs("%消息%", pq.Array([]string{"admin:demo:record"})).WillReturnRows(sqlmock.NewRows([]string{"action_group"}).AddRow("admin:demo:record"))
	groups, err := m.ListGroups(ctx, "消息")
	if err != nil || len(groups.Items) != 1 || groups.Items[0] != "admin:demo:record" {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
	mock.ExpectQuery("SELECT COUNT.*action_group = ANY").WithArgs("%消息%", pq.Array([]string{"admin:demo:record"})).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	if _, err := m.List(ctx, ListActionsInput{Group: "消息"}); err != nil {
		t.Fatal(err)
	}
	labels := localizer.Current().Labels("zh-CN", "actions")
	where, args := actionFilters(ListActionsInput{Name: "查看", GroupExact: stringPointer("admin:demo:record")}, labels, nil)
	if !strings.Contains(where, "AND NOT (word = ANY") || !strings.Contains(where, "OR word = ANY") || !strings.Contains(where, "action_group = $4") || len(args) != 4 {
		t.Fatalf("where=%s args=%v", where, args)
	}
	// A locale with a missing translation inherits the default locale label.
	action := Action{Word: "admin:update:demo:record"}
	action.localizeName(localizer.Current(), "zh-CN")
	if action.Name != "Send message" {
		t.Fatal(action.Name)
	}
	if got := matchingLabelWords(map[string]string{"word": "Literal 100%_!"}, "100%_!"); len(got) != 1 {
		t.Fatal(got)
	}
}

func stringPointer(value string) *string { return &value }

type actionPolicy struct{ allowed bool }

type delegatedActionPolicy struct{ publish bool }

func (delegatedActionPolicy) RequireAllPermissions(context.Context) error {
	return authn.ErrPermissionAdministrationRequired
}

func (delegatedActionPolicy) RequirePermissionGrant(context.Context) error { return nil }

func (p delegatedActionPolicy) RequireCatalogAdministration(context.Context) error {
	if p.publish {
		return nil
	}
	return authn.ErrPermissionAdministrationRequired
}

func TestDelegatedDeletionProtectsAllAndDefaultDefinitions(t *testing.T) {
	for _, test := range []struct {
		name                           string
		all, defaults, publish, denied bool
	}{
		{"all stays protected", true, false, true, true},
		{"default requires publisher", false, true, false, true},
		{"publisher may delete default", false, true, true, false},
		{"ordinary action permits grant manager", false, false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, mock := newTestModule(t)
			m.SetPermissionAdministration(delegatedActionPolicy{publish: test.publish})
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT COALESCE").WithArgs(pq.Array([]int64{9})).WillReturnRows(sqlmock.NewRows([]string{"all", "defaults"}).AddRow(test.all, test.defaults))
			if test.denied {
				mock.ExpectRollback()
			} else {
				mock.ExpectExec("DELETE FROM t_action").WithArgs(pq.Array([]int64{9})).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err := m.Delete(t.Context(), 9)
			if test.denied && !errors.Is(err, authn.ErrPermissionAdministrationRequired) || !test.denied && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGrantManagerCannotEditReservedDefinitions(t *testing.T) {
	for _, builtin := range []string{"all", "default"} {
		t.Run(builtin, func(t *testing.T) {
			m, mock := newTestModule(t)
			m.SetPermissionAdministration(delegatedActionPolicy{})
			word := "admin:" + builtin + ":system:general"
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "action_group", "word", "scope", "config"}).AddRow(9, actionTestTime, actionTestTime, "ABCDEFGH", "admin:system:general", word, "admin", `{"builtin":"`+builtin+`","ui":[],"endpoints":[]}`))
			mock.ExpectRollback()
			if _, err := m.Update(t.Context(), 9, UpdateActionInput{Word: &word}); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
				t.Fatal(err)
			}
		})
	}
}

func (p actionPolicy) RequireAllPermissions(context.Context) error {
	if !p.allowed {
		return authn.ErrPermissionAdministrationRequired
	}
	return nil
}
func TestActionDefinitionPolicyRollsBackBeforeWrites(t *testing.T) {
	m, mock := newTestModule(t)
	m.SetPermissionAdministration(actionPolicy{})
	for _, operation := range []func() error{
		func() error {
			_, err := m.Create(t.Context(), CreateActionInput{Word: "admin:read:demo:record", Config: Config{ResourceKey: "admin:demo:record", Operation: "read", UI: []UIBinding{}, Endpoints: []Endpoint{}}})
			return err
		},
		func() error {
			group := "admin:demo:record"
			_, err := m.Update(t.Context(), 2, UpdateActionInput{Group: &group})
			return err
		},
		func() error { return m.Delete(t.Context(), 2) },
	} {
		mock.ExpectBegin()
		mock.ExpectRollback()
		if err := operation(); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
			t.Fatal(err)
		}
	}
	m.SetPermissionAdministration(actionPolicy{allowed: true})
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM t_action").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := m.Delete(t.Context(), 2); err != nil {
		t.Fatal(err)
	}
}

func expectRegisteredActionCatalog(t *testing.T, mock sqlmock.Sqlmock, localizer *localization.Module) {
	t.Helper()
	catalog := localizer.Current()
	for tag, messages := range catalog.Messages {
		if messages.Frontend == nil {
			messages.Frontend = map[string]any{}
		}
		messages.Frontend["groups"] = map[string]any{"admin:demo:record": "Demo", "admin:system:user": "Users"}
		catalog.Messages[tag] = messages
	}
	raw, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
}

type testDefinitionCatalog struct{ source *localization.Module }

func (s testDefinitionCatalog) ActionCatalog(context.Context) (localization.Catalog, error) {
	return s.source.Current(), nil
}
