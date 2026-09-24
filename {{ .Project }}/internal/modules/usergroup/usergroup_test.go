package usergroup

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
	rolemodule "{{ .Computed.module_name_final }}/internal/modules/role"
	usermodule "{{ .Computed.module_name_final }}/internal/modules/user"
)

var userGroupTestTime = time.Date(2026, 9, 18, 3, 0, 0, 0, time.UTC)

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
	actions := actionmodule.New(store, limits, false)
	roles := rolemodule.New(store, limits, actions, false)
	users := usermodule.New(store, limits, nil, nil, actions, roles, authmodule.Switches{PasswordResetRequired: true})
	return New(store, limits, actions, users), mock
}

func groupRows(id int64, name, word string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "word", "action"}).AddRow(id, userGroupTestTime, userGroupTestTime, name, word, "")
}

func expectGroupGet(mock sqlmock.Sqlmock, id int64, name, word string) {
	mock.ExpectQuery("SELECT .* FROM t_user_group WHERE id").WithArgs(id).WillReturnRows(groupRows(id, name, word))
	mock.ExpectQuery("SELECT u.id").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code"}))
}

func TestUserGroupCRUD(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	if _, err := m.Get(ctx, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM t_user_group WHERE id").WithArgs(int64(99)).WillReturnError(sql.ErrNoRows)
	if _, err := m.Get(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	expectGroupGet(mock, 1, "Read Only", "readonly")
	value, err := m.Get(ctx, 1)
	if err != nil || value.Name != "Read Only" || value.Users == nil {
		t.Fatalf("get: %#v %v", value, err)
	}

	if _, err := m.Create(ctx, CreateUserGroupInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_user_group").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "Auditors", "auditor", "").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))
	mock.ExpectExec("DELETE FROM t_user_user_group_relation").WithArgs(int64(4)).WillReturnResult(sqlmock.NewResult(0, 0))
	expectGroupGet(mock, 4, "Auditors", "auditor")
	mock.ExpectCommit()
	value, err = m.Create(ctx, CreateUserGroupInput{Name: " Auditors ", Word: " auditor "})
	if err != nil || value.ID != 4 {
		t.Fatalf("create: %#v %v", value, err)
	}

	name := "Updated"
	if _, err := m.Update(ctx, 1, UpdateUserGroupInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	empty := " "
	if _, err := m.Update(ctx, 1, UpdateUserGroupInput{Name: &empty}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user_group SET name").WithArgs("Updated", sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectGroupGet(mock, 1, "Updated", "readonly")
	mock.ExpectCommit()
	value, err = m.Update(ctx, 1, UpdateUserGroupInput{Name: &name})
	if err != nil || value.Name != "Updated" {
		t.Fatalf("update: %#v %v", value, err)
	}

	users := []int64{99}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE id").WithArgs(pq.Array(users)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()
	if _, err := m.Update(ctx, 1, UpdateUserGroupInput{UserIDs: &users}); !errors.Is(err, ErrUserNotFound) {
		t.Fatal(err)
	}

	if err := m.Delete(ctx); !errors.Is(err, ErrIDs) {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM t_user_group").WithArgs(pq.Array([]int64{1, 2})).WillReturnResult(sqlmock.NewResult(0, 2))
	if err := m.Delete(ctx, 1, 2, 1); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM t_user_group").WithArgs(pq.Array([]int64{99})).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := m.Delete(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestUserGroupListAndRelations(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group WHERE").WithArgs("%read%").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM t_user_group WHERE").WithArgs("%read%", int32(1), int64(0)).WillReturnRows(groupRows(1, "Read Only", "readonly"))
	mock.ExpectQuery("SELECT relation.user_group_id, u.id").WithArgs(pq.Array([]int64{1})).WillReturnRows(sqlmock.NewRows([]string{"user_group_id", "id", "username", "code"}))
	result, err := m.List(ctx, ListUserGroupsInput{Word: "read"})
	if err != nil || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("list: %#v %v", result, err)
	}
	words := []string{"%readonly%", "%auditor%"}
	actions := []string{"%ABCDEFGH%", "%IJKLMNOP%"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group WHERE.*unnest.*unnest").WithArgs(pq.Array(words), pq.Array(actions)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	result, err = m.List(ctx, ListUserGroupsInput{Word: "readonly,auditor", ActionCode: "ABCDEFGH,IJKLMNOP"})
	if err != nil || result.Total != 0 {
		t.Fatalf("list by words and actions: %#v %v", result, err)
	}
	invalid := int32(0)
	result, err = m.List(ctx, ListUserGroupsInput{Page: &invalid})
	if err != nil || len(result.Items) != 0 {
		t.Fatalf("range: %#v %v", result, err)
	}

	mock.ExpectExec("DELETE FROM t_user_user_group_relation").WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO t_user_user_group_relation").WithArgs(int64(3), int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := replaceUsers(ctx, m.store.DB, 2, []int64{3}); err != nil {
		t.Fatal(err)
	}
	if got := like("a_b"); got != "%a!_b%" {
		t.Fatal(got)
	}
	if !errors.Is(mapWriteError(&pq.Error{Code: "23505"}), ErrConflict) {
		t.Fatal("unique not mapped")
	}
}

func TestUserGroupAssociations(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	codes := []string{"ABCDEFGH"}
	users := []int64{3}
	actionRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}).
			AddRow(1, userGroupTestTime, userGroupTestTime, "ABCDEFGH", "Demo", "Demo", "demo.read", "", "", "")
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(codes)).WillReturnRows(actionRows())
	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE id").WithArgs(pq.Array(users)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("INSERT INTO t_user_group").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(4))
	mock.ExpectExec("DELETE FROM t_user_user_group_relation").WithArgs(int64(4)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO t_user_user_group_relation").WithArgs(int64(3), int64(4)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM t_user_group WHERE id").WithArgs(int64(4)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "word", "action"}).AddRow(4, userGroupTestTime, userGroupTestTime, "Auditors", "auditor", "ABCDEFGH"),
	)
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(codes)).WillReturnRows(actionRows())
	mock.ExpectQuery("SELECT u.id").WithArgs(int64(4)).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code"}).AddRow(3, "readonly", "EXP78RGH"))
	mock.ExpectCommit()
	value, err := m.Create(ctx, CreateUserGroupInput{Name: "Auditors", Word: "auditor", ActionCodes: codes, UserIDs: users})
	if err != nil || len(value.Actions) != 1 || len(value.Users) != 1 {
		t.Fatalf("create associations: %#v %v", value, err)
	}

	emptyCodes := []string{}
	emptyUsers := []int64{}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user_group SET action").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_user_user_group_relation").WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectGroupGet(mock, 1, "Read Only", "readonly")
	mock.ExpectCommit()
	value, err = m.Update(ctx, 1, UpdateUserGroupInput{ActionCodes: &emptyCodes, UserIDs: &emptyUsers})
	if err != nil || len(value.Users) != 0 || len(value.ActionCodes) != 0 {
		t.Fatalf("clear associations: %#v %v", value, err)
	}

	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnError(errors.New("database"))
	if _, err := m.List(ctx, ListUserGroupsInput{}); err == nil {
		t.Fatal("expected list error")
	}
}

func TestUserGroupListBatchesRelations(t *testing.T) {
	m, mock := newTestModule(t)
	size := int32(3)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(3))
	mock.ExpectQuery("SELECT .* FROM t_user_group ORDER BY id DESC LIMIT").WithArgs(size, int64(0)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "word", "action"}).
			AddRow(3, userGroupTestTime, userGroupTestTime, "Editors", "editors", "WRITE,READ,MISSING").
			AddRow(2, userGroupTestTime, userGroupTestTime, "Readers", "readers", "READ").
			AddRow(1, userGroupTestTime, userGroupTestTime, "Empty", "empty", "")).RowsWillBeClosed()
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code = ANY").WithArgs(pq.Array([]string{"WRITE", "READ", "MISSING"})).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}).
			AddRow(1, userGroupTestTime, userGroupTestTime, "READ", "Read", "Demo", "read", "", "", "").
			AddRow(2, userGroupTestTime, userGroupTestTime, "WRITE", "Write", "Demo", "write", "", "", ""))
	mock.ExpectQuery("SELECT relation.user_group_id, u.id.*ORDER BY relation.user_group_id, u.id").WithArgs(pq.Array([]int64{3, 2, 1})).WillReturnRows(
		sqlmock.NewRows([]string{"user_group_id", "id", "username", "code"}).AddRow(2, 4, "reader", "USER0004").AddRow(3, 4, "reader", "USER0004").AddRow(3, 8, "editor", "USER0008"))
	result, err := m.List(t.Context(), ListUserGroupsInput{PageSize: &size})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("list: %#v", result)
	}
	editor, reader, empty := result.Items[0], result.Items[1], result.Items[2]
	if editor.ID != 3 || len(editor.Actions) != 2 || editor.Actions[0].Code != "WRITE" || editor.Actions[1].Code != "READ" || len(editor.Users) != 2 || editor.Users[0].ID != 4 || editor.Users[1].ID != 8 {
		t.Fatalf("editors: %#v", editor)
	}
	if len(reader.Actions) != 1 || len(reader.Users) != 1 || reader.Users[0].ID != 4 {
		t.Fatalf("readers: %#v", reader)
	}
	if empty.Actions == nil || len(empty.Actions) != 0 || empty.Users == nil || len(empty.Users) != 0 {
		t.Fatalf("empty: %#v", empty)
	}
}

func TestUserGroupListRejectsMemberFailure(t *testing.T) {
	m, mock := newTestModule(t)
	mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM t_user_group ORDER BY").WillReturnRows(groupRows(1, "Empty", "empty"))
	mock.ExpectQuery("SELECT relation.user_group_id, u.id").WillReturnError(sql.ErrConnDone)
	if result, err := m.List(t.Context(), ListUserGroupsInput{}); result != nil || !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("partial group: %#v %v", result, err)
	}
}
