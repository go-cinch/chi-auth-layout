package audit

import (
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"slices"
	"strings"
	"testing"
)

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
		s, err := m.Summary(t.Context(), Filter{Kind: "login", From: 1, To: span, Timezone: "Asia/Shanghai"})
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
			value, err := m.Summary(t.Context(), Filter{Kind: "operation", From: 1, To: 3600000}, fields...)
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
