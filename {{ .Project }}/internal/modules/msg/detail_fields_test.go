package msg

import (
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/report"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestSentDetailFieldProjection(t *testing.T) {
	m, mock := testModule(t)
	check := func(fields report.Fields, want string) {
		t.Helper()
		value, err := m.GetSentSelected(t.Context(), 7, fields)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(value)
		if err != nil || string(raw) != want {
			t.Fatalf("response=%s error=%v want=%s", raw, err, want)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
	// Any recipient query or user join would be an unexpected SQL operation.
	mock.ExpectQuery(regexp.QuoteMeta("SELECT m.title, LEFT(m.content, 160), NULL::bigint FROM t_msg m WHERE m.id = $1")).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"title", "summary", "read_at"}).AddRow("", "", nil))
	check(report.Fields{"title", "summary", "read_at"}, `{"read_at":null,"summary":"","title":""}`)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT m.scope FROM t_msg m WHERE m.id = $1")).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"scope"}).AddRow("targeted"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT r.user_id FROM t_msg_recipient r WHERE r.msg_id = $1 ORDER BY r.user_id")).WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
	check(report.Fields{"recipient_ids"}, `{"recipient_ids":[2]}`)
	mock.ExpectQuery("SELECT m.scope FROM t_msg").WillReturnRows(sqlmock.NewRows([]string{"scope"}).AddRow("targeted"))
	mock.ExpectQuery("SELECT r.user_id, .* LEFT JOIN t_user").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code"}).AddRow(2, "reader", "ABCD1234"))
	check(report.Fields{"recipient_ids", "recipients"}, `{"recipient_ids":[2],"recipients":[{"id":2,"username":"reader","code":"ABCD1234"}]}`)
	for _, scope := range []string{"all", "targeted"} {
		mock.ExpectQuery("SELECT m.scope FROM t_msg").WillReturnRows(sqlmock.NewRows([]string{"scope"}).AddRow(scope))
		if scope == "targeted" {
			mock.ExpectQuery("SELECT r.user_id, .* LEFT JOIN t_user").WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code"}))
		}
		check(report.Fields{"recipient_ids", "recipients"}, `{"recipient_ids":[],"recipients":[]}`)
	}
	mock.ExpectQuery("SELECT .* FROM t_msg").WillReturnRows(messageRows("all"))
	value, err := m.GetSentSelected(t.Context(), 7, nil)
	if err != nil || value.Title == "" {
		t.Fatalf("legacy detail: %+v %v", value, err)
	}
}

func TestSentDetailSelectionFailures(t *testing.T) {
	m, mock := testModule(t)
	for _, fields := range []report.Fields{ {}, {"unknown"}, {"title", "title"}} {
		if _, err := m.GetSentSelected(t.Context(), 7, fields); err == nil {
			t.Fatal("invalid fields accepted")
		}
	}
	if _, err := m.GetSentSelected(t.Context(), 0, report.Fields{"title"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	for _, failure := range []error{sql.ErrNoRows, sqlmock.ErrCancelled} {
		mock.ExpectQuery("SELECT m.title FROM t_msg").WillReturnError(failure)
		_, err := m.GetSentSelected(t.Context(), 7, report.Fields{"title"})
		if err == nil || (failure == sql.ErrNoRows && !errors.Is(err, ErrNotFound)) {
			t.Fatal(err)
		}
	}
	for _, failure := range []string{"query", "scan", "rows"} {
		mock.ExpectQuery("SELECT m.scope FROM t_msg").WillReturnRows(sqlmock.NewRows([]string{"scope"}).AddRow("targeted"))
		query := mock.ExpectQuery("SELECT r.user_id, .* LEFT JOIN t_user")
		switch failure {
		case "query":
			query.WillReturnError(sqlmock.ErrCancelled)
		case "scan":
			query.WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code"}).AddRow("bad", "reader", "code"))
		case "rows":
			query.WillReturnRows(sqlmock.NewRows([]string{"id", "username", "code"}).AddRow(2, "reader", "code").RowError(0, sqlmock.ErrCancelled))
		}
		if _, err := m.GetSentSelected(t.Context(), 7, report.Fields{"recipients"}); err == nil {
			t.Fatal("association failure hidden")
		}
	}
}
