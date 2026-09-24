package tests

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/auth"
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
	if _, err := database.ExecContext(t.Context(), `UPDATE t_user SET status=2, metadata='{"lock_expired_at":1790000000000}', last_logged_in_at=TIMESTAMP '2026-09-01 00:00:00' WHERE id=3`); err != nil {
		t.Fatal(err)
	}
	return &db.Store{DB: database}
}

func TestDefaultPermissionHTTP(t *testing.T) {
	store := permissionDatabase(t)
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
	m := auth.New(store, manager, nil, sessions, nil, nil, auth.Switches{PasswordResetRequired: true})
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
	if err := store.DB.QueryRowContext(t.Context(), `SELECT resource FROM t_action WHERE word='default'`).Scan(&defaultResources); err != nil {
		t.Fatal(err)
	}
	for _, restricted := range []bool{false, true} {
		count := 1
		if restricted {
			count = 0
		}
		exec(`UPDATE t_user SET login_count=$1 WHERE id=2`, count)
		for _, granted := range []bool{true, false} {
			resources := ""
			if granted {
				resources = defaultResources
			}
			exec(`UPDATE t_action SET resource=$1 WHERE word='default'`, resources)
			want := 403
			if granted {
				want = 400
			}
			request("POST", "/auth/challenge", want)
			request("POST", "/auth/logout", want)
			request("PATCH", "/auth/reset/pwd", want)
			captchaWant := want
			if restricted {
				captchaWant = 403
			}
			request("POST", "/auth/captcha/verify", captchaWant)
		}
	}
	// Even all permissions cannot lift the first-login restriction, including via the gateway.
	exec(`UPDATE t_action SET resource='*' WHERE word='default'`)
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
	ctx := t.Context()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := store.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	var lockedAt int64
	var loggedAt time.Time
	if err := store.DB.QueryRowContext(ctx, `SELECT (metadata->>'lock_expired_at')::bigint, last_logged_in_at FROM t_user WHERE id=3`).Scan(&lockedAt, &loggedAt); err != nil {
		t.Fatal(err)
	}
	if lockedAt != 1790000000000 || loggedAt.UnixMilli() != time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("timestamp migration: %d %v", lockedAt, loggedAt)
	}
	exec(`UPDATE t_action SET resource='GET|/default', menu='/common', btn='common' WHERE word='default'`)
	exec(`INSERT INTO t_action (name,action_group,code,word,resource,menu,btn) VALUES
		('Direct','Test','DIRECT01','test.direct','GET|/direct','/direct','read'),
		('Role','Test','ROLE0001','test.role','PATCH|/role-target|/test.Role/Update','/common','write'),
		('Group','Test','GROUP001','test.group','DELETE|/group-target','/group','read')`)
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
	m := auth.New(store, manager, nil, nil, nil, nil, auth.Switches{PasswordResetRequired: true})
	info, err := m.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := auth.Permission{Resources: []string{"GET|/default", "GET|/direct", "PATCH|/role-target|/test.Role/Update", "DELETE|/group-target"}, Menus: []string{"/common", "/direct", "/group"}, Buttons: []string{"common", "read", "write"}}
	if !reflect.DeepEqual(info.Permission, want) {
		t.Fatalf("aggregated permission: %#v", info.Permission)
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
	exec(`DELETE FROM t_action WHERE word='default'`)
	check("GET", "/default", false)
	info, err = m.Info(ctx)
	if err != nil || len(info.Permission.Resources) != 0 || info.Permission.Resources == nil {
		t.Fatalf("empty permissions: %#v, %v", info, err)
	}
	limits, _ := pagination.New(100, 100)
	actions := action.New(store, limits, false)
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
