package dictionary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

const UIPreferenceClient = "vben_pc"
const UIPreferenceDictionaryKey = "UI_PREFERENCES_VBEN_PC"

// UIPreferences stores JSON values; numbers use encoding/json's float64 representation.
type UIPreferences map[string]any

// UIPreferenceDefaults always reads the authoritative dictionary without a value cache.
func (m *Module) UIPreferenceDefaults(ctx context.Context, client string) (UIPreferences, error) {
	if client != UIPreferenceClient {
		return nil, ErrInvalid
	}
	raw, err := m.requiredValue(ctx, UIPreferenceDictionaryKey)
	if err != nil {
		return nil, err
	}
	value, err := ParseUIPreferenceDefaults(raw)
	if err != nil {
		return nil, fmt.Errorf("dictionary %s: %w", UIPreferenceDictionaryKey, err)
	}
	return value, nil
}

// ParseUIPreferenceDefaults requires a complete value matching the current contract.
func ParseUIPreferenceDefaults(raw []byte) (UIPreferences, error) {
	var value UIPreferences
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if err := validateUIPreferences(value, true); err != nil {
		return nil, err
	}
	return value, nil
}

type preferenceKind uint8

const (
	preferenceBool preferenceKind = iota
	preferenceNumber
	preferenceString
	preferenceOrder
)

// uiPreferenceFields defines the contract, never the default values.
var uiPreferenceFields = map[string]map[string]preferenceKind{
	"app": {
		"dynamic_title":           preferenceBool,
		"layout":                  preferenceString,
		"color_gray_mode":         preferenceBool,
		"color_weak_mode":         preferenceBool,
		"content_compact":         preferenceString,
		"watermark":               preferenceBool,
		"watermark_content":       preferenceString,
		"enable_check_updates":    preferenceBool,
		"enable_copy_preferences": preferenceBool,
		"enable_sticky_preferences_navigation_bar": preferenceBool,
		"preferences_button_position":              preferenceString,
		"auth_page_layout":                         preferenceString,
	},
	"transition": {
		"progress": preferenceBool,
		"name":     preferenceString,
		"loading":  preferenceBool,
		"enable":   preferenceBool,
	},
	"theme": {
		"color_primary":         preferenceString,
		"builtin_type":          preferenceString,
		"mode":                  preferenceString,
		"radius":                preferenceString,
		"font_size":             preferenceNumber,
		"semi_dark_sidebar":     preferenceBool,
		"semi_dark_sidebar_sub": preferenceBool,
		"semi_dark_header":      preferenceBool,
	},
	"sidebar": {
		"enable":               preferenceBool,
		"width":                preferenceNumber,
		"draggable":            preferenceBool,
		"collapsed":            preferenceBool,
		"collapsed_show_title": preferenceBool,
		"auto_activate_child":  preferenceBool,
		"expand_on_hover":      preferenceBool,
		"collapsed_button":     preferenceBool,
		"fixed_button":         preferenceBool,
		"hidden":               preferenceBool,
		"extra_collapse":       preferenceBool,
	},
	"header": {
		"enable":     preferenceBool,
		"mode":       preferenceString,
		"menu_align": preferenceString,
	},
	"breadcrumb": {
		"enable":        preferenceBool,
		"show_icon":     preferenceBool,
		"show_home":     preferenceBool,
		"style_type":    preferenceString,
		"hide_only_one": preferenceBool,
	},
	"tabbar": {
		"enable":                preferenceBool,
		"show_icon":             preferenceBool,
		"show_more":             preferenceBool,
		"show_maximize":         preferenceBool,
		"persist":               preferenceBool,
		"visit_history":         preferenceBool,
		"draggable":             preferenceBool,
		"wheelable":             preferenceBool,
		"style_type":            preferenceString,
		"max_count":             preferenceNumber,
		"middle_click_to_close": preferenceBool,
	},
	"navigation": {
		"style_type": preferenceString,
		"split":      preferenceBool,
		"accordion":  preferenceBool,
	},
	"footer": {
		"enable": preferenceBool,
		"fixed":  preferenceBool,
	},
	"copyright": {
		"setting_show":      preferenceBool,
		"enable":            preferenceBool,
		"company_name":      preferenceString,
		"company_site_link": preferenceString,
		"date":              preferenceString,
		"icp":               preferenceString,
		"icp_link":          preferenceString,
	},
	"shortcut_keys": {
		"enable":             preferenceBool,
		"global_search":      preferenceBool,
		"global_logout":      preferenceBool,
		"global_escape":      preferenceBool,
		"global_lock_screen": preferenceBool,
	},
	"widget": {
		"global_search_button_position":   preferenceString,
		"fullscreen_button_position":      preferenceString,
		"language_toggle_button_position": preferenceString,
		"notification_button_position":    preferenceString,
		"theme_toggle_button_position":    preferenceString,
		"lock_screen_button_position":     preferenceString,
		"logout_button_position":          preferenceString,
		"order":                           preferenceOrder,
		"refresh_button_position":         preferenceString,
		"timezone_button_position":        preferenceString,
	},
	"custom": {
		"enable_form_fullscreen": preferenceBool,
		"default_table_size":     preferenceNumber,
		"report_title":           preferenceString,
	},
}

// Membership is a contract; the dictionary chooses the display order.
var preferenceWidgets = []string{"fullscreen", "globalSearch", "languageToggle", "lockScreenBtn", "logoutBtn", "notification", "preferences", "refresh", "themeToggle", "timezone"}

var preferenceEnums = map[string][]string{
	"theme.mode":                      {"auto", "dark", "light"},
	"theme.radius":                    {"0", "0.25", "0.5", "0.75", "1"},
	"theme.builtin_type":              {"custom", "deep-blue", "deep-green", "default", "gray", "green", "neutral", "orange", "pink", "red", "rose", "sky-blue", "slate", "stone", "violet", "yellow", "zinc"},
	"app.auth_page_layout":            {"panel-center", "panel-left", "panel-right"},
	"app.layout":                      {"full-content", "header-mixed-nav", "header-nav", "header-sidebar-nav", "mixed-nav", "sidebar-mixed-nav", "sidebar-nav"},
	"app.content_compact":             {"compact", "wide"},
	"app.preferences_button_position": {"auto", "fixed", "header", "none", "user-dropdown"},
	"header.mode":                     {"auto", "auto-scroll", "fixed", "static"},
	"header.menu_align":               {"center", "end", "start"},
	"breadcrumb.style_type":           {"background", "normal"},
	"navigation.style_type":           {"plain", "rounded"},
	"tabbar.style_type":               {"brisk", "card", "chrome", "plain"},
	"transition.name":                 {"fade", "fade-down", "fade-slide", "fade-up"},
}
var preferenceColorPattern = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|(?:rgb|rgba|hsl|hsla)\([0-9.,% /+\-]+\))$`)

func validPreferenceLeaf(path string, value any) bool {
	group, field, ok := strings.Cut(path, ".")
	kind, known := uiPreferenceFields[group][field]
	if !ok || !known {
		return false
	}
	if value == nil {
		return false
	}
	switch kind {
	case preferenceBool:
		_, ok := value.(bool)
		return ok
	case preferenceNumber:
		number, ok := value.(float64)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number {
			return false
		}
		switch path {
		case "theme.font_size":
			return number >= 15 && number <= 22
		case "sidebar.width":
			return number >= 160 && number <= 320
		case "tabbar.max_count":
			return number >= 0 && number <= 30
		case "custom.default_table_size":
			return number >= 10 && number <= 200 && int(number)%10 == 0
		}
		return false
	case preferenceString:
		text, ok := value.(string)
		if !ok {
			return false
		}
		if allowed, exists := preferenceEnums[path]; exists {
			return slices.Contains(allowed, text)
		}
		if strings.HasPrefix(path, "widget.") && strings.HasSuffix(path, "_position") {
			return slices.Contains([]string{"header", "none", "user-dropdown"}, text)
		}
		if path == "theme.color_primary" {
			return preferenceColorPattern.MatchString(text)
		}
		if path == "copyright.company_site_link" || path == "copyright.icp_link" {
			if text == "" || (strings.HasPrefix(text, "/") && !strings.HasPrefix(text, "//")) {
				return true
			}
			parsed, err := url.Parse(text)
			return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" && parsed.User == nil
		}
		return true
	case preferenceOrder:
		array, ok := value.([]any)
		allowed := preferenceWidgets
		if !ok || len(array) != len(allowed) {
			return false
		}
		seen := map[string]bool{}
		for _, item := range array {
			text, ok := item.(string)
			if !ok || seen[text] || !slices.Contains(allowed, text) {
				return false
			}
			seen[text] = true
		}
		return true
	}
	return false
}

// ValidateUIPreferencePatch rejects unknown fields and invalid values. Null resets a group or leaf.
func ValidateUIPreferencePatch(value UIPreferences) error {
	return validateUIPreferences(value, false)
}

func validateUIPreferences(value UIPreferences, complete bool) error {
	if value == nil {
		return errors.New("preferences must be an object")
	}
	if complete && len(value) != len(uiPreferenceFields) {
		return errors.New("incomplete preference defaults")
	}
	for group, item := range value {
		fields, known := uiPreferenceFields[group]
		if !known {
			return fmt.Errorf("unknown preference group %s", group)
		}
		if item == nil && !complete {
			continue
		}
		input, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid preference group %s", group)
		}
		if complete && len(input) != len(fields) {
			return fmt.Errorf("incomplete preference group %s", group)
		}
		for field, leaf := range input {
			path := group + "." + field
			if _, known := fields[field]; !known {
				return fmt.Errorf("unknown preference %s", path)
			}
			if leaf == nil && !complete {
				continue
			}
			if !validPreferenceLeaf(path, leaf) {
				return fmt.Errorf("invalid preference %s", path)
			}
		}
	}
	return nil
}
