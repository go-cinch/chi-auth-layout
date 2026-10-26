package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func sessionTestConfig() SessionConfig {
	return SessionConfig{
		Issuer: "session-test", Key: strings.Repeat("k", 64),
		SessionLifetime: time.Hour, RememberLifetime: 30 * 24 * time.Hour, RetryWindow: 10 * time.Second,
	}
}

func newSessionTestManager(t *testing.T) *Sessions {
	t.Helper()
	store := NewMemorySessionStore()
	t.Cleanup(func() { _ = store.Close() })
	sessions, err := NewSessions(store, sessionTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

func issueSessionForTest(t *testing.T, rememberMe bool) (*Sessions, *SessionToken, authn.Identity) {
	t.Helper()
	sessions := newSessionTestManager(t)
	identity := authn.Identity{UserID: 7, Username: "operator", Code: "CODE0007", CredentialVersion: 1}
	issued, err := sessions.Issue(t.Context(), identity, rememberMe)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = issued.SessionID
	return sessions, issued, identity
}

// Change signed timestamps to exercise exact state boundaries without sleeps.
func rewriteSessionForTest(t *testing.T, sessions *Sessions, sessionID string, change func(*refreshSession)) {
	t.Helper()
	current, err := sessions.read(t.Context(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	expected := current.raw
	deadline := time.UnixMilli(current.ExpiredAt)
	change(&current)
	value, err := sessions.encodeRecord(current)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := sessions.store.Rotate(t.Context(), sessions.storageID(sessionID), expected, value, deadline); err != nil || !changed {
		t.Fatalf("rewrite signed test state = %v, %v", changed, err)
	}
}

func TestSessionsIssueRotateAndRevoke(t *testing.T) {
	sessions, issued, identity := issueSessionForTest(t, true)
	if issued.SessionID == "" || issued.RefreshToken == "" || !issued.RememberMe || !issued.ExpiredAt.After(time.Now().Add(29*24*time.Hour)) {
		t.Fatalf("issued session = %#v", issued)
	}
	if err := sessions.ValidateIdentity(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	record, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
	if err != nil || record.Generation != 1 || record.identity() != identity {
		t.Fatalf("read = %#v, %v", record, err)
	}
	if _, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken); err != nil {
		t.Fatalf("reading consumed a refresh token: %v", err)
	}
	renewed, err := sessions.Renew(t.Context(), record, identity)
	if err != nil || renewed.RefreshToken == issued.RefreshToken || renewed.SessionID != issued.SessionID || !renewed.ExpiredAt.Equal(issued.ExpiredAt) {
		t.Fatalf("renew = %#v, %v", renewed, err)
	}
	rotated, err := sessions.ReadRefresh(t.Context(), renewed.RefreshToken)
	if err != nil || rotated.Generation != 2 {
		t.Fatalf("rotated state = %#v, %v", rotated, err)
	}
	for _, token := range []string{issued.RefreshToken, renewed.RefreshToken, issued.RefreshToken} {
		retryRecord, err := sessions.ReadRefresh(t.Context(), token)
		if err != nil {
			t.Fatal(err)
		}
		retry, err := sessions.Renew(t.Context(), retryRecord, identity)
		if err != nil || retry.RefreshToken != renewed.RefreshToken || !retry.ExpiredAt.Equal(issued.ExpiredAt) {
			t.Fatalf("retry diverged or extended expiry: %#v, %v", retry, err)
		}
	}
	afterRetries, err := sessions.read(t.Context(), issued.SessionID)
	if err != nil || afterRetries.Generation != 2 || afterRetries.RotatedAt != rotated.RotatedAt {
		t.Fatalf("retry moved the rotation window: %#v, %v", afterRetries, err)
	}
	if err := sessions.RevokeRefresh(t.Context(), issued.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if err := sessions.ValidateIdentity(t.Context(), identity); !errors.Is(err, authn.ErrUnauthorized) {
		t.Fatalf("revoked identity = %v", err)
	}
	if err := sessions.RevokeRefresh(t.Context(), renewed.RefreshToken); err != nil {
		t.Fatalf("repeated logout = %v", err)
	}
}

func TestSessionsConcurrentRefreshConverges(t *testing.T) {
	sessions, issued, identity := issueSessionForTest(t, false)
	record, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		token *SessionToken
		err   error
	}
	const count = 32
	start := make(chan struct{})
	results := make(chan outcome, count)
	for range count {
		go func() {
			<-start
			token, err := sessions.Renew(t.Context(), record, identity)
			results <- outcome{token, err}
		}()
	}
	close(start)
	var current string
	for range count {
		result := <-results
		if result.err != nil || result.token == nil {
			t.Fatalf("concurrent refresh = %#v, %v", result.token, result.err)
		}
		if current == "" {
			current = result.token.RefreshToken
		}
		if result.token.RefreshToken != current || !result.token.ExpiredAt.Equal(issued.ExpiredAt) {
			t.Fatalf("concurrent refresh diverged: %#v", result.token)
		}
	}
	if current == issued.RefreshToken {
		t.Fatal("concurrent refresh did not rotate")
	}
}

func TestSessionsPendingRefreshCannotUndoRevocation(t *testing.T) {
	for _, logout := range []bool{false, true} {
		name := "revoke"
		if logout {
			name = "logout"
		}
		t.Run(name, func(t *testing.T) {
			sessions, issued, identity := issueSessionForTest(t, false)
			record, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
			if err != nil {
				t.Fatal(err)
			}
			if logout {
				err = sessions.RevokeRefresh(t.Context(), issued.RefreshToken)
			} else {
				err = sessions.Revoke(t.Context(), issued.SessionID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := sessions.ValidateSession(t.Context(), issued.SessionID); !errors.Is(err, ErrSessionNotFound) {
				t.Fatalf("session was not deleted before pending refresh: %v", err)
			}
			if _, err := sessions.Renew(t.Context(), record, identity); !errors.Is(err, ErrSessionNotFound) {
				t.Fatalf("pending refresh revived revoked session: %v", err)
			}
			if err := sessions.ValidateIdentity(t.Context(), identity); !errors.Is(err, authn.ErrUnauthorized) {
				t.Fatalf("revoked identity still usable: %v", err)
			}
		})
	}
}

func TestSessionsRetryWindowAndReplayRevocation(t *testing.T) {
	for _, oldGeneration := range []int64{1, 2} {
		t.Run(string(rune('0'+oldGeneration)), func(t *testing.T) {
			sessions, issued, identity := issueSessionForTest(t, false)
			record, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
			if err != nil {
				t.Fatal(err)
			}
			second, err := sessions.Renew(t.Context(), record, identity)
			if err != nil {
				t.Fatal(err)
			}
			rewriteSessionForTest(t, sessions, issued.SessionID, func(current *refreshSession) {
				current.RotatedAt = time.Now().Add(-sessions.retryWindow - time.Millisecond).UnixMilli()
				current.CreatedAt = current.RotatedAt - 1
			})
			if oldGeneration == 2 {
				current, err := sessions.ReadRefresh(t.Context(), second.RefreshToken)
				if err != nil {
					t.Fatal(err)
				}
				third, err := sessions.Renew(t.Context(), current, identity)
				if err != nil || third.RefreshToken == second.RefreshToken || !third.ExpiredAt.Equal(issued.ExpiredAt) {
					t.Fatalf("rotation after retry window = %#v, %v", third, err)
				}
			}
			if _, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken); !errors.Is(err, ErrSessionNotFound) {
				t.Fatalf("old token accepted after retry window: %v", err)
			}
			if err := sessions.ValidateIdentity(t.Context(), identity); !errors.Is(err, authn.ErrUnauthorized) {
				t.Fatalf("replay did not revoke the family: %v", err)
			}
		})
	}
}

func TestSessionsAcceptedRefreshCrossesRetryBoundary(t *testing.T) {
	for _, readAfterRotation := range []bool{false, true} {
		name := "accepted_before_rotation"
		if readAfterRotation {
			name = "accepted_during_retry_window"
		}
		t.Run(name, func(t *testing.T) {
			sessions, issued, identity := issueSessionForTest(t, false)
			pending, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
			if err != nil {
				t.Fatal(err)
			}
			second, err := sessions.Renew(t.Context(), pending, identity)
			if err != nil {
				t.Fatal(err)
			}
			if readAfterRotation {
				pending, err = sessions.ReadRefresh(t.Context(), issued.RefreshToken)
				if err != nil {
					t.Fatal(err)
				}
			}
			rewriteSessionForTest(t, sessions, issued.SessionID, func(current *refreshSession) {
				current.RotatedAt = time.Now().Add(-sessions.retryWindow - time.Millisecond).UnixMilli()
				current.CreatedAt = current.RotatedAt - 1
			})
			completed, err := sessions.Renew(t.Context(), pending, identity)
			if err != nil || completed.RefreshToken != second.RefreshToken || !completed.ExpiredAt.Equal(issued.ExpiredAt) {
				t.Fatalf("accepted in-flight refresh crossed retry boundary: %#v, %v", completed, err)
			}
			if err := sessions.ValidateIdentity(t.Context(), identity); err != nil {
				t.Fatalf("slow in-flight refresh revoked the identity: %v", err)
			}
		})
	}
}

func TestSessionsStaleInFlightDoesNotRevokeNewerGeneration(t *testing.T) {
	sessions, issued, identity := issueSessionForTest(t, false)
	pending, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sessions.Renew(t.Context(), pending, identity)
	if err != nil {
		t.Fatal(err)
	}
	rewriteSessionForTest(t, sessions, issued.SessionID, func(current *refreshSession) {
		current.RotatedAt = time.Now().Add(-sessions.retryWindow - time.Millisecond).UnixMilli()
		current.CreatedAt = current.RotatedAt - 1
	})
	current, err := sessions.ReadRefresh(t.Context(), second.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	third, err := sessions.Renew(t.Context(), current, identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Renew(t.Context(), pending, identity); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("request from two generations ago accepted: %v", err)
	}
	if err := sessions.ValidateIdentity(t.Context(), identity); err != nil {
		t.Fatalf("stale in-flight request revoked a newer session: %v", err)
	}
	if _, err := sessions.ReadRefresh(t.Context(), third.RefreshToken); err != nil {
		t.Fatalf("newest refresh token invalidated: %v", err)
	}
}

type sessionDelayedReadStore struct {
	SessionStore
	afterRead func(context.Context, string, string) (string, error)
}

func (s *sessionDelayedReadStore) Get(ctx context.Context, id string) (string, error) {
	value, err := s.SessionStore.Get(ctx, id)
	if err != nil || s.afterRead == nil {
		return value, err
	}
	return s.afterRead(ctx, id, value)
}

func TestSessionsSlowStorageReadCrossesRetryBoundary(t *testing.T) {
	sessions, issued, identity := issueSessionForTest(t, false)
	sessions.retryWindow = 5 * time.Millisecond
	first, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	second, err := sessions.Renew(t.Context(), first, identity)
	if err != nil {
		t.Fatal(err)
	}
	backend := sessions.store
	delayed := &sessionDelayedReadStore{SessionStore: backend}
	var boundary int64
	delayed.afterRead = func(ctx context.Context, id, value string) (string, error) {
		current, err := sessions.decodeRecord(value)
		if err != nil {
			return "", err
		}
		// The request has already entered ReadRefresh. Reset the signed window
		// here, then delay only the storage response until that window closes.
		current.RotatedAt = time.Now().UnixMilli()
		boundary = current.RotatedAt + sessions.retryWindow.Milliseconds()
		updated, err := sessions.encodeRecord(current)
		if err != nil {
			return "", err
		}
		if changed, err := backend.Rotate(ctx, id, value, updated, time.UnixMilli(current.ExpiredAt)); err != nil || !changed {
			return "", errors.New("cannot establish delayed storage fixture")
		}
		timer := time.NewTimer(time.Until(time.UnixMilli(boundary)))
		defer timer.Stop()
		select {
		case <-timer.C:
			return updated, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	sessions.store = delayed
	accepted, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
	if err != nil {
		t.Fatalf("slow storage response turned an accepted retry into replay: %v", err)
	}
	if time.Now().UnixMilli() < boundary {
		t.Fatal("storage fixture did not cross the retry boundary")
	}
	delayed.afterRead = nil
	completed, err := sessions.Renew(t.Context(), accepted, identity)
	if err != nil || completed.RefreshToken != second.RefreshToken {
		t.Fatalf("slow-read retry = %#v, %v", completed, err)
	}
	if err := sessions.ValidateIdentity(t.Context(), identity); err != nil {
		t.Fatalf("slow storage response revoked the session: %v", err)
	}
}

func TestSessionsRejectInvalidValues(t *testing.T) {
	store := NewMemorySessionStore()
	t.Cleanup(func() { _ = store.Close() })
	if _, err := NewSessions(nil, sessionTestConfig()); err == nil {
		t.Fatal("nil store accepted")
	}
	for name, change := range map[string]func(*SessionConfig){
		"issuer":                func(c *SessionConfig) { c.Issuer = " " },
		"key":                   func(c *SessionConfig) { c.Key = "short" },
		"session lifetime":      func(c *SessionConfig) { c.SessionLifetime = 0 },
		"remember lifetime":     func(c *SessionConfig) { c.RememberLifetime = time.Minute },
		"zero retry window":     func(c *SessionConfig) { c.RetryWindow = 0 },
		"long retry window":     func(c *SessionConfig) { c.RetryWindow = 2 * time.Minute },
		"retry exceeds session": func(c *SessionConfig) { c.SessionLifetime, c.RetryWindow = time.Second, time.Second },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := sessionTestConfig()
			change(&cfg)
			if _, err := NewSessions(store, cfg); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	sessions, issued, _ := issueSessionForTest(t, false)
	for _, identity := range []authn.Identity{
		{},
		{UserID: 1, Username: "u", Code: "c"},
		{UserID: 1, CredentialVersion: 1, Username: " ", Code: "c"},
	} {
		if _, err := sessions.Issue(t.Context(), identity, false); !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("invalid identity accepted: %#v, %v", identity, err)
		}
	}
	parts := strings.Split(issued.RefreshToken, ".")
	for _, token := range []string{"", "invalid", strings.Repeat("a", 43), issued.RefreshToken + "!", strings.Repeat("a", 129), strings.Join([]string{parts[0], parts[1], "01", parts[3]}, ".")} {
		if _, err := sessions.ReadRefresh(t.Context(), token); !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("invalid token accepted: %q, %v", token, err)
		}
	}
	if err := sessions.ValidateSession(t.Context(), ""); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("empty session accepted: %v", err)
	}
	if _, err := sessions.Renew(t.Context(), refreshSession{}, authn.Identity{}); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("empty renewal accepted: %v", err)
	}
	if err := sessions.Revoke(t.Context(), "invalid"); err != nil {
		t.Fatalf("invalid session revocation = %v", err)
	}
	if err := sessions.RevokeRefresh(t.Context(), "invalid"); err != nil {
		t.Fatalf("invalid token logout = %v", err)
	}
}

func TestSessionCredentialVersionAndIssuerBinding(t *testing.T) {
	sessions, issued, identity := issueSessionForTest(t, false)
	record, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	changed := identity
	changed.CredentialVersion++
	if _, err := sessions.Renew(t.Context(), record, changed); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("changed credential version accepted: %v", err)
	}
	if err := sessions.ValidateIdentity(t.Context(), changed); !errors.Is(err, authn.ErrUnauthorized) {
		t.Fatalf("changed identity accepted: %v", err)
	}
	for name, change := range map[string]func(*SessionConfig){
		"issuer": func(c *SessionConfig) { c.Issuer = "another-service" },
		"key":    func(c *SessionConfig) { c.Key = strings.Repeat("z", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := sessionTestConfig()
			change(&cfg)
			other, err := NewSessions(sessions.store, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := other.ReadRefresh(t.Context(), issued.RefreshToken); !errors.Is(err, ErrSessionNotFound) {
				t.Fatalf("foreign refresh token accepted: %v", err)
			}
			if err := other.RevokeRefresh(t.Context(), issued.RefreshToken); err != nil {
				t.Fatal(err)
			}
			if err := sessions.ValidateIdentity(t.Context(), identity); err != nil {
				t.Fatalf("foreign manager revoked another namespace: %v", err)
			}
		})
	}
	parts := strings.Split(issued.RefreshToken, ".")
	payload := strings.Join(parts[:3], ".")
	wrongPurpose := payload + "." + sessions.sign("record", payload)
	if _, err := sessions.ReadRefresh(t.Context(), wrongPurpose); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("record MAC accepted as refresh MAC: %v", err)
	}
}

func TestSessionsRejectTamperedAndExpiredRecords(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"identity": func(value map[string]any) { value["username"] = "attacker" },
		"version":  func(value map[string]any) { value["credential_version"] = float64(2) },
		"expiry": func(value map[string]any) {
			value["expired_at"] = float64(time.Now().Add(365 * 24 * time.Hour).UnixMilli())
		},
		"signature": func(value map[string]any) { delete(value, "signature") },
	} {
		t.Run(name, func(t *testing.T) {
			sessions, issued, identity := issueSessionForTest(t, false)
			id := sessions.storageID(issued.SessionID)
			original, err := sessions.store.Get(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			var data map[string]any
			if err := json.Unmarshal([]byte(original), &data); err != nil {
				t.Fatal(err)
			}
			mutate(data)
			tampered, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			if changed, err := sessions.store.Rotate(t.Context(), id, original, string(tampered), issued.ExpiredAt); err != nil || !changed {
				t.Fatalf("inject tampered record = %v, %v", changed, err)
			}
			if _, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken); !errors.Is(err, ErrSessionNotFound) {
				t.Fatalf("tampered record refreshed: %v", err)
			}
			if err := sessions.ValidateIdentity(t.Context(), identity); !errors.Is(err, authn.ErrUnauthorized) {
				t.Fatalf("tampered identity authenticated: %v", err)
			}
			if err := sessions.RevokeRefresh(t.Context(), issued.RefreshToken); err != nil {
				t.Fatalf("logout could not delete corrupt record: %v", err)
			}
			if _, err := sessions.store.Get(t.Context(), id); !errors.Is(err, ErrSessionNotFound) {
				t.Fatalf("corrupt record retained after logout: %v", err)
			}
		})
	}
	sessions, issued, identity := issueSessionForTest(t, false)
	rewriteSessionForTest(t, sessions, issued.SessionID, func(current *refreshSession) {
		current.CreatedAt = time.Now().Add(-time.Hour).UnixMilli()
		current.ExpiredAt = time.Now().Add(-time.Millisecond).UnixMilli()
	})
	if err := sessions.ValidateIdentity(t.Context(), identity); !errors.Is(err, authn.ErrUnauthorized) {
		t.Fatalf("expired signed record authenticated: %v", err)
	}
	if _, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expired signed record refreshed: %v", err)
	}
}

type sessionFailureStore struct {
	SessionStore
	getErr       error
	createErr    error
	deleteErr    error
	rotateErr    error
	commitRotate bool
	conflict     bool
}

func (s *sessionFailureStore) Get(ctx context.Context, id string) (string, error) {
	if s.getErr != nil {
		return "", s.getErr
	}
	return s.SessionStore.Get(ctx, id)
}

func (s *sessionFailureStore) Create(ctx context.Context, id, value string, expiry time.Time) error {
	if s.createErr != nil {
		return s.createErr
	}
	return s.SessionStore.Create(ctx, id, value, expiry)
}

func (s *sessionFailureStore) Delete(ctx context.Context, id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.SessionStore.Delete(ctx, id)
}

func (s *sessionFailureStore) Rotate(ctx context.Context, id, expected, value string, expiry time.Time) (bool, error) {
	if s.conflict {
		return false, nil
	}
	if s.rotateErr != nil && !s.commitRotate {
		return false, s.rotateErr
	}
	updated, err := s.SessionStore.Rotate(ctx, id, expected, value, expiry)
	if err == nil && s.rotateErr != nil {
		return updated, s.rotateErr
	}
	return updated, err
}

func TestSessionsStorageFailuresRemainRetryable(t *testing.T) {
	failure := errors.New("temporary session storage outage")
	for _, operation := range []string{"read", "delete", "rotate", "committed rotate", "CAS conflict"} {
		t.Run(operation, func(t *testing.T) {
			sessions, issued, identity := issueSessionForTest(t, false)
			record, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
			if err != nil {
				t.Fatal(err)
			}
			store := &sessionFailureStore{SessionStore: sessions.store}
			sessions.store = store
			switch operation {
			case "read":
				store.getErr = failure
				if _, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken); !errors.Is(err, failure) {
					t.Fatalf("read failure = %v", err)
				}
				if err := sessions.ValidateIdentity(t.Context(), identity); !errors.Is(err, failure) {
					t.Fatalf("identity storage failure = %v", err)
				}
				store.getErr = nil
			case "delete":
				store.deleteErr = failure
				if err := sessions.RevokeRefresh(t.Context(), issued.RefreshToken); !errors.Is(err, failure) {
					t.Fatalf("logout failure = %v", err)
				}
				store.deleteErr = nil
				if err := sessions.RevokeRefresh(t.Context(), issued.RefreshToken); err != nil {
					t.Fatalf("logout retry = %v", err)
				}
				if err := sessions.ValidateIdentity(t.Context(), identity); !errors.Is(err, authn.ErrUnauthorized) {
					t.Fatalf("logout retry left identity valid: %v", err)
				}
				return
			case "rotate", "committed rotate":
				store.rotateErr, store.commitRotate = failure, operation == "committed rotate"
				if _, err := sessions.Renew(t.Context(), record, identity); !errors.Is(err, failure) {
					t.Fatalf("rotate failure = %v", err)
				}
				store.rotateErr = nil
			case "CAS conflict":
				store.conflict = true
				if _, err := sessions.Renew(t.Context(), record, identity); err == nil {
					t.Fatal("permanent CAS conflict accepted")
				}
				store.conflict = false
			}
			if err := sessions.ValidateIdentity(t.Context(), identity); err != nil {
				t.Fatalf("transient failure invalidated access: %v", err)
			}
			retryRecord, err := sessions.ReadRefresh(t.Context(), issued.RefreshToken)
			if err != nil {
				t.Fatalf("refresh token cannot retry: %v", err)
			}
			renewed, err := sessions.Renew(t.Context(), retryRecord, identity)
			if err != nil || !renewed.ExpiredAt.Equal(issued.ExpiredAt) {
				t.Fatalf("refresh retry = %#v, %v", renewed, err)
			}
			current, err := sessions.ReadRefresh(t.Context(), renewed.RefreshToken)
			if err != nil || current.Generation != 2 {
				t.Fatalf("retry caused extra rotation: %#v, %v", current, err)
			}
		})
	}
	sessions := newSessionTestManager(t)
	sessions.store = &sessionFailureStore{SessionStore: sessions.store, createErr: failure}
	if _, err := sessions.Issue(t.Context(), authn.Identity{UserID: 7, Username: "u", Code: "c", CredentialVersion: 1}, false); !errors.Is(err, failure) {
		t.Fatalf("create failure = %v", err)
	}
}
