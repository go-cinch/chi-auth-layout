package auth

import (
	"sort"
	"strings"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

// PermissionNavigation is a display projection; it never grants route access.
type PermissionNavigation struct {
	ResourceKey string                 `json:"resource_key"`
	MenuKey     string                 `json:"menu_key,omitempty"`
	MenuPath    string                 `json:"menu_path,omitempty"`
	Children    []PermissionNavigation `json:"children,omitempty"`
}

func permissionNavigation(catalog localization.ResourceCatalog, scope string, menus []PermissionMenu) []PermissionNavigation {
	allowed := make(map[string]string, len(menus))
	for _, menu := range menus {
		allowed[menu.MenuKey] = menu.MenuPath
	}
	children := map[string][]string{}
	for key, resource := range catalog.Resources {
		if strings.HasPrefix(key, scope+":") {
			children[resource.Parent] = append(children[resource.Parent], key)
		}
	}
	for parent := range children {
		sort.Slice(children[parent], func(i, j int) bool {
			a, b := children[parent][i], children[parent][j]
			if catalog.Resources[a].Sort != catalog.Resources[b].Sort {
				return catalog.Resources[a].Sort < catalog.Resources[b].Sort
			}
			return a < b
		})
		// Use the canonical node order. References remain links in the editor.
		ordered, seen := []string{}, map[string]bool{}
		for _, item := range catalog.Resources[parent].ChildOrder {
			if node, ok := catalog.Resources[item.Key]; item.Kind == "node" && ok && node.Parent == parent && strings.HasPrefix(item.Key, scope+":") && !seen[item.Key] {
				ordered = append(ordered, item.Key)
				seen[item.Key] = true
			}
		}
		for _, key := range children[parent] {
			if !seen[key] {
				ordered = append(ordered, key)
			}
		}
		children[parent] = ordered
	}
	var visit func(string) []PermissionNavigation
	visit = func(parent string) []PermissionNavigation {
		result := []PermissionNavigation{}
		for _, key := range children[parent] {
			resource := catalog.Resources[key]
			if parent == "" {
				visible := true
				if resource.FrontendVisible != nil {
					visible = *resource.FrontendVisible
				}
				if !visible {
					continue
				}
			}
			if resource.Kind == "directory" {
				nested := visit(key)
				if len(nested) > 0 {
					result = append(result, PermissionNavigation{ResourceKey: key, Children: nested})
				}
				continue
			}
			entries := []string{}
			for menuKey, entry := range resource.Entries {
				if path, ok := allowed[menuKey]; ok && path == entry.Path {
					entries = append(entries, menuKey)
				}
			}
			sort.Strings(entries)
			for _, menuKey := range entries {
				result = append(result, PermissionNavigation{ResourceKey: key, MenuKey: menuKey, MenuPath: resource.Entries[menuKey].Path})
			}
		}
		return result
	}
	return visit("")
}
