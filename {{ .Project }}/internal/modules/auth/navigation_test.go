package auth

import (
	"testing"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

func TestPermissionNavigation(t *testing.T) {
	no := false
	page := func(parent, menu, path string, order int) localization.Resource {
		return localization.Resource{Kind: "resource", Parent: parent, Sort: order, Entries: map[string]localization.ResourceEntry{menu: {Path: path}}}
	}
	catalog := localization.ResourceCatalog{Resources: map[string]localization.Resource{
		"admin:default":         {Kind: "directory", FrontendVisible: &no, Sort: 0, References: []string{"admin:audit:user"}},
		"admin:default:profile": page("admin:default", "admin:menu:profile", "/profile", 0),
		"admin:system":          {Kind: "directory", Sort: 20},
		"admin:system:role":     page("admin:system", "admin:menu:role", "/system/role", 0),
		"admin:audit": {Kind: "directory", Sort: 10, ChildOrder: []localization.ResourceChildOrder{
			{Kind: "node", Key: "admin:audit:logs"}, {Kind: "node", Key: "admin:audit:user"}}},
		"admin:audit:user":  page("admin:audit", "admin:menu:system:user", "/system/user", 0),
		"admin:audit:logs":  page("admin:audit", "admin:menu:logs", "/audit/login", 10),
		"admin:hidden":      {Kind: "directory", Sort: 1, FrontendVisible: &no},
		"admin:hidden:page": page("admin:hidden", "admin:menu:hidden", "/hidden", 0),
		"mini:other":        {Kind: "directory"},
	}}
	menus := []PermissionMenu{
		{MenuKey: "admin:menu:system:user", MenuPath: "/system/user"},
		{MenuKey: "admin:menu:logs", MenuPath: "/audit/login"},
		{MenuKey: "admin:menu:profile", MenuPath: "/profile"},
		{MenuKey: "admin:menu:hidden", MenuPath: "/hidden"},
	}
	navigation := permissionNavigation(catalog, "admin", menus)
	if len(navigation) != 1 || navigation[0].ResourceKey != "admin:audit" || len(navigation[0].Children) != 2 {
		t.Fatalf("unexpected navigation: %+v", navigation)
	}
	if navigation[0].Children[0].MenuPath != "/audit/login" || navigation[0].Children[1].MenuPath != "/system/user" {
		t.Fatalf("catalog ordering or moved route lost: %+v", navigation[0].Children)
	}
	if len(menus) != 4 {
		t.Fatal("navigation must not change permissions")
	}
	yes := true
	root := catalog.Resources["admin:default"]
	root.FrontendVisible = &yes
	catalog.Resources["admin:default"] = root
	navigation = permissionNavigation(catalog, "admin", menus)
	if len(navigation) != 2 || len(navigation[0].Children) != 1 || navigation[0].Children[0].MenuPath != "/profile" {
		t.Fatalf("explicit visibility or reference deduplication failed: %+v", navigation)
	}
	if got := permissionNavigation(catalog, "admin", nil); len(got) != 0 {
		t.Fatalf("unauthorized navigation: %+v", got)
	}
}

func TestNavigationVisibilityUsesExplicitFlag(t *testing.T) {
	no := false
	catalog := localization.ResourceCatalog{Resources: map[string]localization.Resource{
		"admin:renamed":         {Kind: "directory", FrontendVisible: &no},
		"admin:renamed:profile": {Kind: "resource", Parent: "admin:renamed", Entries: map[string]localization.ResourceEntry{"admin:menu:profile": {Path: "/profile"}}},
		"admin:default":         {Kind: "directory"},
		"admin:default:user":    {Kind: "resource", Parent: "admin:default", Entries: map[string]localization.ResourceEntry{"admin:menu:user": {Path: "/system/user"}}},
	}}
	menus := []PermissionMenu{
		{MenuKey: "admin:menu:profile", MenuPath: "/profile"},
		{MenuKey: "admin:menu:user", MenuPath: "/system/user"},
	}
	navigation := permissionNavigation(catalog, "admin", menus)
	if len(navigation) != 1 || navigation[0].ResourceKey != "admin:default" {
		t.Fatalf("visibility depends on key instead of explicit flag: %+v", navigation)
	}
}
