package tests

import (
	"errors"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

func TestActionGroupControlsPostgres(t *testing.T) {
	store := permissionDatabase(t)
	catalog := testLocalization(t, store)
	limits, _ := pagination.New(100, 100)
	groups := action.New(store, limits, catalog)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, catalog)
	if err != nil {
		t.Fatal(err)
	}
	groups.SetGroupStorage(dictionaries)
	dictionaries.SetPermissionGroups(groups)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("UPDATE t_user SET action='GRPCTRL1', role_id=NULL WHERE id=2")
	exec("DELETE FROM t_user_user_group_relation WHERE user_id=2")
	exec(`INSERT INTO t_action(code,word,action_group,config) VALUES('GRPCTRL1','admin:update:test:groups','admin:system:action','{"bindings":[],"ui":[],"endpoints":[]}')`)
	grant := func(operation, feature string) {
		t.Helper()
		exec(`UPDATE t_action SET config=jsonb_build_object('bindings',jsonb_build_array(jsonb_build_object('resource_key','admin:system:action','operation',$1::text,'feature_keys',jsonb_build_array($2::text))),'ui','[]'::jsonb,'endpoints','[]'::jsonb) WHERE code='GRPCTRL1'`, operation, "admin:system:action:"+feature)
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
	req := httptest.NewRequest("GET", "/info", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req, err = manager.AuthenticateRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx := req.Context()
	policy := auth.New(store, manager, nil, nil, nil, nil, nil, catalog, nil, &auth.Verification{})
	groups.SetPermissionAdministration(policy)
	initial, err := dictionaries.ActionCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for control, op := range map[string]string{"create-group": "create", "edit-group": "update", "reorder-groups": "update", "delete-group": "delete"} {
		key := "admin:system:action:" + control
		feature := initial.ResourceCatalog.Buttons[key]
		path := "/action/group"
		if control == "reorder-groups" {
			path += "/order"
		}
		if len(feature.HTTP) != 1 || feature.HTTP[0] != (localization.HTTPPath{Method: "PATCH", Path: path}) ||
			!slices.Contains(feature.Btn, "admin:btn:system:action:"+control) ||
			!slices.Contains(initial.ResourceCatalog.Resources["admin:system:action"].Operations[op].ButtonKeys, key) {
			t.Fatalf("incorrect %s mapping: %+v", control, feature)
		}
	}
	old := "admin:test:groups"
	next := "admin:test:renamed"
	names := map[string]string{"en-US": "Group", "zh-CN": "分组"}
	create := action.GroupChange{Operation: "create", Word: old, Names: names}
	grant("create", "create-group")
	for _, path := range []string{"/dictionary/3", "/dictionary/catalog", "/action/group/order"} {
		if ok, err := policy.AuthorizeHTTP(ctx, "PATCH", path); err != nil || ok {
			t.Fatalf("create-group unexpectedly grants %s: %t %v", path, ok, err)
		}
	}
	if err := groups.UpdateGroup(ctx, create); err != nil {
		t.Fatal(err)
	}
	if err := groups.UpdateGroup(ctx, create); !errors.Is(err, action.ErrGroupConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	change := action.GroupChange{Operation: "update", Word: next, OriginalWord: old, ExpectedNames: names}
	if err := groups.UpdateGroup(ctx, change); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("create grant edited a group: %v", err)
	}
	order := action.GroupOrder{ExpectedOrder: append(slices.Clone(initial.GroupOrder), old), Order: append([]string{old}, initial.GroupOrder...)}
	if err := groups.ReorderGroups(ctx, order); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("create grant reordered: %v", err)
	}
	grant("update", "reorder-groups")
	if err := groups.ReorderGroups(ctx, order); err != nil {
		t.Fatal(err)
	}
	if err := groups.ReorderGroups(ctx, order); !errors.Is(err, action.ErrGroupConflict) {
		t.Fatalf("stale reorder: %v", err)
	}
	grant("update", "edit-group")
	stale := change
	stale.ExpectedNames = map[string]string{"zh-CN": "old", "en-US": "stale"}
	if err := groups.UpdateGroup(ctx, stale); !errors.Is(err, action.ErrGroupConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	exec("UPDATE t_action SET action_group=$1 WHERE code='GRPCTRL1'", old)
	if err := groups.UpdateGroup(ctx, change); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := store.DB.QueryRowContext(ctx, "SELECT action_group FROM t_action WHERE code='GRPCTRL1'").Scan(&stored); err != nil || stored != next {
		t.Fatalf("rename: %s %v", stored, err)
	}
	change = action.GroupChange{Operation: "delete", Word: next, ExpectedNames: names}
	if err := groups.UpdateGroup(ctx, change); !errors.Is(err, authn.ErrPermissionGrantRequired) {
		t.Fatalf("edit grant deleted a group: %v", err)
	}
	grant("delete", "delete-group")
	if err := groups.UpdateGroup(ctx, change); !errors.Is(err, action.ErrGroupNotEmpty) {
		t.Fatalf("occupied: %v", err)
	}
	exec("UPDATE t_action SET action_group='admin:system:action' WHERE code='GRPCTRL1'")
	if err := groups.UpdateGroup(ctx, change); err != nil {
		t.Fatal(err)
	}
	final, err := dictionaries.ActionCatalog(ctx)
	if err != nil || !reflect.DeepEqual(final, initial) {
		t.Fatalf("final group state: %v", err)
	}
}
