package action

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

func TestMutationPermissionMetadataIsTransactionScoped(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := &db.Store{DB: database}
	calls := 0
	want := true
	ctx := db.WithMutationRecorder(t.Context(), WithPermissionMutation, func(tx context.Context) error {
		calls++
		if IsPermissionMutation(tx) != want {
			t.Fatal("permission annotation lost or leaked")
		}
		return nil
	})
	mock.ExpectBegin()
	mock.ExpectCommit()
	if err = store.MutationTx(ctx, func(tx context.Context) error { MarkPermissionMutation(tx); return nil }); err != nil {
		t.Fatal(err)
	}
	want = false
	mock.ExpectBegin()
	mock.ExpectCommit()
	if err = store.MutationTx(ctx, func(tx context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectRollback()
	failure := errors.New("failed mutation")
	if err = store.MutationTx(ctx, func(tx context.Context) error { MarkPermissionMutation(tx); return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if calls != 2 || IsPermissionMutation(ctx) {
		t.Fatal("failed operation recorded or metadata leaked")
	}
	MarkPermissionMutation(t.Context())
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
