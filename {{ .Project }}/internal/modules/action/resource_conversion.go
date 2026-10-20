package action

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"

	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

// MigrateConvertedResourceConfig replaces bindings to resources that became
// directories with an explicit snapshot of their new children and references.
// Catalog keys and action words have no special meaning in this conversion.
func MigrateConvertedResourceConfig(before, after localization.ResourceCatalog, raw json.RawMessage, resourceRenames, featureRenames map[string]string) (json.RawMessage, bool, error) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, false, err
	}
	converted := func(key string) bool {
		return before.Resources[key].Kind == "resource" && after.Resources[migratedKey(resourceRenames, key)].Kind == "directory"
	}
	changed := false
	var bindings []ResourceBinding
	_, multiple := config["bindings"]
	if multiple {
		if err := json.Unmarshal(config["bindings"], &bindings); err != nil {
			return nil, false, err
		}
	} else if _, legacy := config["resource_key"]; legacy {
		var binding ResourceBinding
		if err := json.Unmarshal(raw, &binding); err != nil {
			return nil, false, err
		}
		bindings = []ResourceBinding{binding}
	}
	for _, binding := range bindings {
		changed = changed || converted(binding.ResourceKey)
	}
	if changed {
		migrated := []ResourceBinding{}
		for _, binding := range bindings {
			if converted(binding.ResourceKey) {
				migrated = append(migrated, directoryResourceBindings(after, migratedKey(resourceRenames, binding.ResourceKey), binding.Operation)...)
				continue
			}
			binding.ResourceKey = migratedKey(resourceRenames, binding.ResourceKey)
			for i, feature := range binding.FeatureKeys {
				binding.FeatureKeys[i] = migratedKey(featureRenames, feature)
			}
			migrated = append(migrated, binding)
		}
		encoded, err := json.Marshal(mergeResourceBindings(migrated))
		if err != nil {
			return nil, false, err
		}
		config["bindings"] = encoded
		if !multiple {
			delete(config, "resource_key")
			delete(config, "operation")
			delete(config, "feature_keys")
		}
	}
	var builtin string
	if value, exists := config["builtin"]; exists {
		if err := json.Unmarshal(value, &builtin); err != nil {
			return nil, false, err
		}
	}
	if value, exists := config["resource_keys"]; exists && builtin == "default" {
		var keys, selected []string
		if err := json.Unmarshal(value, &keys); err != nil {
			return nil, false, err
		}
		if value, exists := config["feature_keys"]; exists {
			if err := json.Unmarshal(value, &selected); err != nil {
				return nil, false, err
			}
		}
		if slices.ContainsFunc(keys, converted) {
			migrated := []ResourceBinding{}
			for _, key := range keys {
				if converted(key) {
					migrated = append(migrated, directoryResourceBindings(after, migratedKey(resourceRenames, key), "read")...)
					continue
				}
				target := migratedKey(resourceRenames, key)
				features := []string{}
				for _, feature := range after.Resources[target].Operations["read"].ButtonKeys {
					if selected == nil || slices.ContainsFunc(selected, func(old string) bool { return migratedKey(featureRenames, old) == feature }) {
						features = append(features, feature)
					}
				}
				migrated = append(migrated, ResourceBinding{ResourceKey: target, Operation: "read", FeatureKeys: features})
			}
			keys, selected = []string{}, []string{}
			for _, binding := range mergeResourceBindings(migrated) {
				keys = append(keys, binding.ResourceKey)
				selected = append(selected, binding.FeatureKeys...)
			}
			config["resource_keys"], _ = json.Marshal(keys)
			config["feature_keys"], _ = json.Marshal(selected)
			changed = true
		}
	}
	if !changed {
		return raw, false, nil
	}
	result, err := json.Marshal(config)
	return result, true, err
}

func directoryResourceBindings(catalog localization.ResourceCatalog, owner, operation string) []ResourceBinding {
	result := []ResourceBinding{}
	allowed := []string{operation}
	if operation == "*" {
		allowed = localization.FixedOperations
	}
	scope := strings.Split(owner, ":")[0]
	path := map[string]bool{}
	var visit func(string, []string)
	visit = func(key string, operations []string) {
		node, exists := catalog.Resources[key]
		if !exists || path[key] || !strings.HasPrefix(key, scope+":") || len(operations) == 0 {
			return
		}
		if node.Kind == "resource" {
			for _, operation := range operations {
				if binding, exists := node.Operations[operation]; exists {
					result = append(result, ResourceBinding{ResourceKey: key, Operation: operation, FeatureKeys: append([]string{}, binding.ButtonKeys...)})
				}
			}
			return
		}
		path[key] = true
		defer delete(path, key)
		children := []string{}
		for child, candidate := range catalog.Resources {
			if candidate.Parent == key {
				children = append(children, child)
			}
		}
		sort.Slice(children, func(i, j int) bool {
			a, b := catalog.Resources[children[i]], catalog.Resources[children[j]]
			return a.Sort < b.Sort || a.Sort == b.Sort && children[i] < children[j]
		})
		for _, child := range children {
			visit(child, operations)
		}
		for _, target := range node.References {
			selected, explicit := node.ReferenceOperations[target]
			intersection := []string{}
			for _, operation := range operations {
				if !explicit || slices.Contains(selected, operation) {
					intersection = append(intersection, operation)
				}
			}
			visit(target, intersection)
		}
	}
	visit(owner, allowed)
	return mergeResourceBindings(result)
}

func mergeResourceBindings(bindings []ResourceBinding) []ResourceBinding {
	result := []ResourceBinding{}
	positions := map[string]int{}
	for _, binding := range bindings {
		key := binding.ResourceKey + ":" + binding.Operation
		if index, exists := positions[key]; exists {
			previous := &result[index]
			if previous.FeatureKeys == nil || binding.FeatureKeys == nil {
				previous.FeatureKeys = nil
			} else {
				for _, feature := range binding.FeatureKeys {
					if !slices.Contains(previous.FeatureKeys, feature) {
						previous.FeatureKeys = append(previous.FeatureKeys, feature)
					}
				}
			}
		} else {
			positions[key] = len(result)
			result = append(result, binding)
		}
	}
	return result
}
