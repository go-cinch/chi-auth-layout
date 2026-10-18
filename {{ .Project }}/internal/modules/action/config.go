package action

import (
 "{{ .Computed.module_name_final }}/internal/modules/localization"
 "{{ .Computed.module_name_final }}/internal/common/report"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

)

// Config stores selected logical features and generated display projections.
// FeatureKeys nil selects the registered set; an explicit empty array selects none.
type Config struct {
	Bindings *[]ResourceBinding `json:"bindings,omitempty"`
	ResourceKey string `json:"resource_key,omitempty"`
	Operation string `json:"operation,omitempty"`
	Builtin     string      `json:"builtin,omitempty"`
	FeatureKeys []string    `json:"feature_keys" nullable:"true"`
	ResourceKeys *[]string `json:"resource_keys,omitempty"`
	UI          []UIBinding `json:"ui"`
	Endpoints   []Endpoint  `json:"endpoints"`
}

type ResourceBinding struct {
	ResourceKey string `json:"resource_key"`
	Operation string `json:"operation"`
	FeatureKeys []string `json:"feature_keys" nullable:"true"`
}

func configBindings(config Config) ([]ResourceBinding, bool) {
	if config.Bindings != nil { return *config.Bindings, true }
	if config.ResourceKey != "" || config.Operation != "" {
		return []ResourceBinding{ {ResourceKey: config.ResourceKey, Operation: config.Operation, FeatureKeys: config.FeatureKeys}}, true
	}
	return nil, false
}

func validResourceBinding(word string, binding ResourceBinding) bool {
	if !validIdentity(word, binding.ResourceKey) { return false }
	switch binding.Operation {
	case "read", "create", "update", "delete": return true
	default: return false
	}
}

type UIBinding = localization.ResourceUI

type Endpoint = localization.ResourceEndpoint

func (c *Config) Scan(source any) error {
	var encoded []byte
	switch value := source.(type) {
	case []byte:
		encoded = value
	case string:
		encoded = []byte(value)
	default:
		return fmt.Errorf("invalid action config: %T", source)
	}
	return json.Unmarshal(encoded, c)
}

func (c Config) Value() (driver.Value, error) {
	encoded, err := json.Marshal(c)
	return string(encoded), err
}

var segmentPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func ValidScope(scope string) bool { return len(scope) <= 128 && segmentPattern.MatchString(scope) }

func ValidWord(word string) bool {
	parts := strings.Split(word, ":")
	if len(parts) < 4 || len(word) > 128 {
		return false
	}
	for _, part := range parts {
		if !segmentPattern.MatchString(part) {
			return false
		}
	}
	return true
}

func GroupForWord(word string) string {
	parts := strings.Split(word, ":")
	if len(parts) < 4 {
		return ""
	}
	return parts[0] + ":" + strings.Join(parts[2:], ":")
}

func GroupForConfig(word string, config Config) string {
	if config.Bindings != nil && len(*config.Bindings) > 0 { return (*config.Bindings)[0].ResourceKey }
	if config.ResourceKey != "" { return config.ResourceKey }
	parts := strings.Split(word, ":")
	if config.Builtin != "" || (len(parts) >= 4 && word == parts[0]+":grant:system:permission-admin") {
		return GroupForWord(word)
	}
	return ""
}

func validIdentity(word, group string) bool {
	if !ValidWord(word) || len(group) > 128 {
		return false
	}
	parts := strings.Split(group, ":")
	if len(parts) < 3 || parts[0] != strings.Split(word, ":")[0] {
		return false
	}
	for _, part := range parts {
		if !segmentPattern.MatchString(part) {
			return false
		}
	}
	return true
}

func validUIKey(key, scope, kind string) bool {
	parts := strings.Split(key, ":")
	if len(parts) < 3 || parts[0] != scope || parts[1] != kind {
		return false
	}
	if kind == "btn" && len(parts) < 5 {
		return false
	}
	for _, part := range parts {
		if !segmentPattern.MatchString(part) {
			return false
		}
	}
	return true
}

func (c *Config) Validate(word string) error {
	if !ValidWord(word) || c.UI == nil || c.Endpoints == nil {
		return ErrInvalid
	}
	parts := strings.Split(word, ":")
	scope := parts[0]
	bindings, explicit := configBindings(*c)
	special := c.Builtin != "" || word == scope+":grant:system:permission-admin"
	if special == explicit || (c.Bindings != nil && (c.ResourceKey != "" || c.Operation != "")) { return ErrInvalid }
	resources := map[string]bool{}
	for _, binding := range bindings {
		identity := binding.ResourceKey + ":" + binding.Operation
		if !validResourceBinding(word, binding) || resources[identity] { return ErrInvalid }
		resources[identity] = true
	}
	if c.ResourceKeys != nil && c.Builtin != "default" {
		return ErrInvalid
	}
	if c.Builtin != "" && c.Builtin != "all" && c.Builtin != "default" {
		return ErrInvalid
	}
	if (c.Builtin != "" || parts[1] == "all" || parts[1] == "default") && parts[1] != c.Builtin {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, ui := range c.UI {
		if !validUIKey(ui.MenuKey, scope, "menu") || !validPath(ui.MenuPath, false) || seen[ui.MenuKey] {
			return ErrInvalid
		}
		seen[ui.MenuKey] = true
		if ui.ParentMenuKey != "" && (!validUIKey(ui.ParentMenuKey, scope, "menu") || ui.ParentMenuKey == ui.MenuKey) {
			return ErrInvalid
		}
		buttons := map[string]bool{}
		for _, key := range ui.ButtonKeys {
			if !validUIKey(key, scope, "btn") || buttons[key] {
				return ErrInvalid
			}
			buttons[key] = true
		}
	}
	for _, endpoint := range c.Endpoints {
		if !validPath(endpoint.Path, true) {
			return ErrInvalid
		}
		switch endpoint.Protocol {
		case "http":
			switch endpoint.Method {
			case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
			default:
				return ErrInvalid
			}
			{{ if .Computed.enable_grpc_final }}
		case "grpc":
			if endpoint.Method != "GRPC" || strings.ContainsAny(endpoint.Path, "{}:") {
				return ErrInvalid
			}
			{{ end }}
		default:
			return ErrInvalid
		}
	}
	return nil
}

func validPath(path string, parameters bool) bool {
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "*?#\\\n\r\t ") || strings.Contains(path, "//") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." {
			return false
		}
		if strings.ContainsAny(part, "{}:") {
			if !parameters || len(part) < 3 || part[0] != '{' || part[len(part)-1] != '}' || !segmentPattern.MatchString(part[1:len(part)-1]) {
				return false
			}
		}
	}
	return true
}

// ResolveConfig uses the current dictionary snapshot for both writes and authorization.
func ResolveConfig(catalog localization.Catalog, word string, config Config, selections ...report.Fields) (Config, error) {
 var fields report.Fields
 if len(selections) > 0 { fields = selections[0] }
	if !ValidWord(word) { return Config{}, ErrInvalid }
	parts := strings.Split(word, ":")
	bindings, explicit := configBindings(config)
	grant := localization.ResourceGrant{UI: []UIBinding{}, Endpoints: []Endpoint{}}
	var err error
	var normalized []ResourceBinding
	if explicit {
		if config.Builtin != "" || word == parts[0]+":grant:system:permission-admin" || (config.Bindings != nil && (config.ResourceKey != "" || config.Operation != "")) { return Config{}, ErrInvalid }
		normalized = []ResourceBinding{}
		seen := map[string]bool{}
		for _, binding := range bindings {
			identity := binding.ResourceKey + ":" + binding.Operation
			if !validResourceBinding(word, binding) || seen[identity] { return Config{}, ErrInvalid }
			seen[identity] = true
			resolved, resolveErr := catalog.ResolveResourceBinding(parts[0], binding.ResourceKey, binding.Operation, binding.FeatureKeys, fields)
			if resolveErr != nil { return Config{}, ErrInvalid }
			grant.UI = append(grant.UI, resolved.UI...)
			grant.Endpoints = append(grant.Endpoints, resolved.Endpoints...)
			normalized = append(normalized, binding)
		}
	} else {
		if config.Builtin == "" && word != parts[0]+":grant:system:permission-admin" { return Config{}, ErrInvalid }
		var selections [][]string
		if config.ResourceKeys != nil { selections = append(selections, *config.ResourceKeys) }
		grant, err = catalog.ResolveSelectedPermission(word, config.FeatureKeys, config.Builtin, fields, selections...)
	}
	if err != nil {
		return Config{}, ErrInvalid
	}
	result := Config{Builtin: config.Builtin, FeatureKeys: config.FeatureKeys, ResourceKeys: config.ResourceKeys, UI: grant.UI, Endpoints: grant.Endpoints}
	if explicit { result.Bindings = &normalized; result.FeatureKeys = nil }
	// A resource can grant the same entry through several independent operations.
	ui := []UIBinding{}
	indices := map[string]int{}
	for _, entry := range result.UI {
		if index, ok := indices[entry.MenuKey]; ok {
			ui[index].PageEntry = ui[index].PageEntry || entry.PageEntry
			for _, button := range entry.ButtonKeys {
				found := false
				for _, old := range ui[index].ButtonKeys {
					found = found || old == button
				}
				if !found {
					ui[index].ButtonKeys = append(ui[index].ButtonKeys, button)
				}
			}
		} else {
			indices[entry.MenuKey] = len(ui)
			ui = append(ui, entry)
		}
	}
	result.UI = ui
	seen := map[Endpoint]bool{}
	endpoints := []Endpoint{}
	for _, endpoint := range result.Endpoints {
		if !seen[endpoint] {
			seen[endpoint] = true
			endpoints = append(endpoints, endpoint)
		}
	}
	result.Endpoints = endpoints
	return result, nil
}

// Stored built-in all remains a marker; its expansion is read-only.
func resolveStoredConfig(catalog localization.Catalog, word string, config *Config) error {
 if config.Builtin == "all" {
  config.UI, config.Endpoints = []UIBinding{}, []Endpoint{}
  return config.Validate(word)
 }
 resolved, err := ResolveConfig(catalog, word, *config)
	if err != nil {
		return err
	}
	*config = resolved
	return config.Validate(word)
}
