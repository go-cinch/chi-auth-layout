// Package localization localizes public responses from an atomically replaced dictionary catalog.
package localization

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"{{ .Computed.module_name_final }}/internal/common/i18n"

	"golang.org/x/text/language"
)

const English = "en-US"
const Chinese = "zh-CN"

type Language struct {
	Tag     string `json:"tag"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type Messages struct {
	Shared   map[string]any    `json:"shared,omitempty"`
	Backend  map[string]string `json:"backend,omitempty"`
	Frontend map[string]any    `json:"frontend,omitempty"`
}

// Catalog is the versioned I18N_CATALOG dictionary protocol.
type Catalog struct {
	SchemaVersion   int                 `json:"schema_version"`
	DefaultLocale   string              `json:"default_locale"`
	Scopes          []string            `json:"scopes,omitempty"`
	GroupOrder      []string            `json:"group_order,omitempty"`
	Operations      []string            `json:"operations"`
	ResourceCatalog ResourceCatalog     `json:"resource_catalog"`
	Locales         []Language          `json:"locales"`
	Messages        map[string]Messages `json:"messages"`
}

type snapshot struct {
	catalog Catalog
	matcher language.Matcher
	tags    []string
}

type Module struct {
	active             atomic.Pointer[snapshot]
	httpPaths          map[string]string
	canonicalHTTPPaths map[string]string
}

func New(mappings ...map[string]string) *Module {
	m := &Module{httpPaths: map[string]string{}, canonicalHTTPPaths: map[string]string{}}
	if len(mappings) > 0 {
		for canonical, actual := range mappings[0] {
			m.httpPaths[canonical] = actual
			m.canonicalHTTPPaths[actual] = canonical
		}
	}
	m.Reset()
	return m
}

// Parse validates language registration before a catalog can be committed.
func Parse(raw []byte) (*Catalog, error) {
	var value Catalog
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value.SchemaVersion != 2 || len(value.Locales) == 0 || len(value.Locales) > 128 {
		return nil, errors.New("invalid localization catalog version or languages")
	}
	seen := make(map[string]bool)
	enabledDefault := false
	for _, locale := range value.Locales {
		tag, err := language.Parse(locale.Tag)
		if err != nil || tag == language.Und || tag.String() != locale.Tag || strings.TrimSpace(locale.Name) == "" || seen[locale.Tag] {
			return nil, errors.New("invalid or duplicate locale")
		}
		seen[locale.Tag] = true
		if locale.Enabled {
			if _, ok := value.Messages[locale.Tag]; !ok {
				return nil, errors.New("enabled locale has no messages")
			}
			if locale.Tag == value.DefaultLocale {
				enabledDefault = true
			}
		}
	}
	if !enabledDefault {
		return nil, errors.New("default locale must be enabled")
	}
	for tag := range value.Messages {
		if !seen[tag] {
			return nil, errors.New("messages reference an unregistered locale")
		}
	}
	orderedGroups := make(map[string]bool)
	for group := range value.GroupKeys() {
		if len(group) > 128 || !catalogKey(group, 3, 128) {
			return nil, errors.New("invalid permission group identifier")
		}
	}
	for _, group := range value.GroupOrder {
		if len(group) > 128 || !catalogKey(group, 3, 128) || orderedGroups[group] {
			return nil, errors.New("invalid or duplicate permission group order")
		}
		orderedGroups[group] = true
	}
	if value.Operations == nil {
		value.Operations = slices.Clone(FixedOperations)
	}
	if !slices.Equal(value.Operations, FixedOperations) {
		return nil, errors.New("invalid fixed operations")
	}
	if value.ResourceCatalog.Resources != nil {
		if err := value.ResourceCatalog.Validate(value.Scopes, value.Operations); err != nil {
			return nil, err
		}
	}
	value.NormalizeResourceNames()
	return &value, nil
}

// GroupKeys returns registered business-group identifiers across all languages.
func (c Catalog) GroupKeys() map[string]bool {
	groups := make(map[string]bool)
	for _, messages := range c.Messages {
		labels, _ := messages.Frontend["groups"].(map[string]any)
		for key := range labels {
			groups[key] = true
		}
	}
	return groups
}

// Install copies the validated catalog so readers never observe partial updates.
func (m *Module) Install(raw []byte) error {
	raw, err := m.CanonicalCatalog(raw)
	if err != nil {
		return err
	}
	value, err := Parse(raw)
	if err != nil {
		return err
	}
	tags := []string{value.DefaultLocale}
	parsed := []language.Tag{language.MustParse(value.DefaultLocale)}
	for _, locale := range value.Locales {
		if locale.Enabled && locale.Tag != value.DefaultLocale {
			tags = append(tags, locale.Tag)
			parsed = append(parsed, language.MustParse(locale.Tag))
		}
	}
	m.active.Store(&snapshot{catalog: *value, matcher: language.NewMatcher(parsed), tags: tags})
	return nil
}

// Reset restores safe bootstrap messages when the dictionary is absent or disabled.
func (m *Module) Reset() {
	value := Catalog{SchemaVersion: 2, Scopes: []string{"admin", "h5", "mini", "app"}, Operations: slices.Clone(FixedOperations), DefaultLocale: English, Locales: []Language{
		{English, "English", true}, {Chinese, "简体中文", true},
	}, Messages: map[string]Messages{English: {Backend: map[string]string{}}, Chinese: {Backend: map[string]string{}}}}
	m.active.Store(&snapshot{catalog: value, matcher: language.NewMatcher([]language.Tag{language.AmericanEnglish, language.SimplifiedChinese}), tags: []string{English, Chinese}})
}

// Current returns a detached copy suitable for a public localization response.
func (m *Module) Current() Catalog { return m.Select(nil) }

// Select copies only requested data from one immutable snapshot.
func (m *Module) Select(fields []string) Catalog {
	value := selectCatalog(m.active.Load().catalog, fields)
	rewriteHTTPPaths(&value.ResourceCatalog, m.httpPaths)
	return value
}

func selectCatalog(source Catalog, fields []string) Catalog {
	value := Catalog{SchemaVersion: source.SchemaVersion}
	if hasField(fields, "default_locale") {
		value.DefaultLocale = source.DefaultLocale
	}
	if hasField(fields, "locales") {
		value.Locales = source.Locales
	}
	if hasField(fields, "scopes") {
		value.Scopes = source.Scopes
	}
	if hasField(fields, "operations") {
		value.Operations = source.Operations
	}
	if hasField(fields, "group_order") {
		value.GroupOrder = source.GroupOrder
	}
	if hasField(fields, "resource_catalog") {
		value.ResourceCatalog = source.ResourceCatalog
	}
	if hasField(fields, "messages") {
		value.Messages = source.Messages
	} else if hasField(fields, "frontend_messages") {
		value.Messages = make(map[string]Messages, len(source.Messages))
		for locale, messages := range source.Messages {
			value.Messages[locale] = Messages{Frontend: messages.Frontend}
		}
	}
	raw, _ := json.Marshal(value)
	var detached Catalog
	_ = json.Unmarshal(raw, &detached)
	return detached
}

func (m *Module) Match(accept string) string {
	current := m.active.Load()
	tags, _, err := language.ParseAcceptLanguage(accept)
	if err != nil {
		return current.catalog.DefaultLocale
	}
	_, index, _ := current.matcher.Match(tags...)
	return current.tags[index]
}

func (m *Module) Translate(locale, code, fallback string) string {
	current := m.active.Load()
	if message := current.catalog.Messages[locale].Backend[code]; message != "" {
		return message
	}
	if message := current.catalog.Messages[current.catalog.DefaultLocale].Backend[code]; message != "" {
		return message
	}
	return i18n.Translate(locale, code, fallback)
}

// Labels returns a detached namespace snapshot, using the configured default
// language for missing labels. A list operation can reuse this one snapshot for
// both its database filters and projected display names.
func (c Catalog) Labels(locale, namespace string) map[string]string {
	current := &snapshot{catalog: c}
	result := make(map[string]string)
	add := func(tag string) {
		values, _ := current.catalog.Messages[tag].Frontend[namespace].(map[string]any)
		for key, value := range values {
			if label, ok := value.(string); ok && label != "" {
				result[key] = label
			}
		}
	}
	add(current.catalog.DefaultLocale)
	if locale != current.catalog.DefaultLocale {
		add(locale)
	}
	if namespace == "menus" {
		resources := make(map[string]string)
		for _, tag := range []string{current.catalog.DefaultLocale, locale} {
			values, _ := current.catalog.Messages[tag].Frontend["resources"].(map[string]any)
			for key, value := range values {
				if label, ok := value.(string); ok && label != "" {
					resources[key] = label
				}
			}
		}
		for menu, key := range current.catalog.ResourceCatalog.MenuResourceKeys() {
			if resources[key] != "" {
				result[menu] = resources[key]
			}
		}
	}
	return result
}

func hasField(fields []string, name string) bool {
	return fields == nil || slices.Contains(fields, name)
}
