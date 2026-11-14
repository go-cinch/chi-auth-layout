package app

import (
	"net/http/httptest"
 "os"
 "path/filepath"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/idempotency"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules"
	authmodule "{{ .Computed.module_name_final }}/internal/modules/auth"
	dictionarymodule "{{ .Computed.module_name_final }}/internal/modules/dictionary"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

func TestGeneratedModulesReuseBusinessInstances(t *testing.T) {
	database, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	limits, err := pagination.New(10000, 10000)
	if err != nil {
		t.Fatal(err)
	}
	a := &Application{localization: localization.New(), db: &db.Store{DB: database}, pagination: limits, authenticator: testAuthenticator(t), verification: &authmodule.Verification{}}
	a.dictionaryModule, err = dictionarymodule.New(a.db, dictionarymodule.NewMemoryValueCache(), dictionarymodule.CacheTTL, limits, a.localization)
	if err != nil {
		t.Fatal(err)
	}
	a.auditSettings = config.DefaultAudit()
	a.wireBusinessModules()
	want := map[string]modules.HTTPModule{
		"/audit":         a.auditModule,
		"/action":        a.actionModule,
		"/msg":           a.msgModule,
		"/mail-template": a.mailTemplateModule,
		"":               a.authModule,
		"/dictionary":    a.dictionaryModule,
		"/role":          a.roleModule,
		"/user":          a.userModule,
		"/user-group":    a.groupModule,
	}
	// Rebuilding the transport registry must never rebuild a business module.
	for range 2 {
		registered {{ if .Computed.enable_grpc_final }}, _ {{ end }} := a.generatedModules()
		if len(registered) != len(want) {
			t.Fatalf("registered %d modules, want %d", len(registered), len(want))
		}
		seen := make(map[string]bool)
		for _, module := range registered {
			name := module.Name()
			if seen[name] {
				t.Fatalf("duplicate module %s", name)
			}
			seen[name] = true
			if module != want[name] {
				t.Errorf("%s registered a different instance: got %p, want %p", name, module, want[name])
			}
		}
	}
}

func TestBusinessModuleWiring(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	a := &Application{localization: localization.New(), db: &db.Store{DB: database}, authenticator: testAuthenticator(t), idempotencyKeys: idempotency.NewMemoryStore(), verification: &authmodule.Verification{}}
	a.auditSettings = config.DefaultAudit()
	a.wireBusinessModules()
	cfg := &config.Config{}
	cfg.Idempotency.TTL = time.Minute
	router, err := a.NewRouter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT .* FROM t_user WHERE id").WithArgs(int64(7)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "created_at", "updated_at", "role_id", "action", "username", "code", "last_logged_in_at", "status", "metadata", "wrong", "login_count"}).
			AddRow(7, time.Now().UnixMilli(), time.Now().UnixMilli(), nil, "", "operator", "TEST0007", nil, 1, []byte(`{}`), 0, 1))
	token, _, err := a.authenticator.Issue(authn.Identity{Methods: authn.Methods{Primary: authn.Password}, CredentialVersion: 1, UserID: 7, Username: "operator", Code: "TEST0007"})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/user/7", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"actions":[]`) {
		t.Fatalf("injected user route: %d %s", w.Code, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInitializeDictionaryLoadsLocalizationBeforeWiring(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	limits, err := pagination.New(10000, 10000)
	if err != nil {
		t.Fatal(err)
	}
	a := &Application{db: &db.Store{DB: database}, pagination: limits}
	cfg := &config.Config{}
	cache := dictionarymodule.NewMemoryValueCache()
	if err := a.initializeDictionary(t.Context(), cfg, cache); err == nil || !strings.Contains(err.Error(), "initialize dictionary module") {
		t.Fatalf("invalid dictionary configuration = %v", err)
	}
	cfg.Dictionary.CacheTTL = dictionarymodule.CacheTTL
	failure := errors.New("localization query failed")
	mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(dictionarymodule.LocalizationKey).WillReturnError(failure)
	if err := a.initializeDictionary(t.Context(), cfg, cache); !errors.Is(err, failure) || !strings.Contains(err.Error(), "initialize localization") {
		t.Fatalf("localization error = %v", err)
	}
	if a.dictionaryModule != nil || a.localization != nil {
		t.Fatal("failed localization initialization published a dictionary module")
	}
 // Neither required dictionary may be missing when the server starts.
 for _, key := range []string{dictionarymodule.TimezoneDictionaryKey, dictionarymodule.UIPreferenceDictionaryKey} {
  mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(dictionarymodule.LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"value"}))
  if key == dictionarymodule.UIPreferenceDictionaryKey {
   mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WithArgs(dictionarymodule.TimezoneDictionaryKey).WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}).AddRow(`{"items":[]}`, true))
  }
  mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}))
  if err := a.initializeDictionary(t.Context(), cfg, cache); !errors.Is(err, dictionarymodule.ErrRequiredMissing) { t.Fatal(err) }
  if a.dictionaryModule != nil || a.localization != nil { t.Fatal("invalid dictionaries published") }
 }
 files, err := filepath.Glob("../infra/db/migrations/*-04-dictionary-default-data.sql")
 if err != nil || len(files) != 1 { t.Fatal(files, err) }
 seed, err := os.ReadFile(files[0])
 if err != nil { t.Fatal(err) }
 parts := strings.Split(string(seed), "$preferences$")
 if len(parts) != 3 { t.Fatal("missing preference seed") }
 mock.ExpectQuery("SELECT value FROM t_dictionary").WithArgs(dictionarymodule.LocalizationKey).WillReturnRows(sqlmock.NewRows([]string{"value"}))
 mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WithArgs(dictionarymodule.TimezoneDictionaryKey).WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}).AddRow(`{"items":[]}`, true))
 mock.ExpectQuery("SELECT value, enabled FROM t_dictionary").WithArgs(dictionarymodule.UIPreferenceDictionaryKey).WillReturnRows(sqlmock.NewRows([]string{"value", "enabled"}).AddRow(parts[1], true))
 if err := a.initializeDictionary(t.Context(), cfg, cache); err != nil {
		t.Fatal(err)
	}
	if a.dictionaryModule == nil || a.localization == nil {
		t.Fatal("successful initialization did not publish dictionary and localization")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
