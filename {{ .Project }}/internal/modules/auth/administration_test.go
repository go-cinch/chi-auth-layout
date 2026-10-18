package auth

import (
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func TestPermissionAdministrationCapabilities(t *testing.T) {
	for _, test := range []struct {
		name                         string
		all, grant, requireAll, want bool
	}{
		{"ordinary cannot assign", false, false, false, false},
		{"all can assign", true, false, false, true},
		{"grant can assign", false, true, false, true},
		{"grant cannot acquire all", false, true, true, false},
		{"all remains scoped root", true, false, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, manager, mock := newAuthTestModule(t)
			ctx := authenticatedContext(t, manager, m.sessions)
			mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("WITH assigned AS").WithArgs(int64(3), "admin").WillReturnRows(sqlmock.NewRows([]string{"all", "grant"}).AddRow(test.all, test.grant))
			var err error
			if test.requireAll {
				err = m.RequireAllPermissions(ctx)
			} else {
				err = m.RequirePermissionGrant(ctx)
			}
			if (err == nil) != test.want {
				t.Fatalf("err=%v", err)
			}
		})
	}
	t.Run("missing identity", func(t *testing.T) {
		m, _, _ := newAuthTestModule(t)
		if !errors.Is(m.RequireAllPermissions(t.Context()), authn.ErrPermissionAdministrationRequired) {
			t.Fatal("missing identity accepted")
		}
	})
	t.Run("lock failure", func(t *testing.T) {
		m, manager, mock := newAuthTestModule(t)
		ctx := authenticatedContext(t, manager, m.sessions)
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnError(sqlmock.ErrCancelled)
		if err := m.RequirePermissionGrant(ctx); err == nil {
			t.Fatal("ignored lock failure")
		}
	})
	t.Run("query failure", func(t *testing.T) {
		m, manager, mock := newAuthTestModule(t)
		ctx := authenticatedContext(t, manager, m.sessions)
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("WITH assigned AS").WillReturnError(sqlmock.ErrCancelled)
		if err := m.RequireAllPermissions(ctx); err == nil {
			t.Fatal("ignored query failure")
		}
	})
}
