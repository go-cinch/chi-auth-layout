package action

import (
	"encoding/json"
	"testing"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

func TestConvertedDirectoryKeepsExplicitDescendantGrants(t *testing.T) {
	before := localization.ResourceCatalog{Resources: map[string]localization.Resource{"admin:system:parent": {Kind: "resource"}}}
	after := localization.ResourceCatalog{Resources: map[string]localization.Resource{
		"admin:system:parent":    {Kind: "directory", References: []string{"admin:system:reference", "admin:system:parent", "mini:system:hidden"}, ReferenceOperations: map[string][]string{"admin:system:reference": {"read"}}},
		"admin:system:child":     {Kind: "resource", Parent: "admin:system:parent", Operations: map[string]localization.ResourceOperation{"read": {ButtonKeys: []string{"child:read"}}, "update": {ButtonKeys: []string{"child:update"}}}},
		"admin:system:reference": {Kind: "resource", Operations: map[string]localization.ResourceOperation{"read": {ButtonKeys: []string{"reference:read"}}, "update": {ButtonKeys: []string{"reference:update"}}}},
		"mini:system:hidden":     {Kind: "resource", Operations: map[string]localization.ResourceOperation{"read": {ButtonKeys: []string{"hidden:read"}}}},
	}}
	for _, raw := range []string{
		`{"bindings":[{"resource_key":"admin:system:parent","operation":"read","feature_keys":[]}]}`,
		`{"resource_key":"admin:system:parent","operation":"read","feature_keys":[]}`,
	} {
		encoded, changed, err := MigrateConvertedResourceConfig(before, after, []byte(raw), nil, nil)
		if err != nil || !changed {
			t.Fatal(changed, err)
		}
		var config Config
		if err := json.Unmarshal(encoded, &config); err != nil {
			t.Fatal(err)
		}
		if config.Bindings == nil || len(*config.Bindings) != 2 || config.ResourceKey != "" {
			t.Fatal(string(encoded))
		}
		if (*config.Bindings)[0].ResourceKey != "admin:system:child" || (*config.Bindings)[1].ResourceKey != "admin:system:reference" {
			t.Fatal(string(encoded))
		}
	}
	all := directoryResourceBindings(after, "admin:system:parent", "*")
	if len(all) != 3 {
		t.Fatal("reference operation restrictions lost", all)
	}
	raw := []byte(`{"builtin":"default","resource_keys":["admin:system:parent","admin:system:reference"],"feature_keys":["reference:read"]}`)
	encoded, changed, err := MigrateConvertedResourceConfig(before, after, raw, nil, nil)
	if err != nil || !changed {
		t.Fatal(err)
	}
	var config Config
	if json.Unmarshal(encoded, &config) != nil || config.ResourceKeys == nil || len(*config.ResourceKeys) != 2 || len(config.FeatureKeys) != 2 {
		t.Fatal(string(encoded))
	}
	unchanged := []byte(`{"bindings":[]}`)
	got, changed, err := MigrateConvertedResourceConfig(before, after, unchanged, nil, nil)
	if err != nil || changed || string(got) != string(unchanged) {
		t.Fatal(string(got), changed, err)
	}
}

func TestConvertedResourcesRejectMalformedData(t *testing.T) {
	for _, raw := range []string{`{`, `{"bindings":1}`, `{"resource_key":42}`, `{"builtin":42}`, `{"builtin":"default","resource_keys":42}`, `{"builtin":"default","resource_keys":[],"feature_keys":42}`} {
		if _, _, err := MigrateConvertedResourceConfig(localization.ResourceCatalog{}, localization.ResourceCatalog{}, []byte(raw), nil, nil); err == nil {
			t.Fatal(raw)
		}
	}
}
