package usergroup

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

func TestSelectedGroupFields(t *testing.T) {
	for _, fields := range []report.Fields{
		{"member_count"}, {"member_preview"}, {"member_count", "member_preview"}, {"actions"}, {"users"}} {
		t.Run(fields[0]+string(rune(len(fields)+'0')), func(t *testing.T) {
			m, mock := newTestModule(t)
			mock.ExpectQuery("SELECT COUNT.*FROM t_user_group").WillReturnRows(sqlmock.NewRows([]string{"t"}).AddRow(1))
			mock.ExpectQuery("SELECT .* FROM t_user_group ORDER").WillReturnRows(groupRows(1, "Group", "group"))
			if fields.Has("users") {
				mock.ExpectQuery("SELECT relation.user_group_id").WillReturnRows(sqlmock.NewRows([]string{"group", "id", "username", "code"}))
			}
			cols := []string{"id"}
			if fields.Has("member_count") {
				cols = append(cols, "count")
			}
			if fields.Has("member_preview") {
				cols = append(cols, "preview")
			}
			if len(cols) > 1 {
				rows := sqlmock.NewRows(cols)
				if len(cols) == 3 {
					rows.AddRow(1, 1000, `[{"id":3,"username":"member","code":"USER0003"}]`)
				} else if fields.Has("member_count") {
					rows.AddRow(1, 0)
				} else {
					rows.AddRow(1, `[]`)
				}
				mock.ExpectQuery("SELECT g.id,.*FROM unnest").WithArgs(pq.Array([]int64{1})).WillReturnRows(rows)
			}
			result, err := m.List(t.Context(), ListUserGroupsInput{Fields: fields})
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(result.Items[0])
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]json.RawMessage
			if err := json.Unmarshal(data, &object); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"actions", "users", "member_count", "member_preview"} {
				value, ok := object[name]
				if ok != fields.Has(name) {
					t.Fatalf("%s: %s", name, data)
				}
				if ok && string(value) == "null" {
					t.Fatalf("selected empty value is null: %s", data)
				}
			}
		})
	}
	m, _ := newTestModule(t)
	for _, fields := range []report.Fields{
		{}, {"unknown"}, {"users", "users"}} {
		if _, err := m.List(t.Context(), ListUserGroupsInput{Fields: fields}); err == nil {
			t.Fatal("accepted invalid fields")
		}
		if _, err := m.Get(t.Context(), 1, fields...); err == nil {
			t.Fatal("accepted invalid fields")
		}
	}
}

func TestMemberPages(t *testing.T) {
	m, mock := newTestModule(t)
	p, s := int32(2), int32(20)
	mock.ExpectQuery("SELECT id FROM t_user_group").WithArgs(int64(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT.*u.username ILIKE.*u.code ILIKE").WithArgs(int64(1), "%name!_%").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1000))
	mock.ExpectQuery("SELECT u.id.*ORDER BY u.id LIMIT").WithArgs(int64(1), "%name!_%", s, int64(20)).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code"}).AddRow(21, "name_21", "USER0021"))
	result, err := m.ListMembers(t.Context(), 1, ListMembersInput{Page: &p, PageSize: &s, Q: " name_ "})
	if err != nil || result.Total != 1000 || result.Items[0].ID != 21 {
		t.Fatalf("%+v %v", result, err)
	}
	p = 0
	if result, err := m.ListMembers(t.Context(), 1, ListMembersInput{Page: &p}); err != nil || result.Total != 0 || len(result.Items) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	if _, err := m.ListMembers(t.Context(), 0, ListMembersInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id FROM t_user_group").WillReturnError(sql.ErrNoRows)
	if _, err := m.ListMembers(t.Context(), 99, ListMembersInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id FROM t_user_group").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	if result, err := m.ListMembers(t.Context(), 1, ListMembersInput{}); err != nil || result.Items == nil || result.Total != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	for _, step := range []string{"group", "count", "page", "scan", "rows"} {
		if step == "group" {
			mock.ExpectQuery("SELECT id FROM").WillReturnError(sqlmock.ErrCancelled)
		} else {
			mock.ExpectQuery("SELECT id FROM").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			if step == "count" {
				mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled)
			} else {
				mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				if step == "page" {
					mock.ExpectQuery("SELECT u.id").WillReturnError(sqlmock.ErrCancelled)
				} else {
					rows := sqlmock.NewRows([]string{"id", "username", "code"})
					if step == "scan" {
						rows.AddRow("bad", "name", "code")
					} else {
						rows.AddRow(1, "name", "code").RowError(0, sqlmock.ErrCancelled)
					}
					mock.ExpectQuery("SELECT u.id").WillReturnRows(rows)
				}
			}
		}
		if _, err := m.ListMembers(t.Context(), 1, ListMembersInput{}); err == nil {
			t.Fatal(step)
		}
	}
}

func TestMemberDelta(t *testing.T) {
	m, mock := newTestModule(t)
	add, remove := []int64{3, 3, 2}, []int64{4}
	for _, input := range []UpdateUserGroupInput{
		{UserIDs: &add, AddUserIDs: &add}, {AddUserIDs: &add, RemoveUserIDs: &add}, {AddUserIDs: &[]int64{-1}}, {RemoveUserIDs: &[]int64{0}},
	} {
		if _, err := m.Update(t.Context(), 1, input); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WithArgs(pq.Array([]int64{2, 3})).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec("UPDATE t_user_group SET updated_at").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM t_user_user_group_relation WHERE user_group_id = .* AND user_id = ANY").WithArgs(int64(1), pq.Array(remove)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO t_user_user_group_relation.*ON CONFLICT").WithArgs(int64(1), pq.Array([]int64{2, 3})).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery("SELECT .* FROM t_user_group WHERE id").WillReturnRows(groupRows(1, "Group", "group"))
	mock.ExpectQuery("SELECT g.id,.*COUNT.*FROM unnest").WillReturnRows(sqlmock.NewRows([]string{"id", "count"}).AddRow(1, 1001))
	mock.ExpectCommit()
	result, err := m.Update(t.Context(), 1, UpdateUserGroupInput{AddUserIDs: &add, RemoveUserIDs: &remove}, "member_count")
	if err != nil || *result.MemberCount != 1001 || result.Users != nil {
		t.Fatalf("%+v %v", result, err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()
	if _, err := m.Update(t.Context(), 1, UpdateUserGroupInput{AddUserIDs: &add}); !errors.Is(err, ErrUserNotFound) {
		t.Fatal(err)
	}
	for _, failure := range []error{sqlmock.ErrCancelled, &pq.Error{Code: "23503"}} {
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COUNT.*FROM t_user").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
		mock.ExpectExec("UPDATE t_user_group").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO t_user_user_group_relation").WillReturnError(failure)
		mock.ExpectRollback()
		if _, err := m.Update(t.Context(), 1, UpdateUserGroupInput{AddUserIDs: &add}); err == nil {
			t.Fatal("missing rollback")
		}
	}
	mock.ExpectExec("DELETE FROM t_user_user_group_relation").WillReturnError(sqlmock.ErrCancelled)
	if err := applyMemberDelta(t.Context(), m.store.DB, 1, nil, remove); err == nil {
		t.Fatal("delete failure")
	}
}

func TestMemberPreviewFailures(t *testing.T) {
	for _, kind := range []string{"query", "scan", "json", "rows"} {
		t.Run(kind, func(t *testing.T) {
			m, mock := newTestModule(t)
			fields := report.Fields{"member_preview"}
			if kind == "query" {
				mock.ExpectQuery("SELECT g.id").WillReturnError(sqlmock.ErrCancelled)
			} else {
				rows := sqlmock.NewRows([]string{"id", "preview"})
				switch kind {
				case "scan":
					rows.AddRow("bad", "[]")
				case "json":
					rows.AddRow(1, "bad")
				case "rows":
					rows.AddRow(1, "[]").RowError(0, sqlmock.ErrCancelled)
				}
				mock.ExpectQuery("SELECT g.id").WillReturnRows(rows)
			}
			if err := m.enrich(t.Context(), m.store.DB, []UserGroup{
				{ID: 1}}, fields); err == nil {
				t.Fatal("missing error")
			}
		})
	}
}

func TestFullMembersWithPreviewUseOneQuery(t *testing.T) {
	m, mock := newTestModule(t)
	mock.ExpectQuery("SELECT relation.user_group_id").WillReturnRows(sqlmock.NewRows([]string{"group", "id", "username", "code"}).AddRow(1, 3, "reader", "CODE0003"))
	items := []UserGroup{
		{ID: 1},
	}
	if err := m.enrich(t.Context(), m.store.DB, items, report.Fields{"users", "member_count", "member_preview"}); err != nil {
		t.Fatal(err)
	}
	if *items[0].MemberCount != 1 || len(*items[0].MemberPreview) != 1 {
		t.Fatalf("%+v", items)
	}
}
