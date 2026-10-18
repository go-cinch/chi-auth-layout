package localization

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var FixedOperations = []string{"read", "create", "update", "delete"}
var catalogSegment = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

type ResourceCatalog struct {
	Resources map[string]Resource      `json:"resources"`
	Buttons   map[string]Feature       `json:"buttons"`
	Defaults  map[string]ResourceGrant `json:"defaults"`
}

type ResourceChildOrder struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type Resource struct {
	FrontendVisible *bool                        `json:"frontend_visible,omitempty"`
	ChildOrder      []ResourceChildOrder         `json:"child_order,omitempty"`
	Kind            string                       `json:"kind"`
	Parent          string                       `json:"parent,omitempty"`
	Collection      string                       `json:"collection,omitempty"`
	References      []string                     `json:"references,omitempty"`
	Sort            int                          `json:"sort"`
	Entries         map[string]ResourceEntry     `json:"entries,omitempty"`
	Operations      map[string]ResourceOperation `json:"operations,omitempty"`
}

type ResourceEntry struct {
	Path          string `json:"path"`
	ParentMenuKey string `json:"parent_menu_key,omitempty"`
}

type ResourceOperation struct {
	EntryKeys  []string `json:"entry_keys"`
	ButtonKeys []string `json:"button_keys"`
}

type HTTPPath struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type Feature struct {
	Btn  []string   `json:"btn"`
	HTTP []HTTPPath `json:"http"`
	GRPC []string   `json:"grpc"`
}

type ResourceUI struct {
	MenuKey       string   `json:"menu_key"`
	ParentMenuKey string   `json:"parent_menu_key,omitempty"`
	MenuPath      string   `json:"menu_path"`
	PageEntry     bool     `json:"page_entry"`
	ButtonKeys    []string `json:"button_keys"`
}

type ResourceEndpoint struct {
	Protocol string `json:"protocol"`
	Method   string `json:"method"`
	Path     string `json:"path"`
}

func (e ResourceEndpoint) Display() string {
	return e.Method + "|" + e.Path
}

type ResourceGrant struct {
	UI        []ResourceUI       `json:"ui"`
	Endpoints []ResourceEndpoint `json:"endpoints"`
}

func catalogKey(key string, min, max int) bool {
	parts := strings.Split(key, ":")
	if len(parts) < min || len(parts) > max {
		return false
	}
	for _, part := range parts {
		if !catalogSegment.MatchString(part) {
			return false
		}
	}
	return true
}

func CatalogPath(path string, parameters bool) bool {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "*?#\\\n\r\t %") || strings.Contains(path, "//") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." {
			return false
		}
		if strings.ContainsAny(part, "{}:") && (!parameters || len(part) < 3 || part[0] != '{' || part[len(part)-1] != '}' || !catalogSegment.MatchString(part[1:len(part)-1])) {
			return false
		}
	}
	return true
}

func (c ResourceCatalog) Validate(scopes, operations []string) error {
	if !slices.Equal(operations, FixedOperations) {
		return errors.New("operations must be read,create,update,delete in order")
	}
	seenScopes := map[string]bool{}
	for _, scope := range scopes {
		if !catalogKey(scope, 1, 1) || seenScopes[scope] {
			return errors.New("invalid or duplicate scope")
		}
		seenScopes[scope] = true
	}
	if len(scopes) == 0 || c.Resources == nil || c.Buttons == nil || c.Defaults == nil {
		return errors.New("missing resource catalog")
	}
	featureOwners := map[string]string{}
	entryOwners := map[string]string{}
	collections := map[string]bool{}
	edges := map[string][]string{}
	for key, r := range c.Resources {
		parts := strings.Split(key, ":")
		if !catalogKey(key, 2, 128) || len(key)+7 > 128 || !seenScopes[parts[0]] || (r.Kind != "directory" && r.Kind != "resource") || r.Sort < 0 {
			return fmt.Errorf("invalid resource %s", key)
		}
		if r.Kind == "resource" && (len(parts) < 3 || len(r.Operations) == 0) {
			return fmt.Errorf("resource needs module and resource: %s", key)
		}
		if r.Kind == "directory" && (len(r.Entries) > 0 || len(r.Operations) > 0) {
			return fmt.Errorf("directory cannot grant permissions: %s", key)
		}
		if r.FrontendVisible != nil && (r.Kind != "directory" || r.Parent != "") {
			return fmt.Errorf("frontend visibility requires a root directory: %s", key)
		}
		if r.Collection != "" {
			if r.Kind != "directory" || r.Collection != "default_permissions" || collections[parts[0]] {
				return fmt.Errorf("invalid or duplicate resource collection: %s", key)
			}
			collections[parts[0]] = true
		}
		ordered := map[string]bool{}
		for _, item := range r.ChildOrder {
			id := item.Kind + ":" + item.Key
			target, exists := c.Resources[item.Key]
			if r.Kind != "directory" || ordered[id] || !exists || (item.Kind == "node" && target.Parent != key) || (item.Kind == "reference" && !slices.Contains(r.References, item.Key)) || (item.Kind != "node" && item.Kind != "reference") {
				return fmt.Errorf("invalid directory order: %s", key)
			}
			ordered[id] = true
		}
		if r.Parent != "" {
			edges[r.Parent] = append(edges[r.Parent], key)
		}
		seenReferences := map[string]bool{}
		for _, target := range r.References {
			if r.Kind != "directory" || target == key || seenReferences[target] || strings.Split(target, ":")[0] != parts[0] {
				return fmt.Errorf("invalid resource reference: %s", key)
			}
			if _, ok := c.Resources[target]; !ok {
				return fmt.Errorf("missing resource reference target: %s", target)
			}
			seenReferences[target] = true
			edges[key] = append(edges[key], target)
		}
		visited := map[string]bool{key: true}
		for parent := r.Parent; parent != ""; {
			p, ok := c.Resources[parent]
			if !ok || visited[parent] || strings.Split(parent, ":")[0] != parts[0] || p.Kind != "directory" {
				return fmt.Errorf("invalid resource parent: %s", key)
			}
			visited[parent] = true
			parent = p.Parent
		}
		for entryKey, entry := range r.Entries {
			if !catalogKey(entryKey, 3, 128) || !strings.HasPrefix(entryKey, parts[0]+":menu:") || !CatalogPath(entry.Path, false) || entryOwners[entryKey] != "" {
				return fmt.Errorf("invalid resource entry %s", entryKey)
			}
			if entry.ParentMenuKey != "" && (!catalogKey(entry.ParentMenuKey, 3, 128) || !strings.HasPrefix(entry.ParentMenuKey, parts[0]+":menu:") || entry.ParentMenuKey == entryKey) {
				return fmt.Errorf("invalid entry parent %s", entryKey)
			}
			entryOwners[entryKey] = key
		}
		for operation, binding := range r.Operations {
			if !slices.Contains(operations, operation) || binding.EntryKeys == nil || binding.ButtonKeys == nil {
				return fmt.Errorf("invalid operation %s:%s", key, operation)
			}
			seen := map[string]bool{}
			for _, entry := range binding.EntryKeys {
				if _, ok := r.Entries[entry]; !ok || seen[entry] {
					return fmt.Errorf("unknown or duplicate entry %s", entry)
				}
				seen[entry] = true
			}
			seen = map[string]bool{}
			for _, feature := range binding.ButtonKeys {
				if _, ok := c.Buttons[feature]; !ok || !strings.HasPrefix(feature, key+":") || strings.Contains(strings.TrimPrefix(feature, key+":"), ":") || seen[feature] || featureOwners[feature] != "" {
					return fmt.Errorf("invalid feature reference %s", feature)
				}
				featureOwners[feature] = key
				seen[feature] = true
			}
		}
	}
	states := map[string]uint8{}
	var visit func(string) bool
	visit = func(key string) bool {
		if states[key] == 1 {
			return false
		}
		if states[key] == 2 {
			return true
		}
		states[key] = 1
		for _, target := range edges[key] {
			if !visit(target) {
				return false
			}
		}
		states[key] = 2
		return true
	}
	for key := range c.Resources {
		if !visit(key) {
			return fmt.Errorf("cyclic resource references: %s", key)
		}
	}
	for key, feature := range c.Buttons {
		if !catalogKey(key, 4, 128) || featureOwners[key] == "" || feature.Btn == nil || feature.HTTP == nil || feature.GRPC == nil {
			return fmt.Errorf("invalid or unused feature %s", key)
		}
		owner := featureOwners[key]
		// Menu/button keys are stable frontend bindings, independent of resource ownership.
		validButton := func(button string) bool {
			for entry := range c.Resources[owner].Entries {
				prefix := strings.Replace(entry, ":menu:", ":btn:", 1) + ":"
				if strings.HasPrefix(button, prefix) && !strings.Contains(strings.TrimPrefix(button, prefix), ":") {
					return true
				}
			}
			return false
		}
		if len(feature.Btn) > 0 && len(c.Resources[owner].Entries) == 0 {
			return fmt.Errorf("button needs a resource entry %s", key)
		}
		seen := map[string]bool{}
		for _, btn := range feature.Btn {
			if !catalogKey(btn, 5, 128) || !validButton(btn) || seen[btn] {
				return fmt.Errorf("invalid frontend button %s", btn)
			}
			seen[btn] = true
		}
		for _, endpoint := range feature.HTTP {
			if !slices.Contains([]string{"GET", "POST", "PATCH", "PUT", "DELETE", "HEAD", "OPTIONS"}, endpoint.Method) || !CatalogPath(endpoint.Path, true) {
				return fmt.Errorf("invalid http endpoint %s", key)
			}
		}
		for _, path := range feature.GRPC {
			if !CatalogPath(path, false) || len(strings.Split(path, "/")) != 3 {
				return fmt.Errorf("invalid rpc endpoint %s", key)
			}
		}
	}
	for scope, grant := range c.Defaults {
		if !seenScopes[scope] {
			return fmt.Errorf("unknown default scope %s", scope)
		}
		for _, ui := range grant.UI {
			if !strings.HasPrefix(ui.MenuKey, scope+":menu:") || !catalogKey(ui.MenuKey, 3, 128) || !CatalogPath(ui.MenuPath, false) {
				return errors.New("invalid default entry")
			}
			for _, key := range ui.ButtonKeys {
				if !strings.HasPrefix(key, scope+":btn:") || !catalogKey(key, 5, 128) {
					return errors.New("invalid default button")
				}
			}
		}
		for _, endpoint := range grant.Endpoints {
			if endpoint.Protocol == "http" {
				if !slices.Contains([]string{"GET", "POST", "PATCH", "PUT", "DELETE", "HEAD", "OPTIONS"}, endpoint.Method) || !CatalogPath(endpoint.Path, true) {
					return errors.New("invalid default http endpoint")
				}
			} else if endpoint.Protocol != "grpc" || endpoint.Method != "GRPC" || !CatalogPath(endpoint.Path, false) || len(strings.Split(endpoint.Path, "/")) != 3 {
				return errors.New("invalid default endpoint")
			}
		}
	}
	return nil
}

// Resolve expands only registered feature references. Caller-supplied transport
// paths are never used as authority. Nil selects all configured features; [] none.
func (c ResourceCatalog) Resolve(word string, selected []string) (ResourceGrant, error) {
	parts := strings.Split(word, ":")
	if len(parts) < 4 {
		return ResourceGrant{}, errors.New("invalid permission word")
	}
	return c.ResolveBinding(parts[0], parts[0]+":"+strings.Join(parts[2:], ":"), parts[1], selected)
}

// ResolveBinding expands the resource association explicitly stored on an action.
func (c ResourceCatalog) ResolveBinding(scope, key, operation string, selected []string, selections ...[]string) (ResourceGrant, error) {
	fields := selectedGrantFields(selections)
	result := ResourceGrant{UI: []ResourceUI{}, Endpoints: []ResourceEndpoint{}}
	if !strings.HasPrefix(key, scope+":") {
		return result, errors.New("cross-scope resource binding")
	}
	resource, ok := c.Resources[key]
	binding, supported := resource.Operations[operation]
	if !ok || !supported {
		return result, errors.New("unregistered resource operation")
	}
	if selected == nil {
		selected = binding.ButtonKeys
	}
	seen := map[string]bool{}
	buttons := []string{}
	endpoints := map[ResourceEndpoint]bool{}
	for _, featureKey := range selected {
		feature, exists := c.Buttons[featureKey]
		if !exists || !slices.Contains(binding.ButtonKeys, featureKey) || seen[featureKey] {
			return result, errors.New("feature outside resource operation")
		}
		seen[featureKey] = true
		if hasField(fields, "ui") {
			buttons = append(buttons, feature.Btn...)
		}
		if hasField(fields, "http") {
			for _, http := range feature.HTTP {
				endpoints[ResourceEndpoint{Protocol: "http", Method: http.Method, Path: http.Path}] = true
			}
		}
		if hasField(fields, "grpc") {
			for _, path := range feature.GRPC {
				endpoints[ResourceEndpoint{Protocol: "grpc", Method: "GRPC", Path: path}] = true
			}
		}
	}
	if hasField(fields, "ui") {
		entryKeys := make([]string, 0, len(resource.Entries))
		for entry := range resource.Entries {
			entryKeys = append(entryKeys, entry)
		}
		sort.Strings(entryKeys)
		for _, entryKey := range entryKeys {
			entry := resource.Entries[entryKey]
			result.UI = append(result.UI, ResourceUI{MenuKey: entryKey, ParentMenuKey: entry.ParentMenuKey, MenuPath: entry.Path, PageEntry: slices.Contains(binding.EntryKeys, entryKey), ButtonKeys: func() []string {
				keys := []string{}
				for _, button := range buttons {
					if strings.HasPrefix(button, strings.Replace(entryKey, ":menu:", ":btn:", 1)+":") {
						keys = append(keys, button)
					}
				}
				return keys
			}()})
		}
	}
	for endpoint := range endpoints {
		result.Endpoints = append(result.Endpoints, endpoint)
	}
	sort.Slice(result.Endpoints, func(i, j int) bool { return result.Endpoints[i].Display() < result.Endpoints[j].Display() })
	return result, nil
}

func (c Catalog) ResolveResourceBinding(scope, key, operation string, selected []string, selections ...[]string) (ResourceGrant, error) {
	return c.ResourceCatalog.ResolveBinding(scope, key, operation, selected, selections...)
}

func (m *Module) PermissionCatalog() ResourceCatalog {
	return m.Select([]string{"resource_catalog"}).ResourceCatalog
}

// ResolvePermission reads one immutable snapshot. Request authorization does not
// copy the multilingual dictionary or perform a separate catalog SQL query.
func (c Catalog) ResolvePermission(word string, selected []string, builtin string, resourceSelections ...[]string) (ResourceGrant, error) {
	return c.ResolveSelectedPermission(word, selected, builtin, nil, resourceSelections...)
}

func (c Catalog) ResolveSelectedPermission(word string, selected []string, builtin string, fields []string, resourceSelections ...[]string) (ResourceGrant, error) {
	result := ResourceGrant{UI: []ResourceUI{}, Endpoints: []ResourceEndpoint{}}
	parts := strings.Split(word, ":")
	if len(parts) < 4 {
		return result, errors.New("invalid permission word")
	}
	catalog := c.ResourceCatalog
	appendDefault := func() {
		grant := catalog.Defaults[parts[0]]
		if hasField(fields, "ui") {
			for _, ui := range grant.UI {
				ui.ButtonKeys = slices.Clone(ui.ButtonKeys)
				result.UI = append(result.UI, ui)
			}
		}
		for _, endpoint := range grant.Endpoints {
			if hasField(fields, endpoint.Protocol) {
				result.Endpoints = append(result.Endpoints, endpoint)
			}
		}
	}
	switch builtin {
	case "default":
		if len(resourceSelections) > 0 {
			return catalog.ResolveResources(parts[0], resourceSelections[0], selected, fields)
		}
		appendDefault()
		return result, nil
	case "all":
		appendDefault()
		keys := []string{}
		for key := range catalog.Resources {
			if strings.HasPrefix(key, parts[0]+":") {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			resource := catalog.Resources[key]
			for _, operation := range FixedOperations {
				if _, ok := resource.Operations[operation]; !ok {
					continue
				}
				grant, err := catalog.ResolveBinding(parts[0], key, operation, nil, fields)
				if err != nil {
					return result, err
				}
				result.UI = append(result.UI, grant.UI...)
				result.Endpoints = append(result.Endpoints, grant.Endpoints...)
			}
		}
		return result, nil
	case "":
		if word == parts[0]+":grant:system:permission-admin" {
			return result, nil
		}
		return result, errors.New("ordinary permissions require an explicit resource binding")
	default:
		return result, errors.New("unknown builtin permission")
	}
}

// ResolveResources applies the explicit resource selection stored by permission
// management. Registering a new resource never adds it to default permissions.
func (c ResourceCatalog) ResolveResources(scope string, keys, selected []string, selections ...[]string) (ResourceGrant, error) {
	result := ResourceGrant{UI: []ResourceUI{}, Endpoints: []ResourceEndpoint{}}
	seen, allowed := map[string]bool{}, map[string]bool{}
	for _, key := range keys {
		node, ok := c.Resources[key]
		binding, readable := node.Operations["read"]
		if !ok || node.Kind != "resource" || !strings.HasPrefix(key, scope+":") || !readable || seen[key] {
			return result, errors.New("invalid default resource selection")
		}
		seen[key] = true
		for _, feature := range binding.ButtonKeys {
			allowed[feature] = true
		}
	}
	seenFeatures := map[string]bool{}
	for _, feature := range selected {
		if !allowed[feature] || seenFeatures[feature] {
			return result, errors.New("invalid default feature selection")
		}
		seenFeatures[feature] = true
	}
	for _, key := range keys {
		var features []string
		if selected != nil {
			features = []string{}
			for _, feature := range c.Resources[key].Operations["read"].ButtonKeys {
				if seenFeatures[feature] {
					features = append(features, feature)
				}
			}
		}
		grant, err := c.ResolveBinding(scope, key, "read", features, selections...)
		if err != nil {
			return result, err
		}
		result.UI = append(result.UI, grant.UI...)
		result.Endpoints = append(result.Endpoints, grant.Endpoints...)
	}
	return result, nil
}

// PublicCatalog exposes language metadata without management transport mappings.
func (m *Module) PublicCatalog(fields ...string) PublicCatalog {
	selection := []string(fields)
	source := m.active.Load().catalog
	c := selectCatalog(source, selection)
	if hasField(selection, "messages") || hasField(selection, "frontend_messages") {
		// Resource names are projected only when returning frontend messages.
		c.ResourceCatalog = source.ResourceCatalog
		c.projectMenuNames()
	}
	return PublicCatalog{selectedFields: selection, SchemaVersion: c.SchemaVersion, DefaultLocale: c.DefaultLocale, Scopes: c.Scopes, GroupOrder: c.GroupOrder, Operations: c.Operations, Locales: c.Locales, Messages: c.Messages}
}

func (m *Module) ScopeMetadata() ([]string, []string) {
	c := m.active.Load().catalog
	return slices.Clone(c.Scopes), slices.Clone(c.Operations)
}

// PublicCatalog is the public language and label projection.
type PublicCatalog struct {
	selectedFields []string
	SchemaVersion  int                 `json:"schema_version"`
	DefaultLocale  string              `json:"default_locale"`
	Scopes         []string            `json:"scopes"`
	GroupOrder     []string            `json:"group_order"`
	Operations     []string            `json:"operations"`
	Locales        []Language          `json:"locales"`
	Messages       map[string]Messages `json:"messages"`
}

func (value PublicCatalog) MarshalJSON() ([]byte, error) {
	type plain PublicCatalog
	if value.selectedFields == nil {
		return json.Marshal(plain(value))
	}
	if value.GroupOrder == nil {
		value.GroupOrder = []string{}
	}
	if value.Scopes == nil {
		value.Scopes = []string{}
	}
	if value.Operations == nil {
		value.Operations = []string{}
	}
	if value.Locales == nil {
		value.Locales = []Language{}
	}
	if value.Messages == nil {
		value.Messages = map[string]Messages{}
	}
	result := map[string]any{"schema_version": value.SchemaVersion}
	for _, field := range value.selectedFields {
		switch field {
		case "default_locale":
			result[field] = value.DefaultLocale
		case "locales":
			result[field] = value.Locales
		case "scopes":
			result[field] = value.Scopes
		case "operations":
			result[field] = value.Operations
		case "group_order":
			result[field] = value.GroupOrder
		case "messages", "frontend_messages":
			result["messages"] = value.Messages
		}
	}
	return json.Marshal(result)
}

// Equal compares the persisted resource contract, normalizing omitted empty fields.
func (c ResourceCatalog) Equal(other ResourceCatalog) bool {
	left, _ := json.Marshal(c)
	right, _ := json.Marshal(other)
	return bytes.Equal(left, right)
}

func selectedGrantFields(selections [][]string) []string {
	if len(selections) == 0 {
		return nil
	}
	return selections[0]
}
