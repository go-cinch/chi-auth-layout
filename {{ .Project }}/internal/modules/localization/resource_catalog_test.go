package localization

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func resourceFixture() ResourceCatalog {
	return ResourceCatalog{Resources: map[string]Resource{
		"admin:system": {Kind: "directory", Sort: 10},
		"admin:system:user": {Kind: "resource", Parent: "admin:system", Sort: 10, Entries: map[string]ResourceEntry{"admin:menu:system:user": {Path: "/system/user"}}, Operations: map[string]ResourceOperation{
			"read":   {EntryKeys: []string{"admin:menu:system:user"}, ButtonKeys: []string{"admin:system:user:query", "admin:system:user:detail"}},
			"update": {EntryKeys: []string{}, ButtonKeys: []string{"admin:system:user:edit"}},
		}},
	}, Buttons: map[string]Feature{
		"admin:system:user:query": {Btn: []string{"admin:btn:system:user:query"}, HTTP: []HTTPPath{
			{Method: "GET", Path: "/user"},
		}, GRPC: []string{}},
		"admin:system:user:detail": {Btn: []string{"admin:btn:system:user:view-detail"}, HTTP: []HTTPPath{
			{Method: "GET", Path: "/user/{id}"},
		}, GRPC: []string{"/auth.v1.User/GetUser"}},
		"admin:system:user:edit": {Btn: []string{"admin:btn:system:user:edit"}, HTTP: []HTTPPath{
			{Method: "PATCH", Path: "/user/{id}"},
		}, GRPC: []string{}},
	}}
}

func TestResourceCatalogValidation(t *testing.T) {
	localizer := New()
	_ = localizer
	if err := resourceFixture().Validate([]string{"admin", "mini"}, FixedOperations); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*ResourceCatalog)
	}{
		{"cycle", func(c *ResourceCatalog) {
			r := c.Resources["admin:system"]
			r.Parent = "admin:system"
			c.Resources["admin:system"] = r
		}},
		{"orphan", func(c *ResourceCatalog) {
			r := c.Resources["admin:system:user"]
			r.Parent = "admin:missing"
			c.Resources["admin:system:user"] = r
		}},
		{"cross scope parent", func(c *ResourceCatalog) {
			c.Resources["mini:system"] = Resource{Kind: "directory"}
			r := c.Resources["admin:system:user"]
			r.Parent = "mini:system"
			c.Resources["admin:system:user"] = r
		}},
		{"unknown operation", func(c *ResourceCatalog) {
			r := c.Resources["admin:system:user"]
			r.Operations["export"] = ResourceOperation{EntryKeys: []string{}, ButtonKeys: []string{}}
		}},
		{"missing feature", func(c *ResourceCatalog) { delete(c.Buttons, "admin:system:user:query") }},
		{"cross resource button", func(c *ResourceCatalog) {
			f := c.Buttons["admin:system:user:query"]
			f.Btn = []string{"admin:btn:system:role:query"}
			c.Buttons["admin:system:user:query"] = f
		}},
		{"wildcard", func(c *ResourceCatalog) {
			f := c.Buttons["admin:system:user:query"]
			f.HTTP[0].Path = "/user/*"
			c.Buttons["admin:system:user:query"] = f
		}},
		{"invalid method", func(c *ResourceCatalog) {
			f := c.Buttons["admin:system:user:query"]
			f.HTTP[0].Method = "get"
			c.Buttons["admin:system:user:query"] = f
		}},
		{"invalid rpc", func(c *ResourceCatalog) {
			f := c.Buttons["admin:system:user:detail"]
			f.GRPC = []string{"/user"}
			c.Buttons["admin:system:user:detail"] = f
		}},
		{"unknown entry", func(c *ResourceCatalog) {
			r := c.Resources["admin:system:user"]
			op := r.Operations["read"]
			op.EntryKeys = []string{"admin:menu:system:role"}
			r.Operations["read"] = op
		}},
		{"negative sort", func(c *ResourceCatalog) {
			r := c.Resources["admin:system:user"]
			r.Sort = -1
			c.Resources["admin:system:user"] = r
		}},
		{"directory grants", func(c *ResourceCatalog) {
			r := c.Resources["admin:system"]
			r.Operations = map[string]ResourceOperation{"read": {}}
			c.Resources["admin:system"] = r
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := resourceFixture()
			test.mutate(&c)
			if err := c.Validate([]string{"admin", "mini"}, FixedOperations); err == nil {
				t.Fatal("accepted invalid catalog")
			}
		})
	}
	if err := resourceFixture().Validate([]string{"admin"}, []string{"read", "edit"}); err == nil {
		t.Fatal("accepted custom operations")
	}
}

func TestResolveResourceFeaturesAndPublicProjection(t *testing.T) {
	localizer := New()
	_ = localizer
	c := resourceFixture()
	read, err := c.ResolveBinding("admin", "admin:system:user", "read", []string{"admin:system:user:query"})
	if err != nil || len(read.Endpoints) != 1 || read.Endpoints[0].Path != "/user" || !read.UI[0].PageEntry {
		t.Fatalf("%+v %v", read, err)
	}
	empty, err := c.ResolveBinding("admin", "admin:system:user", "read", []string{})
	if err != nil || len(empty.Endpoints) != 0 || empty.UI[0].ButtonKeys == nil {
		t.Fatalf("%+v %v", empty, err)
	}
	all, err := c.ResolveBinding("admin", "admin:system:user", "read", nil)
	if err != nil || len(all.Endpoints) != 3 {
		t.Fatalf("%+v %v", all, err)
	}
	grpc := ResourceEndpoint{Protocol: "grpc", Method: "GRPC", Path: "/auth.v1.User/GetUser"}
	foundGRPC := false
	for _, endpoint := range all.Endpoints {
		if endpoint == grpc {
			foundGRPC = true
			if endpoint.Display() != "GRPC|/auth.v1.User/GetUser" {
				t.Fatal(endpoint.Display())
			}
		}
	}
	if !foundGRPC {
		t.Fatalf("missing explicit GRPC method: %+v", all.Endpoints)
	}
	for _, keys := range [][]string{
		{"admin:system:user:edit"}, {"admin:system:user:query", "admin:system:user:query"}, {"mini:system:user:query"},
	} {
		if _, err := c.ResolveBinding("admin", "admin:system:user", "read", keys); err == nil {
			t.Fatal(keys)
		}
	}
	for _, invalid := range []struct{ key, operation string }{
		{"bad", "read"}, {"admin:missing:resource", "read"}, {"admin:system:user", "export"},
	} {
		if _, err := c.ResolveBinding("admin", invalid.key, invalid.operation, nil); err == nil {
			t.Fatal(invalid)
		}
	}
	t.Cleanup(localizer.Reset)
	catalog := localizer.Current()
	catalog.ResourceCatalog = c
	raw, _ := json.Marshal(catalog)
	if err := localizer.Install(raw); err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(localizer.PublicCatalog())
	if strings.Contains(string(public), "resource_catalog") || strings.Contains(string(public), "/auth.v1.User/") {
		t.Fatalf("management mappings exposed: %s", public)
	}
	detached := localizer.PermissionCatalog()
	delete(detached.Buttons, "admin:system:user:query")
	if reflect.DeepEqual(detached, localizer.PermissionCatalog()) {
		t.Fatal("catalog snapshot mutated")
	}
}

func TestCatalogPaths(t *testing.T) {
	localizer := New()
	_ = localizer
	for _, path := range []string{"/user/*", "//user", "/user?x=1", "/user/%2f", "/../user", "/user/{bad param}", "/user/:id"} {
		if CatalogPath(path, true) {
			t.Fatal(path)
		}
	}
	if !CatalogPath("/user/{id}", true) || CatalogPath("/user/{id}", false) {
		t.Fatal("invalid parameter handling")
	}
}

func TestRootFrontendVisibility(t *testing.T) {
	localizer := New()
	_ = localizer
	catalog := resourceFixture()
	visible := false
	root := catalog.Resources["admin:system"]
	root.FrontendVisible = &visible
	catalog.Resources["admin:system"] = root
	if err := catalog.Validate([]string{"admin"}, FixedOperations); err != nil {
		t.Fatal(err)
	}
	page := catalog.Resources["admin:system:user"]
	page.FrontendVisible = &visible
	catalog.Resources["admin:system:user"] = page
	if err := catalog.Validate([]string{"admin"}, FixedOperations); err == nil {
		t.Fatal("accepted visibility on a resource")
	}
	page.FrontendVisible = nil
	catalog.Resources["admin:system:user"] = page
	catalog.Resources["admin:system:subdir"] = Resource{Kind: "directory", Parent: "admin:system", FrontendVisible: &visible}
	if err := catalog.Validate([]string{"admin"}, FixedOperations); err == nil {
		t.Fatal("accepted visibility on a nested directory")
	}
}

func TestOrdinaryRootReferences(t *testing.T) {
	localizer := New()
	_ = localizer
	catalog := resourceFixture()
	catalog.Resources["admin:renamed-root"] = Resource{Kind: "directory", References: []string{"admin:system:user"}}
	catalog.Resources["admin:another-root"] = Resource{Kind: "directory", References: []string{"admin:system"}}
	if err := catalog.Validate([]string{"admin"}, FixedOperations); err != nil {
		t.Fatal(err)
	}
	if catalog.Resources["admin:system:user"].Parent != "admin:system" {
		t.Fatal("reference changed canonical ownership")
	}
	raw, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	var restored ResourceCatalog
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate([]string{"admin"}, FixedOperations); err != nil {
		t.Fatal(err)
	}
	if len(restored.Resources["admin:renamed-root"].References) != 1 {
		t.Fatal("reference lost on persistence")
	}
}

func TestResourceCatalogEqualUsesPersistedSemantics(t *testing.T) {
	before := resourceFixture()
	node := before.Resources["admin:system"]
	node.ChildOrder = []ResourceChildOrder{}
	node.Entries = map[string]ResourceEntry{}
	before.Resources["admin:system"] = node
	raw, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	var after ResourceCatalog
	if err := json.Unmarshal(raw, &after); err != nil {
		t.Fatal(err)
	}
	if !before.Equal(after) {
		t.Fatal("omitted empty fields changed the resource contract")
	}
	node = after.Resources["admin:system"]
	node.Sort++
	after.Resources["admin:system"] = node
	if before.Equal(after) {
		t.Fatal("resource change was ignored")
	}
}

func TestScopedBuiltinsAndExplicitDefaultFeatures(t *testing.T) {
	catalog := New().Current()
	catalog.ResourceCatalog = resourceFixture()
	for _, test := range []struct {
		name                string
		resources, features []string
		endpoints           int
		invalid             bool
	}{
		{"all readable features", []string{"admin:system:user"}, nil, 3, false},
		{"explicit empty", []string{"admin:system:user"}, []string{}, 0, false},
		{"only query", []string{"admin:system:user"}, []string{"admin:system:user:query"}, 1, false},
		{"missing resource", []string{"admin:missing:user"}, nil, 0, true},
		{"directory", []string{"admin:system"}, nil, 0, true},
		{"duplicate resource", []string{"admin:system:user", "admin:system:user"}, nil, 0, true},
		{"wrong operation feature", []string{"admin:system:user"}, []string{"admin:system:user:edit"}, 0, true},
		{"duplicate feature", []string{"admin:system:user"}, []string{"admin:system:user:query", "admin:system:user:query"}, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			grant, err := catalog.ResolvePermission("admin:default:system:default", test.features, "default", test.resources)
			if test.invalid {
				if err == nil {
					t.Fatal("invalid default accepted")
				}
				return
			}
			if err != nil || len(grant.Endpoints) != test.endpoints || len(grant.UI) != 1 {
				t.Fatalf("%+v %v", grant, err)
			}
		})
	}
	all, err := catalog.ResolvePermission("admin:all:system:all", nil, "all")
	if err != nil || len(all.Endpoints) != 4 {
		t.Fatalf("scoped all: %+v %v", all, err)
	}
	mini, err := catalog.ResolvePermission("mini:all:system:general", nil, "all")
	if err != nil || len(mini.Endpoints) != 0 || len(mini.UI) != 0 {
		t.Fatalf("cross-scope expansion: %+v %v", mini, err)
	}
	if _, err := catalog.ResolvePermission("admin:grant:system:permission-admin", nil, ""); err == nil {
		t.Fatal("an action word conferred authority without bindings")
	}
	for _, test := range []struct{ word, builtin string }{
		{"bad", "all"}, {"admin:read:system:user", ""}, {"admin:all:system:all", "unknown"},
	} {
		if _, err := catalog.ResolvePermission(test.word, nil, test.builtin); err == nil {
			t.Fatal(test)
		}
	}
	if _, err := catalog.ResolveResourceBinding("admin", "admin:system:user", "read", []string{}); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.ResolveResourceBinding("mini", "admin:system:user", "read", nil); err == nil {
		t.Fatal("cross-scope binding accepted")
	}
}

func TestSelectedResourceExpansion(t *testing.T) {
	catalog := resourceFixture()
	for _, field := range []string{"http", "grpc", "ui"} {
		grant, err := catalog.ResolveBinding("admin", "admin:system:user", "read", nil, []string{field})
		if err != nil {
			t.Fatal(err)
		}
		if (len(grant.UI) > 0) != (field == "ui") {
			t.Fatalf("unselected UI: %s %+v", field, grant)
		}
		for _, endpoint := range grant.Endpoints {
			if endpoint.Protocol != field {
				t.Fatalf("unselected endpoint: %s %+v", field, endpoint)
			}
		}
	}
}

func TestEmptyResourcesAndOrganizationalAssociations(t *testing.T) {
	hidden := false
	graph := ResourceCatalog{
		Buttons: map[string]Feature{},
		Resources: map[string]Resource{
			"admin:high-risk":                  {Kind: "directory", FrontendVisible: &hidden, PermissionGroup: "admin:system:high-risk"},
			"admin:high-risk:all":              {Kind: "resource", Parent: "admin:high-risk", PermissionScope: "*", Operations: map[string]ResourceOperation{}},
			"admin:high-risk:permission-admin": {Kind: "resource", Parent: "admin:high-risk"},
		},
	}
	catalog := Catalog{SchemaVersion: 2, DefaultLocale: "en-US", Scopes: []string{"admin"}, Operations: FixedOperations,
		Locales: []Language{
			{Tag: "en-US", Name: "English", Enabled: true},
		},
		Messages: map[string]Messages{"en-US": {Frontend: map[string]any{}}}, ResourceCatalog: graph}
	raw, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := New().CanonicalCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(normalized)
	if err != nil {
		t.Fatalf("empty resources rejected: %v", err)
	}
	roundtrip, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := Parse(roundtrip)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ResourceCatalog.Resources["admin:high-risk"].PermissionGroup != "admin:system:high-risk" || persisted.ResourceCatalog.Resources["admin:high-risk:all"].PermissionScope != "*" {
		t.Fatal("organizational associations were lost")
	}
	if _, err := graph.ResolveBinding("admin", "admin:high-risk:all", "read", nil); err == nil {
		t.Fatal("an unconfigured resource granted an operation")
	}
	for _, test := range []struct {
		name, key string
		value     Resource
	}{
		{"cross scope group", "admin:high-risk", Resource{Kind: "directory", PermissionGroup: "mini:system:high-risk"}},
		{"group on resource", "admin:high-risk:all", Resource{Kind: "resource", Parent: "admin:high-risk", PermissionGroup: "admin:system:high-risk"}},
		{"cross scope action", "admin:high-risk:all", Resource{Kind: "resource", Parent: "admin:high-risk", PermissionScope: "mini:*"}},
		{"action on directory", "admin:high-risk", Resource{Kind: "directory", PermissionScope: "*"}},
		{"invalid action", "admin:high-risk:all", Resource{Kind: "resource", Parent: "admin:high-risk", PermissionScope: "invalid"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := graph.Resources[test.key]
			graph.Resources[test.key] = test.value
			defer func() { graph.Resources[test.key] = original }()
			if err := graph.Validate([]string{"admin", "mini"}, FixedOperations); err == nil {
				t.Fatal("invalid association accepted")
			}
		})
	}
}
