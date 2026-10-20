package localization

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogHTTPPathsFollowConfiguredMount(t *testing.T) {
	const raw = `{"schema_version":2,"default_locale":"en-US","scopes":["admin"],"locales":[{"tag":"en-US","name":"English","enabled":true}],"messages":{"en-US":{}},"resource_catalog":{"resources":{"admin:session:access":{"kind":"resource","operations":{"read":{"entry_keys":[],"button_keys":["admin:session:access:info"]}}}},"buttons":{"admin:session:access:info":{"btn":[],"http":[{"method":"GET","path":"/auth/info"},{"method":"GET","path":"/user"}],"grpc":["/auth.v1.Auth/Info"]}}}}`
	for _, prefix := range []string{"", "/auth", "/identity/v2"} {
		t.Run(prefix, func(t *testing.T) {
			m := New(map[string]string{"/auth/info": prefix + "/info"})
			if err := m.Install([]byte(raw)); err != nil {
				t.Fatal(err)
			}
			catalog := m.Select([]string{"resource_catalog"})
			feature := catalog.ResourceCatalog.Buttons["admin:session:access:info"]
			if feature.HTTP[0].Path != prefix+"/info" || feature.HTTP[1].Path != "/user" || feature.GRPC[0] != "/auth.v1.Auth/Info" {
				t.Fatal(feature)
			}
			encoded, _ := json.Marshal(m.Current())
			canonical, err := m.CanonicalCatalog(encoded)
			if err != nil || !strings.Contains(string(canonical), `"path":"/auth/info"`) {
				t.Fatal(string(canonical), err)
			}
			feature.HTTP[0].Path = "/mutated"
			if m.Current().ResourceCatalog.Buttons["admin:session:access:info"].HTTP[0].Path != prefix+"/info" {
				t.Fatal("snapshot mutated")
			}
		})
	}
}

func TestCanonicalHTTPPathsPreserveCatalogExtensions(t *testing.T) {
	m := New(map[string]string{"/auth/info": "/info"})
	input := []byte(`{"extension":123,"resource_catalog":{"extension":{"keep":true},"buttons":{"feature":{"extension":"keep","http":[{"method":"GET","path":"/info","extension":"keep"}],"grpc":["/auth.v1.Auth/Info"]}}}}`)
	canonical, err := m.CanonicalCatalog(input)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(canonical, &value); err != nil {
		t.Fatal(err)
	}
	rc := value["resource_catalog"].(map[string]any)
	feature := rc["buttons"].(map[string]any)["feature"].(map[string]any)
	http := feature["http"].([]any)[0].(map[string]any)
	if value["extension"] != float64(123) || rc["extension"] == nil || feature["extension"] != "keep" || http["extension"] != "keep" || http["path"] != "/auth/info" {
		t.Fatal(string(canonical))
	}
	if _, err := m.CanonicalCatalog([]byte("{")); err == nil {
		t.Fatal("accepted malformed JSON")
	}
	if _, err := m.CanonicalCatalog([]byte(`{"resource_catalog":false}`)); err == nil {
		t.Fatal("accepted malformed catalog")
	}
}
