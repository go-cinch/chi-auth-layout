package tests

{{ if .Computed.enable_redis_final -}}
import (
	"context"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/modules/auth"
)

func TestMFARedisRateLimitWindowAndVersion(t *testing.T) {
	client := sessionRedisClient(t)
	state := auth.NewRedisMFAStore(client)
	ctx := t.Context()
	const oldKey, newKey = "101:1", "101:2"
	const window = 800*time.Millisecond
	const limit = int64(3)
	t.Cleanup(func() {
		if err := state.ResetAttempts(context.Background(), oldKey); err != nil { t.Error(err) }
		if err := state.ResetAttempts(context.Background(), newKey); err != nil { t.Error(err) }
	})
	if allowed, err := state.Attempt(ctx, oldKey, window, limit); err != nil || !allowed { t.Fatal("first attempt rejected", err) }
	expires, err := client.Eval(ctx, `return redis.call('PEXPIRETIME', KEYS[1])`, []string{"mfa:attempt:"+oldKey}).Int64()
	if err != nil || expires <= time.Now().UnixMilli() || expires > time.Now().Add(window).UnixMilli() { t.Fatal("configured attempt expiry missing", err) }
	type outcome struct { allowed bool; err error }
	results := make(chan outcome, 8)
	for range 8 {
		go func() { allowed, err := state.Attempt(ctx, oldKey, 5*time.Minute, limit); results <- outcome{allowed, err} }()
	}
	accepted := 1
	for range 8 {
		result := <-results
		if result.err != nil { t.Fatal(result.err) }
		if result.allowed { accepted++ }
	}
	if accepted != int(limit) { t.Fatal("concurrent attempts exceeded configured maximum", accepted) }
	after, err := client.Eval(ctx, `return redis.call('PEXPIRETIME', KEYS[1])`, []string{"mfa:attempt:"+oldKey}).Int64()
	if err != nil || after != expires { t.Fatal("later or blocked attempts extended the fixed window", err) }
	if allowed, err := state.Attempt(ctx, newKey, 5*time.Minute, limit); err != nil || !allowed { t.Fatal("new version inherited prior limit", err) }
	// Observe real Redis expiry of this short fixture window, without altering its TTL.
	timer := time.NewTimer(time.Until(time.UnixMilli(expires).Add(20*time.Millisecond)))
	defer timer.Stop()
	select { case <-timer.C: case <-ctx.Done(): t.Fatal(ctx.Err()) }
	if allowed, err := state.Attempt(ctx, oldKey, window, limit); err != nil || !allowed { t.Fatal("expired window did not reopen", err) }
	if err := state.ResetAttempts(ctx, oldKey); err != nil { t.Fatal(err) }
	oldCount, err := client.Eval(ctx, `return tonumber(redis.call('GET', KEYS[1]) or '0')`, []string{"mfa:attempt:"+oldKey}).Int64()
	if err != nil || oldCount != 0 { t.Fatal("attempt reset did not clear the counter", err) }
	if allowed, err := state.Attempt(ctx, oldKey, window, limit); err != nil || !allowed { t.Fatal("late old-version request failed", err) }
	newCount, err := client.Eval(ctx, `return tonumber(redis.call('GET', KEYS[1]) or '0')`, []string{"mfa:attempt:"+newKey}).Int64()
	if err != nil || newCount != 1 { t.Fatal("old-version cleanup or late request changed the new counter", err) }
	if err := state.ResetAttempts(ctx, oldKey); err != nil { t.Fatal(err) }
	if err := state.ResetAttempts(ctx, oldKey); err != nil { t.Fatal("missing counter reset was not idempotent", err) }
}
{{ end -}}
