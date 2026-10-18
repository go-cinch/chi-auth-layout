package tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"github.com/lib/pq"

	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/role"
	"{{ .Computed.module_name_final }}/internal/modules/user"
)

// CHI_AUTH_TEST_DSN must point to an isolated PostgreSQL test database.
func permissionDatabase(t *testing.T) *db.Store {
	t.Helper()
	dsn := os.Getenv("CHI_AUTH_TEST_DSN")
	if dsn == "" {
		t.Skip("set CHI_AUTH_TEST_DSN to an isolated PostgreSQL test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("CHI_AUTH_TEST_DSN must be a PostgreSQL URL")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := fmt.Sprintf("permission_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	database, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	files, err := fs.Glob(db.SQLFiles, db.SQLRoot+"/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		content, err := db.SQLFiles.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		up, _, found := strings.Cut(string(content), "-- +migrate Down")
		if !found {
			t.Fatalf("migration missing Down: %s", file)
		}
		if _, err := database.ExecContext(t.Context(), up); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}

	if _, err := database.ExecContext(t.Context(), `UPDATE t_user SET status=2, metadata='{"lock_expired_at":1790000000000}', last_logged_in_at=1788220800000 WHERE id=3`); err != nil {
		t.Fatal(err)
	}
	return &db.Store{DB: database}
}

func TestDefaultPermissionHTTP(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	// A user with no assigned actions must receive only the seeded default action.
	exec(`UPDATE t_user SET role_id=NULL, action='', login_count=1 WHERE id=2`)
	var identity authn.Identity
	if err := store.DB.QueryRowContext(t.Context(), `SELECT id, username, code, credential_version FROM t_user WHERE id=2`).Scan(
		&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion,
	); err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions(auth.NewMemorySessionStore(), time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := authn.New("test", strings.Repeat("x", 32), time.Hour, auth.NewIdentityValidator(store, sessions, auth.Switches{PasswordResetRequired: true}))
	if err != nil {
		t.Fatal(err)
	}
	session, err := sessions.Issue(t.Context(), identity, false)
	if err != nil {
		t.Fatal(err)
	}
	identity.SessionID = session.SessionID
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	m := auth.New(store, manager, nil, sessions, nil, nil, nil, auth.Switches{PasswordResetRequired: true}, localizer)
	cfg := &config.Config{}
	cfg.Auth.Authorization.Enabled = true
	handler, err := server.NewRouter(cfg, manager, nil, m)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, want int) {
		t.Helper()
		// Malformed JSON reaches the handler only after authentication and authorization.
		r := httptest.NewRequest(method, path, strings.NewReader("{"))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, w.Code, want, w.Body.String())
		}
	}
	var defaultResources string
	if err := store.DB.QueryRowContext(t.Context(), `SELECT config::text FROM t_action WHERE config->>'builtin'='default' AND scope='admin'`).Scan(&defaultResources); err != nil {
		t.Fatal(err)
	}
	for _, restricted := range []bool{false, true} {
		count := 1
		if restricted {
			count = 0
		}
		exec(`UPDATE t_user SET login_count=$1 WHERE id=2`, count)
		for _, granted := range []bool{true, false} {
			resources := `{"builtin":"default","ui":[],"endpoints":[]}`
			if granted {
				resources = defaultResources
			}
			exec(`UPDATE t_action SET config=$1 WHERE config->>'builtin'='default' AND scope='admin'`, resources)
			want := 403
			if granted {
				want = 400
			}
			request("POST", "/auth/challenge", want)
			request("POST", "/auth/logout", 403)
			request("PATCH", "/auth/reset/pwd", want)
			captchaWant := want
			if restricted {
				captchaWant = 403
			}
			request("POST", "/auth/captcha/verify", captchaWant)
		}
	}
	// Even all permissions cannot lift the first-login restriction, including via the gateway.
	exec(`UPDATE t_action SET config=$1 WHERE config->>'builtin'='default' AND scope='admin'`, defaultResources)
	exec(`UPDATE t_user SET action=(SELECT code FROM t_action WHERE scope='admin' AND config->>'builtin'='all') WHERE id=2`)
	request("GET", "/auth/info", 403)
	for _, target := range []struct {
		method, path string
		want         int
	}{
		{"PATCH", "/auth/reset/pwd", 200},
		{"POST", "/auth/challenge", 200},
		{"GET", "/auth/info", 403},
		{"POST", "/auth/captcha/verify", 403},
	} {
		r := httptest.NewRequest("GET", "/auth/permission", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Original-Method", target.method)
		r.Header.Set("X-Original-URI", target.path)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != target.want {
			t.Fatalf("gateway %s %s: got %d, want %d: %s", target.method, target.path, w.Code, target.want, w.Body.String())
		}
	}
}

func TestPermissionSQL(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	var lockedAt int64
	var loggedAt int64
	if err := store.DB.QueryRowContext(ctx, `SELECT (metadata->>'lock_expired_at')::bigint, last_logged_in_at FROM t_user WHERE id=3`).Scan(&lockedAt, &loggedAt); err != nil {
		t.Fatal(err)
	}
	if lockedAt != 1790000000000 || loggedAt != time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("timestamp migration: %d %v", lockedAt, loggedAt)
	}
	makeConfig := func(method, path, resource, operation string) string {
		c := registerPermissionFixture(t, localizer, "admin:test:"+resource, operation, "/"+resource, []localization.HTTPPath{ {Method: method, Path: path}})
		encoded, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	var defaultConfig action.Config
	if err := json.Unmarshal([]byte(makeConfig("GET", "/default", "common", "read")), &defaultConfig); err != nil {
		t.Fatal(err)
	}
	defaultConfig.Builtin = "default"
	defaultConfig.Bindings = nil
	defaultConfig.ResourceKeys = &[]string{"admin:test:common"}
	defaultConfig.FeatureKeys = []string{"admin:test:common:access"}
	encodedDefault, err := json.Marshal(defaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE t_action SET config=$1 WHERE config->>'builtin'='default' AND scope='admin'`, string(encodedDefault))

	for _, item := range []struct{ code, word, method, path, resource string }{
		{"DIRECT01", "admin:read:test:direct", "GET", "/direct", "direct"},
		{"ROLE0001", "admin:update:test:role", "PATCH", "/role-target", "role"},
		{"GROUP001", "admin:delete:test:group", "DELETE", "/group-target", "group"},
	} {
		exec(`INSERT INTO t_action (action_group,code,word,config) VALUES($1,$2,$3,$4)`, action.GroupForWord(item.word), item.code, item.word, makeConfig(item.method, item.path, item.resource, strings.Split(item.word, ":")[1]))
	}
	exec(`INSERT INTO t_role(id,name,word,action) VALUES(101,'Test','test','ROLE0001')`)
	exec(`INSERT INTO t_user(id,username,code,password,status,role_id,action) VALUES(101,'tester','TEST0001','unused',1,101,' DIRECT01, DIRECT01, ROLE0001, MISSING1 ')`)
	exec(`INSERT INTO t_user_group(id,name,word,action) VALUES(101,'Test','test','GROUP001, DIRECT01')`)
	exec(`INSERT INTO t_user_user_group_relation(user_id,user_group_id) VALUES(101,101)`)
	manager, err := authn.New("test", strings.Repeat("x", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(authn.Identity{CredentialVersion: 1, UserID: 101, Username: "tester", Code: "TEST0001"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/auth/info", nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+token)
	request, err = manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx = request.Context()
	m := auth.New(store, manager, nil, nil, nil, nil, nil, auth.Switches{PasswordResetRequired: true}, localizer)
	info, err := m.Info(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if info.Scope != "admin" || len(info.Permission.Words) != 4 || len(info.Permission.Menus) != 4 {
		t.Fatalf("aggregated permission: %#v", info.Permission)
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{`"resources"`, `"endpoints"`, `GET|`, `/role-target`} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("private transport mapping leaked: %s", encoded)
		}
	}
	check := func(method, path string, want bool) {
		t.Helper()
		_, allowed, err := m.CheckPermission(ctx, auth.PermissionTarget{Method: method, Path: path})
		if err != nil || allowed != want {
			t.Fatalf("%s %s = %v, %v", method, path, allowed, err)
		}
	}
	check("GET", "/default", true)
	check("GET", "/direct", true)
	check("PATCH", "/role-target", true)
	check("DELETE", "/group-target", true)
	check("DELETE", "/direct", false)
	exec(`DELETE FROM t_user_user_group_relation WHERE user_id=101`)
	check("DELETE", "/group-target", false)
	check("GET", "/direct", true)
	exec(`UPDATE t_user SET action='',role_id=NULL WHERE id=101`)
	check("GET", "/direct", false)
	check("PATCH", "/role-target", false)
	check("GET", "/default", true)
	exec(`DELETE FROM t_action WHERE config->>'builtin'='default' AND scope='admin'`)
	check("GET", "/default", false)
	info, err = m.Info(ctx, "admin")
	if err != nil || len(info.Permission.Resources) != 0 || info.Permission.Resources == nil {
		t.Fatalf("empty permissions: %#v, %v", info, err)
	}
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false, localizer)
	roles := role.New(store, limits, actions, false)
	users := user.New(store, limits, nil, nil, actions, roles, auth.Switches{PasswordResetRequired: true})
	value, err := users.Get(ctx, 3)
	if err != nil || value.LastLoggedInAt == nil || value.Metadata["lock_expired_at"] != float64(1790000000000) {
		t.Fatalf("migrated user: %#v %v", value, err)
	}
	rollback := fmt.Errorf("rollback test")
	err = store.Tx(ctx, func(txCtx context.Context) error {
		_, err := store.SQL(txCtx).ExecContext(txCtx, `UPDATE t_user SET action='DIRECT01' WHERE id=101`)
		if err != nil {
			return err
		}
		value, err := users.Get(txCtx, 101)
		if err != nil {
			return err
		}
		if len(value.Actions) != 1 {
			return fmt.Errorf("injected action lookup lost transaction context")
		}
		return rollback
	})
	if err != rollback {
		t.Fatal(err)
	}
	check("GET", "/direct", false)
}

func TestPermissionScopeAndOperationSQL(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(t.Context(), query, args...); err != nil {
			t.Fatal(err)
		}
	}
	miniConfig := registerPermissionFixture(t, localizer, "mini:content:private", "read", "/private", []localization.HTTPPath{ {Method: "GET", Path: "/mini/private"}, {Method: "GET", Path: "/user"}})
	encodedMini, err := json.Marshal(miniConfig)
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO t_action(action_group,code,word,config) VALUES ('mini:content:private','MINI0001','mini:read:content:private',$1)`, string(encodedMini))
	exec(`UPDATE t_user SET role_id=NULL,action=(SELECT code FROM t_action WHERE config->>'builtin'='all' AND scope='admin') WHERE id=2`)
	var identity authn.Identity
	if err := store.DB.QueryRowContext(t.Context(), `SELECT id,username,code,credential_version FROM t_user WHERE id=2`).Scan(&identity.UserID, &identity.Username, &identity.Code, &identity.CredentialVersion); err != nil {
		t.Fatal(err)
	}
	manager, err := authn.New("test", strings.Repeat("s", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(identity)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/auth/info?scope=admin", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request, err = manager.AuthenticateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	ctx := request.Context()
	m := auth.New(store, manager, nil, nil, nil, nil, nil, auth.Switches{}, localizer)
	check := func(method, path string, want bool) {
		t.Helper()
		_, allowed, err := m.CheckPermission(ctx, auth.PermissionTarget{Method: method, Path: path})
		if err != nil || allowed != want {
			t.Fatalf("%s %s: allowed=%v err=%v", method, path, allowed, err)
		}
	}
	check("GET", "/user", true)
	check("DELETE", "/user/42", true)
	check("GET", "/mini/private", false)
	mini, err := m.Info(ctx, "mini")
	if err != nil || len(mini.Permission.Words) != 0 || len(mini.Permission.Menus) != 0 {
		t.Fatalf("admin all leaked into mini: %+v %v", mini, err)
	}
	exec(`UPDATE t_user SET action=action||',MINI0001' WHERE id=2`)
	check("GET", "/mini/private", false)
	if _, allowed, err := m.CheckPermission(authn.WithPermissionScope(ctx, "mini"), auth.PermissionTarget{Method: "GET", Path: "/mini/private"}); err != nil || !allowed {
		t.Fatalf("mini explicit grant: %v %v", allowed, err)
	}
	admin, err := m.Info(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	for _, word := range admin.Permission.Words {
		if !strings.HasPrefix(word, "admin:") {
			t.Fatalf("cross-scope word %s", word)
		}
	}
	mini, err = m.Info(ctx, "mini")
	if err != nil || len(mini.Permission.Menus) != 1 || mini.Permission.Menus[0].MenuKey != "mini:menu:content:private" {
		t.Fatalf("mini projection: %+v %v", mini, err)
	}
	// Assignment changes take effect on every request, including an already issued session.
	exec(`UPDATE t_user SET action=(SELECT code FROM t_action WHERE word='admin:read:system:user') WHERE id=2`)
	check("GET", "/user", true)
	check("POST", "/user", false)
	check("PATCH", "/user/42", false)
	check("DELETE", "/user/42", false)
	check("GET", "/mini/private", false)
	exec(`UPDATE t_user SET action=(SELECT code FROM t_action WHERE word='admin:read:audit:login') WHERE id=2`)
	check("GET", "/audit/login", true)
	check("GET", "/audit/login/export", true)
	exec(`UPDATE t_action SET config=JSONB_SET(config,'{bindings,0,feature_keys}',(config #> '{bindings,0,feature_keys}') - 'admin:audit:login:export-report') WHERE word='admin:read:audit:login'`)
	check("GET", "/audit/login/export", false)
	// A shared endpoint is evaluated within the selected scope, never a union.
	exec(`UPDATE t_user SET action='MINI0001' WHERE id=2`)
	check("GET", "/user", false)
	if _, allowed, err := m.CheckPermission(authn.WithPermissionScope(ctx, "mini"), auth.PermissionTarget{Method: "GET", Path: "/user"}); err != nil || !allowed {
		t.Fatalf("shared mini endpoint: %v %v", allowed, err)
	}
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false, localizer)
	roles := role.New(store, limits, actions, false)
	users := user.New(store, limits, nil, nil, actions, roles, auth.Switches{})
	cfg := &config.Config{}
	cfg.Auth.Authorization.Enabled = true
	cfg.Idempotency.TTL = time.Minute
	handler, err := server.NewRouter(cfg, manager, idempotency.NewMemoryStore(), m, users)
	if err != nil {
		t.Fatal(err)
	}
	httpCheck := func(path string, scopes []string, want int) {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		for _, scope := range scopes {
			r.Header.Add(authn.PermissionScopeHeader, scope)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("HTTP %s scope=%v: %d want %d: %s", path, scopes, w.Code, want, w.Body.String())
		}
	}
	httpCheck("/user", nil, 403)
	httpCheck("/user", []string{"admin"}, 403)
	httpCheck("/user", []string{"mini"}, 200)
	httpCheck("/user", []string{""}, 400)
	httpCheck("/user", []string{"admin", "mini"}, 400)
	httpCheck("/auth/info?scope=admin", []string{"mini"}, 403)
 httpCheck("/auth/info", []string{"admin"}, 200)
 httpCheck("/auth/info?scope=mini", []string{"admin"}, 200)
	// The actual server registers static-route precedence for both direct requests and gateway checks.
	detailConfig := registerPermissionFixture(t, localizer, "admin:system:detail", "read", "/user/detail", []localization.HTTPPath{ {Method: "GET", Path: "/user/{id}"}})
	encodedDetail, err := json.Marshal(detailConfig)
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO t_action(action_group,code,word,config) VALUES('admin:system:detail','DETAIL01','admin:read:system:detail',$1)`, string(encodedDetail))
	exec(`UPDATE t_user SET action='DETAIL01' WHERE id=2`)
	httpCheck("/user/2", nil, 200)
	httpCheck("/user/summary", nil, 403)
	gateway := httptest.NewRequest("GET", "/auth/permission", nil)
	gateway.Header.Set("Authorization", "Bearer "+token)
	gateway.Header.Set("X-Original-Method", "GET")
	gateway.Header.Set("X-Original-URI", "/user/summary")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, gateway)
	if response.Code != 403 {
		t.Fatalf("static gateway boundary: %d %s", response.Code, response.Body.String())
	}

}

func TestFinalizedSeedBindingsPostgres(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	var raw []byte
	if err := store.DB.QueryRowContext(t.Context(), `SELECT value FROM t_dictionary WHERE dictionary_key='I18N_CATALOG'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(localizer.Reset)
	catalog := localizer.Current()
	for locale, messages := range catalog.Messages {
		if len(messages.Shared) != 0 {
			t.Fatalf("%s retains retired shared messages", locale)
		}
	}
	rows, err := store.DB.QueryContext(t.Context(), `SELECT word, action_group, config FROM t_action ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var word, group string
		var rawConfig []byte
		if err := rows.Scan(&word, &group, &rawConfig); err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(word, ":")
		groupParts := strings.Split(group, ":")
		expected := groupParts[0] + ":" + parts[1] + ":" + strings.Join(groupParts[1:], ":")
		if word != expected {
			t.Errorf("seed word %s does not match business group %s", word, group)
		}
		if !catalog.GroupKeys()[group] {
			t.Errorf("unregistered seed group %s", group)
		}
		var config action.Config
		if err := json.Unmarshal(rawConfig, &config); err != nil {
			t.Fatal(err)
		}
		if err := config.Validate(word); err != nil {
			t.Fatalf("invalid seed %s: %v", word, err)
		}
		if config.Builtin != "all" {
			resolved, err := action.ResolveConfig(localizer.Current(), word, config)
			if err != nil {
				t.Fatalf("resolve seed %s: %v", word, err)
			}
			if !reflect.DeepEqual(config.UI, resolved.UI) || !reflect.DeepEqual(config.Endpoints, resolved.Endpoints) {
				t.Errorf("seed %s has stale projections", word)
			}
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 31 {
		t.Fatalf("seed action count = %d, want 31", count)
	}
	feature := catalog.ResourceCatalog.Buttons["admin:system:developer:publish"]
	found := false
	for _, endpoint := range feature.HTTP {
		found = found || endpoint.Method == "PATCH" && endpoint.Path == "/dictionary/{id}"
	}
	if !found {
		t.Fatal("developer publish does not map to dictionary update")
	}
}

func registerPermissionFixture(t *testing.T, localizer *localization.Module, key, operation, entryPath string, endpoints []localization.HTTPPath) action.Config {
	t.Helper()
	catalog := localizer.Current()
	menu := strings.Replace(key, ":", ":menu:", 1)
	button := strings.Replace(key, ":", ":btn:", 1) + ":query"
	feature := key + ":access"
	catalog.ResourceCatalog.Resources[key] = localization.Resource{Kind: "resource", Entries: map[string]localization.ResourceEntry{menu: {Path: entryPath}}, Operations: map[string]localization.ResourceOperation{operation: {EntryKeys: []string{menu}, ButtonKeys: []string{feature}}}}
	catalog.ResourceCatalog.Buttons[feature] = localization.Feature{Btn: []string{button}, HTTP: endpoints, GRPC: []string{}}
	raw, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	resolved, err := action.ResolveConfig(localizer.Current(), strings.Replace(key, ":", ":"+operation+":", 1), action.Config{Bindings: &[]action.ResourceBinding{ {ResourceKey: key, Operation: operation, FeatureKeys: []string{feature}}}, UI: []action.UIBinding{}, Endpoints: []action.Endpoint{}})
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestDefaultRoleProfilesPostgres(t *testing.T) {
	store := permissionDatabase(t)
	localizer := testLocalization(t, store)
	_ = localizer
	manager, err := authn.New("role-profile-test", strings.Repeat("r", 32), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	policy := auth.New(store, manager, nil, nil, nil, nil, nil, auth.Switches{}, localizer)
	limits, _ := pagination.New(100, 100)
	dictionaries, err := dictionary.New(store, dictionary.NewMemoryValueCache(), dictionary.CacheTTL, limits, false, localizer)
	if err != nil {
		t.Fatal(err)
	}
	dictionaries.SetPermissionAdministration(policy)
	if err := dictionaries.LoadLocalization(t.Context()); err != nil {
		t.Fatal(err)
	}
	actions := action.New(store, limits, false, localizer)
	actions.SetPermissionAdministration(policy)
	actions.SetDefinitionCatalog(dictionaries)
	dictionaries.SetResourcePermissions(actions)
	dictionaries.SetPermissionGroups(actions)
	for index, test := range []struct {
		word                  string
		all, developer, users bool
		codes                 int
	}{
		{"super", true, true, true, 1},
		{"admin", false, false, true, 25},
		{"developer", false, true, true, 30},
		{"operator", false, false, false, 2},
	} {
		t.Run(test.word, func(t *testing.T) {
			var roleID int64
			var assigned string
			if err := store.DB.QueryRowContext(t.Context(), `SELECT id,action FROM t_role WHERE word=$1`, test.word).Scan(&roleID, &assigned); err != nil {
				t.Fatal(err)
			}
			if len(strings.Split(assigned, ",")) != test.codes {
				t.Fatalf("%s codes = %s", test.word, assigned)
			}
			var seededRoleID int64
			if err := store.DB.QueryRowContext(t.Context(), `SELECT role_id FROM t_user WHERE username=$1`, test.word).Scan(&seededRoleID); err != nil {
				t.Fatal(err)
			}
			if seededRoleID != roleID {
				t.Fatalf("%s seed role = %d, want %d", test.word, seededRoleID, roleID)
			}
			userID := int64(101 + index)
			code := fmt.Sprintf("ROLE%04d", index)
			if _, err := store.DB.ExecContext(t.Context(), `INSERT INTO t_user(id,role_id,username,code,password,status,login_count) VALUES($1,$2,$3,$4,'unused',1,1)`, userID, roleID, "role_profile_"+test.word, code); err != nil {
				t.Fatal(err)
			}
			identity := authn.Identity{UserID: userID, Username: "role_profile_" + test.word, Code: code, CredentialVersion: 1}
			token, _, err := manager.Issue(identity)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("GET", "/auth/info?scope=admin", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			request, err = manager.AuthenticateRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			ctx := request.Context()
			info, err := policy.Info(ctx, "admin")
			if err != nil {
				t.Fatal(err)
			}
			paths := map[string]bool{}
			for _, menu := range info.Permission.Menus {
				paths[menu.MenuPath] = true
			}
			if paths["/system/developer"] != test.developer || paths["/system/dictionary"] != test.developer || paths["/system/user"] != test.users || paths["/system/whitelist"] {
				t.Fatalf("%s pages = %v", test.word, paths)
			}
			if !paths["/dashboard/workspace"] {
				t.Fatalf("%s lost workspace", test.word)
			}
			all := false
			for _, word := range info.Permission.Words {
				all = all || word == "admin:all:system:general"
			}
			if all != test.all {
				t.Fatalf("%s all=%v", test.word, all)
			}
			if err := policy.RequireCatalogAdministration(ctx); (err == nil) != test.developer {
				t.Fatalf("%s catalog authority: %v", test.word, err)
			}
			if !test.all && test.users {
				if err := actions.Delete(ctx, action.AllPermissionsActionID); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
					t.Fatalf("%s modified all capability: %v", test.word, err)
				}
			}
			if !test.all && test.users {
				var configRaw []byte
				if err := store.DB.QueryRowContext(t.Context(), `SELECT config FROM t_action WHERE id=2`).Scan(&configRaw); err != nil {
					t.Fatal(err)
				}
				var defaultConfig action.Config
				if err := json.Unmarshal(configRaw, &defaultConfig); err != nil {
					t.Fatal(err)
				}
				_, err := actions.Update(ctx, 2, action.UpdateActionInput{Config: &defaultConfig})
				if (err == nil) != test.developer {
					t.Fatalf("%s default-definition authority: %v", test.word, err)
				}
			}
			if test.word == "admin" || test.word == "developer" {
				var raw json.RawMessage
				if err := store.DB.QueryRowContext(t.Context(), `SELECT value FROM t_dictionary WHERE dictionary_key='I18N_CATALOG'`).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var document map[string]any
				if err := json.Unmarshal(raw, &document); err != nil {
					t.Fatal(err)
				}
				messages := document["messages"].(map[string]any)["en-US"].(map[string]any)["frontend"].(map[string]any)
				if test.word == "admin" {
					messages["groups"].(map[string]any)["admin:system:general"] = "Administrator group edit"
				} else {
					messages["resources"].(map[string]any)["admin:system:developer"] = "Developer catalog edit"
				}
				next, err := json.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}
				value := json.RawMessage(next)
				if _, err := dictionaries.Update(ctx, 3, dictionary.UpdateDictionaryInput{Value: &value, ExpectedValue: &raw}); err != nil {
					t.Fatalf("%s permitted edit: %v", test.word, err)
				}
				if test.word == "admin" {
					if err := store.DB.QueryRowContext(t.Context(), `SELECT value FROM t_dictionary WHERE id=3`).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					if err := json.Unmarshal(raw, &document); err != nil {
						t.Fatal(err)
					}
					document["messages"].(map[string]any)["en-US"].(map[string]any)["frontend"].(map[string]any)["resources"].(map[string]any)["admin:system:developer"] = "Unauthorized graph edit"
					next, err = json.Marshal(document)
					if err != nil {
						t.Fatal(err)
					}
					value = json.RawMessage(next)
					if _, err := dictionaries.Update(ctx, 3, dictionary.UpdateDictionaryInput{Value: &value, ExpectedValue: &raw}); !errors.Is(err, authn.ErrPermissionAdministrationRequired) {
						t.Fatalf("admin bypassed catalog boundary: %v", err)
					}
				}
			}
		})
	}
}

func testLocalization(t *testing.T, store *db.Store) *localization.Module {
	t.Helper()
	localizer := localization.New()
	var raw []byte
	if err := store.SQL(t.Context()).QueryRowContext(t.Context(), "SELECT value FROM t_dictionary WHERE dictionary_key='I18N_CATALOG'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	return localizer
}
