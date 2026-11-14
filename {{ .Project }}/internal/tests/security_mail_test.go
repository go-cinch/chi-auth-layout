package tests

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
)

func TestSensitiveMailScansCommittedBursts(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	codes, err := newTemplateEmailCodes(store, &emailSink{}, auth.EmailConfig{
		CodeTTL: time.Minute, ResendInterval: time.Minute, SendLimit: 3, MaxAttempts: 3,
		AlertTiers: []auth.AlertTier{
			{Window: 5 * time.Minute, Count: 3}, {Window: 10 * time.Minute, Count: 5}, {Window: 15 * time.Minute, Count: 6},
		},
	}, strings.Repeat("k", 64))
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO t_user(id,username,code,password,status,login_count,email,email_verified_at) VALUES
        (107,'scan','SCAN0107','unused',1,1,'scan@example.test',1),
        (108,'scan2','SCAN0108','unused',1,1,'scan2@example.test',1)`)
	past := time.Now().Add(-8 * time.Minute).UnixMilli()
	add := func(uid, n int, when int64) {
		exec(`INSERT INTO t_sensitive_operation(user_id,created_at,operation_kind,permission_related)
        SELECT $1,$2,'update',TRUE FROM generate_series(1,$3::int)`, uid, when, n)
	}
	add(107, 8, past)
	add(108, 3, past)
	scan := func() {
		results := make(chan error, 4)
		for range 4 {
			go func() { results <- codes.ScanSensitiveOperations(ctx) }()
		}
		for range 4 {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
	}
	scan()
	check := func(uid, count int, phrase string) {
		t.Helper()
		var n int
		if err := store.DB.QueryRowContext(ctx, `SELECT count(*) FROM t_auth_email_code WHERE user_id=$1 AND purpose='sensitive_notice' AND notice_detail LIKE '%' || $2 || '%'`, uid, phrase).Scan(&n); err != nil || n != count {
			t.Fatalf("user %d got %d notices for %q: %v", uid, n, phrase, err)
		}
	}
	check(107, 1, `"update_count": 8`)
	check(107, 1, "")
	check(108, 1, `"update_count": 3`)
	// Simulate the next scheduled scan without waiting ten minutes.
	exec(`UPDATE t_sensitive_scan SET slot=-id`)
	add(107, 1, past+1)
	scan()
	check(107, 1, "")
	exec(`UPDATE t_sensitive_scan SET slot=-id`)
	add(107, 3, past+time.Hour.Milliseconds())
	scan()
	check(107, 2, "")
	check(107, 1, `"update_count": 3`)
}

func TestSensitiveMailSummaryCountsCommittedWindow(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	codes, err := newTemplateEmailCodes(store, &emailSink{}, auth.EmailConfig{
		CodeTTL: time.Minute, ResendInterval: time.Minute, SendLimit: 3, MaxAttempts: 3,
		AlertTiers: []auth.AlertTier{ {Window: 10 * time.Minute, Count: 3}},
	}, strings.Repeat("k", 64))
	if err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO t_user(id,username,code,password,status,login_count,email,email_verified_at) VALUES (109,'summary','SUMM0109','unused',1,1,'summary@example.test',1)`)
	now := time.Now().UnixMilli()
	// An old event and a different actor must not leak into the selected window.
	exec(`INSERT INTO t_sensitive_operation(user_id,operation_kind,permission_related,created_at) VALUES
     (109,'delete',TRUE,$1), (1,'create',TRUE,$2)`, now-11*time.Minute.Milliseconds(), now)
	for _, event := range []struct {
		kind       string
		permission bool
	}{ {"create", false}, {"delete", false}, {"update", true}, {"create", true}, {"update", false}, {"update", false}} {
		exec(`INSERT INTO t_sensitive_operation(user_id,operation_kind,permission_related,created_at) VALUES (109,$1,$2,$3)`, event.kind, event.permission, now)
	}
    // Already processed events on both exact bounds verify [from,to): include
    // the lower bound, exclude the upper bound, without shifting the pending endpoint.
    exec(`INSERT INTO t_sensitive_operation(user_id,operation_kind,created_at,scan_id)
        VALUES (109,'other',$1,-1),(109,'other',$2,-1)`,
        now+1-10*time.Minute.Milliseconds(), now+1)
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO t_sensitive_operation(user_id,operation_kind,permission_related,created_at) VALUES (109,'delete',TRUE,$1)`, now); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = codes.ScanSensitiveOperations(ctx); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err = store.DB.QueryRowContext(ctx, `SELECT notice_detail FROM t_auth_email_code WHERE user_id=109 AND purpose='sensitive_notice'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var counts map[string]int
	if err = json.Unmarshal([]byte(raw), &counts); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int{"version": 2, "create_count": 2, "delete_count": 1, "update_count": 3, "permission_count": 2, "other_count": 1} {
		if counts[key] != want {
			t.Fatalf("%s = %d, want %d", key, counts[key], want)
		}
	}
	if int64(counts["started_at"]) != now+1-10*time.Minute.Milliseconds() || int64(counts["ended_at"]) != now+1 {
		t.Fatalf("wrong stored statistics window: %v", counts)
	}

	// Payload changes must still be sealed, and a repeat scan must not enqueue another notice.
	var signature string
	if err = store.DB.QueryRowContext(ctx, `SELECT delivery_mac FROM t_auth_email_code WHERE user_id=109 AND purpose='sensitive_notice'`).Scan(&signature); err != nil || len(signature) != 64 {
		t.Fatal("unsealed summary", err)
	}
	if err = codes.ScanSensitiveOperations(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = store.DB.QueryRowContext(ctx, `SELECT count(*) FROM t_auth_email_code WHERE user_id=109 AND purpose='sensitive_notice'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("duplicate notice", n, err)
	}
}
