package whitelist

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
)

var whitelistTestTime = time.Date(2026, 9, 18, 3, 0, 0, 0, time.UTC)

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
	return New(&db.Store{DB: database}, limits), mock
}

func whitelistRows(id int64, category int16, resource string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "created_at", "updated_at", "category", "resource"}).AddRow(id, whitelistTestTime, whitelistTestTime, category, resource)
}

func expectWhitelistGet(mock sqlmock.Sqlmock, id int64, category int16, resource string) {
	mock.ExpectQuery("SELECT .* FROM t_whitelist WHERE id").WithArgs(id).WillReturnRows(whitelistRows(id, category, resource))
}

func TestWhitelistCRUD(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	if _, err := m.Get(ctx, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM t_whitelist WHERE id").WithArgs(int64(99)).WillReturnError(sql.ErrNoRows)
	if _, err := m.Get(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	expectWhitelistGet(mock, 1, 0, "GET|/healthz")
	value, err := m.Get(ctx, 1)
	if err != nil || value.Resource != "GET|/healthz" {
		t.Fatalf("get: %#v %v", value, err)
	}

	if _, err := m.Create(ctx, CreateWhitelistInput{Category: 2}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_whitelist").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), int16(0), "GET|/readyz").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	expectWhitelistGet(mock, 3, 0, "GET|/readyz")
	mock.ExpectCommit()
	value, err = m.Create(ctx, CreateWhitelistInput{Category: 0, Resource: " GET|/readyz "})
	if err != nil || value.ID != 3 {
		t.Fatalf("create: %#v %v", value, err)
	}

	resource := "GET|/livez"
	if _, err := m.Update(ctx, 1, UpdateWhitelistInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_whitelist SET resource").WithArgs(resource, sqlmock.AnyArg(), int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	expectWhitelistGet(mock, 1, 0, resource)
	mock.ExpectCommit()
	value, err = m.Update(ctx, 1, UpdateWhitelistInput{Resource: &resource})
	if err != nil || value.Resource != resource {
		t.Fatalf("update: %#v %v", value, err)
	}

	if err := m.Delete(ctx, -1); !errors.Is(err, ErrIDs) {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM t_whitelist").WithArgs(pq.Array([]int64{1, 2})).WillReturnResult(sqlmock.NewResult(0, 2))
	if err := m.Delete(ctx, 1, 2, 1); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("DELETE FROM t_whitelist").WithArgs(pq.Array([]int64{99})).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := m.Delete(ctx, 99); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestWhitelistListAndHelpers(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	category := int16(0)
	mock.ExpectQuery("SELECT COUNT.*FROM t_whitelist WHERE").WithArgs(category, "%health%").WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM t_whitelist WHERE").WithArgs(category, "%health%", int32(1), int64(0)).WillReturnRows(whitelistRows(1, 0, "GET|/healthz"))
	result, err := m.List(ctx, ListWhitelistsInput{Category: &category, Resource: "health"})
	if err != nil || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("list: %#v %v", result, err)
	}
	resources := []string{"%health%", "%metrics%"}
	mock.ExpectQuery("SELECT COUNT.*FROM t_whitelist WHERE.*unnest").WithArgs(pq.Array(resources)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	result, err = m.List(ctx, ListWhitelistsInput{Resource: "health,metrics"})
	if err != nil || result.Total != 0 {
		t.Fatalf("list by resources: %#v %v", result, err)
	}
	invalid := int32(0)
	result, err = m.List(ctx, ListWhitelistsInput{Page: &invalid})
	if err != nil || len(result.Items) != 0 {
		t.Fatalf("range: %#v %v", result, err)
	}
	if !valid(1, "x") || valid(2, "x") {
		t.Fatal("validation")
	}
	if !errors.Is(mapWriteError(&pq.Error{Code: "23505"}), ErrConflict) {
		t.Fatal("unique not mapped")
	}
}

func TestWhitelistFailures(t *testing.T) {
	m, mock := newTestModule(t)
	ctx := context.Background()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO t_whitelist").WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectRollback()
	if _, err := m.Create(ctx, CreateWhitelistInput{Category: 0, Resource: "GET|/healthz"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	resource := "GET|/missing"
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE t_whitelist").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if _, err := m.Update(ctx, 99, UpdateWhitelistInput{Resource: &resource}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT COUNT.*FROM t_whitelist").WillReturnError(errors.New("database"))
	if _, err := m.List(ctx, ListWhitelistsInput{}); err == nil {
		t.Fatal("expected list error")
	}
}
