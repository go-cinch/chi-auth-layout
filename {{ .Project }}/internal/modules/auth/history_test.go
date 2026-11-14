package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCleanupHistoryBatchesAndErrors(t *testing.T) {
	failure := errors.New("database unavailable")
	for _, stage := range []string{"success", "clock", "delete", "count"} {
		t.Run(stage, func(t *testing.T) {
			e, mock, _ := newEmailCodeTest(t)
			e.config.HistoryRetentionDays = 7
			const now int64 = 1800000000000
			clock := mock.ExpectQuery("SELECT FLOOR")
			if stage == "clock" {
				clock.WillReturnError(failure)
			} else {
				clock.WillReturnRows(sqlmock.NewRows([]string{"now"}).AddRow(now))
				cutoff := now - (7 * 24 * time.Hour).Milliseconds()
				first := mock.ExpectExec("WITH expired AS").WithArgs(cutoff, historyBatchSize)
				switch stage {
				case "delete":
					first.WillReturnError(failure)
				case "count":
					first.WillReturnResult(sqlmock.NewErrorResult(failure))
				default:
					first.WillReturnResult(sqlmock.NewResult(0, historyBatchSize))
					mock.ExpectExec("WITH expired AS").WithArgs(cutoff, historyBatchSize).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec("WITH expired AS").WithArgs(cutoff, historyBatchSize).WillReturnResult(sqlmock.NewResult(0, 0))
					mock.ExpectExec("WITH expired AS").WithArgs(cutoff, historyBatchSize).WillReturnResult(sqlmock.NewResult(0, 0))
				}
			}
			err := e.CleanupHistory(t.Context())
			if stage == "success" && err != nil {
				t.Fatal(err)
			}
			if stage != "success" && !errors.Is(err, failure) {
				t.Fatal("cleanup failure not propagated", err)
			}
		})
	}
}
