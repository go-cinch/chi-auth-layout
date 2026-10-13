package msg

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectStatusTx(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO t_user_msg_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT user_id FROM t_user_msg_status.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
}

func expectStatusRefresh(mock sqlmock.Sqlmock) {
	mock.ExpectExec("UPDATE t_user_msg_status").WillReturnResult(sqlmock.NewResult(0, 1))
}

func statusRows(count int64, latest, expiry any) *sqlmock.Rows {
	if v, ok := latest.(time.Time); ok {
		latest = v.UnixMilli()
	}
	if v, ok := expiry.(time.Time); ok {
		expiry = v.UnixMilli()
	}
	return sqlmock.NewRows([]string{"unread_count", "latest_unread_at", "next_refresh_at", "observed_at"}).AddRow(count, latest, expiry, time.Now().UnixMilli())
}

func TestStatusReadsOnlyPersistedRow(t *testing.T) {
	m, mock := testModule(t)
	at := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	for range 3 {
		mock.ExpectQuery("SELECT unread_count, latest_unread_at, next_refresh_at, .* FROM t_user_msg_status WHERE user_id = \\$1").WithArgs(int64(2)).WillReturnRows(statusRows(4, at, time.Now().Add(time.Hour)))
		value, err := m.Status(t.Context(), 2)
		if err != nil || value.Count != 4 || value.UserID != 2 || value.LatestUnreadAt == nil || *value.LatestUnreadAt != at.UnixMilli() || value.ObservedAt < at.UnixMilli() {
			t.Fatalf("status: %#v %v", value, err)
		}
	}
	// No aggregate, lock, or write is expected on the repeated polling path.
	mock.ExpectQuery("SELECT unread_count").WillReturnRows(statusRows(0, nil, nil))
	value, err := m.Status(t.Context(), 2)
	if err != nil || value.Count != 0 || value.LatestUnreadAt != nil {
		t.Fatal(value, err)
	}
	mock.ExpectQuery("SELECT unread_count").WillReturnError(sqlmock.ErrCancelled)
	if _, err := m.Status(t.Context(), 2); err == nil {
		t.Fatal("database failure hidden")
	}
	if _, err := m.Status(t.Context(), 0); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestStatusRepairsOnlyMissingOrExpiredRows(t *testing.T) {
	for _, missing := range []bool{false, true} {
		m, mock := testModule(t)
		query := mock.ExpectQuery("SELECT unread_count")
		if missing {
			query.WillReturnError(sql.ErrNoRows)
		} else {
			query.WillReturnRows(statusRows(1, time.Now(), time.Now().Add(-time.Second)))
		}
		expectStatusTx(mock)
		expectStatusRefresh(mock)
		mock.ExpectQuery("SELECT unread_count").WillReturnRows(statusRows(0, nil, nil))
		mock.ExpectCommit()
		value, err := m.Status(t.Context(), 2)
		if err != nil || value.Count != 0 {
			t.Fatal(value, err)
		}
	}
}

func TestStatusMutationFailuresRollback(t *testing.T) {
	for stage := 0; stage < 6; stage++ {
		m, mock := testModule(t)
		mock.ExpectBegin()
		lock := mock.ExpectExec("SELECT pg_advisory_xact_lock")
		if stage == 0 {
			lock.WillReturnError(sqlmock.ErrCancelled)
		} else {
			lock.WillReturnResult(sqlmock.NewResult(0, 1))
			insert := mock.ExpectExec("INSERT INTO t_user_msg_status")
			if stage == 1 {
				insert.WillReturnError(sqlmock.ErrCancelled)
			} else {
				insert.WillReturnResult(sqlmock.NewResult(0, 1))
				query := mock.ExpectQuery("SELECT user_id FROM t_user_msg_status.*FOR UPDATE")
				if stage == 2 {
					query.WillReturnError(sqlmock.ErrCancelled)
				} else if stage == 3 {
					query.WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2).RowError(0, sqlmock.ErrCancelled))
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
					mock.ExpectExec("INSERT INTO t_msg_recipient").WillReturnResult(sqlmock.NewResult(0, 1))
					update := mock.ExpectExec("UPDATE t_user_msg_status")
					if stage == 4 {
						update.WillReturnError(sqlmock.ErrCancelled)
					} else {
						update.WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
			}
		}
		if stage == 5 {
			mock.ExpectCommit().WillReturnError(sqlmock.ErrCancelled)
		} else {
			mock.ExpectRollback()
		}
		if err := m.Read(t.Context(), 2, 1); err == nil {
			t.Fatalf("stage %d failure hidden", stage)
		}
	}
}

func TestExpireStatus(t *testing.T) {
	for stage := 0; stage < 5; stage++ {
		m, mock := testModule(t)
		query := mock.ExpectQuery("SELECT user_id FROM t_user_msg_status.*LIMIT 100")
		switch stage {
		case 0:
			query.WillReturnError(sqlmock.ErrCancelled)
		case 1:
			query.WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("invalid"))
		case 2:
			query.WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2).RowError(0, sqlmock.ErrCancelled))
		case 3:
			query.WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
		case 4:
			query.WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(2))
			expectStatusTx(mock)
			expectStatusRefresh(mock)
			mock.ExpectCommit()
		}
		err := m.ExpireStatus(t.Context())
		if (stage < 3) != (err != nil) {
			t.Fatalf("stage %d: %v", stage, err)
		}
	}
}

func TestStatusUsesDatabaseObservationTime(t *testing.T) {
	for _, ahead := range []bool{false, true} {
		t.Run(map[bool]string{false: "database-behind", true: "database-ahead"}[ahead], func(t *testing.T) {
			m, mock := testModule(t)
			observed := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			if ahead {
				observed = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			deadline := observed.Add(time.Minute)
			if ahead {
				deadline = observed.Add(-time.Minute)
			}
			rows := sqlmock.NewRows([]string{"unread_count", "latest_unread_at", "next_refresh_at", "observed_at"}).AddRow(0, nil, deadline.UnixMilli(), observed.UnixMilli())
			mock.ExpectQuery("SELECT unread_count").WillReturnRows(rows)
			if ahead {
				expectStatusTx(mock)
				expectStatusRefresh(mock)
				mock.ExpectQuery("SELECT unread_count").WillReturnRows(statusRows(1, deadline, nil))
				mock.ExpectCommit()
			}
			value, err := m.Status(t.Context(), 2)
			if err != nil || (value.Count == 1) != ahead {
				t.Fatal(value, err)
			}
		})
	}
}
