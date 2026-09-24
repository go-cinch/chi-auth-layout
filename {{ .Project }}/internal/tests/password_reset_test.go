package tests

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
)

func TestFirstLoginPasswordReset(t *testing.T) {
	store := permissionDatabase(t)
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("old-secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO t_user (id,username,code,password,status) VALUES (101,'tester','TEST0001',$1,1)`, string(hash))
	limits, _ := pagination.New(100, 100)
	dictionaryModule, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, false)
	if err != nil {
		t.Fatal(err)
	}
	captcha, err := auth.NewPointCaptcha(auth.NewMemoryPointCaptchaStore(), dictionaryModule, 5, time.Minute, 300, 180, 22)
	if err != nil {
		t.Fatal(err)
	}
	guard, err := auth.NewPasswordChangeGuard(auth.NewMemoryPasswordFailureStore(), captcha, 3, 6, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions(auth.NewMemorySessionStore(), time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := authn.New("test", strings.Repeat("k", 64), time.Hour, auth.NewIdentityValidator(store, sessions, auth.Switches{PasswordResetRequired: true}))
	if err != nil {
		t.Fatal(err)
	}
	m := auth.New(store, manager, nil, sessions, captcha, guard, auth.Switches{PasswordResetRequired: true})
	actions := action.New(store, limits, false)
	users := user.New(store, limits, nil, guard, actions, role.New(store, limits, actions, false), auth.Switches{PasswordResetRequired: true})

	login := func(password string) *auth.LoginResult {
		t.Helper()
		result, err := m.Login(ctx, auth.LoginInput{Username: "tester", Password: password, RememberMe: true})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	authenticate := func(token, path string) (context.Context, error) {
		r := httptest.NewRequest("PATCH", path, nil).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+token)
		verified, err := manager.AuthenticateRequest(r)
		if err != nil {
			return nil, err
		}
		return verified.Context(), nil
	}
	state := func(count, version int64, logged bool) {
		t.Helper()
		var gotCount, gotVersion int64
		var last sql.NullTime
		if err := store.DB.QueryRowContext(ctx, `SELECT login_count,credential_version,last_logged_in_at FROM t_user WHERE id=101`).Scan(&gotCount, &gotVersion, &last); err != nil {
			t.Fatal(err)
		}
		if gotCount != count || gotVersion != version || last.Valid != logged {
			t.Fatalf("state = %d/%d/%v, want %d/%d/%v", gotCount, gotVersion, last.Valid, count, version, logged)
		}
	}
	state(0, 1, false)
	abandoned := login("old-secret")
	if !abandoned.PasswordResetRequired {
		t.Fatal("first login unrestricted")
	}
	state(0, 1, false)
	refreshed, err := m.Refresh(ctx, abandoned.RefreshToken)
	if err != nil || !refreshed.PasswordResetRequired {
		t.Fatalf("refresh: %#v %v", refreshed, err)
	}
	state(0, 1, false)
	if err := m.Logout(ctx, refreshed.RefreshToken); err != nil {
		t.Fatal(err)
	}
	state(0, 1, false)
	a, b := login("old-secret"), login("old-secret")
	for _, token := range []*auth.LoginResult{a, b} {
		if _, err := authenticate(token.AccessToken, "/auth/info"); !errors.Is(err, authn.ErrPasswordResetRequired) {
			t.Fatalf("business bypass: %v", err)
		}
	}
	aCtx, err := authenticate(a.AccessToken, "/auth/reset/pwd")
	if err != nil {
		t.Fatal(err)
	}
	bCtx, err := authenticate(b.AccessToken, "/auth/reset/pwd")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ResetPassword(aCtx, auth.PasswordResetInput{NewPassword: "old-secret"}); !errors.Is(err, auth.ErrSamePassword) {
		t.Fatalf("same password: %v", err)
	}
	state(0, 1, false)
	// Both requests have authenticated with version 1 before either reset commits.
	type outcome struct {
		result *auth.LoginResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	start := make(chan struct{})
	for _, requestCtx := range []context.Context{aCtx, bCtx} {
		go func() {
			<-start
			result, err := m.ResetPassword(requestCtx, auth.PasswordResetInput{NewPassword: "new-secret"})
			outcomes <- outcome{result, err}
		}()
	}
	close(start)
	var winner *auth.LoginResult
	successes := 0
	for range 2 {
		o := <-outcomes
		if o.err == nil {
			successes++
			winner = o.result
		} else if !errors.Is(o.err, auth.ErrUnauthorized) {
			t.Fatal(o.err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successes: %d", successes)
	}
	state(1, 2, true)
	if winner.PasswordResetRequired {
		t.Fatal("reset still required")
	}
	for _, token := range []*auth.LoginResult{a, b} {
		if _, err := authenticate(token.AccessToken, "/auth/reset/pwd"); !errors.Is(err, authn.ErrUnauthorized) {
			t.Fatalf("old access accepted: %v", err)
		}
		if _, err := m.Refresh(ctx, token.RefreshToken); !errors.Is(err, auth.ErrUnauthorized) {
			t.Fatalf("old refresh accepted: %v", err)
		}
	}
	winnerCtx, err := authenticate(winner.AccessToken, "/auth/info")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ResetPassword(winnerCtx, auth.PasswordResetInput{NewPassword: "another-secret"}); !errors.Is(err, auth.ErrPasswordResetNotRequired) {
		t.Fatalf("repeat reset: %v", err)
	}
	if _, err := m.Refresh(ctx, winner.RefreshToken); err != nil {
		t.Fatal(err)
	}
	state(1, 2, true)
	// Parallel normal logins must not lose increments.
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := m.Login(ctx, auth.LoginInput{Username: "tester", Password: "new-secret"})
			if err != nil {
				failures <- err
			}
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	state(9, 2, true)
	adminPassword := "admin-secret"
	if _, err := users.Update(ctx, 101, user.UpdateUserInput{Password: &adminPassword}); err != nil {
		t.Fatal(err)
	}
	state(0, 3, true)
	if _, err := authenticate(winner.AccessToken, "/auth/info"); !errors.Is(err, authn.ErrUnauthorized) {
		t.Fatalf("admin reset left old access valid: %v", err)
	}
	again := login(adminPassword)
	if !again.PasswordResetRequired {
		t.Fatal("admin reset bypassed")
	}
	state(0, 3, true)
	againCtx, err := authenticate(again.AccessToken, "/auth/reset/pwd")
	if err != nil {
		t.Fatal(err)
	}
	// A deferred commit failure must roll back the password, count, version and time together.
	exec(`CREATE FUNCTION reject_reset_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced reset rollback'; END $$`)
	exec(`CREATE CONSTRAINT TRIGGER reject_reset_commit AFTER UPDATE ON t_user DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (OLD.credential_version <> NEW.credential_version) EXECUTE FUNCTION reject_reset_commit()`)
	if _, err := m.ResetPassword(againCtx, auth.PasswordResetInput{NewPassword: "failed-secret"}); err == nil {
		t.Fatal("commit failure ignored")
	}
	state(0, 3, true)
	var storedHash string
	if err := store.DB.QueryRowContext(ctx, `SELECT password FROM t_user WHERE id=101`).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(adminPassword)) != nil {
		t.Fatal("password not rolled back")
	}
	if _, err := authenticate(again.AccessToken, "/auth/reset/pwd"); err != nil {
		t.Fatal(err)
	}
}
