package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

const emailTestUUID = "836be6df-8db4-4890-a888-a6c02b371b2c"

type testEmailSender struct {
	disabled        bool
	err             error
	recipient, body string
}

func (s *testEmailSender) Ready() bool { return !s.disabled }
func (s *testEmailSender) SendHTML(_ context.Context, recipient, _ string, body, _ string) error {
	s.recipient, s.body = recipient, body
	return s.err
}

func emailTestConfig() EmailConfig {
	return EmailConfig{CodeTTL: time.Minute, ResendInterval: time.Minute, SendLimit: 3, MaxAttempts: 3}
}

func newEmailCodeTest(t *testing.T) (*EmailCodes, sqlmock.Sqlmock, *testEmailSender) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	sender := &testEmailSender{}
	codes, err := NewEmailCodes(&db.Store{DB: database}, sender, emailTestConfig(), authTestKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = database.Close()
	})
	codes.config.Templates = testEmailTemplateIDs()
	codes.config.AuditURL = "https://example.test/audit/operation"
	codes.SetEmailTemplates(testEmailTemplates{})
	return codes, mock, sender
}

func TestEmailAddressAndContext(t *testing.T) {
	for _, input := range []string{"not-email", "", "a@example", "a@.com", "a@example.", "a@@example.com", "a b@example.com", "Name <name@example.com>", "a@example.com\r\nBcc:x@example.com"} {
		if _, err := normalizeEmail(input); err == nil {
			t.Fatal("invalid address accepted", input)
		}
	}
	if email, err := normalizeEmail(" Person+tag@Example.COM "); err != nil || email != "person+tag@example.com" {
		t.Fatal(email, err)
	}
	if !validEmailContext(emailTestUUID, "login", "eric") || validEmailContext("invalid", "login", "eric") || validEmailContext(emailTestUUID, "other", "eric") || validEmailContext(emailTestUUID, "bind", " ") {
		t.Fatal("invalid verification context")
	}
	codes, _, _ := newEmailCodeTest(t)
	if codes.digest("id", "123456") == codes.digest("another", "123456") {
		t.Fatal("code digest not bound to challenge")
	}
	if _, err := NewEmailCodes(nil, &testEmailSender{}, emailTestConfig(), authTestKey); err == nil {
		t.Fatal("missing database accepted")
	}
	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.0.2.10:1024"
	request.Header.Set("X-Verified-Client-IP", "198.51.100.1")
	if codes.clientIP(request) != "192.0.2.10" {
		t.Fatal("unresolved header must not bypass router policy")
	}

}

func TestEmailCooldownRestoreAndAtomicRejection(t *testing.T) {
	codes, mock, _ := newEmailCodeTest(t)
	deadline := time.Now().Add(time.Minute).UnixMilli()
	mock.ExpectQuery("SELECT COALESCE\\(MAX").WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(deadline))
	mock.ExpectQuery("SELECT verification_id, expired_at").WithArgs(emailTestUUID, "login", "eric", int64(0), sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "expired_at"}).AddRow("ticket", deadline+60000))
	state, err := codes.Status(context.Background(), emailTestUUID, "192.0.2.1", "login", "eric", 0)
	if err != nil || state.RetryAt != deadline || state.VerificationID != "ticket" {
		t.Fatal(state, err)
	}
	mock.ExpectBegin()
	for range 3 {
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery("SELECT COALESCE\\(MAX").WillReturnRows(sqlmock.NewRows([]string{"retry_at"}).AddRow(deadline))
	mock.ExpectRollback()
	state, err = codes.Send(context.Background(), emailTestUUID, "192.0.2.1", "login", "eric", "eric@example.com", 1, 1)
	if !errors.Is(err, ErrEmailCooldown) || state.RetryAt != deadline {
		t.Fatal("cooldown not enforced", state, err)
	}
}

func TestEmailCodeAtomicConsumption(t *testing.T) {
	for index, scenario := range []struct {
		code                string
		attempts            int
		delivered, consumed bool
		expired             bool
		valid, update       bool
	}{
		{"123456", 0, true, false, false, true, true},
		{"000000", 0, true, false, false, false, true},
		{"000000", 2, true, false, false, false, true},
		{"123456", 3, true, false, false, false, false},
		{"123456", 0, true, true, false, false, false},
		{"123456", 0, false, false, false, false, false},
		{"123456", 0, true, false, true, false, false},
	} {
		t.Run(fmt.Sprintf("case-%d", index), func(t *testing.T) {
			codes, mock, _ := newEmailCodeTest(t)
			expires := time.Now().Add(time.Minute).UnixMilli()
			if scenario.expired {
				expires = 1
			}
			mock.ExpectQuery("SELECT code_hash, attempts").WithArgs("id", emailTestUUID, "login", "eric", "eric@example.com", int64(1), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"hash", "attempts", "expires", "delivered", "consumed"}).AddRow(codes.digest("id", "123456"), scenario.attempts, expires, scenario.delivered, scenario.consumed))
			if scenario.update {
				mock.ExpectExec("UPDATE t_auth_email_code").WithArgs(scenario.valid || scenario.attempts == 2, sqlmock.AnyArg(), "id").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			valid, err := codes.Consume(context.Background(), "id", emailTestUUID, "login", "eric", "eric@example.com", scenario.code, 1, 2)
			if err != nil || valid != scenario.valid {
				t.Fatal(valid, err)
			}
		})
	}
}

func TestEmailSliderSupportsLongValidAddresses(t *testing.T) {
	m, _, _ := newAuthTestModule(t)
	address := strings.Repeat("a", 60) + "@" + strings.Repeat("b", 60) + ".example.com"
	if _, err := normalizeEmail(address); err != nil {
		t.Fatal(err)
	}
	proof := validSliderProof(t, m.sliderCaptcha, "email_login", address)
	if err := m.sliderCaptcha.ConsumeProof(t.Context(), "email_login", address, proof); err != nil {
		t.Fatal("valid email was truncated by the legacy username limit", err)
	}
	if err := m.sliderCaptcha.ConsumeProof(t.Context(), "email_login", address, proof); !errors.Is(err, ErrSliderCaptchaRequired) {
		t.Fatal("email slider proof replay accepted", err)
	}
}

func TestEmailSendLimitValidation(t *testing.T) {
	codes, _, _ := newEmailCodeTest(t)
	for _, limit := range []int{-1, 0, 1, 3, 5} {
		cfg := emailTestConfig()
		cfg.SendLimit = limit
		_, err := NewEmailCodes(codes.store, codes.sender, cfg, authTestKey)
		if (err != nil) != (limit < 1) {
			t.Fatalf("send limit %d: %v", limit, err)
		}
	}
}

func TestHistoryRetentionValidation(t *testing.T) {
	codes, _, _ := newEmailCodeTest(t)
	for _, days := range []int{-1, 0, 1, 30, 106751, 106752} {
		cfg := emailTestConfig()
		cfg.HistoryRetentionDays = days
		got, err := NewEmailCodes(codes.store, codes.sender, cfg, authTestKey)
		if (err != nil) != (days < 0 || days > 106751) {
			t.Fatalf("days %d: %v", days, err)
		}
		if days == 0 && got.config.HistoryRetentionDays != 30 {
			t.Fatal("missing default retention")
		}
	}
}
