package action

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

type jsonDocumentArg []byte

func (want jsonDocumentArg) Match(value driver.Value) bool {
	var raw []byte
	switch v := value.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return false
	}
	decode := func(raw []byte) (any, error) {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		err := decoder.Decode(&value)
		return value, err
	}
	a, err := decode(want)
	if err != nil {
		return false
	}
	b, err := decode(raw)
	return err == nil && reflect.DeepEqual(a, b)
}

func resourceMoveCatalog(key string) localization.Catalog {
	feature := key + ":query"
	return localization.Catalog{SchemaVersion: 2, DefaultLocale: "en-US", Scopes: []string{"admin"}, Operations: localization.FixedOperations, Locales: []localization.Language{ {Tag: "en-US", Name: "English", Enabled: true}}, Messages: map[string]localization.Messages{"en-US": {}}, ResourceCatalog: localization.ResourceCatalog{
		Resources: map[string]localization.Resource{key: {Kind: "resource", Entries: map[string]localization.ResourceEntry{"admin:menu:system:user": {Path: "/system/user"}}, Operations: map[string]localization.ResourceOperation{"read": {EntryKeys: []string{"admin:menu:system:user"}, ButtonKeys: []string{feature}}}}},
		Buttons:   map[string]localization.Feature{feature: {Btn: []string{"admin:btn:system:user:query"}, HTTP: []localization.HTTPPath{ {Method: "GET", Path: "/user"}}, GRPC: []string{}}},
	}}
}

func TestResourceMovePreservesActionIdentityAndMigratesExplicitSelections(t *testing.T) {
	m, mock := newTestModule(t)
	const oldKey = "admin:system:user"
	const newKey = "admin:people:person"
	before, _ := json.Marshal(resourceMoveCatalog(oldKey))
	after, _ := json.Marshal(resourceMoveCatalog(newKey))
	ordinary := `{"bindings":[{"resource_key":"admin:system:user","operation":"read","feature_keys":["admin:system:user:query"]}],"ui":[],"endpoints":[]}`
	legacy := `{"resource_key":"admin:system:user","operation":"read","feature_keys":["admin:system:user:query"],"ui":[],"endpoints":[]}`
	defaults := `{"builtin":"default","resource_keys":["admin:system:user"],"feature_keys":["admin:system:user:query"],"ui":[],"endpoints":[]}`
	mock.ExpectQuery("SELECT id, word, action_group, config FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"id", "word", "action_group", "config"}).
		AddRow(9, "admin:read:business:people", "admin:business:people", []byte(ordinary)).
		AddRow(10, "admin:read:business:legacy", "admin:business:legacy", []byte(legacy)).
		AddRow(2, "admin:default:system:default", "admin:system:general", []byte(defaults)))
	for _, row := range []struct {
		id                  int64
		word, group, config string
	}{
		{9, "admin:read:business:people", "admin:business:people", ordinary},
		{10, "admin:read:business:legacy", "admin:business:legacy", legacy},
		{2, "admin:default:system:default", "admin:system:general", defaults},
	} {
		mock.ExpectExec("UPDATE t_action SET word").WithArgs(row.word, row.group, jsonDocumentArg(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(row.config, oldKey, newKey), `"ui":[]`, `"ui":[{"menu_key":"admin:menu:system:user","menu_path":"/system/user","page_entry":true,"button_keys":["admin:btn:system:user:query"]}]`), `"endpoints":[]`, `"endpoints":[{"protocol":"http","method":"GET","path":"/user"}]`)), sqlmock.AnyArg(), row.id).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	value := after
	if valid, err := m.ReconcileResourcePermissions(t.Context(), before, value, map[string]string{oldKey: newKey}, map[string]string{oldKey + ":query": newKey + ":query"}); err != nil || !valid {
		t.Fatal(valid, err)
	}
}

func TestResourceMigrationRejectsDanglingAndInvalidUpdates(t *testing.T) {
	m, mock := newTestModule(t)
	before, _ := json.Marshal(resourceMoveCatalog("admin:system:user"))
	after, _ := json.Marshal(resourceMoveCatalog("admin:people:person"))
	value := after
	if valid, err := m.ReconcileResourcePermissions(t.Context(), before, value, map[string]string{"admin:missing:user": "admin:people:person"}, nil); err != nil || valid {
		t.Fatal(err)
	}
	if valid, err := m.ReconcileResourcePermissions(t.Context(), before, before, nil, nil); err != nil || !valid {
		t.Fatal(valid, err)
	}
	if valid, err := m.ReconcileResourcePermissions(t.Context(), before, json.RawMessage(`{"schema_version":9}`), nil, nil); err != nil || valid {
		t.Fatal(valid, err)
	}

	mock.ExpectQuery("SELECT id, word, action_group, config FROM t_action").WillReturnRows(sqlmock.NewRows([]string{"id", "word", "action_group", "config"}).AddRow(9, "admin:read:business:people", "admin:business:people", []byte(`{"bindings":[{"resource_key":"admin:system:user","operation":"read","feature_keys":[]}],"ui":[],"endpoints":[]}`)))
	if valid, err := m.ReconcileResourcePermissions(t.Context(), before, value, nil, nil); err != nil || valid {
		t.Fatalf("dangling binding accepted: %v", err)
	}
}

func TestResourceMigrationPreservesExtensionsAndFeatureSelections(t *testing.T) {
	const oldKey = "admin:system:user"
	const newKey = "admin:people:person"
	const extension = `{"count":9007199254740993}`
	before, after := resourceMoveCatalog(oldKey), resourceMoveCatalog(newKey)
	for _, test := range []struct {
		name, selection, wantSelection string
		endpoints                      int
	}{
		{"omitted", "", "", 1},
		{"null", `,"feature_keys":null`, "null", 1},
		{"empty", `,"feature_keys":[]`, "[]", 0},
		{"explicit", `,"feature_keys":["admin:system:user:query"]`, `["admin:people:person:query"]`, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := json.RawMessage(`{"extension":` + extension + `,"bindings":[{"resource_key":"` + oldKey + `","operation":"read","extension":` + extension + test.selection + `}],"ui":[],"endpoints":[]}`)
			result, changed, err := migrateResourceConfig(before.ResourceCatalog, after, "admin:read:business:people", raw, map[string]string{oldKey: newKey}, map[string]string{oldKey + ":query": newKey + ":query"})
			if err != nil || !changed {
				t.Fatalf("migration: changed=%v err=%v", changed, err)
			}
			var document map[string]json.RawMessage
			if err := json.Unmarshal(result, &document); err != nil {
				t.Fatal(err)
			}
			var bindings []map[string]json.RawMessage
			if err := json.Unmarshal(document["bindings"], &bindings); err != nil || len(bindings) != 1 {
				t.Fatalf("bindings=%s err=%v", document["bindings"], err)
			}
			if string(document["extension"]) != extension || string(bindings[0]["extension"]) != extension {
				t.Fatalf("unknown fields changed: %s", result)
			}
			if string(bindings[0]["resource_key"]) != `"`+newKey+`"` || string(bindings[0]["feature_keys"]) != test.wantSelection {
				t.Fatalf("selection changed: %s", document["bindings"])
			}
			var config Config
			if err := json.Unmarshal(result, &config); err != nil || len(config.Endpoints) != test.endpoints {
				t.Fatalf("endpoints=%+v err=%v", config.Endpoints, err)
			}
			if _, changed, err := migrateResourceConfig(after.ResourceCatalog, after, "admin:read:business:people", result, nil, nil); err != nil || changed {
				t.Fatalf("repeat migration: changed=%v err=%v", changed, err)
			}
		})
	}
}

func TestResourceMigrationRefreshesMappingsWithoutRenames(t *testing.T) {
	const key = "admin:system:user"
	before, after := resourceMoveCatalog(key), resourceMoveCatalog(key)
	feature := after.ResourceCatalog.Buttons[key+":query"]
	feature.HTTP = []localization.HTTPPath{ {Method: "GET", Path: "/people"}}
	after.ResourceCatalog.Buttons[key+":query"] = feature
	raw := json.RawMessage(`{"resource_key":"admin:system:user","operation":"read","feature_keys":null,"permission_scope":"*","permission_scopes":["admin"],"extension":{"keep":true},"ui":[],"endpoints":[{"protocol":"http","method":"GET","path":"/user"}]}`)
	result, changed, err := migrateResourceConfig(before.ResourceCatalog, after, "admin:read:business:people", raw, nil, nil)
	if err != nil || !changed {
		t.Fatalf("migration: changed=%v err=%v", changed, err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(result, &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document["permission_scope"]; exists {
		t.Fatalf("stale wildcard projection retained: %s", result)
	}
	if _, exists := document["permission_scopes"]; exists {
		t.Fatalf("stale scope projection retained: %s", result)
	}
	if string(document["feature_keys"]) != "null" || string(document["extension"]) != `{"keep":true}` {
		t.Fatalf("stored selection or extension changed: %s", result)
	}
	want := jsonDocumentArg(`[{"protocol":"http","method":"GET","path":"/people"}]`)
	if !want.Match([]byte(document["endpoints"])) {
		t.Fatalf("outdated endpoints: %s", document["endpoints"])
	}
}

func TestResourceMigrationValidatesAllRowsBeforeWriting(t *testing.T) {
	before, _ := json.Marshal(resourceMoveCatalog("admin:system:user"))
	after, _ := json.Marshal(resourceMoveCatalog("admin:people:person"))
	for _, test := range []struct {
		name, config string
		wantError    bool
	}{
		{"dangling", `{"resource_key":"admin:missing:user","operation":"read","feature_keys":[],"ui":[],"endpoints":[]}`, false},
		{"invalid definition", `{"bindings":[],"builtin":"unknown","ui":[],"endpoints":[]}`, false},
		{"malformed", `{"bindings":`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, mock := newTestModule(t)
			mock.ExpectQuery("SELECT id, word, action_group, config FROM t_action.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"id", "word", "action_group", "config"}).
				AddRow(9, "admin:read:business:people", "admin:business:people", []byte(`{"resource_key":"admin:system:user","operation":"read","feature_keys":[],"ui":[],"endpoints":[]}`)).
				AddRow(10, "admin:read:business:invalid", "admin:business:people", []byte(test.config)))
			valid, err := m.ReconcileResourcePermissions(t.Context(), before, after, map[string]string{"admin:system:user": "admin:people:person"}, map[string]string{"admin:system:user:query": "admin:people:person:query"})
			if valid || (err != nil) != test.wantError {
				t.Fatalf("valid=%v err=%v", valid, err)
			}
			if test.wantError {
				var syntax *json.SyntaxError
				if !errors.As(err, &syntax) {
					t.Fatalf("malformed JSON error lost: %v", err)
				}
			}
		})
	}
}
