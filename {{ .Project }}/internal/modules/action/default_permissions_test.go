package action

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"reflect"
	"testing"
)

func TestDefaultUserCodesJoinTransactionAndDeduplicate(t *testing.T) {
	m, mock := newTestModule(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT a.code FROM t_action AS a").WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("ABCDEFGH").AddRow("QWERTYUI"))
	mock.ExpectCommit()
	err := m.store.Tx(t.Context(), func(ctx context.Context) error {
		values, err := m.DefaultUserCodes(ctx, []string{"ABCDEFGH"})
		if err == nil && !reflect.DeepEqual(values, []string{"ABCDEFGH", "QWERTYUI"}) {
			t.Fatal(values)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDefaultUserCodesPreserveFailures(t *testing.T) {
	for _, stage := range []string{"lock", "query", "scan", "rows"} {
		t.Run(stage, func(t *testing.T) {
			m, mock := newTestModule(t)
			lock := mock.ExpectExec("SELECT pg_advisory_xact_lock")
			if stage == "lock" {
				lock.WillReturnError(sqlmock.ErrCancelled)
			} else {
				lock.WillReturnResult(sqlmock.NewResult(0, 1))
				query := mock.ExpectQuery("SELECT a.code FROM t_action AS a")
				switch stage {
				case "query":
					query.WillReturnError(sqlmock.ErrCancelled)
				case "scan":
					query.WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow(nil))
				case "rows":
					query.WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("ABCDEFGH").RowError(0, sqlmock.ErrCancelled))
				}
			}
			if _, err := m.DefaultUserCodes(t.Context(), nil); err == nil {
				t.Fatal("failure lost")
			}
		})
	}
}
