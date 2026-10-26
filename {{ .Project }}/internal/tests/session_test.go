package tests

{{ if .Computed.enable_redis_final -}}
import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/infra/rds"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
)

// AUTH_SESSION_TEST_REDIS_DSN must point to a disposable Redis instance. Each
// test uses a separate prefix and only deletes the session keys it creates.
func sessionRedisClient(t *testing.T) *rds.Client {
	t.Helper()
	dsn := os.Getenv("AUTH_SESSION_TEST_REDIS_DSN")
	if dsn == "" {
		t.Skip("set AUTH_SESSION_TEST_REDIS_DSN to run real Redis session integration")
	}
	var cfg config.Config
	cfg.Redis.DSN = dsn
	cfg.Redis.Prefix = fmt.Sprintf("test:session-%d:", time.Now().UnixNano())
	client, cleanup, err := rds.New(t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return client
}

func TestRedisSessionAtomicLifecycle(t *testing.T) {
	client := sessionRedisClient(t)
	store := auth.NewRedisSessionStore(client)
	ctx := t.Context()
	const id = "issuer:atomic"
	deadline := time.Now().Add(10 * time.Second)
	t.Cleanup(func() { _ = store.Delete(context.Background(), id) })
	if err := store.Create(ctx, id, "initial", deadline); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, id, "replacement", deadline); err == nil {
		t.Fatal("duplicate create overwrote an existing session")
	}
	if value, err := store.Get(ctx, id); err != nil || value != "initial" {
		t.Fatalf("after duplicate create: %q, %v", value, err)
	}
	if changed, err := store.Rotate(ctx, id, "initial", "expired", time.Now().Add(-time.Minute)); err != nil || changed {
		t.Fatalf("rotate with expired deadline = %v, %v", changed, err)
	}
	if changed, err := store.Rotate(ctx, id, "wrong", "replacement", deadline); err != nil || changed {
		t.Fatalf("stale compare-and-swap = %v, %v", changed, err)
	}
	type result struct {
		changed bool
		err     error
	}
	const contenders = 16
	start := make(chan struct{})
	results := make(chan result, contenders)
	for i := range contenders {
		go func() {
			<-start
			changed, err := store.Rotate(ctx, id, "initial", fmt.Sprintf("winner-%d", i), deadline)
			results <- result{changed: changed, err: err}
		}()
	}
	close(start)
	winners := 0
	for range contenders {
		outcome := <-results
		if outcome.err != nil {
			t.Fatal(outcome.err)
		}
		if outcome.changed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("CAS winners = %d, want 1", winners)
	}
	value, err := store.Get(ctx, id)
	if err != nil || !strings.HasPrefix(value, "winner-") {
		t.Fatalf("winner value = %q, %v", value, err)
	}
	expiredAt, err := client.Eval(ctx, `return redis.call('PEXPIRETIME', KEYS[1])`, []string{"session:data:" + id}).Int64()
	if err != nil || expiredAt != deadline.UnixMilli() {
		t.Fatalf("absolute expiry = %d, %v; want %d", expiredAt, err, deadline.UnixMilli())
	}
	if err := store.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if changed, err := store.Rotate(ctx, id, value, "resurrected", deadline); err != nil || changed {
		t.Fatalf("rotate recreated a deleted session: %v, %v", changed, err)
	}
	if _, err := store.Get(ctx, id); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("deleted session = %v", err)
	}
	if err := store.Create(ctx, id, "already expired", time.Now().Add(-time.Minute)); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("create with expired deadline = %v", err)
	}
	if _, err := store.Get(ctx, id); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("expired create retained a key: %v", err)
	}
}

func TestRedisSessionExpirationAndNamespaceIsolation(t *testing.T) {
	first := auth.NewRedisSessionStore(sessionRedisClient(t))
	second := auth.NewRedisSessionStore(sessionRedisClient(t))
	ctx := t.Context()
	const id = "issuer:isolated"
	t.Cleanup(func() {
		_ = first.Delete(context.Background(), id)
		_ = second.Delete(context.Background(), id)
	})
	if err := first.Create(ctx, id, "short", time.Now().Add(50*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := second.Create(ctx, id, "long", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := first.Get(ctx, id)
		if errors.Is(err, auth.ErrSessionNotFound) {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatalf("Redis session did not expire: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	if changed, err := first.Rotate(ctx, id, "short", "resurrected", time.Now().Add(time.Minute)); err != nil || changed {
		t.Fatalf("rotate recreated an expired session: %v, %v", changed, err)
	}
	if value, err := second.Get(ctx, id); err != nil || value != "long" {
		t.Fatalf("another prefix was affected: %q, %v", value, err)
	}
}

func TestRedisSessionRefreshConvergesAndLogoutWins(t *testing.T) {
	store := auth.NewRedisSessionStore(sessionRedisClient(t))
	sessions, err := auth.NewSessions(store, auth.SessionConfig{
		Issuer: "session-integration", Key: strings.Repeat("k", 64),
		SessionLifetime: time.Hour, RememberLifetime: 24 * time.Hour, RetryWindow: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	identity := authn.Identity{UserID: 7, Username: "operator", Code: "CODE0007", CredentialVersion: 1}
	issued, err := sessions.Issue(ctx, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = issued.SessionID
	t.Cleanup(func() { _ = sessions.Revoke(context.Background(), issued.SessionID) })
	record, err := sessions.ReadRefresh(ctx, issued.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		token *auth.SessionToken
		err   error
	}
	const contenders = 16
	start := make(chan struct{})
	results := make(chan result, contenders)
	for range contenders {
		go func() {
			<-start
			token, err := sessions.Renew(ctx, record, identity)
			results <- result{token: token, err: err}
		}()
	}
	close(start)
	var current string
	for range contenders {
		outcome := <-results
		if outcome.err != nil || outcome.token == nil {
			t.Fatalf("concurrent refresh = %#v, %v", outcome.token, outcome.err)
		}
		if current == "" {
			current = outcome.token.RefreshToken
		}
		if outcome.token.RefreshToken != current || !outcome.token.ExpiredAt.Equal(issued.ExpiredAt) {
			t.Fatalf("concurrent refresh diverged or extended expiry: %#v", outcome.token)
		}
	}
	if current == issued.RefreshToken {
		t.Fatal("first refresh did not rotate the token")
	}
	if err := sessions.RevokeRefresh(ctx, issued.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Renew(ctx, record, identity); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("pending refresh revived logged-out session: %v", err)
	}
	if err := sessions.ValidateIdentity(ctx, identity); !errors.Is(err, authn.ErrUnauthorized) {
		t.Fatalf("logged-out identity remains valid: %v", err)
	}
}
{{ end -}}
