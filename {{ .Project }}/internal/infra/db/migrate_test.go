package db

import (
	"bytes"
	"encoding/json"
	"errors"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
	"io/fs"
	"log/slog"
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

func TestEmbeddedMigrations(t *testing.T) {
	files, err := fs.Glob(SQLFiles, SQLRoot+"/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("required auth migrations missing: %v", files)
	}
	for _, name := range files {
		contents, err := SQLFiles.ReadFile(name)
		if err != nil {
			t.Fatalf("read embedded migration %q: %v", name, err)
		}
		if len(contents) == 0 {
			t.Fatalf("embedded migration %q is empty", name)
		}
	}
}

func TestMigrateUpPreservesDatabaseError(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	failure := errors.New("migration table unavailable")
	mock.ExpectExec(`(?i)create table if not exists "schema_migrations"`).WillReturnError(failure)
	if err := MigrateUp(t.Context(), database, "postgres"); !errors.Is(err, failure) {
		t.Fatalf("migration database failure = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateUpDoesNotReplayAppliedMigrations(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	files, err := fs.Glob(SQLFiles, SQLRoot+"/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	rows := sqlmock.NewRows([]string{"id", "applied_at"})
	for _, name := range files {
		rows.AddRow(path.Base(name), time.Unix(1, 0))
	}
	mock.ExpectExec(`(?i)create table if not exists "schema_migrations"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "schema_migrations"`).WillReturnRows(rows)
	if err := MigrateUp(t.Context(), database, "postgres"); err != nil {
		t.Fatalf("already migrated database: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["msg"] != "database migrations applied: 0" || record["operation"] != "database_migrate" || record["outcome"] != "success" || record["driver"] != "postgres" || record["applied"] != float64(0) {
		t.Fatalf("migration log = %#v", record)
	}
}

func TestDefaultUserPasswordsMatchUsernames(t *testing.T) {
	names, err := fs.Glob(SQLFiles, SQLRoot+"/*-02-auth-default-data.sql")
	if err != nil || len(names) != 1 {
		t.Fatalf("default data migration = %#v, %v", names, err)
	}
	contents, err := SQLFiles.ReadFile(names[0])
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?m)^\s*\(\d+,\s*(\d+|NULL),\s*'([^']+)',\s*'[^']+',\s*'([^']+)',\s*1\)[,;]$`)
	matches := pattern.FindAllSubmatch(contents, -1)
	expectedRoles := map[string]string{
		"super": "1", "guest": "2", "readonly": "NULL", "write": "NULL",
		"nodelete": "NULL", "admin": "3", "developer": "4", "operator": "5",
	}
	if len(matches) != len(expectedRoles) {
		t.Fatalf("seed users = %d, want %d", len(matches), len(expectedRoles))
	}
	for _, match := range matches {
		roleID, username, passwordHash := string(match[1]), string(match[2]), match[3]
		if expected, exists := expectedRoles[username]; !exists || roleID != expected {
			t.Fatalf("seed role for %q = %q, want %q", username, roleID, expected)
		}
		delete(expectedRoles, username)
		if err := bcrypt.CompareHashAndPassword(passwordHash, []byte(username)); err != nil {
			t.Fatalf("seed password for %q does not match its username: %v", username, err)
		}
	}
}

func TestDefaultAssignmentsReferenceSplitActionCatalog(t *testing.T) {
	actions := readSeedMigration(t, "03-action-default-data")
	accounts := readSeedMigration(t, "02-auth-default-data")
	dictionaries := readSeedMigration(t, "04-dictionary-default-data")
	if strings.Contains(accounts, "INSERT INTO t_action ") || strings.Contains(accounts, "INSERT INTO t_dictionary ") {
		t.Fatal("account seed still owns permission or dictionary data")
	}
	if !strings.Contains(dictionaries, "'I18N_CATALOG'") {
		t.Fatal("dictionary seed is missing the localization catalog")
	}
	codes := make(map[string]bool)
	pattern := regexp.MustCompile(`(?m)^\s*\(\d+,\s*'[^']+',\s*'([^']+)',\s*'([^']+)',\s*'`)
	operations := map[string]bool{
		"all": true, "default": true, "read": true, "create": true,
		"update": true, "delete": true, "grant": true,
	}
	deleteActions := 0
	for _, match := range pattern.FindAllStringSubmatch(actions, -1) {
		codes[match[1]] = true
		parts := strings.Split(match[2], ":")
		if len(parts) != 4 || !operations[parts[1]] {
			t.Fatalf("seed permission must use scope:operation:module:resource: %q", match[2])
		}
		if parts[1] == "delete" {
			deleteActions++
		}
	}
	if deleteActions != 6 {
		t.Fatalf("seed delete permissions = %d, want 6", deleteActions)
	}
	if len(codes) != 31 {
		t.Fatalf("seed action codes = %d, want 31", len(codes))
	}
	assignments := regexp.MustCompile(`(?m)^\s*\(\d+,\s*'[^']+',\s*'[^']+',\s*'([^']+)'\)[,;]$`).FindAllStringSubmatch(accounts, -1)
	if len(assignments) != 8 {
		t.Fatalf("seed role/group assignments = %d, want 8", len(assignments))
	}
	for _, assignment := range assignments {
		for _, code := range strings.Split(assignment[1], ",") {
			if !codes[code] {
				t.Errorf("assignment references missing action code %q", code)
			}
		}
	}
}

func TestDefaultActionButtonCatalog(t *testing.T) {
	type binding struct {
		MenuKey       string          `json:"menu_key"`
		ParentMenuKey string          `json:"parent_menu_key"`
		ButtonKeys    []string        `json:"button_keys"`
		LegacyButton  json.RawMessage `json:"button_key"`
	}
	type definition struct {
		UI []binding `json:"ui"`
	}
	actions := readSeedMigration(t, "03-action-default-data")
	pattern := regexp.MustCompile(`(?m)^\s*\(\d+,\s*'[^']+',\s*'[^']+',\s*'([^']+)',\s*'((?:[^']|'')*)'::jsonb\)[,;]`)
	configs := make(map[string]definition)
	owners := make(map[string]string)
	buttons := make(map[string]bool)
	menus := make(map[string]bool)
	for _, match := range pattern.FindAllStringSubmatch(actions, -1) {
		word := match[1]
		var config definition
		if err := json.Unmarshal([]byte(strings.ReplaceAll(match[2], "''", "'")), &config); err != nil {
			t.Fatalf("decode %s: %v", word, err)
		}
		configs[word] = config
		for _, ui := range config.UI {
			if ui.LegacyButton != nil || ui.ButtonKeys == nil {
				t.Fatalf("%s must use an explicit button_keys array", word)
			}
			parts := strings.SplitN(ui.MenuKey, ":", 3)
			if len(parts) != 3 || parts[1] != "menu" || parts[2] == "" {
				t.Fatalf("invalid menu key: %q", ui.MenuKey)
			}
			menus[ui.MenuKey] = true
			if ui.ParentMenuKey != "" {
				if !strings.HasPrefix(ui.ParentMenuKey, parts[0]+":menu:") {
					t.Fatalf("parent menu %q must use the same scope:menu namespace", ui.ParentMenuKey)
				}
				menus[ui.ParentMenuKey] = true
			}
			prefix := parts[0] + ":btn:" + parts[2] + ":"
			for _, button := range ui.ButtonKeys {
				if button == word || !strings.HasPrefix(button, prefix) || button == prefix {
					t.Fatalf("button %q must identify an independent control on %s", button, ui.MenuKey)
				}
				key := ui.MenuKey + "\x00" + button
				if owner, exists := owners[key]; exists {
					t.Fatalf("button %q is repeated across %s and %s", button, owner, word)
				}
				owners[key] = word
				buttons[button] = true
			}
		}
	}
	if len(configs) != 31 {
		t.Fatalf("decoded action definitions = %d, want 31", len(configs))
	}
	hasButton := func(word, button string) bool {
		for _, ui := range configs[word].UI {
			for _, key := range ui.ButtonKeys {
				if key == button {
					return true
				}
			}
		}
		return false
	}
	for _, control := range []string{"query", "refresh", "view-detail"} {
		if !hasButton("admin:read:system:user", "admin:btn:system:user:"+control) {
			t.Errorf("user read permission is missing control %s", control)
		}
	}
	for _, control := range []string{"delete", "delete-selected"} {
		if !hasButton("admin:delete:system:user", "admin:btn:system:user:"+control) {
			t.Errorf("user delete permission is missing control %s", control)
		}
	}
	for _, kind := range []string{"login", "operation"} {
		button := "admin:btn:audit:" + kind + ":export-report"
		if !hasButton("admin:read:audit:"+kind, button) {
			t.Errorf("audit %s export must be a selectable feature within read", kind)
		}
	}
	dictionaries := readSeedMigration(t, "04-dictionary-default-data")
	catalogPattern := regexp.MustCompile(`'I18N_CATALOG',\s*'[^']+',\s*'((?:[^']|'')*)'::jsonb`)
	match := catalogPattern.FindStringSubmatch(dictionaries)
	if len(match) != 2 {
		t.Fatal("localization catalog missing")
	}
	var catalog localization.Catalog
	if err := json.Unmarshal([]byte(strings.ReplaceAll(match[1], "''", "'")), &catalog); err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"zh-CN", "en-US"} {
		frontend := catalog.Messages[locale].Frontend
		resourceLabels, _ := frontend["resources"].(map[string]any)
		menuOwners := catalog.ResourceCatalog.MenuResourceKeys()
		for menu := range menus {
			label, _ := resourceLabels[menuOwners[menu]].(string)
			if strings.TrimSpace(label) == "" {
				t.Errorf("%s is missing the label for %s", locale, menu)
			}
		}
		labels, _ := frontend["buttons"].(map[string]any)
		if len(labels) != len(buttons) {
			t.Errorf("%s has %d button labels for %d controls", locale, len(labels), len(buttons))
		}
		for button := range buttons {
			label, _ := labels[button].(string)
			if strings.TrimSpace(label) == "" {
				t.Errorf("%s is missing the label for %s", locale, button)
			}
		}
	}
}

func readSeedMigration(t *testing.T, suffix string) string {
	t.Helper()
	files, err := fs.Glob(SQLFiles, SQLRoot+"/*-"+suffix+".sql")
	if err != nil || len(files) != 1 {
		t.Fatalf("seed migration %s = %v, %v", suffix, files, err)
	}
	content, err := SQLFiles.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
