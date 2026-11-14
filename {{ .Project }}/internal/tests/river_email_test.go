package tests

import (
	"context"
	"errors"
	"strings"
"sync/atomic"
"github.com/lib/pq"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/email"
)

func TestRiverEmailAdmissionAndDelivery(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	exec := func(query string) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}

	exec(`INSERT INTO t_auth_email_code
        (verification_id, client_ip, purpose, identifier, user_id, recipient, credential_version, code_hash, expired_at, delivery_attempts)
        VALUES ('pending-notice','','totp_bound','',1,'test@example.test',1,'',9999999999999,0)`)
	sink := &emailSink{}
	codes, err := newTemplateEmailCodes(store, sink, auth.EmailConfig{
		CodeTTL: time.Minute, ResendInterval: time.Minute, SendLimit: 3, MaxAttempts: 3,
	}, strings.Repeat("k", 64))
	if err != nil {
		t.Fatal(err)
	}
	send := func() (*auth.EmailCodeStatus, error) {
		return codes.Send(ctx, "836be6df-8db4-4890-a888-a6c02b371b2c", "127.0.0.1", "bind",
			"test@example.test", "test@example.test", 1, 1)
	}
	// A rejected River insert must also roll back the ticket and cooldown.
	exec(`ALTER TABLE river_job ADD CONSTRAINT reject_test_job CHECK (queue <> 'auth_email_interactive')`)
	if _, err := send(); !errors.Is(err, auth.ErrEmailUnavailable) {
		t.Fatalf("failed enqueue: %v", err)
	}
	exec(`ALTER TABLE river_job DROP CONSTRAINT reject_test_job`)
	otp, err := send()
	if err != nil {
		t.Fatal(err)
	}
	var notices, interactive, total int
	if err := store.DB.QueryRowContext(ctx, `SELECT
		count(*) FILTER (WHERE queue='auth_email_notice' AND args=jsonb_build_object('ticket_id','pending-notice'::text)),
		count(*) FILTER (WHERE queue='auth_email_interactive' AND args=jsonb_build_object('ticket_id',$1::text)),
		count(*) FROM river_job WHERE kind='auth_email_deliver'`, otp.VerificationID).Scan(&notices, &interactive, &total); err != nil ||
		notices != 1 || interactive != 1 || total != 2 {
		t.Fatalf("notice=%d interactive=%d total=%d: %v", notices, interactive, total, err)
	}

	stop, err := codes.StartWorkers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		var sent, scans int
		if err := store.DB.QueryRowContext(ctx, `SELECT
			(SELECT count(*) FROM t_auth_email_code WHERE delivered),
			(SELECT count(*) FROM river_job WHERE kind='auth_sensitive_activity_scan' AND queue='email_sensitive_activity_notification' AND state='completed')`).Scan(&sent, &scans); err != nil {
			t.Fatal(err)
		}
		if sent == 1 && scans >= 1 {
			var skipped bool
			if err := store.DB.QueryRowContext(ctx, `SELECT consumed AND NOT delivered FROM t_auth_email_code WHERE verification_id='pending-notice'`).Scan(&skipped); err != nil || !skipped {
				t.Fatal("unsigned legacy notice not rejected", err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("mail delivery or startup scan did not complete")
}

func TestRiverEmailDebugAdmissionDeliveryAndConsumption(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	sender, err := email.New(email.SMTPConfig{Enabled: true, Debug: true})
	if err != nil {
		t.Fatal(err)
	}
	codes, err := newTemplateEmailCodes(store, sender, auth.EmailConfig{
		Debug: true, CodeTTL: time.Minute, ResendInterval: time.Minute, SendLimit: 3, MaxAttempts: 3,
	}, strings.Repeat("k", 64))
	if err != nil {
		t.Fatal(err)
	}
	const uuid = "836be6df-8db4-4890-a888-a6c02b371b2c"
	const address = "debug@example.test"
	send := func() (*auth.EmailCodeStatus, error) {
		return codes.Send(ctx, uuid, "127.0.0.1", "bind", address, address, 1, 1)
	}
	// Queue admission is still mandatory and a rejected insert rolls back cooldowns.
	if _, err := store.DB.ExecContext(ctx, `ALTER TABLE river_job ADD CONSTRAINT reject_debug_job CHECK (queue <> 'auth_email_interactive')`); err != nil {
		t.Fatal(err)
	}
	if _, err := send(); !errors.Is(err, auth.ErrEmailUnavailable) {
		t.Fatal("debug skipped queue admission", err)
	}
	if _, err := store.DB.ExecContext(ctx, `ALTER TABLE river_job DROP CONSTRAINT reject_debug_job`); err != nil {
		t.Fatal(err)
	}
	ticket, err := send()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := send(); !errors.Is(err, auth.ErrEmailCooldown) {
		t.Fatal("debug skipped send cooldown", err)
	}
	consume := func(id, code string) bool {
		t.Helper()
		var valid bool
		err := store.Tx(ctx, func(txCtx context.Context) error {
			var err error
			valid, err = codes.Consume(txCtx, id, uuid, "bind", address, address, code, 1, 1)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return valid
	}
	if consume(ticket.VerificationID, "000000") {
		t.Fatal("debug code accepted before queue delivery")
	}
	var queued int
	if err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM river_job WHERE queue='auth_email_interactive' AND args=jsonb_build_object('ticket_id',$1::text)`, ticket.VerificationID).Scan(&queued); err != nil || queued != 1 {
		t.Fatal("missing queued delivery", queued, err)
	}
	stop, err := codes.StartWorkers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	completed := false
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		var delivered bool
		var attempts int
		if err := store.DB.QueryRowContext(ctx, `SELECT delivered, delivery_attempts FROM t_auth_email_code WHERE verification_id=$1`, ticket.VerificationID).Scan(&delivered, &attempts); err != nil {
			t.Fatal(err)
		}
		if delivered {
			if attempts != 1 {
				t.Fatal("debug delivery retried", attempts)
			}
			completed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !completed {
		t.Fatal("debug worker did not finalize delivery")
	}
	if consume("missing-ticket", "000000") {
		t.Fatal("debug bypassed ticket lookup")
	}
	if consume(ticket.VerificationID, "999999") {
		t.Fatal("Verification code accepted as email code")
	}
	if !consume(ticket.VerificationID, "000000") {
		t.Fatal("fixed email code rejected after delivery")
	}
	if consume(ticket.VerificationID, "000000") {
		t.Fatal("fixed email code replay accepted")
	}
}

// Exercise the real constraints and worker together: SQL mocks cannot detect a
// schema limit that rejects the fifth delivery attempt.
type retryEmailSink struct{ attempts atomic.Int32 }

func (*retryEmailSink) Ready() bool { return true }

func (s *retryEmailSink) SendHTML(context.Context, string, string, string, string) error {
	if s.attempts.Add(1) < 5 {
		return errors.New("temporary delivery failure")
	}
	return nil
}

func TestRiverEmailFifthAttemptAndIndependentKeys(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	sink := &retryEmailSink{}
	codes, err := newTemplateEmailCodes(store, sink, auth.EmailConfig{
		CodeTTL: time.Minute, ResendInterval: time.Second, SendLimit: 5, MaxAttempts: 3,
	}, strings.Repeat("k", 64))
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := codes.Send(ctx, "836be6df-8db4-4890-a888-a6c02b371b2c", "127.0.0.1", "bind", "retry@example.test", "retry@example.test", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	var rowID int64
	if err = store.DB.QueryRowContext(ctx, `SELECT id FROM t_auth_email_code WHERE verification_id=$1`, ticket.VerificationID).Scan(&rowID); err != nil || rowID <= 0 {
		t.Fatal("ticket must have an independent numeric primary key", rowID, err)
	}
	var cooldownID int64
	if err = store.DB.QueryRowContext(ctx, `SELECT id FROM t_auth_email_cooldown WHERE cooldown_key=$1`, "ip:127.0.0.1").Scan(&cooldownID); err != nil || cooldownID <= 0 {
		t.Fatal("cooldown must have an independent numeric primary key", cooldownID, err)
	}
	for _, check := range []struct{ query, code, constraint string }{
		{`UPDATE t_auth_email_code SET delivery_attempts=-1 WHERE verification_id=$1`, "23514", "ck_auth_email_code_delivery_attempts"},
		{`INSERT INTO t_auth_email_code(verification_id,client_ip,purpose,identifier,recipient,credential_version,code_hash,expired_at)
            SELECT verification_id,client_ip,purpose,identifier,recipient,credential_version,code_hash,expired_at FROM t_auth_email_code WHERE verification_id=$1`, "23505", "uk_auth_email_code_verification_id"},
		{`INSERT INTO t_auth_email_cooldown(cooldown_key,retry_at) SELECT cooldown_key,retry_at FROM t_auth_email_cooldown WHERE $1 <> '' LIMIT 1`, "23505", "uk_auth_email_cooldown_key"},
	} {
		_, err = store.DB.ExecContext(ctx, check.query, ticket.VerificationID)
		var constraint *pq.Error
		if !errors.As(err, &constraint) || string(constraint.Code) != check.code || constraint.Constraint != check.constraint {
			t.Fatalf("expected %s, got %v", check.constraint, err)
		}
	}
	stop, err := codes.StartWorkers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	for deadline := time.Now().Add(25 * time.Second); time.Now().Before(deadline); {
		var delivered, consumed bool
		var attempts int
		if err = store.DB.QueryRowContext(ctx, `SELECT delivered,consumed,delivery_attempts FROM t_auth_email_code WHERE verification_id=$1`, ticket.VerificationID).Scan(&delivered, &consumed, &attempts); err != nil {
			t.Fatal(err)
		}
		if delivered {
			if consumed || attempts != 5 || sink.attempts.Load() != 5 {
				t.Fatal("wrong retry outcome", delivered, consumed, attempts)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("fifth SMTP attempt did not complete")
}
