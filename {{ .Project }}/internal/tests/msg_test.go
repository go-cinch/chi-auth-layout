package tests

import (
	"context"
	"errors"
	"fmt"

	"net/http/httptest"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/msg"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMessageDeliveryAndIsolation(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := msg.New(store, limits)
	ctx := t.Context()
	// Stable registration times also exercise exclusion of later registrations.
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET created_at = FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint - 86400000`); err != nil {
		t.Fatal(err)
	}
	broadcast, err := m.Create(ctx, 1, msg.CreateInput{Title: "broadcast", Content: "<script>plain text</script>", Type: "notice", Scope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_msg_recipient WHERE msg_id=$1`, broadcast.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if count, err := m.UnreadCount(ctx, 2); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if _, err := m.Get(ctx, 2, broadcast.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_msg_recipient WHERE msg_id=$1`, broadcast.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal("read query inserted state", n, err)
	}
	if err := m.Read(ctx, 2, broadcast.ID); err != nil {
		t.Fatal(err)
	}
	first, err := m.Get(ctx, 2, broadcast.ID)
	if err != nil || first.ReadAt == nil {
		t.Fatal(first, err)
	}
	if err := m.Read(ctx, 2, broadcast.ID); err != nil {
		t.Fatal(err)
	}
	again, _ := m.Get(ctx, 2, broadcast.ID)
	if *again.ReadAt != *first.ReadAt {
		t.Fatal("read timestamp changed")
	}
	if count, err := m.UnreadCount(ctx, 3); err != nil || count != 1 {
		t.Fatal("cross-user state", count, err)
	}
	for range 2 {
		if err := m.Delete(ctx, 3, broadcast.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Get(ctx, 3, broadcast.ID); !errors.Is(err, msg.ErrNotFound) {
		t.Fatal(err)
	}
	if err := m.ReadAll(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if count, err := m.UnreadCount(ctx, 3); err != nil || count != 0 {
		t.Fatal("deleted broadcast reappeared", count, err)
	}
	targeted, err := m.Create(ctx, 1, msg.CreateInput{Title: "targeted", Content: "body", Type: "system", Scope: "targeted", RecipientIDs: []int64{2, 2}})
	if err != nil {
		t.Fatal(err)
	}
	var recipientName, recipientCode string
	if err := store.DB.QueryRowContext(ctx, `SELECT username, code FROM t_user WHERE id = 2`).Scan(&recipientName, &recipientCode); err != nil {
		t.Fatal(err)
	}
	if len(targeted.Recipients) != 1 || targeted.Recipients[0].Username != recipientName || targeted.Recipients[0].Code != recipientCode {
		t.Fatalf("recipient identity: %#v", targeted.Recipients)
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET username = 'renamed-recipient' WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	current, err := m.GetSent(ctx, targeted.ID)
	if err != nil || len(current.Recipients) != 1 || current.Recipients[0].Username != "renamed-recipient" || current.Recipients[0].Code != recipientCode {
		t.Fatalf("current recipient identity: %#v %v", current, err)
	}
	if _, err := m.Get(ctx, 3, targeted.ID); !errors.Is(err, msg.ErrNotFound) {
		t.Fatal("non-recipient can read", err)
	}
	if err := m.Read(ctx, 3, targeted.ID); !errors.Is(err, msg.ErrNotFound) {
		t.Fatal("non-recipient can mutate", err)
	}
	if err := m.Delete(ctx, 3, targeted.ID); !errors.Is(err, msg.ErrNotFound) {
		t.Fatal("non-recipient can delete", err)
	}
	if _, err := m.Create(ctx, 1, msg.CreateInput{Title: "rollback", Content: "body", Type: "system", Scope: "targeted", RecipientIDs: []int64{2, 999999}}); !errors.Is(err, msg.ErrRecipient) {
		t.Fatal(err)
	}
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_msg WHERE title='rollback'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("partial delivery committed", n, err)
	}
	if err := m.ReadAll(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if count, err := m.UnreadCount(ctx, 2); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET created_at=$1 WHERE id=4`, time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(ctx, 4, broadcast.ID); !errors.Is(err, msg.ErrNotFound) {
		t.Fatal("late registration sees old broadcast", err)
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_msg SET published_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint - 7200000, expired_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint - 3600000 WHERE id=$1`, targeted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(ctx, 2, targeted.ID); !errors.Is(err, msg.ErrNotFound) {
		t.Fatal("expired message visible", err)
	}
	if err := m.DeleteSent(ctx, broadcast.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_msg_recipient WHERE msg_id=$1`, broadcast.ID).Scan(&n); err != nil || n != 0 {
		t.Fatal("orphan states", n, err)
	}
}

func TestMessageHTTPPermissions(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	messages := msg.New(store, limits)
	manager, err := authn.New("test", strings.Repeat("k", 64), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	authorizer := auth.New(store, manager, nil, nil, nil, nil, nil, auth.Switches{}, localizer)
	cfg := &config.Config{}
	cfg.Auth.Authorization.Enabled = true
	cfg.Idempotency.TTL = time.Minute
	handler, err := server.NewRouter(cfg, manager, idempotency.NewMemoryStore(), authorizer, messages)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(authn.Identity{UserID: 2, Username: "guest", Code: "12345678", CredentialVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/msg/inbox", "", 200}, {"GET", "/msg/unread-count", "", 200},
		{"GET", "/msg/inbox/999", "", 404}, {"PATCH", "/msg/inbox/999", `{"read":true}`, 404},
		{"DELETE", "/msg/inbox/999", "", 404}, {"POST", "/msg/inbox/read-all", "", 200},
		{"POST", "/msg", `{}`, 403}, {"GET", "/msg/sent", "", 403},
		{"GET", "/msg/sent/1", "", 403}, {"DELETE", "/msg/sent/1", "", 403},
		{"GET", "/msg/recipient-option", "", 403},
	} {
		r := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != item.status {
			t.Fatalf("%s %s: %d %s", item.method, item.path, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/msg/inbox", nil))
	if w.Code != 401 {
		t.Fatalf("anonymous inbox: %d", w.Code)
	}
}

// Status must remain a compact, exact snapshot after every delivery/state change.
func TestMessageStatusLifecycle(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := msg.New(store, limits)
	ctx := t.Context()
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET created_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint - 86400000`); err != nil {
		t.Fatal(err)
	}
	check := func(userID, count int64, latest *int64) {
		t.Helper()
		value, err := m.Status(ctx, userID)
		if err != nil {
			t.Fatal(err)
		}
		if value.UserID != userID || value.Count != count || (latest == nil) != (value.LatestUnreadAt == nil) || (latest != nil && *latest != *value.LatestUnreadAt) {
			t.Fatalf("status for %d: %#v; want count=%d latest=%v", userID, value, count, latest)
		}
	}
	check(2, 0, nil)
	first, err := m.Create(ctx, 1, msg.CreateInput{Title: "broadcast", Content: "one", Type: "notice", Scope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	check(2, 1, &first.PublishedAt)
	second, err := m.Create(ctx, 1, msg.CreateInput{Title: "targeted", Content: "two", Type: "system", Scope: "targeted", RecipientIDs: []int64{2}})
	if err != nil {
		t.Fatal(err)
	}
	check(2, 2, &second.PublishedAt)
	check(3, 1, &first.PublishedAt)
	if err := m.Read(ctx, 2, second.ID); err != nil {
		t.Fatal(err)
	}
	check(2, 1, &first.PublishedAt)
	if err := m.Delete(ctx, 2, first.ID); err != nil {
		t.Fatal(err)
	}
	check(2, 0, nil)
	if err := m.DeleteSent(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	check(3, 0, nil)
	expiry := time.Now().Add(300 * time.Millisecond).UnixMilli()
	expiring, err := m.Create(ctx, 1, msg.CreateInput{Title: "expires", Content: "three", Type: "notice", Scope: "all", ExpiredAt: &expiry})
	if err != nil {
		t.Fatal(err)
	}
	check(2, 1, &expiring.PublishedAt)
	// PostgreSQL owns expiry visibility; wait on its clock, as in the future-publication test.
	waitCtx, cancelWait := context.WithTimeout(ctx, 2*time.Second)
	_, err = store.DB.ExecContext(waitCtx, `SELECT pg_sleep(GREATEST(0, ($1::bigint - FLOOR(EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::bigint) / 1000.0)+0.02)`, expiry)
	cancelWait()
	if err != nil {
		t.Fatal(err)
	}

	check(2, 0, nil)
	check(3, 0, nil)
	var late int64
	if err := store.DB.QueryRowContext(ctx, `INSERT INTO t_user(username,code,password) VALUES ('late','LATE0001','test') RETURNING id`).Scan(&late); err != nil {
		t.Fatal(err)
	}
	check(late, 0, nil)
}

// Concurrent writers must update the materialized row in the same commit as
// delivery/receipts. Compare it with inbox truth without invoking Status repair.
func TestMessagePersistedStatusConcurrency(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := msg.New(store, limits)
	ctx := t.Context()
	var wg sync.WaitGroup
	failures := make(chan error, 32)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := msg.CreateInput{Title: fmt.Sprint("concurrent-", i), Content: "body", Type: "notice", Scope: "all"}
			if i%2 == 0 {
				input.Scope = "targeted"
				input.RecipientIDs = []int64{2, 3}
			}
			value, err := m.Create(ctx, 1, input)
			if err != nil {
				failures <- err
				return
			}
			if i%3 == 0 {
				err = m.Read(ctx, 2, value.ID)
			} else if i%3 == 1 {
				err = m.Delete(ctx, 2, value.ID)
			} else {
				err = m.DeleteSent(ctx, value.ID)
			}
			if err != nil {
				failures <- err
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	assertStored := func(userID int64) {
		t.Helper()
		var stored, actual int64
		var same bool
		err := store.DB.QueryRowContext(ctx, `SELECT s.unread_count, COUNT(m.id), s.latest_unread_at IS NOT DISTINCT FROM MAX(m.published_at)
 FROM t_user_msg_status s JOIN t_user u ON u.id=s.user_id
 LEFT JOIN t_msg m ON (m.scope='all' AND m.published_at>=u.created_at OR EXISTS(SELECT 1 FROM t_msg_recipient d WHERE d.msg_id=m.id AND d.user_id=u.id))
 AND NOT EXISTS(SELECT 1 FROM t_msg_recipient r WHERE r.msg_id=m.id AND r.user_id=u.id AND (r.read_at IS NOT NULL OR r.deleted_at IS NOT NULL))
 WHERE s.user_id=$1 GROUP BY s.unread_count,s.latest_unread_at`, userID).Scan(&stored, &actual, &same)
		if err != nil || stored != actual || !same {
			t.Fatalf("persisted user %d: stored=%d actual=%d latest=%t error=%v", userID, stored, actual, same, err)
		}
	}
	assertStored(2)
	assertStored(3)
	// A partially delivered invalid recipient set must roll back counters too.
	if _, err := m.Create(ctx, 1, msg.CreateInput{Title: "rollback", Content: "body", Type: "notice", Scope: "targeted", RecipientIDs: []int64{2, 999999}}); !errors.Is(err, msg.ErrRecipient) {
		t.Fatal(err)
	}
	assertStored(2)
	if err := m.ReadAll(ctx, 2); err != nil {
		t.Fatal(err)
	}
	assertStored(2)
}

func TestMessageStatusExpiryWorkerAndBackfill(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := msg.New(store, limits)
	ctx := t.Context()
	first, err := m.Create(ctx, 1, msg.CreateInput{Title: "lasting", Content: "body", Type: "notice", Scope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(200 * time.Millisecond).UnixMilli()
	if _, err := m.Create(ctx, 1, msg.CreateInput{Title: "expires", Content: "body", Type: "system", Scope: "all", ExpiredAt: &expiry}); err != nil {
		t.Fatal(err)
	}
	// PostgreSQL owns expiry visibility; wait on its clock, as in the future-publication test.
	waitCtx, cancelWait := context.WithTimeout(ctx, 2*time.Second)
	_, err = store.DB.ExecContext(waitCtx, `SELECT pg_sleep(GREATEST(0, ($1::bigint - FLOOR(EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::bigint) / 1000.0)+0.02)`, expiry)
	cancelWait()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ExpireStatus(ctx); err != nil {
		t.Fatal(err)
	}
	var count int64
	var latest int64
	if err := store.DB.QueryRowContext(ctx, `SELECT unread_count,latest_unread_at FROM t_user_msg_status WHERE user_id=2`).Scan(&count, &latest); err != nil || count != 1 || latest != first.PublishedAt {
		t.Fatal(count, latest, err)
	}
	if err := m.Delete(ctx, 3, first.ID); err != nil {
		t.Fatal(err)
	}
	// Rebuild derived rows from source messages after a lost statistics table entry.
	if _, err := store.DB.ExecContext(ctx, `DELETE FROM t_user_msg_status`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{2, 3} {
		if _, err := m.Status(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	for userID, want := range map[int64]int64{2: 1, 3: 0} {
		if err := store.DB.QueryRowContext(ctx, `SELECT unread_count FROM t_user_msg_status WHERE user_id=$1`, userID).Scan(&count); err != nil || count != want {
			t.Fatal(userID, count, err)
		}
	}
	if _, err := store.DB.ExecContext(ctx, `DELETE FROM t_user WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_user_msg_status WHERE user_id=2`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

// Simulate a publication from an older writer whose host clock was ahead.
// Both polling and the worker must discover it without another message mutation.
func TestMessageStatusFuturePublication(t *testing.T) {
	for _, scope := range []string{"all", "targeted"} {
		for _, repair := range []string{"read-all", "initialize", "rebuild"} {
			t.Run(scope+"/"+repair, func(t *testing.T) {
				store := permissionDatabase(t)
				localizer := testLocalization(t, store)
				_ = localizer
				limits, _ := pagination.New(100, 100)
				m := msg.New(store, limits)
				ctx := t.Context()
				input := msg.CreateInput{Title: "future", Content: "body", Type: "notice", Scope: scope}
				if scope == "targeted" {
					input.RecipientIDs = []int64{2, 3}
				}
				value, err := m.Create(ctx, 1, input)
				if err != nil {
					t.Fatal(err)
				}
				var published, expiry int64
				if err := store.DB.QueryRowContext(ctx, `UPDATE t_msg SET published_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint + 2000, expired_at=FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint + 3600000 WHERE id=$1 RETURNING published_at,expired_at`, value.ID).Scan(&published, &expiry); err != nil {
					t.Fatal(err)
				}
				switch repair {
				case "read-all":
					for _, userID := range []int64{2, 3} {
						if err := m.ReadAll(ctx, userID); err != nil {
							t.Fatal(err)
						}
					}
				case "initialize":
					if _, err := store.DB.ExecContext(ctx, `DELETE FROM t_user_msg_status`); err != nil {
						t.Fatal(err)
					}
				case "rebuild":
					if _, err := store.DB.ExecContext(ctx, `DELETE FROM t_user_msg_status`); err != nil {
						t.Fatal(err)
					}
				}
				for _, userID := range []int64{2, 3} {
					status, err := m.Status(ctx, userID)
					if err != nil || status.Count != 0 || status.LatestUnreadAt != nil {
						t.Fatal(status, err)
					}
					var deadline int64
					if err := store.DB.QueryRowContext(ctx, `SELECT next_refresh_at FROM t_user_msg_status WHERE user_id=$1`, userID).Scan(&deadline); err != nil || deadline != published {
						t.Fatalf("missing publication deadline: %v %v", deadline, err)
					}
				}
				if _, err := m.Get(ctx, 2, value.ID); !errors.Is(err, msg.ErrNotFound) {
					t.Fatalf("future publication is visible: %v", err)
				}
				// Wait on the database clock, independent of the test host's clock.
				if _, err := store.DB.ExecContext(ctx, `SELECT pg_sleep(GREATEST(0, ($1::bigint - FLOOR(EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::bigint) / 1000.0)+0.02)`, published); err != nil {
					t.Fatal(err)
				}
				status, err := m.Status(ctx, 2)
				if err != nil || status.Count != 1 || status.LatestUnreadAt == nil || *status.LatestUnreadAt != published {
					t.Fatalf("poll did not discover publication: %#v %v", status, err)
				}
				if err := m.ExpireStatus(ctx); err != nil {
					t.Fatal(err)
				}
				for _, userID := range []int64{2, 3} {
					var count int64
					var deadline int64
					if err := store.DB.QueryRowContext(ctx, `SELECT unread_count,next_refresh_at FROM t_user_msg_status WHERE user_id=$1`, userID).Scan(&count, &deadline); err != nil || count != 1 || deadline != expiry {
						t.Fatalf("user %d: count=%d deadline=%v error=%v", userID, count, deadline, err)
					}
					unread := false
					inbox, err := m.List(ctx, userID, msg.ListInput{Read: &unread})
					if err != nil || inbox.Total != 1 {
						t.Fatalf("inbox disagrees: %#v %v", inbox, err)
					}
				}
			})
		}
	}
}

func TestMessageCreateChecksExpiryAfterLock(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	limits, _ := pagination.New(100, 100)
	m := msg.New(store, limits)
	ctx := t.Context()
	tx, err := store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1735290211, 1)`); err != nil {
		t.Fatal(err)
	}
	var expiry int64
	if err := tx.QueryRowContext(ctx, `SELECT (EXTRACT(EPOCH FROM clock_timestamp())*1000)::bigint+200`).Scan(&expiry); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := m.Create(ctx, 1, msg.CreateInput{Title: "expires while waiting", Content: "body", Type: "notice", Scope: "all", ExpiredAt: &expiry})
		done <- err
	}()
	if _, err := tx.ExecContext(ctx, `SELECT pg_sleep(0.3)`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, msg.ErrInvalid) {
		t.Fatalf("expired send accepted: %v", err)
	}
	var count int64
	if err := store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM t_msg`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestMessageAndRegistrationShareDatabaseClock(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	ctx := t.Context()
	// Require the statement's exact timestamp, so passing an application timestamp
	// fails even when the database and application clocks are almost synchronized.
	_, err := store.DB.ExecContext(ctx, `CREATE FUNCTION require_statement_time() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN
  IF NEW.created_at IS DISTINCT FROM FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint
   OR NEW.updated_at IS DISTINCT FROM FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint THEN
   RAISE EXCEPTION 'creation must use the database clock';
  END IF;
  RETURN NEW;
 END $$;
 CREATE TRIGGER user_clock BEFORE INSERT ON t_user FOR EACH ROW EXECUTE FUNCTION require_statement_time();
 CREATE TRIGGER message_clock BEFORE INSERT ON t_msg FOR EACH ROW EXECUTE FUNCTION require_statement_time();`)
	if err != nil {
		t.Fatal(err)
	}
	limits, _ := pagination.New(100, 100)
	messages := msg.New(store, limits)
	if _, err := messages.Create(ctx, 1, msg.CreateInput{Title: "before registration", Content: "body", Type: "notice", Scope: "all"}); err != nil {
		t.Fatal(err)
	}
	authentication := auth.New(store, nil, nil, nil, nil, nil, nil, auth.Switches{}, localizer)
	if err := authentication.Register(ctx, auth.RegisterInput{Username: "clock-registered", Password: "secret123"}); err != nil {
		t.Fatal(err)
	}
	var registeredID int64
	if err := store.DB.QueryRowContext(ctx, `SELECT id FROM t_user WHERE username='clock-registered'`).Scan(&registeredID); err != nil {
		t.Fatal(err)
	}
	actions := action.New(store, limits, false, localizer)
	roles := role.New(store, limits, actions, false)
	users := user.New(store, limits, nil, nil, actions, roles, auth.Switches{})
	created, err := users.Create(ctx, user.CreateUserInput{Username: "clock-created", Password: "secret123"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := messages.Create(ctx, 1, msg.CreateInput{Title: "after registration", Content: "body", Type: "notice", Scope: "all"})
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []int64{registeredID, created.ID} {
		status, err := messages.Status(ctx, userID)
		if err != nil || status.Count != 1 || status.LatestUnreadAt == nil || *status.LatestUnreadAt != message.PublishedAt {
			t.Fatalf("registration cutoff: %#v %v", status, err)
		}
	}
}

func TestInboxIDFilterIsolation(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	ctx := t.Context()
	limits, _ := pagination.New(100, 100)
	m := msg.New(store, limits)
	message, err := m.Create(ctx, 1, msg.CreateInput{Title: "same title", Content: "target", Type: "notice", Scope: "targeted", RecipientIDs: []int64{2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Create(ctx, 1, msg.CreateInput{Title: "same title", Content: "other", Type: "notice", Scope: "targeted", RecipientIDs: []int64{2}}); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []int64{2, 3} {
		value, err := m.List(ctx, userID, msg.ListInput{ID: &message.ID})
		if err != nil {
			t.Fatal(err)
		}
		if userID == 2 {
			if value.Total != 1 || len(value.Items) != 1 || value.Items[0].ID != message.ID {
				t.Fatalf("%+v", value)
			}
		} else if value.Total != 0 || len(value.Items) != 0 {
			t.Fatal("ID filter exposed another inbox")
		}
	}
	value, err := m.List(ctx, 2, msg.ListInput{ID: &message.ID, Query: "missing"})
	if err != nil || value.Total != 0 {
		t.Fatalf("combined filter: %+v %v", value, err)
	}
	if err := m.Delete(ctx, 2, message.ID); err != nil {
		t.Fatal(err)
	}
	value, err = m.List(ctx, 2, msg.ListInput{ID: &message.ID})
	if err != nil || value.Total != 0 {
		t.Fatalf("deleted message returned: %+v %v", value, err)
	}
}
