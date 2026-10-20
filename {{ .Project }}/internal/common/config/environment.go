package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/redact"
	"github.com/knadh/koanf/v2"
)

// applyEnvironment visits YAML leaves so list indices preserve their container
// shape. Koanf.Set on an indexed path would turn the list into a map.
func applyEnvironment(values *koanf.Koanf) ([]Override, error) {
	var overrides []Override
	for key, value := range values.Raw() {
		before := len(overrides)
		updated := overrideValue(value, key, &overrides)
		if len(overrides) != before {
			if err := values.Set(key, updated); err != nil {
				return nil, fmt.Errorf("set environment config %q: %w", key, err)
			}
		}
	}
	// Resolve all leaves first: redact.keys may itself be overridden, and map
	// traversal order must never decide whether another override is safe to log.
	policy := redact.New(values.Strings("redact.keys")...)
	for index := range overrides {
		override := &overrides[index]
		if policy.IsSensitive(override.Key) {
			override.Value = policy.Value(override.Key, override.Value)
		} else if policy.IsSensitive(override.Environment) {
			override.Value = policy.Value(override.Environment, override.Value)
		}
	}
	sort.Slice(overrides, func(i, j int) bool { return overrides[i].Environment < overrides[j].Environment })
	return overrides, nil
}

func overrideValue(value any, key string, overrides *[]Override) any {
	switch current := value.(type) {
	case map[string]any:
		for child, item := range current {
			current[child] = overrideValue(item, key+"."+child, overrides)
		}
		return current
	case []any:
		for index, item := range current {
			current[index] = overrideValue(item, key+"."+strconv.Itoa(index), overrides)
		}
		return current
	default:
		name := envPrefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		raw, exists := os.LookupEnv(name)
		if !exists {
			return value
		}
		*overrides = append(*overrides, Override{Environment: name, Key: key, Value: raw})
		return raw
	}
}
