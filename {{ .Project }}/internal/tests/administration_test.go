package tests

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
)

func TestPermissionAdministrationPostgres(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.SQL(t.Context()).ExecContext(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	code := func(word string) string {
		t.Helper()
		var value string
		if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT code FROM t_action WHERE word=$1`, word).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	allCode, grantCode, updateCode, readCode := code("admin:all:system:all"), code("admin:grant:system:permission-admin"), code("admin:update:system:user"), code("admin:read:system:user")
	developerCode := code("admin:update:system:developer")
	exec(`UPDATE t_action SET word='admin:read:renamed:root' WHERE code=$1`, allCode)
	exec(`UPDATE t_action SET word='admin:read:renamed:manager' WHERE code=$1`, grantCode)
	exec(`UPDATE t_user SET role_id=NULL,action='',login_count=1,status=1,metadata='{}' WHERE id IN (2,3)`)
	manager, err := authn.New("test", strings.Repeat("a", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var identity authn.Identity
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT id,username,code,credential_version FROM t_user WHERE id=2`).Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/info", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set(authn.PermissionScopeHeader, "admin")
	verified, err := manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx := verified.Context()
	policy := auth.New(store, manager, nil, nil, nil, nil, nil, localizer, nil)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, localizer)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, localizer)
	if err != nil {
		t.Fatal(err)
	}
	actions.SetDefinitionCatalog(dictionaries)
	actions.SetPermissionAdministration(policy)
	roles := role.New(store, limits, actions)
	roles.SetPermissionAdministration(policy)
	users := user.New(store, limits, nil, nil, actions, roles)
	users.SetPermissionAdministration(policy)
	assign := func(codes ...string) { exec(`UPDATE t_user SET action=$1 WHERE id=2`, strings.Join(codes, ",")) }
	allowed := func(method, path string, want bool) {
		t.Helper()
		_, ok, err := policy.CheckPermission(ctx, auth.PermissionTarget{Method: method, Path: path})
		if err != nil || ok != want {
			t.Fatalf("%s %s: allowed=%v want=%v err=%v", method, path, ok, want, err)
		}
	}
	allowed("PATCH", "/user/3", false)
	assign(updateCode)
	allowed("PATCH", "/user/3", true)
	allowed("POST", "/user", false)
	allowed("PATCH", "/role/3", false)
	allowed("POST", "/action", false)
	selected := []string{readCode}
	if _, err := users.Update(ctx, 3, user.UpdateUserInput{ActionCodes: &selected}); err != nil {
		t.Fatalf("selected user update could not update user assignments: %v", err)
	}
	exec(`INSERT INTO t_action(id,action_group,code,word,config) VALUES(9000,'admin:system:high-risk','PARTIAL1','admin:read:custom:partial','{"bindings":[{"resource_key":"admin:system:user","operation":"update","feature_keys":["admin:system:user:edit"]}],"ui":[],"endpoints":[]}')`)
	assign("PARTIAL1")
	if _, err := users.Update(ctx, 3, user.UpdateUserInput{ActionCodes: &selected}); err != nil {
		t.Fatalf("explicit user update could not assign: %v", err)
	}

	if _, err := roles.Create(ctx, role.CreateRoleInput{Name: "Denied", Word: "denied", ActionCodes: selected}); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("user update granted role create: %v", err)
	}
	exec(`INSERT INTO t_action(id,action_group,code,word,config) VALUES(9001,'admin:system:user','FAKE0001','admin:grant:system:permission-admin','{"bindings":[],"ui":[],"endpoints":[]}'),(9002,'admin:system:user','FAKE0002','admin:all:system:all','{"bindings":[],"ui":[],"endpoints":[]}')`)
	assign("FAKE0001", "FAKE0002")
	allowed("PATCH", "/user/3", false)
	allowed("DELETE", "/unregistered/3", false)
	assign(grantCode)
	for _, path := range []string{"/user", "/role", "/action"} {
		allowed("POST", path, true)
	}
	allowed("POST", "/user-group", true)
	if err := policy.RequireAllPermissions(ctx); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
		t.Fatalf("CRUD permission became wildcard: %v", err)
	}
	wildcard := action.Config{Bindings: &[]action.ResourceBinding{
		{ResourceKey: "admin:high-risk:all", Operation: "*", FeatureKeys: []string{}},
	}, UI: []action.UIBinding{}, Endpoints: []action.Endpoint{}}
	if _, err := actions.Create(ctx, action.CreateActionInput{Word: "admin:read:custom:escalation", Group: "admin:system:high-risk", Config: wildcard}); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
		t.Fatalf("CRUD manager created wildcard: %v", err)
	}
	// The current developer action explicitly references all management resources
	// and catalog publishing, while remaining separate from a scope wildcard.
	exec(`UPDATE t_action SET word='admin:update:renamed:publisher' WHERE code=$1`, developerCode)
	assign(developerCode)
	for _, operation := range []string{"read", "create", "update", "delete"} {
		if err := policy.RequirePermissionOperation(ctx, "action", operation); err != nil {
			t.Fatalf("developer lacks action %s: %v", operation, err)
		}
		for _, resource := range []string{"user", "role"} {
			if err := policy.RequirePermissionOperation(ctx, resource, operation); err != nil {
				t.Fatalf("developer lost referenced %s %s: %v", resource, operation, err)
			}
		}
	}
	allowed("POST", "/action", true)
	allowed("POST", "/user", true)
	if err := policy.RequireAllPermissions(ctx); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
		t.Fatalf("developer became a scope wildcard: %v", err)
	}
	for _, targetCode := range []string{allCode, grantCode, code("admin:default:system:default")} {
		var id int64
		var group string
		if err := store.SQL(ctx).QueryRowContext(ctx, `SELECT id,action_group FROM t_action WHERE code=$1`, targetCode).Scan(&id, &group); err != nil {
			t.Fatal(err)
		}
		if _, err := actions.Update(ctx, id, action.UpdateActionInput{Group: &group}); err != nil {
			t.Fatalf("developer cannot edit definition %d: %v", id, err)
		}
	}
	created, err := actions.Create(ctx, action.CreateActionInput{Word: "admin:read:custom:published", Group: "admin:system:high-risk", Config: wildcard})
	if err != nil {
		t.Fatalf("developer cannot create wildcard definition: %v", err)
	}
	if err := actions.Delete(ctx, created.ID); err != nil {
		t.Fatalf("developer cannot delete wildcard definition: %v", err)
	}
	exec(`INSERT INTO t_action(id,action_group,code,word,config) VALUES(9003,'admin:system:developer','FAKEDEV1','admin:update:system:developer','{"bindings":[],"ui":[],"endpoints":[]}')`)
	assign("FAKEDEV1")
	if err := policy.RequirePermissionOperation(ctx, "action", "update"); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("developer word bypassed resource bindings: %v", err)
	}
	assign(allCode)
	allowed("DELETE", "/unregistered/3", true)
	info, err := policy.Info(ctx, "admin")
	if err != nil || info.Permission.PermissionScope != "*" {
		t.Fatalf("renamed wildcard identity lost scope: %v", err)
	}
	if err := policy.RequireAllPermissions(authn.WithPermissionScope(ctx, "mini")); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
		t.Fatalf("wildcard leaked across scopes: %v", err)
	}
	assign("PARTIAL1")
	exec(`UPDATE t_user SET action='' WHERE id=3`)
	blocker, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := blocker.ExecContext(t.Context(), `SELECT pg_advisory_xact_lock(73482301991337)`); err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.ExecContext(t.Context(), `UPDATE t_user SET action='' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	queued, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := users.Update(queued, 3, user.UpdateUserInput{ActionCodes: &selected}); done <- err }()
	select {
	case err := <-done:
		t.Fatalf("mutation escaped permission lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("revoked permission reused: %v", err)
	}
	var assigned string
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT action FROM t_user WHERE id=3`).Scan(&assigned); err != nil || assigned != "" {
		t.Fatalf("denied assignment persisted: %q %v", assigned, err)
	}
}
