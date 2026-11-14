package action

import (
	"context"
	"errors"
	"fmt"
	"github.com/lib/pq"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

type ListActionGroupsResult struct {
	Items []string `json:"items"`
}

func (m *Module) ListGroups(ctx context.Context, keyword string) (*ListActionGroupsResult, error) {
	keyword = strings.TrimSpace(keyword)
	if utf8.RuneCountInString(keyword) > 50 {
		return nil, ErrInvalid
	}
	value := &ListActionGroupsResult{Items: make([]string, 0)}
	query := `SELECT MIN(action_group) FROM t_action WHERE BTRIM(action_group) <> ''`
	args := make([]any, 0, 1)
	if keyword != "" {
		args = append(args, like(keyword))
		clause := `LOWER(action_group) LIKE LOWER($1) ESCAPE '!'`
		if matched := matchingLabelWords(m.catalogSource.Current().Labels(i18n.Locale(ctx, ""), "groups"), keyword); len(matched) > 0 {
			args = append(args, pq.Array(matched))
			clause += " OR action_group = ANY($2)"
		}
		query += " AND (" + clause + ")"
	}
	query += ` GROUP BY LOWER(action_group) ORDER BY LOWER(action_group) LIMIT 20`
	rows, err := m.store.SQL(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list action groups: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return nil, fmt.Errorf("scan action group: %w", err)
		}
		value.Items = append(value.Items, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list action groups: %w", err)
	}
	return value, nil
}

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
