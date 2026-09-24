package action

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var actionTestTime = time.Date(2026, 9, 18, 3, 0, 0, 123000000, time.UTC)

func newTestModule(t *testing.T) (*Module, sqlmock.Sqlmock) {
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
	return New(&db.Store{DB: database}, limits, false), mock
}

func actionRows(id int64, code, name string) *sqlmock.Rows {
	return actionRowsWithGroup(id, code, name, "Demo")
}

func actionRowsWithGroup(id int64, code, name, group string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}).
		AddRow(id, actionTestTime, actionTestTime, code, name, group, "demo.read", "GET|/demo|/demo.v1.Demo/Get", "/demo", "demo.read")
}

func expectActionGet(mock sqlmock.Sqlmock, id int64, code, name string) {
	mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(id).WillReturnRows(actionRows(id, code, name))
}

func TestActionCRUD(t *testing.T) {
	m, mock := newTestModule(t)
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
	if err != nil || value.ID != 1 || value.Group != "Demo" || value.CreatedAt != actionTestTime.UnixMilli() {
		t.Fatalf("get: %#v %v", value, err)
	}

	if _, err := m.Create(ctx, CreateActionInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, CreateActionInput{Name: "Demo", Group: "Demo", Word: "demo.read", Resource: "GET|/demo|"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty rpc resource was accepted: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(24))
	mock.ExpectExec("INSERT INTO t_action").WithArgs(int64(24), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "Demo", "Demo", "demo.read", "GET|/demo|/demo.v1.Demo/Get", "/demo", "demo.read").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO t_action").WithArgs(int64(24), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "Demo", "Demo", "demo.read", "GET|/demo|/demo.v1.Demo/Get", "/demo", "demo.read").WillReturnResult(sqlmock.NewResult(0, 1))
	expectActionGet(mock, 24, "ABCDEFGH", "Demo")
	mock.ExpectCommit()
	value, err = m.Create(ctx, CreateActionInput{Name: " Demo ", Group: " Demo ", Word: " demo.read ", Resource: " GET|/demo|/demo.v1.Demo/Get ", Menu: "/demo", Button: "demo.read"})
	if err != nil || value.Code != "ABCDEFGH" || value.Name != "Demo" || value.Group != "Demo" {
		t.Fatalf("create: %#v %v", value, err)
	}

	group := "Updated"
	if _, err := m.Update(ctx, 0, UpdateActionInput{Group: &group}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := m.Update(ctx, 1, UpdateActionInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	legacyResource := "GET|/demo|"
	if _, err := m.Update(ctx, 1, UpdateActionInput{Resource: &legacyResource}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("legacy resource update was accepted: %v", err)
	}
	m.protectSuper = true
	protectedValue := "changed"
	for name, input := range map[string]UpdateActionInput{
		"resource": {Resource: &protectedValue},
		"menu":     {Menu: &protectedValue},
		"button":   {Button: &protectedValue},
	} {
		if _, err := m.Update(ctx, AllPermissionsActionID, input); !errors.Is(err, apperror.FeatureDisabled) {
			t.Fatalf("update All Permissions %s: %v", name, err)
		}
	}
	if err := m.Delete(ctx, AllPermissionsActionID, 2); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("delete All Permissions: %v", err)
	}
	m.protectSuper = false
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_action SET action_group").WithArgs("Updated", sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(int64(1)).WillReturnRows(actionRowsWithGroup(1, "ABCDEFGH", "Demo", "Updated"))
	mock.ExpectCommit()
	value, err = m.Update(ctx, 1, UpdateActionInput{Group: &group})
	if err != nil || value.Group != "Updated" {
		t.Fatalf("update: %#v %v", value, err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_action").WithArgs("Updated", sqlmock.AnyArg(), int64(99)).WillReturnResult(sqlmock.NewResult(0, 0))
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
	resources := []string{"%GET|/user%", "%POST|/role%"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_action WHERE.*unnest").WithArgs(pq.Array(resources)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	result, err = m.List(ctx, ListActionsInput{Resource: "GET|/user,POST|/role"})
	if err != nil || result.Total != 0 {
		t.Fatalf("list by resources: %#v %v", result, err)
	}
	menus := []string{"%/system/user%", "%/system/role%"}
	buttons := []string{"%system.user.read%", "%system.role.read%"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_action WHERE.*unnest.*unnest").WithArgs(pq.Array(menus), pq.Array(buttons)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	result, err = m.List(ctx, ListActionsInput{Menu: "/system/user,/system/role", Button: "system.user.read,system.role.read"})
	if err != nil || result.Total != 0 {
		t.Fatalf("list by menus and buttons: %#v %v", result, err)
	}

	codes := []string{"ABCDEFGH", "IJKLMNOP"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_action WHERE code = ANY").WithArgs(pq.Array(codes)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(2))
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code = ANY").WithArgs(pq.Array(codes), int32(1), int64(0)).WillReturnRows(
		actionRows(1, "ABCDEFGH", "A").AddRow(2, actionTestTime, actionTestTime, "IJKLMNOP", "B", "Demo", "b", "", "", ""),
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
		actionRows(1, "AAAAAAAA", "A").AddRow(2, actionTestTime, actionTestTime, "BBBBBBBB", "B", "Demo", "b", "", "", ""),
	)
	items, err := m.Lookup(ctx, m.store.DB, []string{"BBBBBBBB", "AAAAAAAA", "BBBBBBBB"})
	if err != nil || len(items) != 2 || items[0].Code != "BBBBBBBB" {
		t.Fatalf("lookup: %#v %v", items, err)
	}
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array([]string{"BBBBBBBB", "AAAAAAAA"})).WillReturnRows(
		actionRows(1, "AAAAAAAA", "A").AddRow(2, actionTestTime, actionTestTime, "BBBBBBBB", "B", "Demo", "b", "", "", ""),
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
	for input, want := range map[string]string{
		"":  "",
		"*": "*",
		" GET|/user|/auth.v1.User/List \n ||/auth.v1.User/Get ": "GET|/user|/auth.v1.User/List\n||/auth.v1.User/Get",
		" GET|/user ":       "GET|/user",
		"GET,PATCH|/user/*": "GET,PATCH|/user/*",
	} {
		if got, ok := normalizeResourceRules(input); !ok || got != want {
			t.Fatalf("normalizeResourceRules(%q) = %q, %v", input, got, ok)
		}
	}
	for _, input := range []string{"GET|/user|", "GET|/user|  ", "|/user", "GET|", "|/user|", "GET||", "||Auth/Get", "GET|user|/Auth/Get"} {
		if _, ok := normalizeResourceRules(input); ok {
			t.Fatalf("invalid resource was accepted: %q", input)
		}
	}
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
