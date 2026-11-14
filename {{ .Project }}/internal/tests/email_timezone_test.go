package tests

import (
	"context"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
)

type timezoneEmailSink struct{ messages chan string }

func (*timezoneEmailSink) Ready() bool { return true }
func (s *timezoneEmailSink) SendHTML(_ context.Context, _, _ string, plain, _ string) error {
	s.messages <- plain
	return nil
}

func TestQueuedEmailReadsCurrentUserTimezone(t *testing.T) {
	for _, zone := range []string{"", "Asia/Tokyo", "America/New_York"} {
		t.Run(zone, func(t *testing.T) {
			store := permissionDatabase(t)
			ctx := t.Context()
			expires := time.Now().Add(time.Hour).Truncate(time.Second)
			if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET metadata='{"language":"en-US","timezone":"UTC"}'::jsonb WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			if _, err := store.DB.ExecContext(ctx, `INSERT INTO t_auth_email_code(verification_id,client_ip,purpose,identifier,user_id,recipient,credential_version,code_hash,expired_at,delivery_attempts) VALUES ('template-timezone','','login','',1,'timezone@example.test',1,'',$1,0)`, expires.UnixMilli()); err != nil {
				t.Fatal(err)
			}
			sealFixtureEmails(t, store)
			// Timezone follows the current account, independently of the signed language snapshot.
			if zone == "" {
				if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET metadata=metadata-'timezone' WHERE id=1`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET metadata=jsonb_set(metadata,'{timezone}',to_jsonb($1::text)) WHERE id=1`, zone); err != nil {
					t.Fatal(err)
				}
			}
			sink := &timezoneEmailSink{messages: make(chan string, 1)}
			codes, err := newTemplateEmailCodes(store, sink, auth.EmailConfig{CodeTTL: time.Minute, ResendInterval: time.Minute, SendLimit: 1, MaxAttempts: 3}, strings.Repeat("k", 64))
			if err != nil {
				t.Fatal(err)
			}
			stop, err := codes.StartWorkers(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			locationName := zone
			if locationName == "" {
				locationName = "Asia/Shanghai"
			}
			location, err := time.LoadLocation(locationName)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case body := <-sink.messages:
				expected := expires.In(location).Format(time.RFC3339)
				if !strings.Contains(body, expected) {
					t.Fatalf("missing recipient-local expiry %q in mail", expected)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("email did not reach test sink")
			}
		})
	}
}
