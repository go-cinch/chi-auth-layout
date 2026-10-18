package localization

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDynamicLanguageRegistrationAndReplacement(t *testing.T) {
	localizer := New()
	_ = localizer
	t.Cleanup(localizer.Reset)
	raw := []byte(`{"schema_version":2,"default_locale":"fr-FR","scopes":["admin","h5","mini","app"],"locales":[{"tag":"fr-FR","name":"Français","enabled":true},{"tag":"zh-CN","name":"简体中文","enabled":true}],"messages":{"fr-FR":{"backend":{"USER_NOT_FOUND":"Utilisateur introuvable","CUSTOM":"Bonjour"}},"zh-CN":{"backend":{"CUSTOM":"你好"}}}}`)
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	if localizer.Match("fr-CA") != "fr-FR" || localizer.Match("") != "fr-FR" || localizer.Match("zh-CN") != Chinese {
		t.Fatal("dynamic locale negotiation failed")
	}
	if localizer.Translate("fr-FR", "USER_NOT_FOUND", "fallback") != "Utilisateur introuvable" || localizer.Translate(Chinese, "CUSTOM", "") != "你好" {
		t.Fatal("dictionary messages not used")
	}
	if localizer.Translate(Chinese, "USER_NOT_FOUND", "") != "Utilisateur introuvable" {
		t.Fatal("configured default fallback not used")
	}
	detached := localizer.Current()
	if len(detached.Scopes) != 4 || detached.Scopes[1] != "h5" || detached.Scopes[2] != "mini" || detached.Scopes[3] != "app" {
		t.Fatalf("scope list lost: %v", detached.Scopes)
	}
	detached.Scopes[0] = "changed"
	if localizer.Current().Scopes[0] != "admin" {
		t.Fatal("caller mutated runtime scopes")
	}
	detached.Messages["fr-FR"].Backend["CUSTOM"] = "changed"
	if localizer.Translate("fr-FR", "CUSTOM", "") != "Bonjour" {
		t.Fatal("caller mutated runtime catalog")
	}
	if err := localizer.Install([]byte(`{"schema_version":2}`)); err == nil {
		t.Fatal("accepted invalid catalog")
	}
	if localizer.Translate("fr-FR", "CUSTOM", "") != "Bonjour" {
		t.Fatal("invalid update replaced valid catalog")
	}
	localizer.Reset()
	if localizer.Match("fr-FR") != English || localizer.Translate("fr-FR", "CUSTOM", "fallback") != "fallback" {
		t.Fatal("removed language still active")
	}
}

func TestCatalogValidation(t *testing.T) {
	localizer := New()
	_ = localizer
	for _, raw := range []string{
		`null`, `{}`, `{"schema_version":2,"default_locale":"en-US","locales":[{"tag":"en-US","name":"English","enabled":false}]}`,
		`{"schema_version":2,"default_locale":"en-US","locales":[{"tag":"en-US","name":"English","enabled":true}],"messages":{}}`,
		`{"schema_version":2,"default_locale":"bad_!","locales":[{"tag":"bad_!","name":"Invalid","enabled":true}],"messages":{"bad_!":{}}}`,
		`{"schema_version":2,"default_locale":"en-US","locales":[{"tag":"en-US","name":"English","enabled":true},{"tag":"en-US","name":"English","enabled":true}],"messages":{"en-US":{}}}`,
		`{"schema_version":2,"default_locale":"en-US","locales":[{"tag":"en-US","name":"English","enabled":true}],"messages":{"en-US":{},"fr-FR":{}}}`,
	} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("accepted invalid catalog: %s", raw)
		}
	}
}

func TestLabelSnapshotUsesDefaultAndIsDetached(t *testing.T) {
	localizer := New()
	_ = localizer
	t.Cleanup(localizer.Reset)
	raw := []byte(`{"schema_version":2,"default_locale":"en-US","locales":[{"tag":"en-US","name":"English","enabled":true},{"tag":"zh-CN","name":"简体中文","enabled":true}],"messages":{"en-US":{"frontend":{"actions":{"admin:read:system:user":"View users","admin:update:system:user":"Edit users","nested":{}}}},"zh-CN":{"frontend":{"actions":{"admin:read:system:user":"查看用户","admin:update:system:user":""}}}}}`)
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	labels := localizer.Current().Labels(Chinese, "actions")
	if len(labels) != 2 || labels["admin:read:system:user"] != "查看用户" || labels["admin:update:system:user"] != "Edit users" {
		t.Fatalf("labels: %v", labels)
	}
	labels["admin:read:system:user"] = "mutated"
	if localizer.Current().Labels(Chinese, "actions")["admin:read:system:user"] != "查看用户" {
		t.Fatal("caller mutated runtime catalog")
	}
	if localizer.Current().Labels("unsupported", "actions")["admin:read:system:user"] != "View users" || len(localizer.Current().Labels(English, "unknown")) != 0 {
		t.Fatal("label fallback failed")
	}
	localizer.Reset()
	if len(labels) != 2 || len(localizer.Current().Labels(English, "actions")) != 0 {
		t.Fatal("snapshot changed after replacement")
	}
}

func TestCatalogInstancesAndDetachedSnapshots(t *testing.T) {
	first, second := New(), New()
	catalog := first.Current()
	catalog.DefaultLocale = "fr-FR"
	catalog.Locales = []Language{ {Tag: "fr-FR", Name: "French", Enabled: true}}
	catalog.Messages = map[string]Messages{"fr-FR": {Backend: map[string]string{"CUSTOM": "Bonjour"}}}
	raw, _ := json.Marshal(catalog)
	if err := first.Install(raw); err != nil {
		t.Fatal(err)
	}
	if first.Match("fr-CA") != "fr-FR" || second.Match("fr-CA") != English {
		t.Fatal("independent applications share language state")
	}
	if first.Translate("fr-FR", "CUSTOM", "") != "Bonjour" || second.Translate("fr-FR", "CUSTOM", "fallback") != "fallback" {
		t.Fatal("independent applications share messages")
	}
	detached := first.Current()
	detached.Messages["fr-FR"].Backend["CUSTOM"] = "mutated"
	if first.Translate("fr-FR", "CUSTOM", "") != "Bonjour" {
		t.Fatal("snapshot mutation escaped")
	}
	scopes, operations := first.ScopeMetadata()
	scopes[0] = "changed"
	operations[0] = "changed"
	if reflect.DeepEqual(scopes, first.Current().Scopes) || reflect.DeepEqual(operations, first.Current().Operations) {
		t.Fatal("metadata mutation escaped")
	}
	if first.Match("invalid_!") != "fr-FR" {
		t.Fatal("invalid header lost configured default")
	}
}

// Count actual serialization, not merely the final response's keys.
type countedMessage struct{ calls *int }

func (m countedMessage) MarshalJSON() ([]byte, error) { *m.calls++; return []byte(`"label"`), nil }

func TestSelectedCatalogSkipsUnrequestedSerialization(t *testing.T) {
	m := New()
	calls := 0
	c := m.active.Load()
	c.catalog.Messages[English] = Messages{Frontend: map[string]any{"label": countedMessage{&calls}}}
	selected := m.Select([]string{"scopes", "operations"})
	if calls != 0 || len(selected.Scopes) == 0 || selected.Messages != nil {
		t.Fatalf("unselected messages serialized: %d %+v", calls, selected)
	}
	selected.Scopes[0] = "changed"
	if m.Select([]string{"scopes"}).Scopes[0] != "admin" {
		t.Fatal("mutable snapshot escaped")
	}
	public := m.PublicCatalog("default_locale", "locales")
	raw, err := json.Marshal(public)
	if err != nil || calls != 0 {
		t.Fatalf("metadata serialized messages: %s %v calls=%d", raw, err, calls)
	}
	var keys map[string]any
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 || keys["schema_version"] != float64(2) {
		t.Fatal(string(raw))
	}
	frontend := m.PublicCatalog("frontend_messages")
	if calls != 1 || frontend.Messages[English].Backend != nil {
		t.Fatalf("frontend selection: calls=%d %+v", calls, frontend)
	}
	frontend.Messages[English].Frontend["label"] = "mutated"
	if m.Translate(English, "label", "fallback") != "fallback" {
		t.Fatal("public mutation escaped")
	}
}
