package audit

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

func TestDeliveryRetriesWholeTransaction(t *testing.T) {
	for _, recover := range []bool{false, true} {
		name := "exhausted"
		if recover {
			name = "last retry commits"
		}
		t.Run(name, func(t *testing.T) {
			database, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			m := Module{store: &db.Store{DB: database}, settings: config.DefaultAudit()}
			for attempt := 1; attempt <= 4; attempt++ {
				mock.ExpectBegin()
				mock.ExpectExec("INSERT INTO t_audit_event").WillReturnResult(driver.RowsAffected(1))
				second := mock.ExpectExec("INSERT INTO t_audit_event")
				if recover && attempt == 4 {
					second.WillReturnResult(driver.RowsAffected(1))
					mock.ExpectCommit()
				} else {
					second.WillReturnError(errors.New("write interrupted"))
					mock.ExpectRollback()
				}
			}
			fields := map[string]any{"event": `{"kind":"login","status":200}`, "delivery_id": "id", "additional_user_operation": "recover_totp"}
			if err := m.persistDelivery(t.Context(), fields); (err == nil) != recover {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInvalidOrCanceledDelivery(t *testing.T) {
	var m Module
	if err := m.persistDelivery(t.Context(), map[string]any{"event": "oops"}); err == nil {
		t.Fatal("malformed delivery accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if waitStream(ctx, config.DefaultAudit().RecordTimeout) {
		t.Fatal("canceled wait did not stop")
	}
}
