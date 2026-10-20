package action

import (
	"context"
	"encoding/json"
	"reflect"
	"{{ .Computed.module_name_final }}/internal/modules/localization"

	"strings"
	"time"
)

// Runs under the catalog row lock and permission-mutation lock in Update's
// transaction. Assignment codes/IDs, UI bindings and transport paths stay stable.
func (m *Module) ReconcileResourcePermissions(ctx context.Context, before, after json.RawMessage, resources, featureRenames map[string]string) (bool, error) {
	catalog, err := localization.Parse(after)
	if err != nil {
		return false, nil
	}
	rc := catalog.ResourceCatalog
	var previous localization.Catalog
	if len(before) > 0 {
		if err := json.Unmarshal(before, &previous); err != nil {
			return false, nil
		}
	}
	if !validCatalogRenames(resources, previous.ResourceCatalog.Resources, rc.Resources) || !validCatalogRenames(featureRenames, previous.ResourceCatalog.Buttons, rc.Buttons) {
		return false, nil
	}
	if len(resources) == 0 && len(featureRenames) == 0 && previous.ResourceCatalog.Equal(rc) {
		return true, nil
	}
	type change struct {
		id          int64
		word, group string
		config      []byte
	}
	changes := []change{}
	rows, err := m.store.SQL(ctx).QueryContext(ctx, `SELECT id, word, action_group, config FROM t_action ORDER BY id FOR UPDATE`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var current change
		if err := rows.Scan(&current.id, &current.word, &current.group, &current.config); err != nil {
			return false, err
		}
		converted, changed, err := MigrateConvertedResourceConfig(previous.ResourceCatalog, rc, current.config, resources, featureRenames)
		if err != nil {
			return false, err
		}
		current.config = converted
		var config map[string]json.RawMessage
		if err := json.Unmarshal(current.config, &config); err != nil {
			return false, err
		}
		if raw, exists := config["bindings"]; exists {
			var bindings []map[string]json.RawMessage
			if err := json.Unmarshal(raw, &bindings); err != nil {
				return false, err
			}
			bindingsChanged := false
			for _, binding := range bindings {
				var key string
				if err := json.Unmarshal(binding["resource_key"], &key); err != nil {
					return false, err
				}
				target := migratedKey(resources, key)
				if target != key {
					binding["resource_key"], err = json.Marshal(target)
					if err != nil {
						return false, err
					}
					bindingsChanged = true
				}
				var features []string
				if rawFeatures, exists := binding["feature_keys"]; exists {
					if err := json.Unmarshal(rawFeatures, &features); err != nil {
						return false, err
					}
					featuresChanged := false
					for i, feature := range features {
						canonical := migratedKey(featureRenames, feature)
						if canonical != feature {
							features[i] = canonical
							featuresChanged = true
						}
					}
					if featuresChanged {
						binding["feature_keys"], err = json.Marshal(features)
						if err != nil {
							return false, err
						}
						bindingsChanged = true
					}
				}
			}
			if bindingsChanged {
				config["bindings"], err = json.Marshal(bindings)
				if err != nil {
					return false, err
				}
				changed = true
			}
		}
		var resourceKey string
		if raw, exists := config["resource_key"]; exists {
			if err := json.Unmarshal(raw, &resourceKey); err != nil {
				return false, err
			}
			if resourceKey != "" {
				target := migratedKey(resources, resourceKey)
				if target != resourceKey {
					config["resource_key"], err = json.Marshal(target)
					if err != nil {
						return false, err
					}
					changed = true
				}
			}
		}
		for _, field := range []string{"resource_keys", "feature_keys"} {
			raw, exists := config[field]
			if !exists {
				continue
			}
			var keys []string
			if err := json.Unmarshal(raw, &keys); err != nil {
				return false, err
			}
			fieldChanged := false
			for i, key := range keys {
				target := migratedKey(resources, key)
				if field == "feature_keys" {
					target = migratedKey(featureRenames, key)
				}
				if target != key {
					keys[i] = target
					fieldChanged = true
				}
			}
			if fieldChanged {
				encoded, err := json.Marshal(keys)
				if err != nil {
					return false, err
				}
				config[field] = encoded
				changed = true
			}
		}
		current.config, err = json.Marshal(config)
		if err != nil {
			return false, err
		}
		if !validStoredResourceSelection(rc, current.word, current.config) {
			return false, nil
		}
		// Transport and UI edits also invalidate the projection when identifiers stay unchanged.
		var stored Config
		if err := json.Unmarshal(current.config, &stored); err != nil {
			return false, err
		}
		projected := stored
		if err := resolveStoredConfig(*catalog, current.word, &projected); err != nil {
			return false, nil
		}
		if !reflect.DeepEqual(stored.UI, projected.UI) || !reflect.DeepEqual(stored.Endpoints, projected.Endpoints) || stored.PermissionScope != projected.PermissionScope || !reflect.DeepEqual(stored.PermissionScopes, projected.PermissionScopes) {
			if len(projected.PermissionScopes) == 0 {
				delete(config, "permission_scopes")
			} else {
				config["permission_scopes"], err = json.Marshal(projected.PermissionScopes)
				if err != nil {
					return false, err
				}
			}
			if projected.PermissionScope == "" {
				delete(config, "permission_scope")
			} else {
				config["permission_scope"], err = json.Marshal(projected.PermissionScope)
				if err != nil {
					return false, err
				}
			}
			config["ui"], err = json.Marshal(projected.UI)
			if err != nil {
				return false, err
			}
			config["endpoints"], err = json.Marshal(projected.Endpoints)
			if err != nil {
				return false, err
			}
			current.config, err = json.Marshal(config)
			if err != nil {
				return false, err
			}
			changed = true
		}
		if changed {
			changes = append(changes, current)
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	for _, current := range changes {
		if _, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_action SET word = $1, action_group = $2, config = $3::jsonb, updated_at = $4 WHERE id = $5`, current.word, current.group, string(current.config), time.Now().UnixMilli(), current.id); err != nil {
			return false, mapWriteError(err)
		}
	}
	return true, nil
}

func migratedKey(renames map[string]string, key string) string {
	if target := renames[key]; target != "" {
		return target
	}
	return key
}

func validCatalogRenames[T any](renames map[string]string, previous, current map[string]T) bool {
	for from, to := range renames {
		_, existed := previous[from]
		_, oldStillExists := current[from]
		_, targetExists := current[to]
		if !existed || oldStillExists || !targetExists || from == to || strings.Split(from, ":")[0] != strings.Split(to, ":")[0] {
			return false
		}
	}
	return true
}

// Validate the final references before committing a move or removing a resource.
// IDs, assignment codes and permission words remain unchanged.
func validStoredResourceSelection(catalog localization.ResourceCatalog, word string, raw json.RawMessage) bool {
	type binding struct {
		ResourceKey string   `json:"resource_key"`
		Operation   string   `json:"operation"`
		FeatureKeys []string `json:"feature_keys"`
	}
	var config struct {
		Bindings     *[]binding `json:"bindings"`
		ResourceKey  string     `json:"resource_key"`
		Operation    string     `json:"operation"`
		Builtin      string     `json:"builtin"`
		FeatureKeys  []string   `json:"feature_keys"`
		ResourceKeys *[]string  `json:"resource_keys"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return false
	}
	scope := strings.Split(word, ":")[0]
	if config.Bindings != nil {
		for _, binding := range *config.Bindings {
			if _, err := catalog.ResolveBinding(scope, binding.ResourceKey, binding.Operation, binding.FeatureKeys); err != nil {
				return false
			}
		}
	} else if config.ResourceKey != "" || config.Operation != "" {
		if _, err := catalog.ResolveBinding(scope, config.ResourceKey, config.Operation, config.FeatureKeys); err != nil {
			return false
		}
	}
	if config.Builtin == "default" && config.ResourceKeys != nil {
		if _, err := catalog.ResolveResources(scope, *config.ResourceKeys, config.FeatureKeys); err != nil {
			return false
		}
	}
	return true
}
