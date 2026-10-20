package audit

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

func TestDetailPayloadSelection(t *testing.T) {
	m, mock := setup(t)
	mock.ExpectQuery("SELECT id, header FROM t_audit_event WHERE kind").WithArgs("login", int64(7)).WillReturnRows(sqlmock.NewRows([]string{"id", "header"}).AddRow(7, []byte(`{"user_agent":"test"}`)))
	value, err := m.GetSelected(t.Context(), "login", 7, report.Fields{"header", "id"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(value)
	if string(data) != `{"header":{"user_agent":"test"},"id":7}` {
		t.Fatal(string(data))
	}
	for _, fields := range []report.Fields{ {}, {"bad"}, {"id", "id"}} {
		if _, err := m.GetSelected(t.Context(), "login", 7, fields); err == nil {
			t.Fatal(fields)
		}
	}
	if _, err := m.GetSelected(t.Context(), "bad", 7, report.Fields{"id"}); err == nil {
		t.Fatal("bad kind")
	}
	if _, err := m.GetSelected(t.Context(), "login", 0, report.Fields{"id"}); err == nil {
		t.Fatal("bad id")
	}
	mock.ExpectQuery("SELECT id FROM t_audit_event").WillReturnError(sql.ErrNoRows)
	if _, err := m.GetSelected(t.Context(), "login", 7, report.Fields{"id"}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id FROM t_audit_event").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.GetSelected(t.Context(), "login", 7, report.Fields{"id"}); err == nil {
		t.Fatal("lost database failure")
	}
}
