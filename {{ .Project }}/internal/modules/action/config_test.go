package action

import (
 "{{ .Computed.module_name_final }}/internal/modules/localization"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

)

func actionCatalogFixture() localization.Catalog {
	return localization.Catalog{SchemaVersion: 2, DefaultLocale: "en-US", Scopes: []string{"admin", "mini"}, Operations: []string{"read", "create", "update", "delete"}, Locales: []localization.Language{ {Tag: "en-US", Name: "English", Enabled: true}}, Messages: map[string]localization.Messages{"en-US": {}}, ResourceCatalog: localization.ResourceCatalog{
		Defaults: map[string]localization.ResourceGrant{},
		Resources: map[string]localization.Resource{
			"admin:demo:record": {Kind: "resource", Operations: map[string]localization.ResourceOperation{"read": {EntryKeys: []string{}, ButtonKeys: []string{}}}},
			"admin:system:user": {Kind: "resource", Entries: map[string]localization.ResourceEntry{"admin:menu:system:user": {Path: "/system/user"}}, Operations: map[string]localization.ResourceOperation{
				"read":   {EntryKeys: []string{"admin:menu:system:user"}, ButtonKeys: []string{"admin:system:user:query", "admin:system:user:detail"}},
				"update": {EntryKeys: []string{}, ButtonKeys: []string{"admin:system:user:edit"}},
			}},
		},
		Buttons: map[string]localization.Feature{
			"admin:system:user:query":  {Btn: []string{"admin:btn:system:user:query"}, HTTP: []localization.HTTPPath{ {Method: "GET", Path: "/user"}}, GRPC: []string{}},
			"admin:system:user:detail": {Btn: []string{"admin:btn:system:user:view-detail"}, HTTP: []localization.HTTPPath{ {Method: "GET", Path: "/user/{id}"}}, GRPC: []string{}},
			"admin:system:user:edit":   {Btn: []string{"admin:btn:system:user:edit"}, HTTP: []localization.HTTPPath{ {Method: "PATCH", Path: "/user/{id}"}}, GRPC: []string{}},
		},
	}}
}

func installActionCatalog(t *testing.T, localizer *localization.Module) {
	t.Helper()
	fixture := actionCatalogFixture()
	current := localizer.Current()
	fixture.Messages = current.Messages
	fixture.Locales = current.Locales
	fixture.DefaultLocale = current.DefaultLocale

	raw, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(localizer.Reset)
}

func TestConfigSelectedFeaturesDetermineEndpoints(t *testing.T) {
 localizer := localization.New()
	installActionCatalog(t, localizer)
	for _, test := range []struct {
		name               string
		keys               []string
		buttons, endpoints int
		invalid            bool
	}{
		{"all registered features", nil, 2, 2, false},
		{"explicit empty grants no endpoints", []string{}, 0, 0, false},
		{"query excludes detail", []string{"admin:system:user:query"}, 1, 1, false},
		{"update cannot enter read", []string{"admin:system:user:edit"}, 0, 0, true},
		{"cross scope", []string{"mini:system:user:query"}, 0, 0, true},
		{"duplicate", []string{"admin:system:user:query", "admin:system:user:query"}, 0, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, err := ResolveConfig(localizer.Current(), "admin:read:system:user", Config{ResourceKey: "admin:system:user", Operation: "read", FeatureKeys: test.keys})
			if test.invalid {
				if !errors.Is(err, ErrInvalid) {
					t.Fatal(err)
				}
				return
			}
			if err != nil || len(c.UI) != 1 || len(c.UI[0].ButtonKeys) != test.buttons || len(c.Endpoints) != test.endpoints {
				t.Fatalf("config=%+v err=%v", c, err)
			}
			if test.name == "query excludes detail" && c.Endpoints[0].Path != "/user" {
				t.Fatal("detail permission leaked")
			}
		})
	}
}

func TestConfigIgnoresCallerTransportMappings(t *testing.T) {
 localizer := localization.New()
	installActionCatalog(t, localizer)
	input := Config{ResourceKey: "admin:system:user", Operation: "read", FeatureKeys: []string{"admin:system:user:query"}, UI: []UIBinding{ {MenuKey: "mini:menu:system:user", PageEntry: true}}, Endpoints: []Endpoint{ {Protocol: "http", Method: "DELETE", Path: "/user/{id}"}}}
	result, err := ResolveConfig(localizer.Current(), "admin:read:system:user", input)
	if err != nil || result.Endpoints[0].Method != "GET" || result.UI[0].MenuKey != "admin:menu:system:user" {
		t.Fatalf("%+v %v", result, err)
	}
	for _, word := range []string{"admin:edit:system:user", "admin:export:system:user", "admin:read:missing:resource"} {
		if _, err := ResolveConfig(localizer.Current(), word, Config{}); !errors.Is(err, ErrInvalid) {
			t.Fatal(word, err)
		}
	}
}

{{ if .Computed.enable_grpc_final }}
func TestConfigGRPCMethod(t *testing.T) {
 localizer := localization.New()
	installActionCatalog(t, localizer)
	catalog := localizer.Current()
	feature := catalog.ResourceCatalog.Buttons["admin:system:user:detail"]
	feature.GRPC = []string{"/auth.v1.User/GetUser"}
	catalog.ResourceCatalog.Buttons["admin:system:user:detail"] = feature
	raw, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	word := "admin:read:system:user"
	c, err := ResolveConfig(localizer.Current(), word, Config{ResourceKey: "admin:system:user", Operation: "read", FeatureKeys: []string{"admin:system:user:detail"}})
	if err != nil || c.Validate(word) != nil {
		t.Fatalf("gRPC config: %+v %v", c, err)
	}
	for index := range c.Endpoints {
		if c.Endpoints[index].Protocol == "grpc" {
			c.Endpoints[index].Method = ""
			if !errors.Is(c.Validate(word), ErrInvalid) {
				t.Fatal("accepted gRPC endpoint without GRPC method")
			}
			return
		}
	}
	t.Fatal("missing gRPC endpoint")
}
{{ end }}

func TestConfigEmptySelectionSurvivesJSON(t *testing.T) {
	c := Config{FeatureKeys: []string{}, UI: []UIBinding{}, Endpoints: []Endpoint{}}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Config
	if err := decoded.Scan(raw); err != nil || decoded.FeatureKeys == nil {
		t.Fatalf("%s %v", raw, err)
	}
	if !reflect.DeepEqual(decoded.FeatureKeys, c.FeatureKeys) {
		t.Fatal("empty selection changed")
	}
}

func TestBuiltinOperationMustMatchInBothDirections(t *testing.T) {
	for _, test := range []struct {
		operation, builtin string
		valid              bool
	}{
		{"read", "", true}, {"all", "all", true}, {"default", "default", true}, {"read", "all", false}, {"read", "default", false}, {"default", "all", false}, {"all", "default", false}, {"all", "", false}, {"default", "", false},
	} {
		c := Config{Builtin: test.builtin, UI: []UIBinding{}, Endpoints: []Endpoint{}}
        if test.builtin == "" && test.operation == "read" { c.ResourceKey = "admin:system:access"; c.Operation = "read" }
		if (c.Validate("admin:"+test.operation+":system:access") == nil) != test.valid {
			t.Fatal(test)
		}
	}
}

func TestStoredAllRemainsMarkerWhileReadsExpand(t *testing.T) {
 catalog:=resourceMoveCatalog("admin:system:user")
 config:=Config{Builtin:"all",UI:[]UIBinding{},Endpoints:[]Endpoint{}}
 if err:=resolveStoredConfig(catalog,"admin:all:system:general",&config);err!=nil {t.Fatal(err)}
 if len(config.UI)!=0 || len(config.Endpoints)!=0 {t.Fatal("stored all expanded")}
 expanded,err:=ResolveConfig(catalog,"admin:all:system:general",config)
 if err!=nil || len(expanded.UI)==0 || len(expanded.Endpoints)==0 {t.Fatal(expanded,err)}
 ordinary:=Config{Bindings:&[]ResourceBinding{ {ResourceKey:"admin:system:user",Operation:"read"}},UI:[]UIBinding{},Endpoints:[]Endpoint{}}
 if err:=resolveStoredConfig(catalog,"admin:read:system:user",&ordinary);err!=nil || len(ordinary.Endpoints)==0 {t.Fatal(ordinary,err)}
 if err:=resolveStoredConfig(catalog,"invalid",&ordinary);err==nil {t.Fatal("invalid definition accepted")}
}
