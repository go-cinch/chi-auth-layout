package msg

import (
	"database/sql"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

func testModule(t *testing.T) (*Module, sqlmock.Sqlmock) {
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
	limits, _ := pagination.New(100, 100)
	return New(&db.Store{DB: database}, limits), mock
}

func messageRows(scope string) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "title", "content", "type", "scope", "sender_id", "published_at", "expired_at", "read_at"}).AddRow(1, now, now, "title", "body", "notice", scope, 1, now, now.Add(time.Hour), now)
}

func TestCreate(t *testing.T) {
	for _, scope := range []string{"all", "targeted"} {
		t.Run(scope, func(t *testing.T) {
			m, mock := testModule(t)
			input := CreateInput{Title: " title ", Content: " body ", Type: "notice", Scope: scope}
			if scope == "targeted" {
				input.RecipientIDs = []int64{2, 2}
			}
			expiry := time.Now().Add(time.Hour).UnixMilli()
			input.ExpiredAt = &expiry
			mock.ExpectBegin()
			mock.ExpectQuery("INSERT INTO t_msg").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			if scope == "targeted" {
				mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnResult(sqlmock.NewResult(1, 1))
			}
			mock.ExpectQuery("SELECT .* FROM t_msg m WHERE").WillReturnRows(messageRows(scope))
			if scope == "targeted" {
				mock.ExpectQuery("SELECT user_id").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
			}
			mock.ExpectCommit()
			value, err := m.Create(t.Context(), 1, input)
			if err != nil || value.Title != "title" || value.ExpiredAt == nil || value.ReadAt == nil {
				t.Fatalf("create: %#v %v", value, err)
			}
		})
	}
	m, mock := testModule(t)
	for _, input := range []CreateInput{
		{}, {Title: "x", Content: "x", Scope: "other", Type: "system"}, {Title: "x", Content: "x", Scope: "targeted", Type: "system"}, {Title: "x", Content: "x", Scope: "all", Type: "system", RecipientIDs: []int64{1}}, {Title: "x", Content: "x", Scope: "targeted", Type: "system", RecipientIDs: []int64{-1}},
	} {
		if _, err := m.Create(t.Context(), 1, input); err == nil {
			t.Fatal("invalid create accepted")
		}
	}
	expired := int64(0)
	if _, err := m.Create(t.Context(), 1, CreateInput{Title: "x", Content: "x", Scope: "all", Type: "system", ExpiredAt: &expired}); err == nil {
		t.Fatal("expired create accepted")
	}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_msg").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if _, err := m.Create(t.Context(), 1, CreateInput{Title: "x", Content: "x", Scope: "targeted", Type: "system", RecipientIDs: []int64{999}}); !errors.Is(err, ErrRecipient) {
		t.Fatal(err)
	}
}

func TestInboxAndManagement(t *testing.T) {
	m, mock := testModule(t)
	mock.ExpectQuery("SELECT .* FROM").WithArgs(int64(2), sqlmock.AnyArg(), int64(1)).WillReturnRows(messageRows("all"))
	if _, err := m.Get(t.Context(), 2, 1); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM").WillReturnError(sql.ErrNoRows)
	if _, err := m.Get(t.Context(), 2, 999); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Get(t.Context(), 2, 1); err == nil {
		t.Fatal("query failure hidden")
	}
	for _, sent := range []bool{false, true} {
		mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery("SELECT .* FROM").WillReturnRows(messageRows("all"))
		read := true
		input := ListInput{Type: "notice", Scope: "all"}
		if !sent {
			input.Read = &read
		}
		var result *ListResult
		var err error
		if sent {
			result, err = m.ListSent(t.Context(), input)
		} else {
			result, err = m.List(t.Context(), 2, input)
		}
		if err != nil || result.Total != 1 || len(result.Items) != 1 {
			t.Fatalf("list: %#v %v", result, err)
		}
	}
	zero := int32(0)
	if value, err := m.List(t.Context(), 2, ListInput{Page: &zero}); err != nil || value.Total != 0 {
		t.Fatal(value, err)
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	read := false
	if _, err := m.List(t.Context(), 2, ListInput{Read: &read}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	if n, err := m.UnreadCount(t.Context(), 2); err != nil || n != 3 {
		t.Fatal(n, err)
	}
	for _, operation := range []func() error{func() error { return m.Read(t.Context(), 2, 1) }, func() error { return m.Delete(t.Context(), 2, 1) }, func() error { return m.ReadAll(t.Context(), 2) }} {
		mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnResult(sqlmock.NewResult(0, 1))
		if err := operation(); err != nil {
			t.Fatal(err)
		}
	}
	mock.ExpectExec("DELETE FROM t_msg").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := m.DeleteSent(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, username").WillReturnRows(sqlmock.NewRows([]string{"id", "username"}).AddRow(2, "guest"))
	if values, err := m.RecipientOptions(t.Context(), "guest"); err != nil || len(values) != 1 {
		t.Fatal(values, err)
	}
}

func TestFailures(t *testing.T) {
	m, mock := testModule(t)
	if _, err := m.Get(t.Context(), 0, 1); err == nil {
		t.Fatal("invalid get")
	}
	if _, err := m.GetSent(t.Context(), 0); err == nil {
		t.Fatal("invalid sent get")
	}
	if _, err := m.List(t.Context(), 0, ListInput{}); err == nil {
		t.Fatal("invalid user")
	}
	if _, err := m.ListSent(t.Context(), ListInput{Type: "invalid"}); err == nil {
		t.Fatal("invalid type")
	}
	if _, err := m.UnreadCount(t.Context(), 0); err == nil {
		t.Fatal("invalid count")
	}
	for _, err := range []error{m.Read(t.Context(), 0, 1), m.Delete(t.Context(), 1, 0), m.ReadAll(t.Context(), 0), m.DeleteSent(t.Context(), 0)} {
		if err == nil {
			t.Fatal("invalid operation")
		}
	}
	if !errors.Is(affected(sqlmock.NewResult(0, 0), nil), ErrNotFound) {
		t.Fatal("missing result")
	}
	if affected(nil, sqlmock.ErrCancelled) == nil || affected(sqlmock.NewErrorResult(sqlmock.ErrCancelled), nil) == nil {
		t.Fatal("result error hidden")
	}
	mock.ExpectQuery("SELECT .* FROM").WillReturnError(sql.ErrNoRows)
	if _, err := m.GetSent(t.Context(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.GetSent(t.Context(), 1); err == nil {
		t.Fatal("error hidden")
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.ListSent(t.Context(), ListInput{}); err == nil {
		t.Fatal("error hidden")
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.ListSent(t.Context(), ListInput{}); err == nil {
		t.Fatal("error hidden")
	}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_msg").WillReturnError(sqlmock.ErrCancelled)
	mock.ExpectRollback()
	if _, err := m.Create(t.Context(), 1, CreateInput{Title: "x", Content: "x", Scope: "all", Type: "system"}); err == nil {
		t.Fatal("insert failure hidden")
	}
	mock.ExpectQuery("SELECT id, username").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.RecipientOptions(t.Context(), ""); err == nil {
		t.Fatal("options failure hidden")
	}
}
