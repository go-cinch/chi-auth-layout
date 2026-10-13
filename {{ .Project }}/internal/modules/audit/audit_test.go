package audit

import (
	"context"
	"database/sql"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"strings"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

func setup(t *testing.T) (*Module, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		database.Close()
	})
	limits, _ := pagination.New(100, 100)
	return New(&db.Store{DB: database}, limits), mock
}

func eventRows() *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(strings.ReplaceAll(columns, "COALESCE(user_id,0)", "user_id"), ","))
}

func row() *sqlmock.Rows {
	return eventRows().AddRow(1, int64(100000), "login", 1, "alice", "auth", "POST", "/auth/pub/login", "127.0.0.1", "agent", "trace", "", 200, 12, "login", "session", "", []byte("null"), []byte("null"))
}

func TestRecordAndRead(t *testing.T) {
	m, mock := setup(t)
	ctx := context.Background()
	if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "invalid"}); !errors.Is(err, ErrFilter) {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO t_audit_event").WillReturnResult(sqlmock.NewResult(1, 1))
	if err := m.RecordHTTP(ctx, auditctx.Event{Kind: "login", Username: strings.Repeat("中", 300) + "\n", Status: 200, OccurredAt: 100000}); err != nil {
		t.Fatal(err)
	}
	if clean(" \nabc\t ", 2) != "ab" {
		t.Fatal("clean")
	}
	mock.ExpectQuery("SELECT .* WHERE kind=\\$1 AND id=\\$2").WithArgs("login", int64(1)).WillReturnRows(row())
	e, err := m.Get(ctx, "login", 1)
	if err != nil || e.Username != "alice" || e.OccurredAt != 100000 {
		t.Fatal(e, err)
	}
	mock.ExpectQuery("SELECT .* WHERE kind=").WillReturnError(sql.ErrNoRows)
	if _, err := m.Get(ctx, "operation", 2); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* WHERE kind=").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Get(ctx, "login", 2); err == nil {
		t.Fatal("missing failure")
	}
	if _, err := m.Get(ctx, "login", 0); !errors.Is(err, ErrFilter) {
		t.Fatal(err)
	}
	f := Filter{Kind: "login", Query: "a%_", Module: "auth", Result: "failure"}
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* ORDER BY occurred_at").WillReturnRows(row())
	p, err := m.List(ctx, f)
	if err != nil || p.Total != 1 || len(p.Items) != 1 {
		t.Fatal(p, err)
	}
	zero := int32(0)
	f.Page = &zero
	p, err = m.List(ctx, f)
	if err != nil || p.Total != 0 || len(p.Items) != 0 {
		t.Fatal(p, err)
	}
	f.Page = nil
	mock.ExpectQuery("SELECT COUNT").WillReturnError(sqlmock.ErrCancelled)
	if _, err = m.List(ctx, f); err == nil {
		t.Fatal("count failure")
	}
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* ORDER BY occurred_at").WillReturnError(sqlmock.ErrCancelled)
	if _, err = m.List(ctx, f); err == nil {
		t.Fatal("list failure")
	}
	mock.ExpectQuery("SELECT DISTINCT").WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("alice"))
	v, err := m.Options(ctx, f)
	if err != nil || len(v) != 1 {
		t.Fatal(v, err)
	}
	mock.ExpectQuery("SELECT DISTINCT").WillReturnError(sqlmock.ErrCancelled)
	if _, err = m.Options(ctx, f); err == nil {
		t.Fatal("options failure")
	}
	mock.ExpectQuery("SELECT .* LIMIT 10001").WillReturnRows(row())
	items, err := m.Export(ctx, f)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	mock.ExpectQuery("SELECT .* LIMIT 10001").WillReturnError(sqlmock.ErrCancelled)
	if _, err = m.Export(ctx, f); err == nil {
		t.Fatal("export failure")
	}
}

func TestFilters(t *testing.T) {
	for _, f := range []Filter{
		{Kind: "login", From: 253402300799999, To: 253402300800000}, {Kind: "login", Timezone: "Local"}, {Kind: "bad"}, {Kind: "login", From: -1}, {Kind: "login", From: 2, To: 1}, {Kind: "login", From: 1, To: 1000000000000}, {Kind: "login", Result: "bad"}, {Kind: "login", Timezone: "missing/zone"}, {Kind: "login", Query: strings.Repeat("x", 257)}} {
		if _, err := normalize(f); !errors.Is(err, ErrFilter) {
			t.Fatal(f, err)
		}
	}
	f, err := normalize(Filter{Kind: "operation", Result: "success"})
	if err != nil || f.Timezone != "UTC" {
		t.Fatal(f, err)
	}
	clause, _ := where(f)
	if !strings.Contains(clause, "status < 400") {
		t.Fatal(clause)
	}
	m, _ := setup(t)
	ctx := t.Context()
	bad := Filter{Kind: "bad"}
	if _, err := m.List(ctx, bad); err == nil {
		t.Fatal("invalid list")
	}
	if _, err := m.Options(ctx, bad); err == nil {
		t.Fatal("invalid options")
	}
	if _, err := m.Export(ctx, bad); err == nil {
		t.Fatal("invalid export")
	}
	if _, err := m.Summary(ctx, bad); err == nil {
		t.Fatal("invalid summary")
	}
}

func TestInvestigationFilters(t *testing.T) {
	day, hour := int32(0), int32(8)
	f, err := normalize(Filter{Kind: "login", Weekday: &day, Hour: &hour, Timezone: "Asia/Shanghai", Username: "alice", IP: "127.0.0.1", Path: "/auth/pub/login", Sort: "failure"})
	if err != nil {
		t.Fatal(err)
	}
	clause, args := where(f)
	for _, part := range []string{"username=$4", "ip=$5", "path=$6", "EXTRACT(ISODOW", "EXTRACT(HOUR"} {
		if !strings.Contains(clause, part) {
			t.Fatal(clause)
		}
	}
	if len(args) != 9 || args[6] != "Asia/Shanghai" {
		t.Fatal(args)
	}
	if !strings.Contains(order(f), "status < 200") {
		t.Fatal(order(f))
	}
	f.Sort = "duration"
	if !strings.Contains(order(f), "duration_ms DESC") {
		t.Fatal(order(f))
	}
	f.Sort = "bad"
	if _, err := normalize(f); err == nil {
		t.Fatal("invalid sort")
	}
	f.Sort = ""
	f.Hour = nil
	if _, err := normalize(f); err == nil {
		t.Fatal("incomplete cell")
	}
	f.Hour = &hour
	day = 7
	if _, err := normalize(f); err == nil {
		t.Fatal("invalid weekday")
	}
	day = 0
	hour = 24
	if _, err := normalize(f); err == nil {
		t.Fatal("invalid hour")
	}
}
