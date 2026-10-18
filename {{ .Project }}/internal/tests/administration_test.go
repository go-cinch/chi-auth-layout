package tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
	"{{ .Computed.module_name_final }}/internal/modules/usergroup"
)

func TestPermissionAdministrationPostgres(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	code := func(word string) string {
		t.Helper()
		var result string
		if err := store.DB.QueryRowContext(t.Context(), `SELECT code FROM t_action WHERE word=$1`, word).Scan(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	baseWords := []string{"admin:read:system:user", "admin:create:system:user", "admin:update:system:user", "admin:delete:system:user", "admin:read:system:role", "admin:create:system:role", "admin:update:system:role", "admin:delete:system:role", "admin:read:system:user-group", "admin:create:system:user-group", "admin:update:system:user-group", "admin:delete:system:user-group", "admin:read:system:action", "admin:create:system:action", "admin:update:system:action", "admin:delete:system:action"}
	baseCodes := []string{}
	for _, word := range baseWords {
		baseCodes = append(baseCodes, code(word))
	}
	allCode, grantCode, readCode := code("admin:all:system:general"), code("admin:grant:system:permission-admin"), code("admin:read:system:user")
	assign := func(codes []string) {
		exec(`UPDATE t_user SET role_id=NULL,action=$1,login_count=1,status=1 WHERE id=2`, strings.Join(codes, ","))
	}
	assign(baseCodes)
	exec(`UPDATE t_user SET role_id=NULL,action='',login_count=1,status=1,metadata='{}' WHERE id=3`)
	manager, err := authn.New("test", strings.Repeat("a", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var identity authn.Identity
	if err := store.DB.QueryRowContext(t.Context(), `SELECT id,username,code,credential_version FROM t_user WHERE id=2`).Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	original := httptest.NewRequest("GET", "/info?scope=admin", nil)
	original.Header.Set("Authorization", "Bearer "+token)
	authenticated, err := manager.AuthenticateRequest(original)
	if err != nil {
		t.Fatal(err)
	}
	ctx := authenticated.Context()
	policy := auth.New(store, manager, nil, nil, nil, nil, nil, auth.Switches{}, localizer)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false, localizer)
	catalogModule, catalogErr := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, false, localizer)
	if catalogErr != nil {
		t.Fatal(catalogErr)
	}
	actions.SetDefinitionCatalog(catalogModule)
	roles := role.New(store, limits, actions, false)
	users := user.New(store, limits, nil, nil, actions, roles, auth.Switches{})
	groups := usergroup.New(store, limits, actions, users)
	actions.SetPermissionAdministration(policy)
	roles.SetPermissionAdministration(policy)
	users.SetPermissionAdministration(policy)
	groups.SetPermissionAdministration(policy)
	cfg := &config.Config{}
	cfg.Auth.Authorization.Enabled = true
	cfg.Idempotency.TTL = time.Minute
	handler, err := server.NewRouter(cfg, manager, idempotency.NewMemoryStore(), policy, actions, roles, users, groups)
	if err != nil {
		t.Fatal(err)
	}
	requestWithHandler := func(handler http.Handler, method, path string, body any, want int) string {
		t.Helper()
		encoded, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(encoded)))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set(authn.PermissionScopeHeader, "admin")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s status=%d want=%d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.String()
	}
	request := func(method, path string, body any, want int) string {
		t.Helper()
		return requestWithHandler(handler, method, path, body, want)
	}
	request("PATCH", "/user/3", map[string]any{"role_id": 1}, 403)
	request("PATCH", "/user/3", map[string]any{"action_codes": []string{allCode}}, 403)
	request("PATCH", "/user/3", map[string]any{"username": "account-takeover"}, 403)
	password := "unprivileged-reset"
	if _, err := users.Update(ctx, 3, user.UpdateUserInput{Password: &password}); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("password takeover: %v", err)
	}
	request("PATCH", "/user/3", map[string]any{"metadata": map[string]any{"note": "ordinary profile"}}, 200)
	request("POST", "/role", map[string]any{"name": "Escalation", "word": "escalation", "action_codes": []string{allCode}}, 403)
	request("POST", "/user-group", map[string]any{"name": "Escalation", "word": "escalation", "user_ids": []int64{2}}, 403)
	// Use the same valid definition before and after granting administration,
	// so the denied request reaches permission validation instead of failing input validation.
	createActionInput := map[string]any{
		"word":  "admin:read:test:created",
		"group": "admin:system:user",
		"config": map[string]any{
			"bindings":  []any{map[string]any{"resource_key": "admin:system:user", "operation": "read", "feature_keys": []any{}}},
			"ui":        []any{},
			"endpoints": []any{},
		},
	}
	request("POST", "/action", createActionInput, 403)
	// Route authorization also rejects action-definition writes. Disable that
	// layer independently to verify the business administration guard itself.
	withoutRouteAuthorization := *cfg
	withoutRouteAuthorization.Auth.Authorization.Enabled = false
	businessGuardHandler, err := server.NewRouter(&withoutRouteAuthorization, manager, idempotency.NewMemoryStore(), policy, actions, roles, users, groups)
	if err != nil {
		t.Fatal(err)
	}
	deniedAction := requestWithHandler(businessGuardHandler, "POST", "/action", createActionInput, 403)
	var deniedActionError server.ErrorResponse
	if err := json.Unmarshal([]byte(deniedAction), &deniedActionError); err != nil {
		t.Fatal(err)
	}
	if deniedActionError.ErrorCode != "AUTH_PERMISSION_GRANT_REQUIRED" {
		t.Fatalf("action creation did not reach permission validation: %s", deniedAction)
	}
	if _, err := actions.Create(ctx, action.CreateActionInput{Word: "admin:read:test:created", Config: action.Config{ResourceKey: "admin:system:user", Operation: "read", UI: []action.UIBinding{}, Endpoints: []action.Endpoint{}}}); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("direct action definition: %v", err)
	}
	exec(`INSERT INTO t_role(id,name,word,action) VALUES(101,'Empty','empty',''),(102,'Assigned','assigned',$1)`, readCode)
	exec(`INSERT INTO t_user_group(id,name,word,action) VALUES(101,'Empty','empty',''),(102,'Assigned','assigned',$1),(103,'Members','members','')`, readCode)
	exec(`INSERT INTO t_user_user_group_relation(user_id,user_group_id) VALUES(3,103)`)
	request("DELETE", "/role/101,102", nil, 403)
	request("DELETE", "/role/101", nil, 200)
	request("DELETE", "/user-group/102", nil, 403)
	request("DELETE", "/user-group/103", nil, 403)
	request("DELETE", "/user-group/101", nil, 200)
	request("PATCH", "/user-group/102", map[string]any{"add_user_ids": []int64{2}}, 403)
	info, err := policy.Info(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, menu := range info.Permission.Menus {
		for _, button := range menu.Buttons {
			if button == "admin:btn:system:action:create" || button == "admin:btn:system:action:edit" || button == "admin:btn:system:action:delete" || button == "admin:btn:system:user-group:add-members" {
				t.Fatalf("ineffective management control leaked: %s", button)
			}
		}
	}
	// Ordinary write still creates and edits a new account without granting access.
	ordinary, err := users.Create(ctx, user.CreateUserInput{Username: "ordinary-managed", Password: "initial-password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := users.Delete(ctx, ordinary.ID); err != nil {
		t.Fatalf("ordinary account removal: %v", err)
	}
	withGrant := append(append([]string{}, baseCodes...), grantCode)
	assign(withGrant)
	request("PATCH", "/user/3", map[string]any{"action_codes": []string{readCode}}, 200)
	request("PATCH", "/user/3", map[string]any{"username": "grant-managed"}, 200)
	request("POST", "/action", createActionInput, 200)
	request("PATCH", "/user-group/102", map[string]any{"add_user_ids": []int64{2}}, 200)
	request("PATCH", "/role/102", map[string]any{"action_codes": []string{grantCode}}, 200)
	request("DELETE", "/role/102", nil, 200)
	request("DELETE", "/user-group/103", nil, 200)
	if _, err := users.Update(authn.WithPermissionScope(ctx, "mini"), 3, user.UpdateUserInput{ActionCodes: &withGrant}); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("cross-scope grant: %v", err)
	}
	assign([]string{grantCode})
	request("PATCH", "/user/3", map[string]any{"action_codes": []string{readCode}}, 403)
	assign(withGrant)
	// The explicitly granted privilege may assign existing all/grant capabilities to others or itself.
	request("PATCH", "/user/3", map[string]any{"action_codes": []string{allCode, grantCode}}, 200)
	request("PATCH", "/user/2", map[string]any{"action_codes": append(append([]string{}, withGrant...), allCode)}, 200)
	request("PATCH", "/user/3", map[string]any{"username": "all-managed"}, 200)
	if _, err := users.Update(ctx, 3, user.UpdateUserInput{Password: &password}); err != nil {
		t.Fatalf("all credential reset: %v", err)
	}
	// A queued assignment must re-check grants after the preceding revocation commits.
	assign(withGrant)
	exec(`UPDATE t_user SET action='' WHERE id=3`)
	blocker, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := blocker.ExecContext(t.Context(), `SELECT pg_advisory_xact_lock(73482301991337)`); err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.ExecContext(t.Context(), `UPDATE t_user SET action=$1 WHERE id=2`, strings.Join(baseCodes, ",")); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	queuedCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go func() {
		_, err := users.Update(queuedCtx, 3, user.UpdateUserInput{ActionCodes: &withGrant})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("queued change escaped transaction lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("revoked grant reused: %v", err)
	}
	var assigned string
	if err := store.DB.QueryRowContext(t.Context(), `SELECT action FROM t_user WHERE id=3`).Scan(&assigned); err != nil || assigned != "" {
		t.Fatalf("rejected assignment persisted: %q %v", assigned, err)
	}
	// Every test-owned row lives in the isolated schema removed by permissionDatabase.
}
