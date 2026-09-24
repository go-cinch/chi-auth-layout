package auth

import (
	"errors"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func newSessionTestManager(t *testing.T) *Sessions {
	t.Helper()
	sessions, err := NewSessions(NewMemorySessionStore(), time.Hour, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

func TestSessionsIssueRotateAndRevoke(t *testing.T) {
	sessions := newSessionTestManager(t)
	identity := authn.Identity{CredentialVersion: 1, UserID: 7, Username: "operator", Code: "CODE0007"}
	issued, err := sessions.Issue(t.Context(), identity, true)
	if err != nil || issued.SessionID == "" || issued.RefreshToken == "" || !issued.RememberMe || !issued.ExpiredAt.After(time.Now().Add(29*24*time.Hour)) {
		t.Fatalf("issue = %#v, %v", issued, err)
	}
	if err := sessions.ValidateSession(t.Context(), issued.SessionID); err != nil {
		t.Fatal(err)
	}
	record, err := sessions.ConsumeRefresh(t.Context(), issued.RefreshToken)
	if err != nil || record.identity().UserID != identity.UserID {
		t.Fatalf("consume = %#v, %v", record, err)
	}
	if _, err := sessions.ConsumeRefresh(t.Context(), issued.RefreshToken); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("refresh replay error = %v", err)
	}
	identity.Username = "renamed"
	renewed, err := sessions.Renew(t.Context(), record, identity)
	if err != nil || renewed.RefreshToken == issued.RefreshToken || renewed.SessionID != issued.SessionID {
		t.Fatalf("renew = %#v, %v", renewed, err)
	}
	if err := sessions.RevokeRefresh(t.Context(), renewed.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if err := sessions.ValidateSession(t.Context(), issued.SessionID); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("revoked session error = %v", err)
	}
	if err := sessions.RevokeRefresh(t.Context(), renewed.RefreshToken); err != nil {
		t.Fatal(err)
	}
}

func TestSessionsRejectInvalidValues(t *testing.T) {
	store := NewMemorySessionStore()
	for _, test := range []struct {
		store            SessionStore
		sessionLifetime  time.Duration
		rememberLifetime time.Duration
	}{
		{nil, time.Hour, 24 * time.Hour},
		{store, 0, 24 * time.Hour},
		{store, 24 * time.Hour, time.Hour},
	} {
		if _, err := NewSessions(test.store, test.sessionLifetime, test.rememberLifetime); err == nil {
			t.Fatalf("accepted config: %#v", test)
		}
	}
	sessions := newSessionTestManager(t)
	if _, err := sessions.Issue(t.Context(), authn.Identity{}, false); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("empty identity error = %v", err)
	}
	if _, err := sessions.ConsumeRefresh(t.Context(), "invalid"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("invalid refresh error = %v", err)
	}
	if err := sessions.ValidateSession(t.Context(), ""); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("empty session error = %v", err)
	}
	if _, err := sessions.Renew(t.Context(), refreshSession{}, authn.Identity{}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("invalid renewal error = %v", err)
	}
}

func TestMemorySessionStoreExpiration(t *testing.T) {
	store := NewMemorySessionStore()
	if err := store.Put(t.Context(), "session", "digest", `{}`, -time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), "session"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expired get error = %v", err)
	}
	if _, err := store.TakeRefresh(t.Context(), "digest"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expired refresh error = %v", err)
	}
	if err := store.Delete(t.Context(), "missing", "missing"); err != nil {
		t.Fatal(err)
	}
}

{{ if .Computed.enable_redis_final -}}
func TestRedisSessionStore(t *testing.T) {
	client := &fakeRedisChallengeClient{values: make(map[string]string)}
	store := NewRedisSessionStore(client)
	if err := store.Put(t.Context(), "session", "digest", `{"session_id":"session"}`, time.Hour); err != nil {
		t.Fatal(err)
	}
	if value, err := store.Get(t.Context(), "session"); err != nil || value == "" {
		t.Fatalf("get = %q, %v", value, err)
	}
	if value, err := store.TakeRefresh(t.Context(), "digest"); err != nil || value == "" {
		t.Fatalf("take = %q, %v", value, err)
	}
	if _, err := store.TakeRefresh(t.Context(), "digest"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("replay error = %v", err)
	}
	if err := store.Delete(t.Context(), "session", "digest"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(t.Context(), "session"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("deleted get error = %v", err)
	}
	client.err = errors.New("redis unavailable")
	if err := store.Put(t.Context(), "failed", "failed", `{}`, time.Hour); err == nil {
		t.Fatal("put error was ignored")
	}
	if _, err := store.Get(t.Context(), "failed"); err == nil {
		t.Fatal("get error was ignored")
	}
	if err := store.Delete(t.Context(), "failed", "failed"); err == nil {
		t.Fatal("delete error was ignored")
	}
}
{{ end -}}

func TestSessionCredentialVersion(t *testing.T) {
 sessions := newSessionTestManager(t)
 identity := authn.Identity{UserID:7, Username:"operator", Code:"CODE0007", CredentialVersion:1}
 issued, err := sessions.Issue(t.Context(), identity, false)
 if err != nil { t.Fatal(err) }
 identity.SessionID = issued.SessionID
 if err := sessions.ValidateIdentity(t.Context(), identity); err != nil { t.Fatal(err) }
 record, err := sessions.ConsumeRefresh(t.Context(), issued.RefreshToken)
 if err != nil { t.Fatal(err) }
 identity.CredentialVersion = 2
 if _, err := sessions.Renew(t.Context(), record, identity); !errors.Is(err, ErrSessionNotFound) { t.Fatalf("upgraded stale session: %v", err) }
 identity.CredentialVersion = 0
 if _, err := sessions.Issue(t.Context(), identity, false); !errors.Is(err, ErrSessionNotFound) { t.Fatalf("issued unversioned session: %v", err) }
 // Simulate records written before the upgrade, retaining valid user and digest fields.
 record.CredentialVersion = 0
 legacy, err := sessions.save(t.Context(), record)
 if err != nil { t.Fatal(err) }
 identity.CredentialVersion = 1
 if err := sessions.ValidateIdentity(t.Context(), identity); !errors.Is(err, ErrSessionNotFound) { t.Fatalf("accepted legacy access session: %v", err) }
 if _, err := sessions.ConsumeRefresh(t.Context(), legacy.RefreshToken); !errors.Is(err, ErrSessionNotFound) { t.Fatalf("accepted legacy refresh session: %v", err) }
 if err := sessions.store.Put(t.Context(), identity.SessionID, "digest", "broken json", time.Hour); err != nil { t.Fatal(err) }
 if err := sessions.ValidateIdentity(t.Context(), identity); !errors.Is(err, ErrSessionNotFound) { t.Fatalf("accepted corrupt session: %v", err) }
}
