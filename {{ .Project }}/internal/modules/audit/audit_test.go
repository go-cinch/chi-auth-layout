package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/config"
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
	return New(&db.Store{DB: database}, limits, config.DefaultAudit()), mock
}

// Fixtures spell out SQL column order independently of the production projection.
const metadataColumns = "id,occurred_at,kind,user_id,username,module,method,path,ip,user_agent,trace_id,error_code,status,duration_ms,operation,resource_type,resource_id,step_up_fingerprint,step_up_verified_at"

func eventRows() *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(metadataColumns, ","))
}

func row() *sqlmock.Rows {
	return eventRows().AddRow(1, int64(100000), "login", 1, "alice", "auth", "POST", "/pub/login", "127.0.0.1", "agent", "trace", "", 200, 12, "login", "session", "", "fingerprint", int64(90000))
}

func detailRow() *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(metadataColumns+",header,request_params,response_params", ",")).AddRow(1, int64(100000), "login", 1, "alice", "auth", "POST", "/pub/login", "127.0.0.1", "agent", "trace", "", 200, 12, "login", "session", "", "fingerprint", int64(90000), []byte(`{"x-test":"header"}`), []byte(`{"input":1}`), []byte(`{"output":2}`))
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
	mock.ExpectQuery("SELECT .* WHERE kind=\\$1 AND id=\\$2").WithArgs("login", int64(1)).WillReturnRows(detailRow())
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
	mock.ExpectQuery("SELECT .* LIMIT").WillReturnRows(row())
	items, err := m.Export(ctx, f)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	mock.ExpectQuery("SELECT .* LIMIT").WillReturnError(sqlmock.ErrCancelled)
	if _, err = m.Export(ctx, f); err == nil {
		t.Fatal("export failure")
	}
}

func TestReadProjectionCompatibility(t *testing.T) {
	metadataSQL := "id, occurred_at, kind, COALESCE(user_id,0), username, module, method, path, ip, user_agent, trace_id, error_code, status, duration_ms, operation, resource_type, resource_id, step_up_fingerprint, COALESCE(step_up_verified_at,0)"
	for _, operation := range []string{"list", "export", "get", "detail_default", "detail_all"} {
		t.Run(operation, func(t *testing.T) {
			m, mock := setup(t)
			detail := operation != "list" && operation != "export"
			query := "SELECT " + metadataSQL
			rows := row()
			if detail {
				query += ", header, request_params, response_params"
				rows = detailRow()
			}
			if operation == "list" {
				mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
			}
			// An exact SELECT list catches payload loading in metadata-only reads.
			mock.ExpectQuery(regexp.QuoteMeta(query + " FROM t_audit_event")).WillReturnRows(rows).RowsWillBeClosed()
			var value *Event
			var err error
			switch operation {
			case "list":
				var page *Page
				page, err = m.List(t.Context(), Filter{Kind: "login"})
				if err == nil && len(page.Items) == 1 {
					value = &page.Items[0]
				}
			case "export":
				var items []Event
				// Export has always used its fixed metadata shape, independent of f.
				items, err = m.Export(t.Context(), Filter{Kind: "login", Fields: []string{"username"}})
				if err == nil && len(items) == 1 {
					value = &items[0]
				}
			case "get":
				value, err = m.Get(t.Context(), "login", 1)
			case "detail_default":
				value, err = m.GetSelected(t.Context(), "login", 1, nil)
			case "detail_all":
				value, err = m.GetSelected(t.Context(), "login", 1, detailFields)
			}
			if err != nil || value == nil {
				t.Fatalf("read: %+v, %v", value, err)
			}
			if !detail && (string(value.Header) != "null" || string(value.RequestParams) != "null" || string(value.ResponseParams) != "null") {
				t.Fatal("metadata reads must preserve RawMessage null values")
			}
			want := map[string]any{}
			if err := json.Unmarshal([]byte(`{"id":1,"occurred_at":100000,"kind":"login","user_id":1,"username":"alice","module":"auth","method":"POST","path":"/pub/login","ip":"127.0.0.1","user_agent":"agent","trace_id":"trace","error_code":"","status":200,"duration_ms":12,"operation":"login","resource_type":"session","resource_id":"","step_up_fingerprint":"fingerprint","step_up_verified_at":90000,"header":null,"request_params":null,"response_params":null}`), &want); err != nil {
				t.Fatal(err)
			}
			if detail {
				want["header"] = map[string]any{"x-test": "header"}
				want["request_params"] = map[string]any{"input": float64(1)}
				want["response_params"] = map[string]any{"output": float64(2)}
			}
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]any{}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("response changed: %s", data)
			}
		})
	}
}

func TestFilters(t *testing.T) {
	for _, f := range []Filter{
		{Kind: "login", From: auditTime(253402300799999), To: auditTime(253402300800000)}, {Kind: "login", Timezone: "Local"}, {Kind: "bad"}, {Kind: "login", From: auditTime(-1)}, {Kind: "login", From: auditTime(2), To: auditTime(1)}, {Kind: "login", From: auditTime(1), To: auditTime(1000000000000)}, {Kind: "login", Result: "bad"}, {Kind: "login", Timezone: "missing/zone"}, {Kind: "login", Query: strings.Repeat("x", 257)}} {
		if _, err := normalize(f, config.DefaultAudit()); !errors.Is(err, ErrFilter) {
			t.Fatal(f, err)
		}
	}
	f, err := normalize(Filter{Kind: "operation", Result: "success"}, config.DefaultAudit())
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
	f, err := normalize(Filter{Kind: "login", Weekday: &day, Hour: &hour, Timezone: "Asia/Shanghai", Username: "alice", IP: "127.0.0.1", Path: "/pub/login", Sort: "failure"}, config.DefaultAudit())
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
	if _, err := normalize(f, config.DefaultAudit()); err == nil {
		t.Fatal("invalid sort")
	}
	f.Sort = ""
	f.Hour = nil
	if _, err := normalize(f, config.DefaultAudit()); err == nil {
		t.Fatal("incomplete cell")
	}
	f.Hour = &hour
	day = 7
	if _, err := normalize(f, config.DefaultAudit()); err == nil {
		t.Fatal("invalid weekday")
	}
	day = 0
	hour = 24
	if _, err := normalize(f, config.DefaultAudit()); err == nil {
		t.Fatal("invalid hour")
	}
}

func auditTime(value int64) *int64 { return &value }

func TestConfiguredAuditWindowAndExportLimit(t *testing.T) {
	m, mock := setup(t)
	m.settings.DefaultWindow = 2 * time.Hour
	m.settings.MaxWindow = 4 * time.Hour
	m.settings.ExportMaxRows = 2
	f, err := normalize(Filter{Kind: "login"}, m.settings)
	if err != nil || *f.To-*f.From != int64(2*time.Hour/time.Millisecond) {
		t.Fatal("default window ignored", err)
	}
	from := int64(1000)
	to := from + int64(4*time.Hour/time.Millisecond)
	f = Filter{Kind: "login", From: &from, To: &to}
	if _, err := normalize(f, m.settings); err != nil {
		t.Fatal("configured boundary rejected", err)
	}
	to++
	if _, err := m.List(t.Context(), f); !errors.Is(err, ErrFilter) {
		t.Fatal("list ignored maximum window", err)
	}
	if _, err := m.Options(t.Context(), f); !errors.Is(err, ErrFilter) {
		t.Fatal("options ignored maximum window", err)
	}
	if _, err := m.Export(t.Context(), f); !errors.Is(err, ErrFilter) {
		t.Fatal("export ignored maximum window", err)
	}
	to--
	for _, count := range []int{2, 3} {
		rows := eventRows()
		for i := 0; i < count; i++ {
			rows.AddRow(i+1, int64(100000), "login", 1, "alice", "auth", "POST", "/pub/login", "127.0.0.1", "agent", "trace", "", 200, 12, "login", "session", "", "", 0)
		}
		mock.ExpectQuery("SELECT .* LIMIT \\$4").WithArgs("login", from, to, 3).WillReturnRows(rows)
		items, err := m.Export(t.Context(), f)
		if count == 2 && (err != nil || len(items) != 2) {
			t.Fatal("export boundary rejected", err)
		}
		if count == 3 && !errors.Is(err, ErrExportLimit) {
			t.Fatal("extra row did not trigger export limit", err)
		}
	}
}

func TestOptionalTimeBoundsAndBlankKeyword(t *testing.T) {
	for _, input := range []Filter{ {Kind: "login", From: auditTime(123), Query: "   "}, {Kind: "login", To: auditTime(456), Query: "  alice  "}} {
		value, err := normalize(input, config.DefaultAudit())
		if err != nil {
			t.Fatal(err)
		}
		if (input.From == nil) != (value.From == nil) || (input.To == nil) != (value.To == nil) {
			t.Fatal("missing boundary synthesized", value)
		}
		clause, args := where(value)
		if input.From == nil && strings.Contains(clause, ">=") {
			t.Fatal(clause)
		}
		if input.To == nil && strings.Contains(clause, "occurred_at <") {
			t.Fatal(clause)
		}
		if value.Query == "" && strings.Contains(clause, "ILIKE") {
			t.Fatal(clause)
		}
		if value.Query != "" && (value.Query != "alice" || args[len(args)-1] != "%alice%" || !strings.Contains(clause, "ILIKE $3")) {
			t.Fatal(value.Query, clause, args)
		}
	}
}
