package audit

import (
	"github.com/DATA-DOG/go-sqlmock"
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
