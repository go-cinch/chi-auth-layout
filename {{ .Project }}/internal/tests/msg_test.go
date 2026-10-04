package tests

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/msg"
)

func TestMessageDeliveryAndIsolation(t *testing.T) {
	store := permissionDatabase(t)
	limits, _ := pagination.New(100, 100)
	m := msg.New(store, limits)
	ctx := t.Context()
	// Stable registration times also exercise exclusion of later registrations.
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET created_at = CURRENT_TIMESTAMP - INTERVAL '1 day'`); err != nil {
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
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_user SET created_at=$1 WHERE id=4`, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(ctx, 4, broadcast.ID); !errors.Is(err, msg.ErrNotFound) {
		t.Fatal("late registration sees old broadcast", err)
	}
	if _, err := store.DB.ExecContext(ctx, `UPDATE t_msg SET published_at=CURRENT_TIMESTAMP-INTERVAL '2 hours', expired_at=CURRENT_TIMESTAMP-INTERVAL '1 hour' WHERE id=$1`, targeted.ID); err != nil {
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
	limits, _ := pagination.New(100, 100)
	messages := msg.New(store, limits)
	manager, err := authn.New("test", strings.Repeat("k", 64), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	authorizer := auth.New(store, manager, nil, nil, nil, nil, nil, auth.Switches{})
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
