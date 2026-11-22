package tests

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
)

func TestUserAccountActionsPostgres(t *testing.T) {
	store := permissionDatabase(t)
	catalog := testLocalization(t, store)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE t_user SET action='USERCTRL', role_id=NULL, login_count=1 WHERE id=2`)
	exec(`DELETE FROM t_user_user_group_relation WHERE user_id=2`)
	exec(`UPDATE t_user SET status=1, metadata='{"password_change_failures":2,"note":"old"}' WHERE id=3`)
	exec(`INSERT INTO t_action(code, word, action_group, config) VALUES('USERCTRL','admin:update:custom:user','admin:system:user','{"bindings":[],"ui":[],"endpoints":[]}')`)
	allow := func(control string) {
		t.Helper()
		exec(`UPDATE t_action SET config=jsonb_build_object('bindings', jsonb_build_array(jsonb_build_object('resource_key','admin:system:user','operation','update','feature_keys',jsonb_build_array($1::text))), 'ui','[]'::jsonb, 'endpoints','[]'::jsonb) WHERE code='USERCTRL'`, "admin:system:user:"+control)
	}
	manager, err := authn.New("test", strings.Repeat("a", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	identity := authn.Identity{Methods: authn.Methods{Primary: authn.Password}}
	if err := store.DB.QueryRowContext(t.Context(), "SELECT id,username,code,credential_version FROM t_user WHERE id=2").Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/info", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	verified, err := manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx := verified.Context()
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, catalog)
	users := user.New(store, limits, nil, nil, actions, role.New(store, limits, actions))
	users.SetPermissionAdministration(auth.New(store, manager, nil, nil, nil, nil, nil, catalog, nil, &auth.Verification{}))

	allow("lock")
	locked, err := users.Lock(ctx, 3, 0)
	if err != nil || locked.Status != user.StatusLocked {
		t.Fatalf("lock: %v %v", locked, err)
	}
	if _, err := users.Lock(ctx, 3, 0); !errors.Is(err, user.ErrInvalid) {
		t.Fatalf("second lock: %v", err)
	}
	if _, err := users.Unlock(ctx, 3); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("lock granted unlock: %v", err)
	}
	if _, err := users.ResetPassword(ctx, 3, "changed"); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("lock granted reset: %v", err)
	}

	allow("edit")
	metadata := map[string]any{"note": "new"}
	edited, err := users.Update(ctx, 3, user.UpdateUserInput{Metadata: &metadata})
	if err != nil || edited.Metadata["lock_expired_at"] != float64(0) || edited.Metadata["password_change_failures"] != float64(2) {
		t.Fatalf("profile lost action metadata: %v %v", edited, err)
	}
	if _, err := users.Review(ctx, 3, "approve", ""); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("edit granted approval: %v", err)
	}
	allow("reset-password")
	reset, err := users.ResetPassword(ctx, 3, " changed ")
	if err != nil || reset.Status != user.StatusLocked || reset.Metadata["password_change_failures"] != nil || reset.Metadata["note"] != "new" {
		t.Fatalf("reset: %v %v", reset, err)
	}

	allow("unlock")
	unlocked, err := users.Unlock(ctx, 3)
	if err != nil || unlocked.Status != user.StatusActive || unlocked.Metadata["lock_expired_at"] != nil {
		t.Fatalf("unlock: %v %v", unlocked, err)
	}
	exec(`UPDATE t_user SET status=2,metadata='{"lock_expired_at":1}' WHERE id=3`)
	allow("lock")
	if _, err := users.Lock(ctx, 3, time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatalf("lock an expired lock: %v", err)
	}
	allow("approve")
	if _, err := users.Review(ctx, 3, "approve", ""); !errors.Is(err, user.ErrInvalid) {
		t.Fatalf("review changed a locked user: %v", err)
	}
	if _, err := users.Update(ctx, 3, user.UpdateUserInput{Metadata: &metadata}); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("review granted edit: %v", err)
	}
	exec(`UPDATE t_user SET status=0,metadata='{}' WHERE id=3`)
	if _, err := users.Review(ctx, 3, "reject", "duplicate"); err != nil {
		t.Fatal(err)
	}
	approved, err := users.Review(ctx, 3, "approve", "")
	if err != nil || approved.Status != user.StatusActive || approved.Metadata["reject_register_reason"] != nil {
		t.Fatalf("review: %v %v", approved, err)
	}
}
