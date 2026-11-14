package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testEmailTemplateIDs() EmailTemplateIDs {
	return EmailTemplateIDs{EmailLinked: "auth.email_linked", SignInCode: "auth.sign_in_code", EmailBindingCode: "auth.email_binding_code", SensitiveOperationCode: "auth.sensitive_operation_code", AuthenticatorResetCode: "auth.totp_reset_code", ConfirmEmailUnlinking: "auth.confirm_email_unlinking", EmailUnlinked: "auth.email_unlinked", AuthenticatorLinked: "auth.totp_linked", AuthenticatorRelinked: "auth.totp_linked", SensitiveOperationNotification: "auth.sensitive_operation_notification"}
}

type testEmailTemplates struct{}

func (testEmailTemplates) RenderPublished(_ context.Context, id, language string, values map[string]string) (string, string, string, error) {
	raw, _ := json.Marshal(values)
	return id, string(raw), "<p>rendered</p>", nil
}

type captureEmailTemplate struct {
	id       string
	language string
	values   map[string]string
	err      error
}

func (r *captureEmailTemplate) RenderPublished(_ context.Context, id, language string, values map[string]string) (string, string, string, error) {
	r.id, r.language, r.values = id, language, values
	return "subject", "plain", "<p>html</p>", r.err
}

func TestDeliveryTemplateContracts(t *testing.T) {
	e, _, _ := newEmailCodeTest(t)
	e.config.UnlinkURL = "https://pc.example.test/#/auth/unbind-email"
	e.config.SensitiveLifetime = 10 * time.Minute
	for _, tc := range []struct {
		purpose, id string
		keys        []string
	}{
		{"login", "auth.sign_in_code", []string{"account", "code", "expires_at"}},
		{"bind", "auth.email_binding_code", []string{"account", "code", "expires_at"}},
		{"sensitive", "auth.sensitive_operation_code", []string{"account", "code", "expires_at", "authorization_duration"}},
		{"totp_reset", "auth.totp_reset_code", []string{"account", "code", "expires_at"}},
		{"unlink", "auth.confirm_email_unlinking", []string{"account", "action_url", "expires_at"}},
		{"unlink_notice", "auth.email_unlinked", []string{"account", "audit_url"}},
		{"email_linked", "auth.email_linked", []string{"account", "audit_url"}},
		{"totp_bound", "auth.totp_linked", []string{"account", "audit_url"}},
		{"totp_rebound", "auth.totp_linked", []string{"account", "audit_url"}},
		{"sensitive_notice", "auth.sensitive_operation_notification", []string{"account", "audit_url", "activity_summary", "activity_started_at", "activity_ended_at"}},
	} {
		t.Run(tc.purpose, func(t *testing.T) {
			renderer := &captureEmailTemplate{}
			e.SetEmailTemplates(renderer)
			job := emailAttempt{purpose: tc.purpose, language: "en-US", account: "owner+&测试", expires: 1900000000123, detail: "3 sensitive operations"}
			subject, plain, html, err := e.deliveryMessage(t.Context(), "ticket", job)
			if err != nil || subject != "subject" || plain != "plain" || html != "<p>html</p>" || renderer.id != tc.id || renderer.language != job.language {
				t.Fatal("wrong rendering", err, renderer.id)
			}
			if len(renderer.values) != len(tc.keys) {
				t.Fatal(renderer.values)
			}
			for _, key := range tc.keys {
				if renderer.values[key] == "" {
					t.Fatal("missing parameter", key)
				}
			}
			if renderer.values["account"] != job.account {
				t.Fatal("account lost")
			}
			if code, ok := renderer.values["code"]; ok && code != e.verificationCode("ticket") {
				t.Fatal("wrong OTP")
			}
			if link, ok := renderer.values["action_url"]; ok && !strings.HasSuffix(link, "#token=ticket."+e.digest("unlink-link", "ticket")) {
				t.Fatal("wrong proof")
			}
		})
	}
	renderer := &captureEmailTemplate{}
	e.SetEmailTemplates(renderer)
	e.config.Templates.SignInCode = "custom.production.login"
	_, _, _, err := e.deliveryMessage(t.Context(), "ticket", emailAttempt{purpose: "login", account: "owner"})
	if err != nil || renderer.id != "custom.production.login" {
		t.Fatal("ignored configured ID")
	}
	renderer.err = errors.New("unpublished")
	if _, _, _, err = e.deliveryMessage(t.Context(), "ticket", emailAttempt{purpose: "login"}); err == nil {
		t.Fatal("ignored rendering error")
	}
	for _, purpose := range []string{"unknown", "unlink_notice", "unlink"} {
		e.config.AuditURL = ""
		e.config.UnlinkURL = ""
		if _, _, _, err = e.deliveryMessage(t.Context(), "ticket", emailAttempt{purpose: purpose}); err == nil {
			t.Fatal("invalid delivery accepted")
		}
	}
	e.SetEmailTemplates(nil)
	if _, _, _, err = e.deliveryMessage(t.Context(), "ticket", emailAttempt{purpose: "login"}); err == nil {
		t.Fatal("missing renderer accepted")
	}
	e.SetEmailTemplates(renderer)
	e.config.Templates.SignInCode = ""
	if _, _, _, err = e.deliveryMessage(t.Context(), "ticket", emailAttempt{purpose: "login"}); err == nil {
		t.Fatal("empty ID accepted")
	}
}

func TestEmailAuditURL(t *testing.T) {
	for _, base := range []string{"https://example.test/audit/operation", "https://example.test/#/audit/operation"} {
		value, err := emailAuditURL(base)
		if err != nil {
			t.Fatal(err)
		}
		link, _ := url.Parse(value)
		if link.Fragment != "" {
			link, _ = url.Parse(link.Fragment)
		}
		if !reflect.DeepEqual(link.Query(), url.Values{"days": {"30"}}) {
			t.Fatal(value)
		}
	}
	for _, base := range []string{"", "javascript:alert(1)", "https://example.test/#/%zz"} {
		if _, err := emailAuditURL(base); err == nil {
			t.Fatal(base)
		}
	}
}

func TestEmailAuthorizationDuration(t *testing.T) {
	for _, tc := range []struct { duration time.Duration; language, want string }{
		{10*time.Minute, "zh-CN", "10 分钟"},
		{time.Hour, "zh-CN", "1 小时"},
		{time.Hour, "en-US", "1 hour"},
		{10*time.Minute, "en-US", "10 minutes"},
		{90*time.Second, "en-US", "1 minute 30 seconds"},
		{time.Minute, "fr-FR", "1 分钟"},
		{1500*time.Millisecond, "en-US", "2 seconds"},
		{500*time.Millisecond, "en-US", "500ms"},
	} {
		if got := emailAuthorizationDuration(tc.duration, tc.language); got != tc.want { t.Fatalf("got %q, want %q", got, tc.want) }
	}
}

func TestTemplateFailureDoesNotSend(t *testing.T) {
	e, _, smtp := newEmailCodeTest(t)
	e.SetEmailTemplates(&captureEmailTemplate{err: errors.New("unpublished")})
	if e.sendAttempt(t.Context(), "ticket", emailAttempt{purpose: "login", expires: time.Now().Add(time.Minute).UnixMilli()}) || smtp.body != "" {
		t.Fatal("failed template sent")
	}
}

func TestEmailExpirationUsesRecipientTimezone(t *testing.T) {
	e, _, _ := newEmailCodeTest(t)
	e.config.UnlinkURL = "https://pc.example.test/auth/unbind-email"
	expires := time.Date(2026, time.November, 13, 8, 30, 0, 0, time.UTC).UnixMilli()
	for _, tc := range []struct{ zone, want string }{
		{"Asia/Tokyo", "2026-11-13T17:30:00+09:00"},
		{"America/New_York", "2026-11-13T03:30:00-05:00"},
		{"UTC", "2026-11-13T08:30:00Z"},
		{"", "2026-11-13T16:30:00+08:00"},
		{"Invalid/Zone", "2026-11-13T16:30:00+08:00"},
		{"Local", "2026-11-13T16:30:00+08:00"},
	} {
		for _, purpose := range []string{"login", "bind", "sensitive", "totp_reset", "unlink"} {
			renderer := &captureEmailTemplate{}
			e.SetEmailTemplates(renderer)
			_, _, _, err := e.deliveryMessage(t.Context(), "ticket", emailAttempt{purpose: purpose, expires: expires, timezone: tc.zone, language: "en-US", account: "owner"})
			if err != nil || renderer.values["expires_at"] != tc.want {
				t.Fatalf("%s/%s: %q %v", purpose, tc.zone, renderer.values["expires_at"], err)
			}
		}
	}
	summer := time.Date(2026, time.July, 13, 8, 30, 0, 0, time.UTC).In(emailTimeZone("America/New_York"))
	if summer.Format(time.RFC3339) != "2026-07-13T04:30:00-04:00" {
		t.Fatal("daylight saving offset lost", summer)
	}
}

func TestSensitiveNoticePeriodUsesStoredBoundsAndTimezone(t *testing.T) {
	e, _, _ := newEmailCodeTest(t)
	start := time.Date(2026, time.November, 13, 7, 20, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2026, time.November, 13, 7, 35, 0, 0, time.UTC).UnixMilli()
	raw, err := json.Marshal(sensitiveActivityCounts{Version: 2, StartedAt: &start, EndedAt: &end, Created: 3, Deleted: 2, Updated: 4, Permissions: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ zone, from, to string }{
		{"", "2026-11-13T15:20:00+08:00", "2026-11-13T15:35:00+08:00"},
		{"Asia/Tokyo", "2026-11-13T16:20:00+09:00", "2026-11-13T16:35:00+09:00"},
		{"America/New_York", "2026-11-13T02:20:00-05:00", "2026-11-13T02:35:00-05:00"},
	} {
		renderer := &captureEmailTemplate{}
		e.SetEmailTemplates(renderer)
		job := emailAttempt{purpose: "sensitive_notice", language: "en-US", timezone: tc.zone, account: "owner", detail: string(raw)}
		for range 2 {
			if _, _, _, err := e.deliveryMessage(t.Context(), "ticket", job); err != nil {
				t.Fatal(err)
			}
			if renderer.values["activity_started_at"] != tc.from || renderer.values["activity_ended_at"] != tc.to {
				t.Fatal(renderer.values)
			}
			if renderer.values["activity_summary"] != "Created: 3\nDeleted: 2\nUpdated: 4\nOf these, permission-related: 2" {
				t.Fatal(renderer.values)
			}
		}
	}
	// Include both offsets when a real window crosses a daylight-saving transition.
	start = time.Date(2026, time.November, 1, 5, 55, 0, 0, time.UTC).UnixMilli()
	end = time.Date(2026, time.November, 1, 6, 10, 0, 0, time.UTC).UnixMilli()
	raw, _ = json.Marshal(sensitiveActivityCounts{Version: 2, StartedAt: &start, EndedAt: &end})
	from, to := sensitiveActivityPeriod(string(raw), "en-US", "America/New_York")
	if from != "2026-11-01T01:55:00-04:00" || to != "2026-11-01T01:10:00-05:00" {
		t.Fatal(from, to)
	}
	for _, legacy := range []string{"old signed description", `{"version":1,"create_count":2}`, `{"version":2}`, `{"version":2,"started_at":3,"ended_at":2}`} {
		from, to = sensitiveActivityPeriod(legacy, "en-US", "")
		if from != "Not recorded" || to != "Not recorded" {
			t.Fatal("fabricated legacy times", from, to)
		}
		from, to = sensitiveActivityPeriod(legacy, "zh-CN", "")
		if from != "未记录" || to != "未记录" {
			t.Fatal(from, to)
		}
	}
}
