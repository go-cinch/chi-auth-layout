package tests

import (
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/modules/auth"
)

type asyncTestSink struct{ emailSink }

func (*asyncTestSink) TestReady() bool { return true }

func TestEmailQueuesRejectDatabaseTampering(t *testing.T) {
	for _, kind := range []string{"auth", "test"} {
		for _, mutation := range []string{"valid", "recipient", "parameters", "signature"} {
			t.Run(kind+"/"+mutation, func(t *testing.T) {
				store := permissionDatabase(t)
				ctx := t.Context()
				sink := &asyncTestSink{}
				codes, err := newTemplateEmailCodes(store, sink, auth.EmailConfig{CodeTTL: time.Minute, ResendInterval: time.Second, MaxAttempts: 3, SendLimit: 1}, strings.Repeat("k", 64))
				if err != nil {
					t.Fatal(err)
				}
				var ticket string
				if kind == "auth" {
					status, err := codes.Send(ctx, "836be6df-8db4-4890-a888-a6c02b371b2c", "127.0.0.1", "bind", "owner@example.test", "owner@example.test", 1, 1)
					if err != nil {
						t.Fatal(err)
					}
					ticket = status.VerificationID
				} else {
					if err = codes.EnqueueHTML(ctx, "owner@example.test", "subject", "plain", "<p>test</p>"); err != nil {
						t.Fatal(err)
					}
				}
				// Workers have not started: API admission must not invoke SMTP.
				sink.mu.Lock()
				before := sink.sent
				sink.mu.Unlock()
				if before != 0 {
					t.Fatal("synchronous SMTP")
				}
				var query string
				if kind == "auth" {
					switch mutation {
					case "recipient":
						query = `UPDATE t_auth_email_code SET recipient='attacker@example.test'`
					case "parameters":
						query = `UPDATE t_auth_email_code SET delivery_account='forged'`
					case "signature":
						query = `UPDATE t_auth_email_code SET delivery_mac=''`
					}
				} else {
					switch mutation {
					case "recipient":
						query = `UPDATE river_job SET args=jsonb_set(args,'{payload,recipient}','"attacker@example.test"') WHERE kind='mail_template_test_deliver'`
					case "parameters":
						query = `UPDATE river_job SET args=jsonb_set(args,'{payload,html}','"<p>forged</p>"') WHERE kind='mail_template_test_deliver'`
					case "signature":
						query = `UPDATE river_job SET args=args-'signature' WHERE kind='mail_template_test_deliver'`
					}
				}
				if query != "" {
					if _, err = store.DB.ExecContext(ctx, query); err != nil {
						t.Fatal(err)
					}
				}
				stop, err := codes.StartWorkers(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer stop()
				done := false
				for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
					if kind == "auth" {
						err = store.DB.QueryRowContext(ctx, `SELECT delivered OR consumed FROM t_auth_email_code WHERE verification_id=$1`, ticket).Scan(&done)
					} else {
						err = store.DB.QueryRowContext(ctx, `SELECT state IN ('completed','cancelled','discarded') FROM river_job WHERE kind='mail_template_test_deliver'`).Scan(&done)
					}
					if err != nil {
						t.Fatal(err)
					}
					if done {
						break
					}
					time.Sleep(20 * time.Millisecond)
				}
				if !done {
					t.Fatal("job not finalized")
				}
				sink.mu.Lock()
				count := sink.sent
				sink.mu.Unlock()
				want := 0
				if mutation == "valid" {
					want = 1
				}
				if count != want {
					t.Fatalf("sent %d, want %d", count, want)
				}
			})
		}
	}
}
