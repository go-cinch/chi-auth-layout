package db

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestMutationTxRollbackAndCommit(t *testing.T) {
	for _, outcome := range []string{"plain", "success", "change", "record", "commit"} {
		t.Run(outcome, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			store := &Store{DB: database}
			failure := errors.New("transaction failed")
			ctx := t.Context()
			recorded := false
			if outcome != "plain" {
				ctx = WithMutationRecorder(ctx, nil, func(tx context.Context) error {
					if store.SQL(tx) == store.DB {
						t.Fatal("recorder escaped transaction")
					}
					recorded = true
					if outcome == "record" {
						return failure
					}
					return nil
				})
			}
			if HasMutationRecorder(ctx) != (outcome != "plain") {
				t.Fatal("wrong recorder presence")
			}
			mock.ExpectBegin()
			switch outcome {
			case "change", "record":
				mock.ExpectRollback()
			case "commit":
				mock.ExpectCommit().WillReturnError(failure)
			default:
				mock.ExpectCommit()
			}
			err = store.MutationTx(ctx, func(tx context.Context) error {
				if store.SQL(tx) == store.DB {
					t.Fatal("change escaped transaction")
				}
				if outcome == "change" {
					return failure
				}
				return nil
			})
			wantError := outcome == "change" || outcome == "record" || outcome == "commit"
			if errors.Is(err, failure) != wantError {
				t.Fatal(err)
			}
			if recorded != (outcome != "plain" && outcome != "change") {
				t.Fatal("wrong recording outcome")
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
