package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http/httptest"
	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/modules"
	dictionarymodule "{{ .Computed.module_name_final }}/internal/modules/dictionary"

)

func TestGeneratedModulesReuseBusinessInstances(t *testing.T) {
	database, _, err := sqlmock.New()
	if err != nil { t.Fatal(err) }
	defer database.Close()
	limits, err := pagination.New(10000, 10000)
	if err != nil { t.Fatal(err) }
	a := &Application{db: &db.Store{DB: database}, pagination: limits, authenticator: testAuthenticator(t)}
	a.dictionaryModule, err = dictionarymodule.New(a.db, dictionarymodule.NewMemoryValueCache(), dictionarymodule.CacheTTL, limits, false)
	if err != nil { t.Fatal(err) }
	a.wireBusinessModules()
	want := map[string]modules.HTTPModule{
		"/action": a.actionModule,
		"/auth": a.authModule,
		"/dictionary": a.dictionaryModule,
		"/role": a.roleModule,
		"/user": a.userModule,
		"/user-group": a.groupModule,
		"/whitelist": a.whitelistModule,
	}
	// Rebuilding the transport registry must never rebuild a business module.
	for range 2 {
		registered{{ if .Computed.enable_grpc_final }}, _{{ end }} := a.generatedModules()
		if len(registered) != len(want) { t.Fatalf("registered %d modules, want %d", len(registered), len(want)) }
		seen := make(map[string]bool)
		for _, module := range registered {
			name := module.Name()
			if seen[name] { t.Fatalf("duplicate module %s", name) }
			seen[name] = true
			if module != want[name] { t.Errorf("%s registered a different instance: got %p, want %p", name, module, want[name]) }
		}
	}
}

func TestAuthSwitchesFromConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.Switches.PasswordResetRequired = true
	cfg.Auth.Switches.ProtectSuper = true
	cfg.Auth.Switches.ProtectCaptchaDictionaries = true
	switches := authSwitchesFromConfig(cfg)
	if !switches.PasswordResetRequired || !switches.ProtectSuper || !switches.ProtectCaptchaDictionaries {
		t.Fatalf("auth switches = %#v", switches)
	}
}

func TestBusinessModuleWiring(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil { t.Fatal(err) }
	defer database.Close()
	a := &Application{db: &db.Store{DB:database}, authenticator:testAuthenticator(t), idempotencyKeys:idempotency.NewMemoryStore()}
	a.wireBusinessModules()
	cfg := &config.Config{}
	cfg.Idempotency.TTL = time.Minute
	router, err := a.NewRouter(cfg)
	if err != nil { t.Fatal(err) }
	mock.ExpectQuery("SELECT .* FROM t_user WHERE id").WithArgs(int64(7)).WillReturnRows(
		sqlmock.NewRows([]string{"id","created_at","updated_at","role_id","action","username","code","last_logged_in_at","status","metadata","wrong","login_count"}).
		AddRow(7,time.Now(),time.Now(),nil,"","operator","TEST0007",nil,1,[]byte(`{}`),0,1))
	token, _, err := a.authenticator.Issue(authn.Identity{CredentialVersion: 1, UserID:7,Username:"operator",Code:"TEST0007"})
	if err != nil { t.Fatal(err) }
	r := httptest.NewRequest("GET","/user/7",nil)
	r.Header.Set("Authorization","Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w,r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"actions":[]`) { t.Fatalf("injected user route: %d %s",w.Code,w.Body.String()) }
	if err := mock.ExpectationsWereMet(); err != nil { t.Fatal(err) }
}

func TestSignalContextCanBeCanceled(t *testing.T) {
	ctx, cancel := SignalContext()
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("signal context was not canceled")
	}
}

func TestBusinessHTTPAlwaysRequiresAuthentication(t *testing.T) {
	a := &Application{
		db: &db.Store{},
		authenticator: testAuthenticator(t),
		idempotencyKeys: idempotency.NewMemoryStore(),
	}
	a.wireBusinessModules()
	httpModules{{ if .Computed.enable_grpc_final }}, _{{ end }} := a.generatedModules()
	cfg := &config.Config{}
	cfg.HTTP.Addr = "127.0.0.1:0"
	cfg.HTTP.Profiler.Enabled = true
	cfg.HTTP.Profiler.Addr = "127.0.0.1:0"
	if err := a.configureTransports(cfg, httpModules{{ if .Computed.enable_grpc_final }}, nil{{ end }}); err == nil || !strings.Contains(err.Error(), "initialize http router: idempotency store and positive ttl are required") {
		t.Fatalf("invalid idempotency configuration error = %v", err)
	}
	if a.server != nil || a.profilerServer != nil {
		t.Fatal("servers configured after failed module registration")
	}
	cfg.Idempotency.TTL = time.Minute
	if err := a.configureTransports(cfg, httpModules{{ if .Computed.enable_grpc_final }}, nil{{ end }}); err != nil {
		t.Fatal(err)
	}
	if a.server == nil {
		t.Fatal("required business HTTP server was not configured")
	}
	if a.profilerServer == nil || a.profilerServer.Addr != cfg.HTTP.Profiler.Addr {
		t.Fatal("enabled profiler server was not configured")
	}
	w := httptest.NewRecorder()
	a.server.Handler.ServeHTTP(w, httptest.NewRequest("GET", "/user/7", nil))
	if w.Code != 401 {
		t.Fatalf("unauthenticated user route = %d, want 401", w.Code)
	}
}

func TestNewErrors(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	if _, err := New(t.Context(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing config directory was accepted")
	}
	if _, err := New(t.Context(), writeConfig(t, "log:\n  level: invalid\n")); err == nil || !strings.Contains(err.Error(), "parse log level") {
		t.Fatalf("invalid log error = %v", err)
	}

	if _, err := New(t.Context(), writeConfig(t, "log:\n  level: info\ndatabase:\n  driver: sqlite\n")); err == nil || !strings.Contains(err.Error(), "unsupported database driver") {
		t.Fatalf("invalid database error = %v", err)
	}

{{- if .Computed.enable_trace_final }}
	traced := "server:\n  name: test\nlog:\n  level: info\ntracer:\n  enabled: true\n  ratio: 0\n  stdout: true\ndatabase:\n  driver: sqlite\n"
	if _, err := New(t.Context(), writeConfig(t, traced)); err == nil || !strings.Contains(err.Error(), "unsupported database driver") {
		t.Fatalf("traced invalid database error = %v", err)
	}
{{- end }}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInvalidPaginationConfig(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	if _, err := New(t.Context(), writeConfig(t, "log:\n  level: info\npagination:\n  maxP: -1\n  maxS: 10000\n")); err == nil || !strings.Contains(err.Error(), "pagination.maxP") {
		t.Fatalf("pagination: %v", err)
	}
}
