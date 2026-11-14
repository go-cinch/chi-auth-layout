package auth

import (
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestDisabledEmailDoesNotRequireBinding(t *testing.T) {
	m, manager, mock := newAuthTestModule(t)
	sender := attachTestEmail(t, m)
	sender.disabled = true
	m.emailCodes.config.RequireBinding = true
	ctx := authenticatedContext(t, manager, m.sessions)
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"email", "email_verified_at"}).AddRow("", nil))
	var info UserInfo
	if err := m.populateEmailInfo(ctx, &info); err != nil || info.EmailBindingRequired {
		t.Fatalf("unexpected binding policy: required=%t, err=%v", info.EmailBindingRequired, err)
	}
	if err := m.checkEmailTarget(ctx, "GET", "/user"); err != nil {
		t.Fatal(err)
	}
}
