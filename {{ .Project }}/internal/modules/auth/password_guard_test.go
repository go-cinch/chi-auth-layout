package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func newPasswordGuardTestManager(t *testing.T, failures PasswordFailureStore) (*PasswordChangeGuard, *memoryPointCaptchaStore) {
	t.Helper()
	captchaStore := NewMemoryPointCaptchaStore().(*memoryPointCaptchaStore)
	captcha := newPointCaptchaTestManager(t, captchaStore)
	guard, err := NewPasswordChangeGuard(failures, captcha, 3, 6, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return guard, captchaStore
}

func TestPasswordChangeGuard(t *testing.T) {
	failures := NewMemoryPasswordFailureStore()
	guard, captchaStore := newPasswordGuardTestManager(t, failures)
	if required, err := guard.Required(t.Context(), 7); err != nil || required {
		t.Fatalf("initial required = %v, %v", required, err)
	}
	if guard.LockThreshold() != 6 || guard.LockRequired(5) || !guard.LockRequired(6) {
		t.Fatal("password change lock threshold is incorrect")
	}
	for attempt := 1; attempt <= 3; attempt++ {
		count, err := guard.RecordFailure(t.Context(), 7)
		if err != nil || count != int64(attempt) || guard.CaptchaRequired(count) != (attempt == 3) {
			t.Fatalf("attempt %d count = %d, %v", attempt, count, err)
		}
	}
	if verified, err := guard.Verify(t.Context(), 7, "", nil); err != nil || verified {
		t.Fatalf("empty captcha verify = %v, %v", verified, err)
	}
	challenge, err := guard.NewChallenge(t.Context(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(challenge.CaptchaID, "password-change:") || strings.Count(challenge.CaptchaID, ":") != 1 || strings.Contains(challenge.CaptchaID, ".") {
		t.Fatalf("password-change captcha id = %q", challenge.CaptchaID)
	}
	answer := storedPointCaptchaAnswer(t, captchaStore, challenge.CaptchaID)
	if verified, err := guard.Check(t.Context(), 7, challenge.CaptchaID, answer.Points); err != nil || !verified {
		t.Fatalf("captcha check = %v, %v", verified, err)
	}
	if verified, err := guard.Verify(t.Context(), 7, challenge.CaptchaID, answer.Points); err != nil || !verified {
		t.Fatalf("captcha verify = %v, %v", verified, err)
	}
	if err := guard.Clear(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	if required, err := guard.Required(t.Context(), 7); err != nil || required {
		t.Fatalf("cleared required = %v, %v", required, err)
	}
	if verified, err := guard.Verify(t.Context(), 7, "", nil); err != nil || !verified {
		t.Fatalf("captcha-free verify = %v, %v", verified, err)
	}
	if _, err := guard.Required(t.Context(), 0); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("invalid user required: %v", err)
	}
}

func TestPasswordChangeGuardValidationAndExpiration(t *testing.T) {
	captcha := newPointCaptchaTestManager(t, NewMemoryPointCaptchaStore())
	for name, item := range map[string]struct {
		store     PasswordFailureStore
		captcha   *PointCaptcha
		threshold int64
		lockThreshold int64
		ttl       time.Duration
	}{
		"store":          {nil, captcha, 3, 6, time.Minute},
		"captcha":        {NewMemoryPasswordFailureStore(), nil, 3, 6, time.Minute},
		"threshold":      {NewMemoryPasswordFailureStore(), captcha, 0, 6, time.Minute},
		"lock threshold": {NewMemoryPasswordFailureStore(), captcha, 3, 3, time.Minute},
		"ttl":            {NewMemoryPasswordFailureStore(), captcha, 3, 6, 0},
		"long ttl":       {NewMemoryPasswordFailureStore(), captcha, 3, 6, 25 * time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewPasswordChangeGuard(item.store, item.captcha, item.threshold, item.lockThreshold, item.ttl); err == nil {
				t.Fatal("invalid password guard configuration was accepted")
			}
		})
	}
	store := NewMemoryPasswordFailureStore()
	if _, err := store.Increment(t.Context(), 7, -time.Second); err != nil {
		t.Fatal(err)
	}
	if count, err := store.Count(t.Context(), 7); err != nil || count != 0 {
		t.Fatalf("expired count = %d, %v", count, err)
	}
}

type failingPasswordFailureStore struct{ err error }

func (s failingPasswordFailureStore) Count(context.Context, int64) (int64, error) {
	return 0, s.err
}

func (s failingPasswordFailureStore) Increment(context.Context, int64, time.Duration) (int64, error) {
	return 0, s.err
}

func (s failingPasswordFailureStore) Clear(context.Context, int64) error { return s.err }

func TestPasswordChangeGuardStoreFailures(t *testing.T) {
	expected := errors.New("store unavailable")
	guard, _ := newPasswordGuardTestManager(t, failingPasswordFailureStore{err: expected})
	if _, err := guard.Required(t.Context(), 7); !errors.Is(err, expected) {
		t.Fatalf("required error = %v", err)
	}
	if _, err := guard.RecordFailure(t.Context(), 7); !errors.Is(err, expected) {
		t.Fatalf("record error = %v", err)
	}
	if err := guard.Clear(t.Context(), 7); !errors.Is(err, expected) {
		t.Fatalf("clear error = %v", err)
	}
}

{{ if .Computed.enable_redis_final -}}
func TestRedisPasswordFailureStore(t *testing.T) {
	client := &fakeRedisChallengeClient{values: make(map[string]string)}
	store := NewRedisPasswordFailureStore(client)
	if count, err := store.Count(t.Context(), 7); err != nil || count != 0 {
		t.Fatalf("initial count = %d, %v", count, err)
	}
	if count, err := store.Increment(t.Context(), 7, time.Minute); err != nil || count != 1 {
		t.Fatalf("increment count = %d, %v", count, err)
	}
	if count, err := store.Count(t.Context(), 7); err != nil || count != 1 {
		t.Fatalf("stored count = %d, %v", count, err)
	}
	if err := store.Clear(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	client.err = errors.New("redis unavailable")
	if _, err := store.Count(t.Context(), 7); err == nil {
		t.Fatal("count error was ignored")
	}
	if _, err := store.Increment(t.Context(), 7, time.Minute); err == nil {
		t.Fatal("increment error was ignored")
	}
	if err := store.Clear(t.Context(), 7); err == nil {
		t.Fatal("clear error was ignored")
	}
}
{{ end -}}
