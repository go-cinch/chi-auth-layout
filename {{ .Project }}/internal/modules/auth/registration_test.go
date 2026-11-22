package auth

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"testing"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
)

func TestRegister(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	ctx, state := auditctx.WithState(t.Context())
	for _, username := range []string{"", "   ", "\t\n"} {
		if err := m.Register(ctx, RegisterInput{Username: username, Password: "secret1"}); !errors.Is(err, ErrInvalidRegistration) {
			t.Fatalf("invalid username %q: %v", username, err)
		}
	}
	if err := m.Register(ctx, RegisterInput{Username: "new-user", Password: "   "}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("blank password: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnError(sqlmock.ErrCancelled)
	mock.ExpectRollback()
	if err := m.Register(ctx, RegisterInput{Username: "new-user", Password: "secret1"}); err == nil || errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("sequence error: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(36)))
	mock.ExpectExec("INSERT INTO t_user").WithArgs(
		int64(36), "new-user", sqlmock.AnyArg(), sqlmock.AnyArg(), "",
	).WillReturnError(&pq.Error{Code: "23505", Constraint: "uk_user_username"})
	mock.ExpectRollback()
	if err := m.Register(ctx, RegisterInput{Username: "new-user", Password: "secret1"}); !errors.Is(err, ErrUsernameExists) {
		t.Fatalf("duplicate username: %v", err)
	}
	if got := state.Snapshot().ResourceID; got != "" {
		t.Fatalf("rolled-back registration recorded target %q", got)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(38)))
	mock.ExpectExec("INSERT INTO t_user").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(sqlmock.ErrCancelled)
	if err := m.Register(ctx, RegisterInput{Username: "uncommitted-user", Password: "x"}); err == nil || state.Snapshot().ResourceID != "" {
		t.Fatal("failed commit must not publish a registration target")
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT nextval").WillReturnRows(sqlmock.NewRows([]string{"nextval"}).AddRow(int64(37)))
	mock.ExpectExec("INSERT INTO t_user").WithArgs(
		int64(37), "new-user", sqlmock.AnyArg(), sqlmock.AnyArg(), "",
	).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO t_user").WithArgs(
		int64(37), "new-user", sqlmock.AnyArg(), sqlmock.AnyArg(), "",
	).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := m.Register(ctx, RegisterInput{Username: "new-user", Password: "x"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if event := state.Snapshot(); event.Operation != "register" || event.ResourceID != "37" || event.UserID != 0 {
		t.Fatalf("registration audit: operation=%q target=%q actor=%d", event.Operation, event.ResourceID, event.UserID)
	}
}

func TestUsernameAvailability(t *testing.T) {
	m, _, mock := newAuthTestModule(t)
	ctx := context.Background()
	for _, username := range []string{"", "   ", "\t\n"} {
		if _, err := m.UsernameAvailability(ctx, username); !errors.Is(err, ErrInvalidRegistration) {
			t.Fatalf("invalid username %q: %v", username, err)
		}
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("new-user").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	value, err := m.UsernameAvailability(ctx, "new-user")
	if err != nil || !value.Available {
		t.Fatalf("available username: %#v %v", value, err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("existing-user").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	value, err = m.UsernameAvailability(ctx, "existing-user")
	if err != nil || value.Available {
		t.Fatalf("existing username: %#v %v", value, err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("broken-user").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.UsernameAvailability(ctx, "broken-user"); err == nil {
		t.Fatal("database failure was ignored")
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("用户 名称").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	value, err = m.UsernameAvailability(ctx, "  用户 名称  ")
	if err != nil || !value.Available {
		t.Fatalf("trimmed username: %#v %v", value, err)
	}
}
