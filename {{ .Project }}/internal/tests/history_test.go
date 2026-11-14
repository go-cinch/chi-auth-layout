package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/modules/auth"
)

type disabledHistorySender struct{}

func (disabledHistorySender) Ready() bool { return false }

func (disabledHistorySender) SendHTML(context.Context, string, string, string, string) error {
	panic("cleanup must not send email with disabled transport")
}

func TestHistoryMaintenanceBaselineSchema(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	var count int
	if err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM information_schema.columns
        WHERE table_schema=current_schema() AND table_name='t_sensitive_operation' AND column_name='operation'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("operation column still present", count, err)
	}
	for name, want := range map[string]bool{
		"idx_user_user_group_relation_user_group_id": false, "idx_action_config": false,
		"idx_auth_pending_notice": false, "idx_user_group_member": true,
		"idx_auth_email_code_expiry": true, "idx_auth_email_code_user": true,
		"idx_auth_email_cooldown_retry": true, "idx_sensitive_scan_created": true,
		"idx_sensitive_operation_scan": true,
	} {
		var exists bool
		if err := store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes
            WHERE schemaname=current_schema() AND indexname=$1)`, name).Scan(&exists); err != nil || exists != want {
			t.Fatal("unexpected index state", name, exists, err)
		}
	}
}

func TestHistoryRetentionAndRiverWithoutSMTP(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UnixMilli()
	old := now - (31 * 24 * time.Hour).Milliseconds()
	recent := now - (29 * 24 * time.Hour).Milliseconds()
	future := now + time.Hour.Milliseconds()
	// More than two batches, plus old-created but recently expired/active tickets.
	exec(`INSERT INTO t_auth_email_code
        (verification_id,created_at,client_ip,purpose,identifier,recipient,credential_version,code_hash,expired_at)
        SELECT 'old-'||n,$1,'','login','','',1,'',$1 FROM generate_series(1,1001) n`, old)
	exec(`INSERT INTO t_auth_email_code
        (verification_id,created_at,client_ip,purpose,identifier,recipient,credential_version,code_hash,expired_at)
        VALUES ('recent',$1,'','login','','',1,'',$2), ('active',$1,'','login','','',1,'',$3)`, old, recent, future)
	exec(`INSERT INTO t_auth_email_cooldown(cooldown_key,created_at,retry_at)
        VALUES ('old',$1,$1),('recent',$1,$2),('renewed',$1,$3)`, old, recent, future)
	exec(`INSERT INTO t_sensitive_scan(id,slot,created_at)
        VALUES (10001,-1,$1),(10002,-2,$2),(10003,-3,$1)`, old, recent)
	exec(`INSERT INTO t_sensitive_operation(user_id,scan_id,operation_kind)
        VALUES (1,10003,'update')`)
	codes, err := auth.NewEmailCodes(store, disabledHistorySender{}, auth.EmailConfig{
		CodeTTL: time.Minute, ResendInterval: time.Minute, SendLimit: 3, MaxAttempts: 3,
	}, strings.Repeat("k", 64))
	if err != nil {
		t.Fatal(err)
	}
	stop, err := codes.StartWorkers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	done := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		var completed int
		if err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM river_job
            WHERE kind='auth_history_cleanup' AND state='completed'`).Scan(&completed); err != nil {
			t.Fatal(err)
		}
		if completed > 0 {
			done = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !done {
		t.Fatal("startup cleanup did not complete without SMTP")
	}
	count := func(query string, want int) {
		t.Helper()
		var got int
		if err := store.DB.QueryRowContext(ctx, query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: got %d want %d: %v", query, got, want, err)
		}
	}
	count(`SELECT count(*) FROM t_auth_email_code`, 2)
	count(`SELECT count(*) FROM t_auth_email_cooldown`, 2)
	count(`SELECT count(*) FROM t_sensitive_scan`, 2)
	count(`SELECT count(*) FROM t_sensitive_operation`, 1)
	count(`SELECT count(*) FROM river_job WHERE kind='auth_sensitive_activity_scan'`, 0)
	// A shorter configured policy changes the cutoff; active state and references survive.
	cfg := auth.EmailConfig{HistoryRetentionDays: 7, CodeTTL: time.Minute, ResendInterval: time.Minute, SendLimit: 3, MaxAttempts: 3}
	shorter, err := auth.NewEmailCodes(store, disabledHistorySender{}, cfg, strings.Repeat("k", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := shorter.CleanupHistory(ctx); err != nil {
		t.Fatal(err)
	}
	count(`SELECT count(*) FROM t_auth_email_code WHERE verification_id='active'`, 1)
	count(`SELECT count(*) FROM t_auth_email_code`, 1)
	count(`SELECT count(*) FROM t_auth_email_cooldown WHERE cooldown_key='renewed'`, 1)
	count(`SELECT count(*) FROM t_auth_email_cooldown`, 1)
	count(`SELECT count(*) FROM t_sensitive_scan WHERE id=10003`, 1)
	count(`SELECT count(*) FROM t_sensitive_scan`, 1)
	if err := shorter.CleanupHistory(ctx); err != nil {
		t.Fatal("repeat cleanup", err)
	}
}
