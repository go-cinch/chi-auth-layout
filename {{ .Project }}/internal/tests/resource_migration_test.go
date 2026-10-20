package tests

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/server"
	migrationdb "{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/auth"

	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

type resourcePermissionWriter struct {
	actions *action.Module
	failure error
}

func (w resourcePermissionWriter) ReconcileResourcePermissions(ctx context.Context, before, after json.RawMessage, resources, features map[string]string) (bool, error) {
	valid, err := w.actions.ReconcileResourcePermissions(ctx, before, after, resources, features)
	if err != nil || !valid {
		return valid, err
	}
	return true, w.failure
}

func TestCatalogResourceMoveCommitsAndRollsBackAcrossModules(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, localizer)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, localizer)
	if err != nil {
		t.Fatal(err)
	}
	if err := dictionaries.LoadLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	dictionaries.SetPermissionGroups(actions)
	const oldKey = "admin:system:user"
	const newKey = "admin:people:person"
	var dictionaryID int64
	var before json.RawMessage
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT id,value FROM t_dictionary WHERE dictionary_key='I18N_CATALOG'`).Scan(&dictionaryID, &before); err != nil {
		t.Fatal(err)
	}
	after := json.RawMessage(strings.ReplaceAll(strings.ReplaceAll(string(before), oldKey+`"`, newKey+`"`), oldKey+":", newKey+":"))
	features := map[string]string{}
	for key := range localizer.Current().ResourceCatalog.Buttons {
		if strings.HasPrefix(key, oldKey+":") {
			features[key] = strings.Replace(key, oldKey, newKey, 1)
		}
	}
	input := dictionary.UpdateDictionaryInput{Value: &after, ExpectedValue: &before, ResourceRenames: map[string]string{oldKey: newKey}, FeatureRenames: features, GroupRenames: map[string]string{oldKey: newKey}}
	failure := errors.New("injected failure after action writes")
	dictionaries.SetResourcePermissions(resourcePermissionWriter{actions: actions, failure: failure})
	if _, err := dictionaries.Update(t.Context(), dictionaryID, input); !errors.Is(err, failure) {
		t.Fatalf("expected rollback: %v", err)
	}
	var key, group, code string
	read := func() {
		t.Helper()
		if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT config->'bindings'->0->>'resource_key',action_group,code FROM t_action WHERE word='admin:read:system:user'`).Scan(&key, &group, &code); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if key != oldKey || group != oldKey {
		t.Fatalf("action escaped rollback: %s %s", key, group)
	}
	var unchanged bool
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT value=$2::jsonb FROM t_dictionary WHERE id=$1`, dictionaryID, string(before)).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("dictionary escaped rollback", err)
	}
	stableCode := code
	dictionaries.SetResourcePermissions(actions)
	if _, err := dictionaries.Update(t.Context(), dictionaryID, input); err != nil {
		t.Fatal(err)
	}
	read()
	if key != newKey || group != newKey || code != stableCode {
		t.Fatalf("inconsistent resource move: %s %s %s", key, group, code)
	}
	if _, ok := localizer.Current().ResourceCatalog.Resources[newKey]; !ok {
		t.Fatal("committed catalog was not installed")
	}
}

func TestDeveloperPublishRefreshesFiltersAndStoredProjections(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, localizer)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, localizer)
	if err != nil {
		t.Fatal(err)
	}
	if err := dictionaries.LoadLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	actions.SetDefinitionCatalog(dictionaries)
	dictionaries.SetResourcePermissions(actions)
	manager, err := authn.New("publish-test", strings.Repeat("s", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	policy := auth.New(store, manager, nil, nil, nil, nil, nil, localizer, nil)
	actions.SetPermissionAdministration(policy)
	dictionaries.SetPermissionAdministration(policy)
	var identity authn.Identity
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT id,username,code,credential_version FROM t_user WHERE id=1`).Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Auth.Authorization.Enabled = true
	cfg.Idempotency.TTL = time.Minute
	handler, err := server.NewRouter(cfg, manager, idempotency.NewMemoryStore(), policy, actions, dictionaries)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body []byte, want int) []byte {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(string(body)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	var before json.RawMessage
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT value FROM t_dictionary WHERE dictionary_key='I18N_CATALOG'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	c := localizer.Current()
	for key, feature := range c.ResourceCatalog.Buttons {
		for i, endpoint := range feature.HTTP {
			if endpoint.Path == "/user" || strings.HasPrefix(endpoint.Path, "/user/") {
				feature.HTTP[i].Path = strings.Replace(endpoint.Path, "/user", "/review-users", 1)
			}
		}
		for i, button := range feature.Btn {
			if button == "admin:btn:system:user:query" {
				feature.Btn[i] = "admin:btn:system:user:search"
			}
		}
		c.ResourceCatalog.Buttons[key] = feature
	}
	node := c.ResourceCatalog.Resources["admin:system:user"]
	entry := node.Entries["admin:menu:system:user"]
	entry.Path = "/people/users"
	node.Entries["admin:menu:system:user"] = entry
	c.ResourceCatalog.Resources["admin:system:user"] = node
	after, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"expected_value": before, "value": json.RawMessage(after)})
	dictionaries.SetResourcePermissions(resourcePermissionWriter{actions: actions, failure: errors.New("rollback projection writes")})
	request("PATCH", "/dictionary/3", body, 500)
	var unchanged bool
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT value=$1::jsonb FROM t_dictionary WHERE id=3`, string(before)).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("failed publication escaped rollback", err)
	}
	dictionaries.SetResourcePermissions(actions)
	request("PATCH", "/dictionary/3", body, 200)
	for _, test := range []struct {
		filter string
		want   int64
	}{
		{"resource=" + url.QueryEscape("/review-users"), 1}, {"resource=" + url.QueryEscape("/user"), 0},
		{"menu=" + url.QueryEscape("/people/users"), 1}, {"menu=" + url.QueryEscape("/system/user"), 0},
		{"button=" + url.QueryEscape("admin:btn:system:user:search"), 1}, {"button=" + url.QueryEscape("admin:btn:system:user:query"), 0},
	} {
		raw := request("GET", "/action?f=id,resource,menu,button&word=admin:read:system:user&group_exact=admin:system:user&"+test.filter, nil, 200)
		var result action.ListActionsResult
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.Total != test.want || int64(len(result.Items)) != test.want {
			t.Fatalf("%s: %s", test.filter, raw)
		}
	}
	var stored action.Config
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT config FROM t_action WHERE id=4`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range stored.Endpoints {
		if strings.HasPrefix(endpoint.Path, "/user") {
			t.Fatal("stored endpoint not refreshed", endpoint)
		}
	}
	if stored.UI[0].MenuPath != "/people/users" {
		t.Fatal(stored.UI)
	}
	// Built-in all participates in filtering through the same expansion, without violating its storage constraint.
	raw := request("GET", "/action?f=id,resource&word=admin:all:system:all&resource="+url.QueryEscape("/review-users"), nil, 200)
	var all action.ListActionsResult
	if err := json.Unmarshal(raw, &all); err != nil || all.Total != 1 {
		t.Fatal(string(raw), err)
	}
	// Recreate a stale pre-fix projection; snapshot-based filters must still match the displayed route.
	if _, err := store.SQL(t.Context()).ExecContext(t.Context(), `UPDATE t_action SET config=jsonb_set(config,'{endpoints}','[]'::jsonb),updated_at=updated_at+1 WHERE id=4`); err != nil {
		t.Fatal(err)
	}
	raw = request("GET", "/action?f=id,resource&word=admin:read:system:user&group_exact=admin:system:user&resource="+url.QueryEscape("/review-users"), nil, 200)
	var stale action.ListActionsResult
	if err := json.Unmarshal(raw, &stale); err != nil || stale.Total != 1 || !strings.Contains(stale.Items[0].Resource, "/review-users") {
		t.Fatal(string(raw), err)
	}
	// Exercise the same authenticated PATCH used by the developer workbench.
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT value FROM t_dictionary WHERE id=3`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	emptyCatalog := localizer.Current()
	hidden := false
	emptyCatalog.ResourceCatalog.Resources["admin:high-risk"] = localization.Resource{Kind: "directory", Sort: 5, FrontendVisible: &hidden, PermissionGroup: "admin:system:high-risk"}
	emptyCatalog.ResourceCatalog.Resources["admin:high-risk:all"] = localization.Resource{Kind: "resource", Parent: "admin:high-risk", PermissionScope: "*"}
	emptyCatalog.ResourceCatalog.Resources["admin:high-risk:unconfigured"] = localization.Resource{Kind: "resource", Parent: "admin:high-risk"}
	emptyValue, err := json.Marshal(emptyCatalog)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]any{"expected_value": before, "value": json.RawMessage(emptyValue)})
	request("PATCH", "/dictionary/3", body, 200)

}

func TestPublishEmptyResourcesKeepsAssociations(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, localizer)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, localizer)
	if err != nil {
		t.Fatal(err)
	}
	if err := dictionaries.LoadLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	dictionaries.SetResourcePermissions(actions)
	var before json.RawMessage
	var dictionaryID int64
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT id,value FROM t_dictionary WHERE dictionary_key='I18N_CATALOG'`).Scan(&dictionaryID, &before); err != nil {
		t.Fatal(err)
	}
	catalog := localizer.Current()
	hidden := false
	catalog.ResourceCatalog.Resources["admin:high-risk"] = localization.Resource{Kind: "directory", Sort: 5, FrontendVisible: &hidden, PermissionGroup: "admin:system:high-risk"}
	catalog.ResourceCatalog.Resources["admin:high-risk:all"] = localization.Resource{Kind: "resource", Parent: "admin:high-risk", PermissionScope: "*"}
	catalog.ResourceCatalog.Resources["admin:high-risk:unconfigured"] = localization.Resource{Kind: "resource", Parent: "admin:high-risk", Operations: map[string]localization.ResourceOperation{}}
	after, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	value := json.RawMessage(after)
	input := dictionary.UpdateDictionaryInput{Value: &value, ExpectedValue: &before}
	if _, err := dictionaries.Update(t.Context(), dictionaryID, input); err != nil {
		t.Fatalf("publish empty resources: %v", err)
	}
	var saved json.RawMessage
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT value FROM t_dictionary WHERE id=$1`, dictionaryID).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	persisted, err := localization.Parse(saved)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ResourceCatalog.Resources["admin:high-risk"].PermissionGroup != "admin:system:high-risk" || persisted.ResourceCatalog.Resources["admin:high-risk:all"].PermissionScope != "*" {
		t.Fatal("saved associations were lost")
	}
	if len(persisted.ResourceCatalog.Resources["admin:high-risk:all"].Operations) != 0 {
		t.Fatal("publication invented an operation")
	}
	if _, err := dictionaries.Update(t.Context(), dictionaryID, input); !errors.Is(err, dictionary.ErrVersionConflict) {
		t.Fatalf("stale version did not conflict: %v", err)
	}
}

func TestResourcePermissionMigrationsAreReversible(t *testing.T) {
	store := permissionDatabase(t)
	sqlPart := func(suffix string, down bool) string {
		t.Helper()
		files, err := fs.Glob(migrationdb.SQLFiles, migrationdb.SQLRoot+"/*-"+suffix+".sql")
		if err != nil || len(files) != 1 {
			t.Fatalf("migration lookup %s: %v %v", suffix, files, err)
		}
		raw, err := migrationdb.SQLFiles.ReadFile(files[0])
		if err != nil {
			t.Fatal(err)
		}
		up, undo, ok := strings.Cut(string(raw), "-- +migrate Down")
		if !ok {
			t.Fatal("missing Down")
		}
		if down {
			return undo
		}
		return up
	}
	suffixes := []string{"01-auth-schema", "02-auth-default-data", "03-action-default-data", "04-dictionary-default-data"}
	if err := store.Tx(t.Context(), func(ctx context.Context) error {
		for i := len(suffixes) - 1; i >= 0; i-- {
			if _, err := store.SQL(ctx).ExecContext(ctx, sqlPart(suffixes[i], true)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("rollback four consolidated migrations: %v", err)
	}
	var removed bool
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT to_regclass('t_user') IS NULL AND to_regclass('t_action') IS NULL AND to_regclass('t_dictionary') IS NULL`).Scan(&removed); err != nil || !removed {
		t.Fatalf("schema was not removed: %v", err)
	}
	if err := store.Tx(t.Context(), func(ctx context.Context) error {
		for _, suffix := range suffixes {
			if _, err := store.SQL(ctx).ExecContext(ctx, sqlPart(suffix, false)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("reapply four consolidated migrations: %v", err)
	}
	var users, actions int
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), `SELECT (SELECT COUNT(*) FROM t_user),(SELECT COUNT(*) FROM t_action)`).Scan(&users, &actions); err != nil || users != 8 || actions != 31 {
		t.Fatalf("seed restoration: users=%d actions=%d err=%v", users, actions, err)
	}
}
