package audit

import (
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestConfiguredAuditChartWindow(t *testing.T) {
	m, mock := setup(t)
	m.settings.MaxWindow = 4 * time.Hour
	from, to := int64(1000), int64(1000)+int64(4*time.Hour/time.Millisecond)+1
	if _, err := m.Summary(t.Context(), Filter{Kind: "login", From: &from, To: &to}, "trend"); !errors.Is(err, ErrFilter) {
		t.Fatal("paired chart range ignored maximum", err)
	}
	mock.ExpectQuery("SELECT MIN.*MAX").WillReturnRows(sqlmock.NewRows([]string{"first", "last"}).AddRow(from, to))
	if _, err := m.Summary(t.Context(), Filter{Kind: "login", From: &from}, "trend"); !errors.Is(err, ErrFilter) {
		t.Fatal("derived chart range ignored maximum", err)
	}
}

func TestSummaryChartsShareOptionalBounds(t *testing.T) {
	for _, test := range []struct {
		name        string
		from, to    *int64
		first, last any
		start, end  int64
	}{
		{"from", auditTime(3600000), nil, int64(3600000), int64(7199999), 3600000, 7200000},
		{"to", nil, auditTime(7200000), int64(3600000), int64(7199999), 3600000, 7200000},
		{"empty_from", auditTime(3600000), nil, nil, nil, 3600000, 3600001},
		{"empty_to", nil, auditTime(7200000), nil, nil, 7199999, 7200000},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, mock := setup(t)
			clause := " WHERE kind=$1 AND occurred_at >= $2"
			bound := test.from
			if bound == nil {
				clause = " WHERE kind=$1 AND occurred_at < $2"
				bound = test.to
			}
			// Both charts share one extent lookup and retain the original SQL bounds.
			mock.ExpectQuery(regexp.QuoteMeta("SELECT MIN(occurred_at), MAX(occurred_at) FROM t_audit_event"+clause)).WithArgs("login", *bound).
				WillReturnRows(sqlmock.NewRows([]string{"first", "last"}).AddRow(test.first, test.last)).RowsWillBeClosed()
			mock.ExpectQuery("SELECT .*occurred_at /.*"+regexp.QuoteMeta(clause+" GROUP BY 1 ORDER BY 1")).WithArgs("login", *bound, int64(3600)).
				WillReturnRows(sqlmock.NewRows([]string{"at", "success", "failure"})).RowsWillBeClosed()
			mock.ExpectQuery("SELECT width_bucket.*"+regexp.QuoteMeta(clause+" GROUP BY 1")).WithArgs("login", *bound, sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"index", "count", "failed"})).RowsWillBeClosed()
			value, err := m.Summary(t.Context(), Filter{Kind: "login", From: test.from, To: test.to}, "trend", "heatmap")
			if err != nil {
				t.Fatal(err)
			}
			if value.From != test.from || value.To != test.to {
				t.Fatal("chart allocation changed public bounds")
			}
			if len(value.Trend) != 1 || value.Trend[0].At != test.start/3600000*3600000 || value.BucketMS != 3600000 {
				t.Fatalf("trend: %+v", value)
			}
			if len(value.Heatmap) != 1 || value.Heatmap[0].StartedAt != test.start || value.Heatmap[0].EndedAt != test.end {
				t.Fatalf("heatmap: %+v", value.Heatmap)
			}
		})
	}
}

func TestSummaryChartReadErrors(t *testing.T) {
	for _, field := range []string{"trend", "heatmap"} {
		for _, failure := range []string{"scan", "iteration"} {
			t.Run(field+"/"+failure, func(t *testing.T) {
				m, mock := setup(t)
				rows := sqlmock.NewRows([]string{"bucket", "success", "failure"})
				if failure == "scan" {
					rows.AddRow("invalid integer", 1, 0)
				} else {
					rows.AddRow(0, 1, 0).RowError(0, sqlmock.ErrCancelled)
				}
				mock.ExpectQuery("SELECT").WillReturnRows(rows).RowsWillBeClosed()
				_, err := m.Summary(t.Context(), Filter{Kind: "login", From: auditTime(1), To: auditTime(3600000)}, field)
				if err == nil {
					t.Fatal("lost chart read error")
				}
				if failure == "iteration" && !errors.Is(err, sqlmock.ErrCancelled) {
					t.Fatal(err)
				}
			})
		}
	}
}

func expectSummary(mock sqlmock.Sqlmock, fail int) {
	queries := []string{"SELECT COUNT", "SELECT .*occurred_at /", "SELECT width_bucket", "SELECT module", "SELECT username", "SELECT username", "SELECT ip"}
	rows := []*sqlmock.Rows{
		sqlmock.NewRows([]string{"total", "success", "users", "ips", "average", "p95"}).AddRow(10, 7, 2, 3, 20.5, 40.0),
		sqlmock.NewRows([]string{"at", "success", "failure"}).AddRow(0, 7, 3),
		sqlmock.NewRows([]string{"index", "count", "failed"}).AddRow(1, 10, 3),
		sqlmock.NewRows([]string{"name", "total", "failed"}).AddRow("auth", 10, 3),
		sqlmock.NewRows([]string{"name", "total", "failed"}).AddRow("alice", 7, 5),
		sqlmock.NewRows([]string{"name", "total", "failed"}).AddRow("alice", 7, 5),
		sqlmock.NewRows([]string{"name", "total", "failed"}).AddRow("127.0.0.1", 10, 3),
	}
	for i, q := range queries {
		e := mock.ExpectQuery(q)
		if i == fail {
			e.WillReturnError(sqlmock.ErrCancelled)
			break
		}
		e.WillReturnRows(rows[i])
	}
}

func TestSummary(t *testing.T) {
	for _, span := range []int64{3600000, 7 * 86400000} {
		m, mock := setup(t)
		expectSummary(mock, -1)
		s, err := m.Summary(t.Context(), Filter{Kind: "login", From: auditTime(1), To: auditTime(span), Timezone: "Asia/Shanghai"})
		if err != nil || s.Total != 10 || s.Failed != 3 || s.SuccessRate != 70 || len(s.Heatmap) == 0 || s.Heatmap[0].StartedAt != 1 || s.Heatmap[len(s.Heatmap)-1].EndedAt != span || s.Heatmap[0].Count != 10 || len(s.Modules) != 1 || len(s.RepeatedFailures) != 1 {
			t.Fatal(s, err)
		}
	}
	for fail := 0; fail < 7; fail++ {
		m, mock := setup(t)
		expectSummary(mock, fail)
		if _, err := m.Summary(t.Context(), Filter{Kind: "operation"}); err == nil {
			t.Fatalf("query %d failure ignored", fail)
		}
	}
}

func TestBreakdownRejectsIdentifiers(t *testing.T) {
	m, _ := setup(t)
	if _, err := m.breakdown(t.Context(), "", nil, "injected", 0); err == nil {
		t.Fatal("accepted unsafe identifier")
	}
}

func TestSummarySelectionSkipsUnrequestedQueries(t *testing.T) {
	for _, fields := range [][]string{
		{"metrics", "trend", "modules"}, {"metrics"}, {"trend"}, {"heatmap"}, {"accounts"}, {"sources"}, {"repeated_failures"}, {"modules"}} {
		t.Run(strings.Join(fields, ","), func(t *testing.T) {
			m, mock := setup(t)
			has := func(field string) bool { return slices.Contains(fields, field) }
			if has("metrics") {
				mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"total", "success", "users", "ips", "avg", "p95"}).AddRow(0, 0, 0, 0, 0, 0))
			}
			if has("trend") {
				mock.ExpectQuery("SELECT .*occurred_at /").WillReturnRows(sqlmock.NewRows([]string{"at", "success", "failure"}))
			}
			if has("heatmap") {
				mock.ExpectQuery("SELECT width_bucket").WillReturnRows(sqlmock.NewRows([]string{"index", "count", "failed"}))
			}
			for _, item := range []struct{ field, column string }{
				{"modules", "module"}, {"repeated_failures", "username"}, {"accounts", "username"}, {"sources", "ip"}} {
				if has(item.field) {
					mock.ExpectQuery("SELECT " + item.column).WillReturnRows(sqlmock.NewRows([]string{"name", "total", "failed"}))
				}
			}
			value, err := m.Summary(t.Context(), Filter{Kind: "operation", From: auditTime(1), To: auditTime(3600000)}, fields...)
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			body := map[string]json.RawMessage{}
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"total", "success", "failed", "unique_users", "unique_ips", "average_ms", "p95_ms", "success_rate", "trend", "bucket_ms", "heatmap", "heatmap_granularity", "accounts", "sources", "modules", "repeated_failures"} {
				field := name
				switch name {
				case "total", "success", "failed", "unique_users", "unique_ips", "average_ms", "p95_ms", "success_rate":
					field = "metrics"
				case "bucket_ms":
					field = "trend"
				case "heatmap_granularity":
					field = "heatmap"
				}
				if _, exists := body[name]; exists != has(field) {
					t.Fatalf("%s: %s", name, data)
				}
			}
			if has("metrics") && string(body["total"]) != "0" {
				t.Fatal(string(data))
			}
			if has("modules") && string(body["modules"]) != "[]" {
				t.Fatal(string(data))
			}
		})
	}
	m, _ := setup(t)
	if _, err := m.Summary(t.Context(), Filter{Kind: "login"}, "unknown"); err == nil {
		t.Fatal("accepted unknown field")
	}
}
