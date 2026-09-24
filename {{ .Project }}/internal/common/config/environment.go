package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/knadh/koanf/v2"
	"{{ .Computed.module_name_final }}/internal/common/redact"
)

// applyEnvironment visits YAML leaves so list indices preserve their container
// shape. Koanf.Set on an indexed path would turn the list into a map.
func applyEnvironment(values *koanf.Koanf) ([]Override, error) {
	policy := redact.New(values.Strings("redact.keys")...)
	var overrides []Override
	for key, value := range values.Raw() {
		before := len(overrides)
		updated := overrideValue(value, key, policy, &overrides)
		if len(overrides) != before {
			if err := values.Set(key, updated); err != nil {
				return nil, fmt.Errorf("set environment config %q: %w", key, err)
			}
		}
	}
	sort.Slice(overrides, func(i, j int) bool { return overrides[i].Environment < overrides[j].Environment })
	return overrides, nil
}

func overrideValue(value any, key string, policy redact.Policy, overrides *[]Override) any {
	switch current := value.(type) {
	case map[string]any:
		for child, item := range current {
			current[child] = overrideValue(item, key+"."+child, policy, overrides)
		}
		return current
	case []any:
		for index, item := range current {
			current[index] = overrideValue(item, key+"."+strconv.Itoa(index), policy, overrides)
		}
		return current
	default:
		name := envPrefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
		raw, exists := os.LookupEnv(name)
		if !exists {
			return value
		}
		logged := raw
		if policy.IsSensitive(key) {
			logged = policy.Value(key, raw)
		} else if policy.IsSensitive(name) {
			logged = policy.Value(name, raw)
		}
		*overrides = append(*overrides, Override{Environment: name, Key: key, Value: logged})
		return raw
	}
}
