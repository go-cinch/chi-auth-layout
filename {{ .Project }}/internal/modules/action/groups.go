package action

import (
	"context"
	"errors"
	"github.com/lib/pq"
	"sort"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

// Read the committed catalog under the shared permission mutation lock, rather
// than a process snapshot that may still contain a renamed group.
func (m *Module) requireRegisteredDefinition(ctx context.Context, word, group string, config Config) (localization.Catalog, error) {
	if m.definitions == nil {
		return localization.Catalog{}, errors.New("action definition catalog is unavailable")
	}
	catalog, err := m.definitions.ActionCatalog(ctx)
	if err != nil {
		return localization.Catalog{}, err
	}
	if !catalog.GroupKeys()[group] {
		return localization.Catalog{}, ErrInvalid
	}
	scope := strings.Split(word, ":")[0]
	if bindings, explicit := configBindings(config); explicit {
		for _, binding := range bindings {
			if _, err := catalog.ResourceCatalog.ResolveBinding(scope, binding.ResourceKey, binding.Operation, binding.FeatureKeys); err != nil {
				return localization.Catalog{}, ErrInvalid
			}
		}
	}
	if config.Builtin == "default" && config.ResourceKeys != nil {
		if _, err := catalog.ResourceCatalog.ResolveResources(scope, *config.ResourceKeys, config.FeatureKeys); err != nil {
			return localization.Catalog{}, ErrInvalid
		}
	}
	return catalog, nil
}

// ReconcilePermissionGroups runs under the caller's permission mutation lock.
// Labels, references and group membership commit or roll back together.
func (m *Module) ReconcilePermissionGroups(ctx context.Context, moves map[string]string, removed []string) (bool, error) {
	keys := make([]string, 0, len(moves))
	for key := range moves {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, from := range keys {
		if _, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_action SET action_group = $2, updated_at = $3 WHERE action_group = $1`, from, moves[from], time.Now().UnixMilli()); err != nil {
			return false, err
		}
	}
	if len(removed) == 0 {
		return true, nil
	}
	var used bool
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM t_action WHERE action_group = ANY($1))`, pq.Array(removed)).Scan(&used)
	return !used, err
}

// DefinitionCatalog reads the authoritative dictionary in the caller's transaction.
type DefinitionCatalog interface {
	ActionCatalog(context.Context) (localization.Catalog, error)
}

func (m *Module) SetDefinitionCatalog(catalog DefinitionCatalog) { m.definitions = catalog }
