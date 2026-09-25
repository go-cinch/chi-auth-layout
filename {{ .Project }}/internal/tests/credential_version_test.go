package tests

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
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

func TestCredentialVersionTransactions(t *testing.T) {
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
	exec(`INSERT INTO t_user (id,username,code,password,status,login_count) VALUES (101,'tester','TEST0001',$1,1,1)`, string(hash))
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
	m := auth.New(store, manager, nil, sessions, captcha, nil, guard, auth.Switches{PasswordResetRequired: true})
	actions := action.New(store, limits, false)
	users := user.New(store, limits, nil, guard, actions, role.New(store, limits, actions, false), auth.Switches{PasswordResetRequired: true})
	login := func(password string) *auth.LoginResult {
		t.Helper()
		result, err := m.Login(ctx, auth.LoginInput{Username: "tester", Password: password})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	authenticate := func(token string) (context.Context, error) {
		request := httptest.NewRequest("GET", "/auth/info", nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		verified, err := manager.AuthenticateRequest(request)
		if err != nil {
			return nil, err
		}
		return verified.Context(), nil
	}
	version := func(want int64) {
		t.Helper()
		var got int64
		if err := store.DB.QueryRowContext(ctx, `SELECT credential_version FROM t_user WHERE id=101`).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("version = %d, want %d", got, want)
		}
	}
	reject := func(tokens ...*auth.LoginResult) {
		t.Helper()
		for _, token := range tokens {
			if _, err := authenticate(token.AccessToken); !errors.Is(err, authn.ErrUnauthorized) {
				t.Fatalf("old access accepted: %v", err)
			}
			if _, err := m.Refresh(ctx, token.RefreshToken); !errors.Is(err, auth.ErrUnauthorized) {
				t.Fatalf("old refresh accepted: %v", err)
			}
		}
	}
	a, b := login("old-secret"), login("old-secret")
	aCtx, err := authenticate(a.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	bCtx, err := authenticate(b.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	version(1)
	if err := m.ChangePassword(aCtx, auth.PasswordChangeInput{OldPassword: "old-secret", NewPassword: "new-secret"}); err != nil {
		t.Fatal(err)
	}
	version(2)
	reject(a, b)
	// A request authenticated before the other transaction committed must fail under the row lock.
	if err := m.ChangePassword(bCtx, auth.PasswordChangeInput{OldPassword: "new-secret", NewPassword: "other-secret"}); !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("concurrent stale request = %v", err)
	}
	version(2)
	a, b = login("new-secret"), login("new-secret")
	// Profile/role metadata does not rotate credentials.
	metadata := map[string]any{"note": "profile edit"}
	if _, err := users.Update(ctx, 101, user.UpdateUserInput{Metadata: &metadata}); err != nil {
		t.Fatal(err)
	}
	version(2)
	if _, err := authenticate(a.AccessToken); err != nil {
		t.Fatal(err)
	}
	resetPassword := "admin-secret"
	if _, err := users.Update(ctx, 101, user.UpdateUserInput{Password: &resetPassword}); err != nil {
		t.Fatal(err)
	}
	version(3)
	reject(a, b)
	exec(`UPDATE t_user SET login_count=1 WHERE id=101`)
	a, b = login(resetPassword), login(resetPassword)
	aCtx, err = authenticate(a.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	// A deferred constraint trigger fails COMMIT, after the password/version UPDATE succeeded.
	exec(`CREATE FUNCTION reject_credential_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced credential rollback'; END $$`)
	exec(`CREATE CONSTRAINT TRIGGER reject_credential_commit AFTER UPDATE ON t_user DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (OLD.credential_version <> NEW.credential_version) EXECUTE FUNCTION reject_credential_commit()`)
	if err := m.ChangePassword(aCtx, auth.PasswordChangeInput{OldPassword: resetPassword, NewPassword: "failed-secret"}); err == nil {
		t.Fatal("password change commit unexpectedly succeeded")
	}
	version(3)
	if _, err := users.Update(ctx, 101, user.UpdateUserInput{Password: &resetPassword}); err == nil {
		t.Fatal("admin reset commit unexpectedly succeeded")
	}
	version(3)
	var storedHash string
	if err := store.DB.QueryRowContext(ctx, `SELECT password FROM t_user WHERE id=101`).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(resetPassword)); err != nil {
		t.Fatal("rolled-back password changed")
	}
	for i, token := range []*auth.LoginResult{a, b} {
		if _, err := authenticate(token.AccessToken); err != nil {
			t.Fatal(err)
		}
		renewed, err := m.Refresh(ctx, token.RefreshToken)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := manager.Parse(renewed.AccessToken)
		if err != nil || identity.CredentialVersion != 3 {
			t.Fatal(fmt.Sprintf("device %d after rollback: %#v %v", i, identity, err))
		}
	}
}
