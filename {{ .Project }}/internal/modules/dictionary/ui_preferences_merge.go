package dictionary

import (
	"encoding/json"
	"reflect"
)

// MergeUIPreferences treats arrays as values, preserves false/zero/empty strings and ignores obsolete or invalid stored fields.
func MergeUIPreferences(defaults, overrides UIPreferences) UIPreferences {
	return mergeUIPreferences(defaults, overrides, "")
}

func mergeUIPreferences(defaults, overrides UIPreferences, prefix string) UIPreferences {
	result := UIPreferences{}
	for key, fallback := range defaults {
		value, exists := overrides[key]
		if children, group := fallback.(map[string]any); group {
			overlay, _ := value.(map[string]any)
			result[key] = map[string]any(mergeUIPreferences(children, overlay, prefix+key+"."))
		} else if exists && validPreferenceLeaf(prefix+key, value) {
			result[key] = value
		} else {
			result[key] = fallback
		}
	}
	return result
}

// ApplyUIPreferences changes only submitted paths. Existing overrides keep their meaning when defaults change.
// Values equal to today's defaults remove the corresponding stored override.
func ApplyUIPreferences(defaults, overrides, patch UIPreferences) (UIPreferences, UIPreferences, error) {
	if patch != nil {
		if err := ValidateUIPreferencePatch(patch); err != nil {
			return nil, nil, err
		}
	}
	if patch == nil {
		return UIPreferences{}, MergeUIPreferences(defaults, nil), nil
	}
	raw, _ := json.Marshal(overrides)
	result := UIPreferences{}
	_ = json.Unmarshal(raw, &result)
	if result == nil {
		result = UIPreferences{}
	}
	applyUIPreferences(result, defaults, patch)
	return result, MergeUIPreferences(defaults, result), nil
}

func applyUIPreferences(target, defaults, patch UIPreferences) {
	for key, value := range patch {
		if value == nil {
			delete(target, key)
			continue
		}
		if children, group := value.(map[string]any); group {
			current, _ := target[key].(map[string]any)
			if current == nil {
				current = map[string]any{}
			}
			base, _ := defaults[key].(map[string]any)
			applyUIPreferences(current, base, children)
			if len(current) == 0 {
				delete(target, key)
			} else {
				target[key] = current
			}
		} else if reflect.DeepEqual(value, defaults[key]) {
			delete(target, key)
		} else {
			target[key] = value
		}
	}
}
