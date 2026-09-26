package user

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	actionmodule "{{ .Computed.module_name_final }}/internal/modules/action"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
	rolemodule "{{ .Computed.module_name_final }}/internal/modules/role"
)

var userTestTime = time.Date(2026, 9, 18, 3, 0, 0, 0, time.UTC)

type passwordFailureClearerStub struct {
	ids []int64
}

func (s *passwordFailureClearerStub) Clear(_ context.Context, userID int64) error {
	s.ids = append(s.ids, userID)
	return nil
}

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
	return New(store, limits, nil, nil, actions, rolemodule.New(store, limits, actions, false), authmodule.Switches{PasswordResetRequired: true}), mock
}

func userRows(id int64, username, code string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "role_id", "action", "username", "code", "last_logged_in_at", "status", "metadata", "wrong", "login_count"}).
		AddRow(id, userTestTime, userTestTime, nil, "", username, code, nil, StatusActive, []byte(`{}`), int64(0), int64(1))
}

func expectUserGet(mock sqlmock.Sqlmock, id int64, username, code string) {
	mock.ExpectQuery("SELECT .* FROM t_user WHERE id").WithArgs(id).WillReturnRows(userRows(id, username, code))
}

func TestUserCRUD(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	if _, err := m.Get(ctx, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM t_user WHERE id").WithArgs(int64(99)).WillReturnError(sql.ErrNoRows)
	if _, err := m.Get(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	expectUserGet(mock, 1, "super", "89HEK28Y")
	value, err := m.Get(ctx, 1)
	if err != nil || value.Username != "super" || value.CreatedAt != userTestTime.UnixMilli() {
		t.Fatalf("get: %#v %v", value, err)
	}
	mock.ExpectQuery("SELECT .* FROM t_user WHERE id").WithArgs(int64(2)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "role_id", "action", "username", "code", "last_logged_in_at", "status", "metadata", "wrong", "login_count"}).
			AddRow(2, userTestTime, userTestTime, nil, "", "broken", "00000002", nil, StatusActive, []byte(`{`), int64(0), int64(1)),
	)
	if _, err := m.Get(ctx, 2); err == nil {
		t.Fatal("invalid metadata was accepted")
	}

	if _, err := m.Create(ctx, CreateUserInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, CreateUserInput{Username: "   ", Password: "secret1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank username: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(6))
	mock.ExpectExec("INSERT INTO t_user").WithArgs(int64(6), sqlmock.AnyArg(), sqlmock.AnyArg(), nil, "", "operator", sqlmock.AnyArg(), sqlmock.AnyArg(), StatusActive, "{}").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO t_user").WithArgs(int64(6), sqlmock.AnyArg(), sqlmock.AnyArg(), nil, "", "operator", sqlmock.AnyArg(), sqlmock.AnyArg(), StatusActive, "{}").WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 6, "operator", "ABCDEFGH")
	mock.ExpectCommit()
	value, err = m.Create(ctx, CreateUserInput{Username: " operator ", Password: "x"})
	if err != nil || value.ID != 6 || value.Code != "ABCDEFGH" {
		t.Fatalf("create: %#v %v", value, err)
	}

	username := "updated"
	if _, err := m.Update(ctx, 1, UpdateUserInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	password := "secret2"
	m.switches.ProtectSuper = true
	if _, err := m.Update(ctx, authmodule.SuperUserID, UpdateUserInput{Password: &password}); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("reset super password: %v", err)
	}
	otherUsername := "renamed"
	otherRole := int64(2)
	lockedStatus := StatusLocked
	for name, input := range map[string]UpdateUserInput{
		"username": {Username: &otherUsername},
		"role":     {RoleID: &otherRole},
		"status":   {Status: &lockedStatus},
	} {
		if _, err := m.Update(ctx, authmodule.SuperUserID, input); !errors.Is(err, apperror.FeatureDisabled) {
			t.Fatalf("change super %s: %v", name, err)
		}
	}
	m.switches.ProtectSuper = false
	locked := StatusLocked
	past := time.Now().Add(-time.Minute).UnixMilli()
	pastMetadata := map[string]any{"lock_expired_at": past}
	if _, err := m.Update(ctx, 1, UpdateUserInput{Status: &locked, Metadata: &pastMetadata}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("past lock expiration: %v", err)
	}
	if _, err := m.Update(ctx, 1, UpdateUserInput{Status: &locked}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing lock metadata: %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user SET username").WithArgs("updated", sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 1, "updated", "89HEK28Y")
	mock.ExpectCommit()
	value, err = m.Update(ctx, 1, UpdateUserInput{Username: &username})
	if err != nil || value.Username != "updated" {
		t.Fatalf("update: %#v %v", value, err)
	}

	clearer := &passwordFailureClearerStub{}
	m.passwordFailures = clearer
	metadata := map[string]any{"source": "admin"}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user SET metadata").WithArgs(`{"source":"admin"}`, sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 1, "updated", "89HEK28Y")
	mock.ExpectCommit()
	if _, err := m.Update(ctx, 1, UpdateUserInput{Metadata: &metadata}); err != nil || len(clearer.ids) != 1 || clearer.ids[0] != 1 {
		t.Fatalf("clear password failures: %#v, %v", clearer.ids, err)
	}

	roleID := int64(99)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(roleID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectRollback()
	if _, err := m.Update(ctx, 1, UpdateUserInput{RoleID: &roleID}); !errors.Is(err, ErrRoleNotFound) {
		t.Fatal(err)
	}

	if err := m.Delete(ctx, 0); !errors.Is(err, ErrIDs) {
		t.Fatal(err)
	}
	m.switches.ProtectSuper = true
	if err := m.Delete(ctx, authmodule.SuperUserID, 2); !errors.Is(err, apperror.FeatureDisabled) {
		t.Fatalf("delete super user: %v", err)
	}
	m.switches.ProtectSuper = false
	mock.ExpectExec("DELETE FROM t_user").WithArgs(pq.Array([]int64{1, 2})).WillReturnResult(sqlmock.NewResult(0, 2))
	if err := m.Delete(ctx, 1, 2, 1); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM t_user").WithArgs(pq.Array([]int64{99})).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := m.Delete(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestUserListAndRelations(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE").WithArgs("%super%", sqlmock.AnyArg(), StatusActive).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM t_user WHERE").WithArgs("%super%", sqlmock.AnyArg(), StatusActive, sqlmock.AnyArg(), int32(1), int64(0)).WillReturnRows(userRows(1, "super", "89HEK28Y"))
	status := StatusActive
	result, err := m.List(ctx, ListUsersInput{Username: "super", Statuses: []int16{status}})
	if err != nil || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("list: %#v %v", result, err)
	}
	codes := []string{"%ABCDEFGH%", "%IJKLMNOP%"}
	statuses := []int16{StatusPending, StatusLocked}
	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE.*unnest.*END.* = ANY").WithArgs(pq.Array(codes), sqlmock.AnyArg(), pq.Array(statuses)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	result, err = m.List(ctx, ListUsersInput{Code: "ABCDEFGH,IJKLMNOP", Statuses: statuses})
	if err != nil || result.Total != 0 {
		t.Fatalf("list by codes and statuses: %#v %v", result, err)
	}
	invalid := int32(-1)
	result, err = m.List(ctx, ListUsersInput{Page: &invalid})
	if err != nil || len(result.Items) != 0 {
		t.Fatalf("range: %#v %v", result, err)
	}
	invalidStatus := int16(3)
	if _, err := m.List(ctx, ListUsersInput{Statuses: []int16{invalidStatus}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid status: %v", err)
	}

	mock.ExpectQuery("SELECT u.id").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code"}).AddRow(3, "readonly", "EXP78RGH"))
	summaries, err := m.SummariesByGroup(ctx, m.store.DB, 1)
	if err != nil || len(summaries) != 1 || summaries[0].Username != "readonly" {
		t.Fatalf("summaries: %#v %v", summaries, err)
	}
	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE id").WithArgs(pq.Array([]int64{1, 2})).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	ids, err := m.ValidateIDs(ctx, m.store.DB, []int64{1, 2, 1})
	if err != nil || len(ids) != 2 {
		t.Fatalf("validate: %#v %v", ids, err)
	}
}

func TestUserHelpers(t *testing.T) {
	for _, username := range []string{"a", "用户名称一", "1user", "user name", " abcde ", "a" + strings.Repeat("b", 100)} {
		if !validUsername(username) {
			t.Fatalf("valid username %q was rejected", username)
		}
	}
	for _, username := range []string{"", "   ", "\t\n"} {
		if validUsername(username) {
			t.Fatalf("invalid username %q was accepted", username)
		}
	}
	if !validPassword("x") || !validPassword(strings.Repeat("x", 100)) || validPassword("   ") {
		t.Fatal("password validation")
	}
	zero := int64(0)
	if nullableRole(&zero) != nil {
		t.Fatal("zero role must clear role")
	}
	if _, err := normalizeReferenceIDs([]int64{-1}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if !errors.Is(mapWriteError(&pq.Error{Code: "23505"}), ErrConflict) {
		t.Fatal("unique not mapped")
	}
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	if _, err := encodeMetadata(cyclic); err == nil {
		t.Fatal("cyclic metadata was accepted")
	}
	if _, err := encodeMetadata(map[string]any{"loginPolicy": map[string]any{"lock_expired_at": 0}}); err == nil {
		t.Fatal("camel-case metadata key was accepted")
	}
	if encoded, err := encodeMetadata(map[string]any{"login_policy": map[string]any{"lock_expired_at": 0}}); err != nil || encoded != `{"login_policy":{"lock_expired_at":0}}` {
		t.Fatalf("snake-case metadata: %s %v", encoded, err)
	}
	active := StatusActive
	normalized, err := normalizeUpdateMetadata(map[string]any{"lock_expired_at": int64(100), "source": "admin"}, &active)
	if err != nil || normalized["source"] != "admin" {
		t.Fatalf("normalize active metadata: %#v %v", normalized, err)
	}
	if _, exists := normalized["lock_expired_at"]; exists {
		t.Fatal("active metadata retained lock_expired_at")
	}
	locked := StatusLocked
	future := time.Now().Add(time.Minute).UnixMilli()
	normalized, err = normalizeUpdateMetadata(map[string]any{"lock_expired_at": float64(future)}, &locked)
	if err != nil || normalized["lock_expired_at"] != future {
		t.Fatalf("normalize locked metadata: %#v %v", normalized, err)
	}
}

func TestUserRelationsAndFullUpdate(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	roleID := int64(1)
	codes := []string{"ABCDEFGH"}
	actionRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}).
			AddRow(1, userTestTime, userTestTime, "ABCDEFGH", "Demo", "Demo", "demo.read", "", "", "")
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").WithArgs(roleID).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(codes)).WillReturnRows(actionRows())
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	mock.ExpectExec("INSERT INTO t_user").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM t_user WHERE id").WithArgs(int64(7)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "role_id", "action", "username", "code", "last_logged_in_at", "status", "metadata", "wrong", "login_count"}).
			AddRow(7, userTestTime, userTestTime, 1, "ABCDEFGH", "operator", "QWERTYUI", userTestTime, StatusLocked, []byte(`{"source":"admin","lock_expired_at":0}`), int64(2), int64(1)),
	)
	mock.ExpectQuery("SELECT .* FROM t_action WHERE code").WithArgs(pq.Array(codes)).WillReturnRows(actionRows())
	mock.ExpectQuery("SELECT .* FROM t_role WHERE id").WithArgs(roleID).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "word", "action"}).AddRow(1, userTestTime, userTestTime, "Admin", "admin", ""),
	)
	mock.ExpectCommit()
	value, err := m.Create(ctx, CreateUserInput{Username: "operator", Password: "secret1", RoleID: &roleID, ActionCodes: codes})
	if err != nil || value.Role == nil || len(value.Actions) != 1 || value.LastLoggedInAt == nil || value.Status != StatusLocked || value.Metadata["source"] != "admin" {
		t.Fatalf("create relations: %#v %v", value, err)
	}

	password, status := "secret2", StatusActive
	metadata := map[string]any{"reject_register_reason": "duplicate request"}
	zeroRole := int64(0)
	emptyCodes := []string{}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_user SET password").WillReturnResult(sqlmock.NewResult(0, 1))
	expectUserGet(mock, 1, "super", "89HEK28Y")
	mock.ExpectCommit()
	value, err = m.Update(ctx, 1, UpdateUserInput{Password: &password, RoleID: &zeroRole, ActionCodes: &emptyCodes, Status: &status, Metadata: &metadata})
	if err != nil || value.Username != "super" {
		t.Fatalf("full update: %#v %v", value, err)
	}

	mock.ExpectQuery("SELECT COUNT.*FROM t_user WHERE id").WithArgs(pq.Array([]int64{99})).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	if _, err := m.ValidateIDs(ctx, m.store.DB, []int64{99}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	negative := int64(-1)
	if err := m.validateRole(ctx, m.store.DB, &negative); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestUserListBatchesRelations(t *testing.T) {
	for _, size := range []int32{10, 50, 100} {
		t.Run(strconv.Itoa(int(size)), func(t *testing.T) {
			m, mock := newTestModule(t)
			// A single connection also detects nested queries before page rows are closed.
			m.store.DB.SetMaxOpenConns(1)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(size))
			rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "role_id", "action", "username", "code", "last_logged_in_at", "status", "metadata", "wrong", "login_count"})
			for i := size; i > 0; i-- {
				rows.AddRow(i, userTestTime, userTestTime, 7, "WRITE,READ,MISSING", "tester", "USERCODE", userTestTime, StatusActive, []byte(`{"source":"admin"}`), 2, 5)
			}
			mock.ExpectQuery("SELECT .* FROM t_user ORDER BY id DESC LIMIT").WithArgs(sqlmock.AnyArg(), size, int64(0)).WillReturnRows(rows).RowsWillBeClosed()
			mock.ExpectQuery(`SELECT .* FROM t_role WHERE id = ANY`).WithArgs(pq.Array([]int64{7})).WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "word", "action"}).AddRow(7, userTestTime, userTestTime, "Editor", "editor", "READ,DELETE,WRITE"))
			mock.ExpectQuery(`SELECT .* FROM t_action WHERE code = ANY`).WithArgs(pq.Array([]string{"WRITE", "READ", "MISSING", "DELETE"})).WillReturnRows(
				sqlmock.NewRows([]string{"id", "created_at", "updated_at", "code", "name", "action_group", "word", "resource", "menu", "btn"}).
					AddRow(1, userTestTime, userTestTime, "READ", "Read", "Demo", "read", "", "", "").
					AddRow(3, userTestTime, userTestTime, "DELETE", "Delete", "Demo", "delete", "", "", "").
					AddRow(2, userTestTime, userTestTime, "WRITE", "Write", "Demo", "write", "", "", ""))
			result, err := m.List(ctx, ListUsersInput{PageSize: &size})
			if err != nil {
				t.Fatal(err)
			}
			if result.Total != int64(size) || len(result.Items) != int(size) {
				t.Fatalf("page: %#v", result)
			}
			for i, item := range result.Items {
				if item.ID != int64(size)-int64(i) || item.LoginCount != 5 || item.Wrong != 2 || item.LastLoggedInAt == nil || item.Metadata["source"] != "admin" {
					t.Fatalf("user: %#v", item)
				}
				if len(item.Actions) != 2 || item.Actions[0].Code != "WRITE" || item.Actions[1].Code != "READ" {
					t.Fatalf("actions: %#v", item.Actions)
				}
				if item.Role == nil || len(item.Role.Actions) != 3 || item.Role.Actions[0].Code != "READ" || item.Role.Actions[1].Code != "DELETE" || item.Role.Actions[2].Code != "WRITE" {
					t.Fatalf("role: %#v", item.Role)
				}
			}
		})
	}
}

func TestUserListRejectsPartialResults(t *testing.T) {
	for _, stage := range []string{"page", "scan", "iteration", "roles", "actions"} {
		t.Run(stage, func(t *testing.T) {
			m, mock := newTestModule(t)
			mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
			page := mock.ExpectQuery("SELECT .* FROM t_user ORDER BY")
			rows := sqlmock.NewRows([]string{"id", "created_at", "updated_at", "role_id", "action", "username", "code", "last_logged_in_at", "status", "metadata", "wrong", "login_count"}).AddRow(1, userTestTime, userTestTime, 7, "READ", "tester", "USERCODE", nil, StatusActive, []byte(`{}`), 0, 1)
			switch stage {
			case "page":
				page.WillReturnError(sql.ErrConnDone)
			case "scan":
				page.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("invalid"))
			case "iteration":
				page.WillReturnRows(rows.RowError(0, sql.ErrConnDone))
			default:
				page.WillReturnRows(rows)
				roles := mock.ExpectQuery("SELECT .* FROM t_role WHERE id = ANY")
				if stage == "roles" {
					roles.WillReturnError(sql.ErrConnDone)
				} else {
					roles.WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at", "name", "word", "action"}).AddRow(7, userTestTime, userTestTime, "Reader", "reader", "READ"))
					mock.ExpectQuery("SELECT .* FROM t_action WHERE code = ANY").WillReturnError(sql.ErrConnDone)
				}
			}
			result, err := m.List(t.Context(), ListUsersInput{})
			if err == nil || result != nil {
				t.Fatalf("partial result on %s failure: %#v %v", stage, result, err)
			}
		})
	}
}
