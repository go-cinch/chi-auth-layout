package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

func TestInfoSelectedSkipsPermissions(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	ctx := authenticatedContext(t, manager, m.sessions)
	mock.ExpectQuery("SELECT u.username FROM t_user AS u WHERE").WithArgs(int64(3), "EXP78RGH").WillReturnRows(sqlmock.NewRows([]string{"username"}).AddRow("readonly"))
	value, err := m.InfoSelected(ctx, "admin", report.Fields{"username"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(value)
	if string(data) != `{"username":"readonly"}` {
		t.Fatal(string(data))
	}
	mock.ExpectQuery("SELECT u.id, r.id, r.name, r.word FROM t_user AS u LEFT JOIN").WillReturnRows(sqlmock.NewRows([]string{"id", "role_id", "name", "word"}).AddRow(3, nil, nil, nil))
	value, err = m.InfoSelected(ctx, "admin", report.Fields{"role"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(value)
	if string(data) != `{"role":null}` {
		t.Fatal(string(data))
	}
	mock.ExpectQuery("SELECT u.id FROM t_user AS u WHERE").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	value, err = m.InfoSelected(ctx, "admin", report.Fields{"scope"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(value)
	if string(data) != `{"scope":"admin"}` {
		t.Fatal(string(data))
	}
	for _, scope := range []string{"", "Invalid Scope"} {
		if _, err := m.InfoSelected(ctx, scope, report.Fields{"id"}); err == nil {
			t.Fatal(scope)
		}
	}
	if _, err := m.InfoSelected(t.Context(), "admin", report.Fields{"id"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	if _, err := m.InfoSelected(ctx, "admin", report.Fields{"bad"}); err == nil {
		t.Fatal("bad selector")
	}
	mock.ExpectQuery("SELECT u.id FROM t_user AS u").WillReturnError(sql.ErrNoRows)
	if _, err := m.InfoSelected(ctx, "admin", report.Fields{"id"}); !errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT u.id FROM t_user AS u").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.InfoSelected(ctx, "admin", report.Fields{"id"}); err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatal(err)
	}
}
