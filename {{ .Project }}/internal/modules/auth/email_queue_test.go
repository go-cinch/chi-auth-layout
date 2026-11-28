package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type retrySMTP struct {
	failures int
	bodies   []string
}

func (*retrySMTP) Ready() bool { return true }

func (s *retrySMTP) SendHTML(_ context.Context, _ string, _ string, body, _ string) error {
	s.bodies = append(s.bodies, body)
	if len(s.bodies) <= s.failures {
		return errors.New("temporary SMTP failure")
	}
	return nil
}

func TestEmailWorkerSMTPOutcomes(t *testing.T) {
	for _, scenario := range []struct{ limit, failures int }{
		{1, 0}, {1, 1}, {3, 0}, {3, 2}, {3, 3}, {5, 4}, {5, 5},
	} {
		limit, failures := scenario.limit, scenario.failures
		t.Run(fmt.Sprintf("limit_%d_failures_%d", limit, failures), func(t *testing.T) {
			codes, mock, _ := newEmailCodeTest(t)
			smtp := &retrySMTP{failures: failures}
			codes.sender = smtp
			codes.config.SendLimit = limit
			codes.sentQueue = newSentEmailQueue("test-token")
			expires := time.Now().Add(time.Minute).UnixMilli()
			n := min(failures+1, limit)
			for attempt := 1; attempt <= n; attempt++ {
				mock.ExpectBegin()
				mock.ExpectQuery("SELECT recipient, expired_at").WillReturnRows(sqlmock.NewRows([]string{"recipient", "expired_at", "delivery_attempts", "delivery_after", "purpose", "created_at", "notice_detail", "account", "language", "user_id", "delivery_mac", "timezone"}).AddRow("to@example.test", expires, attempt-1, 0, "login", expires-60000, "", "t***", "en-US", 0, codes.deliveryMAC("ticket", emailAttempt{recipient: "to@example.test", expires: expires, purpose: "login", created: expires - 60000, account: "t***", language: "en-US"}), "Asia/Tokyo"))
				mock.ExpectExec("UPDATE t_auth_email_code SET delivery_attempts").WithArgs("ticket", attempt, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
				accepted := attempt > failures
				mock.ExpectExec("UPDATE t_auth_email_code SET").WithArgs("ticket", attempt, accepted, !accepted && attempt == limit, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			if err := codes.handleEmail(t.Context(), "ticket", "auth_email_interactive"); err != nil {
				t.Fatal(err)
			}
			if len(smtp.bodies) != n {
				t.Fatalf("sent %d times, want %d", len(smtp.bodies), n)
			}
			for _, body := range smtp.bodies {
				if body != smtp.bodies[0] {
					t.Fatal("OTP changed during retry")
				}
			}
			items := codes.sentQueue.list("to@example.test", "ticket")
			if failures == limit {
				if len(items) != 0 {
					t.Fatal("failed delivery appeared in sent queue")
				}
			} else if len(items) != 1 || items[0].Queue != "auth_email_interactive" || items[0].Purpose != "login" || items[0].Code != codes.verificationCode("ticket") || !strings.Contains(smtp.bodies[0], items[0].Code) || items[0].SentAt == 0 {
				t.Fatal("sent queue should contain exactly one copy of the successful delivery")
			}
		})
	}
}

func TestEmailWorkerSkipsStaleOrAlreadyDelivered(t *testing.T) {
	codes, mock, smtp := newEmailCodeTest(t)
	mock.ExpectBegin()
	mock.ExpectQuery("AND NOT delivered AND NOT consumed AND expired_at >").WillReturnRows(sqlmock.NewRows([]string{"recipient", "expired_at", "delivery_attempts", "delivery_after", "purpose", "created_at", "notice_detail", "account", "language", "user_id", "delivery_mac", "timezone"}))
	mock.ExpectCommit()
	if err := codes.handleEmail(t.Context(), "stale", "auth_email_interactive"); err != nil || smtp.body != "" {
		t.Fatal("stale job was sent or retained")
	}
}

func TestRiverScanScheduleAndStartupError(t *testing.T) {
	slot := tenMinuteSlot{}
	boundary := time.Date(2026, 11, 10, 14, 10, 0, 0, time.UTC)
	if next := slot.Next(boundary.Add(-time.Nanosecond)); !next.Equal(boundary) {
		t.Fatalf("scan shifted to %s", next)
	}
	if next := slot.Next(boundary); !next.Equal(boundary.Add(10 * time.Minute)) {
		t.Fatalf("repeated scan slot %s", next)
	}
	codes, _, _ := newEmailCodeTest(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if stop, err := codes.StartWorkers(ctx); err == nil || stop != nil {
		t.Fatal("cancelled worker startup reported success")
	}
}

func TestEmailWorkerHonorsReducedSendLimit(t *testing.T) {
	codes, mock, smtp := newEmailCodeTest(t)
	codes.config.SendLimit = 2
	expires := time.Now().Add(time.Minute).UnixMilli()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT recipient, expired_at").WillReturnRows(sqlmock.NewRows([]string{"recipient", "expired_at", "delivery_attempts", "delivery_after", "purpose", "created_at", "notice_detail", "account", "language", "user_id", "delivery_mac", "timezone"}).AddRow("to@example.test", expires, 3, 0, "login", 0, "", "t***", "en-US", 0, codes.deliveryMAC("ticket", emailAttempt{recipient: "to@example.test", expires: expires, purpose: "login", account: "t***", language: "en-US"}), "Asia/Tokyo"))
	mock.ExpectExec("UPDATE t_auth_email_code SET consumed = TRUE").WithArgs("ticket", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := codes.handleEmail(t.Context(), "ticket", "auth_email_interactive"); err != nil || smtp.body != "" {
		t.Fatal("sent beyond lowered limit", err)
	}
}
