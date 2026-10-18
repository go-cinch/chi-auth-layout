package localization

import (
	"sort"
	"strings"
)

// MenuResourceKeys resolves stable menu identifiers to their name-owning node.
func (c ResourceCatalog) MenuResourceKeys() map[string]string {
	owners := make(map[string]string)
	for key := range c.Resources {
		owners[strings.Replace(key, ":", ":menu:", 1)] = key
	}
	for key, node := range c.Resources {
		for menu := range node.Entries {
			owners[menu] = key
		}
	}
	return owners
}

// NormalizeResourceNames retains one stored translation per directory/resource.
// Existing basic names take precedence over former independent menu labels.
func (c *Catalog) NormalizeResourceNames() {
	owners := c.ResourceCatalog.MenuResourceKeys()
	keys := make([]string, 0, len(owners))
	for menu := range owners {
		keys = append(keys, menu)
	}
	sort.Strings(keys)
	for _, messages := range c.Messages {
		menus, ok := messages.Frontend["menus"].(map[string]any)
		if !ok {
			continue
		}
		names, _ := messages.Frontend["resources"].(map[string]any)
		for _, menu := range keys {
			key := owners[menu]
			label, exists := menus[menu]
			if !exists {
				continue
			}
			if _, exists := names[key]; !exists {
				if label, ok := label.(string); ok {
					if names == nil {
						names = make(map[string]any)
					}
					names[key] = label
				}
			}
			delete(menus, menu)
		}
		if len(names) > 0 {
			messages.Frontend["resources"] = names
		}
		if len(menus) == 0 {
			delete(messages.Frontend, "menus")
		}
	}
}

// Public clients keep the menus namespace as a read-only projection so existing
// route identifiers still work. These labels are never stored independently.
func (c *Catalog) projectMenuNames() {
	owners := c.ResourceCatalog.MenuResourceKeys()
	for _, messages := range c.Messages {
		names, _ := messages.Frontend["resources"].(map[string]any)
		menus, _ := messages.Frontend["menus"].(map[string]any)
		for menu, key := range owners {
			if label, ok := names[key].(string); ok && label != "" {
				if menus == nil {
					menus = make(map[string]any)
				}
				menus[menu] = label
			}
		}
		if len(menus) > 0 {
			messages.Frontend["menus"] = menus
		}
	}
}
