package action

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
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
