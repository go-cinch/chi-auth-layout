package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestScanReservationAndRollback(t *testing.T) {
	tiers := []AlertTier{ {Window: time.Minute, Count: 2}, {Window: 3 * time.Minute, Count: 5}}
	for _, invalid := range [][]AlertTier{ { {Window: 0, Count: 2}}, {tiers[1], tiers[0]}, { {Window: time.Minute, Count: 0}}} {
		if validateAlertTiers(invalid) == nil {
			t.Fatal("invalid tiers")
		}
	}
	for _, outcome := range []string{"claimed", "busy", "previous", "failed"} {
		e, mock, _ := newEmailCodeTest(t)
		e.config.AlertTiers = tiers
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT pg_try_advisory_xact_lock").WillReturnRows(sqlmock.NewRows([]string{"ok"}).AddRow(outcome != "busy"))
		if outcome != "busy" {
			rows := sqlmock.NewRows([]string{"id"})
			if outcome != "previous" {
				rows.AddRow(7)
			}
			mock.ExpectQuery("INSERT INTO t_sensitive_scan").WillReturnRows(rows)
			if outcome != "previous" {
				stmt := mock.ExpectQuery("WITH pending AS")
				if outcome == "failed" {
					stmt.WillReturnError(errors.New("unavailable"))
					mock.ExpectRollback()
				} else {
					stmt.WillReturnRows(sqlmock.NewRows([]string{"id"}))
					mock.ExpectExec("DELETE FROM t_sensitive_operation").WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
		}
		if outcome != "failed" {
			mock.ExpectCommit()
		}
		if err := e.ScanSensitiveOperations(t.Context()); (err != nil) != (outcome == "failed") {
			t.Fatal(outcome, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSecurityNoticeOutbox(t *testing.T) {
	m, mock, ctx, identity, _ := newVerificationTest(t)
	if err := m.enrollmentNotice(ctx, identity.UserID); err != nil {
		t.Fatal(err)
	}
	attachTestEmail(t, m)
	e := m.emailCodes
	e.config.AlertTiers = []AlertTier{ {Window: time.Minute, Count: 3}}
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO t_sensitive_operation").WithArgs(identity.UserID, "delete", false).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := e.recordSensitiveOperation(ctx, identity.UserID, "DELETE /msg/sent/1"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO t_auth_email_code").WillReturnResult(sqlmock.NewResult(0, 1))
	expectEmailSeal(mock)
	if err := m.enrollmentNotice(ctx, identity.UserID); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO t_auth_email_code").WillReturnResult(sqlmock.NewResult(0, 1))
	expectEmailSeal(mock)
	if err := e.unlinkNotice(ctx, identity.UserID, identity.CredentialVersion, "old@example.test"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO t_auth_email_code").WillReturnResult(sqlmock.NewResult(0, 1))
	expectEmailSeal(mock)
	if err := e.emailLinkedNotice(ctx, identity.UserID); err != nil { t.Fatal(err) }
	mock.ExpectExec("INSERT INTO t_auth_email_code").WillReturnError(errors.New("outbox unavailable"))
	if err := e.emailLinkedNotice(ctx, identity.UserID); err == nil { t.Fatal("outbox error ignored") }
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSensitiveOperationCategories(t *testing.T) {
	for _, tc := range []struct {
		operation, kind string
		permission      bool
	}{
		{"POST /user", "create", true}, {"POST /dictionary/", "create", false},
		{"POST /mail-template/9/publish", "update", false}, {"POST /auth/totp/confirm", "update", false},
		{"PATCH /role/1", "update", true}, {"PUT /action/1", "update", true},
		{"PATCH /user-group/1", "update", true}, {"PATCH /user/1", "update", false},
		{"DELETE /msg/sent/1,2", "delete", false}, {"DELETE /user/1", "delete", true},
		{"PATCH /role-other/1", "update", false}, {"unknown", "other", false},
	} {
		kind, permission := sensitiveOperationCategory(tc.operation)
		if kind != tc.kind || permission != tc.permission {
			t.Fatalf("%s: %s %v", tc.operation, kind, permission)
		}
	}
}

func TestSensitiveActivitySummaryLanguages(t *testing.T) {
	detail := `{"version":1,"create_count":3,"delete_count":2,"update_count":4,"permission_count":2}`
	if got := sensitiveActivitySummary(detail, "zh-CN"); got != "创建：3 次\n删除：2 次\n修改：4 次\n其中涉及权限：2 次" {
		t.Fatal(got)
	}
	if got := sensitiveActivitySummary(detail, "en-US"); got != "Created: 3\nDeleted: 2\nUpdated: 4\nOf these, permission-related: 2" {
		t.Fatal(got)
	}
	if sensitiveActivitySummary(detail, "") != sensitiveActivitySummary(detail, "zh-CN") {
		t.Fatal("language fallback mismatch")
	}
	if got := sensitiveActivitySummary("legacy signed detail", "en-US"); got != "legacy signed detail" {
		t.Fatal(got)
	}
	if got := sensitiveActivitySummary(`{"version":1,"other_count":2}`, "en-US"); got != "Created: 0\nDeleted: 0\nUpdated: 0\nOther: 2\nOf these, permission-related: 0" {
		t.Fatal(got)
	}
}
