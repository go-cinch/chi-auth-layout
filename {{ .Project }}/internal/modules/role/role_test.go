package role

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
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
)

var roleTestTime = time.Date(2026, 9, 18, 3, 0, 0, 0, time.UTC)

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
	store := &db.Store{DB: database}
	return New(store, limits, actionmodule.New(store, limits, false), false), mock
}

func TestRoleExists(t *testing.T) {
	m, mock := newTestModule(t)
	for _, exists := range []bool{true, false} {
		mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
		if got, err := m.Exists(t.Context(), m.store.DB, 42); err != nil || got != exists {
			t.Fatalf("exists = %v, %v", got, err)
		}
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(int64(42)).WillReturnError(sql.ErrConnDone)
	if _, err := m.Exists(t.Context(), m.store.DB, 42); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("database error = %v", err)
	}
}

func roleRows(id int64, name, word, codes string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "word", "action"}).AddRow(id, roleTestTime, roleTestTime, name, word, codes)
}

func expectRoleGet(mock sqlmock.Sqlmock, id int64, name, word string) {
	mock.ExpectQuery("SELECT .* FROM t_role WHERE id").WithArgs(id).WillReturnRows(roleRows(id, name, word, ""))
}

func TestRoleCRUD(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	if _, err := m.Get(ctx, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM t_role WHERE id").WithArgs(int64(99)).WillReturnError(sql.ErrNoRows)
	if _, err := m.Get(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	expectRoleGet(mock, 1, "Admin", "admin")
	value, err := m.Get(ctx, 1)
	if err != nil || value.Name != "Admin" || value.CreatedAt != roleTestTime.UnixMilli() {
		t.Fatalf("get: %#v %v", value, err)
	}

	if _, err := m.Create(ctx, CreateRoleInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_role").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "Operator", "operator", "").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	expectRoleGet(mock, 3, "Operator", "operator")
	mock.ExpectCommit()
	value, err = m.Create(ctx, CreateRoleInput{Name: " Operator ", Word: " operator "})
	if err != nil || value.ID != 3 {
		t.Fatalf("create: %#v %v", value, err)
	}

	name := "Updated"
	if _, err := m.Update(ctx, 1, UpdateRoleInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_role SET name").WithArgs("Updated", sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectRoleGet(mock, 1, "Updated", "admin")
	mock.ExpectCommit()
	value, err = m.Update(ctx, 1, UpdateRoleInput{Name: &name})
	if err != nil || value.Name != "Updated" {
		t.Fatalf("update: %#v %v", value, err)
	}

	codes := []string{"MISSING1"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(codes)).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}))
	mock.ExpectRollback()
	if _, err := m.Update(ctx, 1, UpdateRoleInput{ActionCodes: &codes}); !errors.Is(err, ErrActionNotFound) {
		t.Fatal(err)
	}

	if err := m.Delete(ctx, -1); !errors.Is(err, ErrIDs) {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM t_role").WithArgs(pq.Array([]int64{1, 2})).WillReturnResult(sqlmock.NewResult(0, 2))
	if err := m.Delete(ctx, 1, 2, 1); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM t_role").WithArgs(pq.Array([]int64{99})).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := m.Delete(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestRoleListAndHelpers(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	mock.ExpectQuery("SELECT COUNT.*FROM t_role WHERE").WithArgs("%adm%").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM t_role WHERE").WithArgs("%adm%", int32(1), int64(0)).WillReturnRows(roleRows(1, "Admin", "admin", ""))
	result, err := m.List(ctx, ListRolesInput{Word: "adm"})
	if err != nil || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("list: %#v %v", result, err)
	}
	words := []string{"%admin%", "%operator%"}
	actions := []string{"%ABCDEFGH%", "%IJKLMNOP%"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_role WHERE.*unnest.*unnest").WithArgs(pq.Array(words), pq.Array(actions)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	result, err = m.List(ctx, ListRolesInput{Word: "admin,operator", ActionCode: "ABCDEFGH,IJKLMNOP"})
	if err != nil || result.Total != 0 {
		t.Fatalf("list by words and actions: %#v %v", result, err)
	}
	invalid := int32(0)
	result, err = m.List(ctx, ListRolesInput{PageSize: &invalid})
	if err != nil || len(result.Items) != 0 {
		t.Fatalf("range: %#v %v", result, err)
	}
	if got := like("a%b"); got != "%a!%b%" {
		t.Fatal(got)
	}
	if _, err := normalizeIDs(nil); !errors.Is(err, ErrIDs) {
		t.Fatal(err)
	}
	if !errors.Is(mapWriteError(&pq.Error{Code: "23505"}), ErrConflict) {
		t.Fatal("unique not mapped")
	}
}

func TestRoleActionCodesAndFailures(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	codes := []string{"ABCDEFGH"}
	actionRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}).
			AddRow(1, roleTestTime, roleTestTime, "ABCDEFGH", "Demo", "Demo", "demo.read", "", "", "")
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(codes)).WillReturnRows(actionRows())
	mock.ExpectQuery("INSERT INTO t_role").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	mock.ExpectQuery("SELECT .* FROM t_role WHERE id").WithArgs(int64(3)).WillReturnRows(roleRows(3, "Operator", "operator", "ABCDEFGH"))
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(codes)).WillReturnRows(actionRows())
	mock.ExpectCommit()
	value, err := m.Create(ctx, CreateRoleInput{Name: "Operator", Word: "operator", ActionCodes: codes})
	if err != nil || len(value.Actions) != 1 || value.ActionCodes[0] != "ABCDEFGH" {
		t.Fatalf("create with actions: %#v %v", value, err)
	}

	name := "missing"
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_role").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if _, err := m.Update(ctx, 99, UpdateRoleInput{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnError(errors.New("database"))
	if _, err := m.List(ctx, ListRolesInput{}); err == nil {
		t.Fatal("expected list error")
	}
}

func TestAdminRoleProtection(t *testing.T) {
	m, mock := newTestModule(t)
	m.protectSuper = true
	if err := m.Delete(t.Context(), AdminRoleID, 2); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("delete Admin role: %v", err)
	}
	actionRows := func(codes ...string) *sqlmock.Rows {
		rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"})
		for index, code := range codes {
			rows.AddRow(index+1, roleTestTime, roleTestTime, code, code, "Demo", "demo."+code, "*", "*", "*")
		}
		return rows
	}

	withoutAll := []string{"OTHER001"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(withoutAll)).WillReturnRows(actionRows(withoutAll...))
	mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(actionmodule.AllPermissionsActionID).WillReturnRows(actionRows("ALLPERM"))
	mock.ExpectRollback()
	if _, err := m.Update(t.Context(), AdminRoleID, UpdateRoleInput{ActionCodes: &withoutAll}); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("remove All Permissions from Admin: %v", err)
	}

	for _, codes := range [][]string{[]string{"ALLPERM", "OTHER001"}, []string{"ALLPERM"}} {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(codes)).WillReturnRows(actionRows(codes...))
		mock.ExpectQuery("SELECT .* FROM t_action WHERE id").WithArgs(actionmodule.AllPermissionsActionID).WillReturnRows(actionRows("ALLPERM"))
		mock.ExpectExec("UPDATE t_role SET action").WithArgs(strings.Join(codes, ","), sqlmock.AnyArg(), AdminRoleID).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("SELECT .* FROM t_role WHERE id").WithArgs(AdminRoleID).WillReturnRows(roleRows(AdminRoleID, "Admin", "admin", strings.Join(codes, ",")))
		mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(codes)).WillReturnRows(actionRows(codes...))
		mock.ExpectCommit()
		value, err := m.Update(t.Context(), AdminRoleID, UpdateRoleInput{ActionCodes: &codes})
		if err != nil || !containsCode(value.ActionCodes, "ALLPERM") || len(value.ActionCodes) != len(codes) {
			t.Fatalf("allowed Admin action update %v: %#v %v", codes, value, err)
		}
	}
}

func TestRoleListBatchesActions(t *testing.T) {
	m, mock := newTestModule(t)
	size := int32(3)
	mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(3))
	mock.ExpectQuery("SELECT .* FROM t_role ORDER BY id DESC LIMIT").WithArgs(size, int64(0)).WillReturnRows(
		roleRows(3, "Editor", "editor", "WRITE,READ,MISSING").AddRow(2, roleTestTime, roleTestTime, "Reader", "reader", "READ").AddRow(1, roleTestTime, roleTestTime, "Empty", "empty", "")).RowsWillBeClosed()
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code = ANY").WithArgs(pq.Array([]string{"WRITE", "READ", "MISSING"})).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}).
			AddRow(1, roleTestTime, roleTestTime, "READ", "Read", "Demo", "read", "", "", "").
			AddRow(2, roleTestTime, roleTestTime, "WRITE", "Write", "Demo", "write", "", "", ""))
	result, err := m.List(t.Context(), ListRolesInput{PageSize: &size})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 || len(result.Items[0].Actions) != 2 || result.Items[0].Actions[0].Code != "WRITE" || result.Items[0].Actions[1].Code != "READ" || len(result.Items[1].Actions) != 1 || result.Items[2].Actions == nil || len(result.Items[2].Actions) != 0 {
		t.Fatalf("list: %#v", result)
	}
}

func TestLookupRecords(t *testing.T) {
	m, mock := newTestModule(t)
	if items, err := m.LookupRecords(t.Context(), m.store.DB, nil); err != nil || len(items) != 0 {
		t.Fatalf("empty: %v %v", items, err)
	}
	mock.ExpectQuery("SELECT .* FROM t_role WHERE id = ANY").WithArgs(pq.Array([]int64{7, 8})).WillReturnRows(roleRows(8, "Reader", "reader", "READ").AddRow(7, roleTestTime, roleTestTime, "Editor", "editor", "WRITE,READ"))
	items, err := m.LookupRecords(t.Context(), m.store.DB, []int64{7, 8, 7})
	if err != nil || len(items) != 2 || len(items[7].ActionCodes) != 2 {
		t.Fatalf("batch: %v %v", items, err)
	}
	mock.ExpectQuery("SELECT .* FROM t_role WHERE id = ANY").WithArgs(pq.Array([]int64{99})).WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "word", "action"}))
	if _, err := m.LookupRecords(t.Context(), m.store.DB, []int64{99}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing role: %v", err)
	}
	mock.ExpectQuery("SELECT .* FROM t_role WHERE id = ANY").WillReturnError(sql.ErrConnDone)
	if _, err := m.LookupRecords(t.Context(), m.store.DB, []int64{7}); !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("database failure: %v", err)
	}
}

func TestRoleListRejectsActionFailure(t *testing.T) {
	m, mock := newTestModule(t)
	mock.ExpectQuery("SELECT COUNT.*FROM t_role").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM t_role ORDER BY").WillReturnRows(roleRows(1, "Reader", "reader", "READ"))
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code = ANY").WillReturnError(sql.ErrConnDone)
	if result, err := m.List(t.Context(), ListRolesInput{}); result != nil || !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("partial role: %#v %v", result, err)
	}
}
