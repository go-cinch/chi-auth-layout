package dictionary

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

// The action module holds the permission mutation lock and owns the transaction.
// Preserve every unrelated catalog field, including extensions unknown to this service.
func (m *Module) editGroupDocument(ctx context.Context, renames map[string]string, change func(*localization.Catalog, map[string]any) error) (int64, error) {
	var id int64
	var raw []byte
	executor := m.store.SQL(ctx)
	if err := executor.QueryRowContext(ctx, "SELECT id, value FROM t_dictionary WHERE dictionary_key = $1 AND enabled FOR UPDATE", LocalizationKey).Scan(&id, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, action.ErrGroupConflict
		}
		return 0, err
	}
	catalog, err := localization.Parse(raw)
	if err != nil {
		return 0, err
	}
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return 0, err
	}
	if err := change(catalog, document); err != nil {
		return 0, err
	}
	next, err := json.Marshal(document)
	if err != nil {
		return 0, err
	}
	if _, err := localization.Parse(next); err != nil {
		return 0, action.ErrInvalid
	}
	value := &Dictionary{ID: id, Key: LocalizationKey, Value: next}
	if err := m.reconcilePermissionGroups(ctx, raw, value, renames); err != nil {
		if errors.Is(err, ErrGroupNotEmpty) {
			return 0, action.ErrGroupNotEmpty
		}
		return 0, err
	}
	_, err = executor.ExecContext(ctx, "UPDATE t_dictionary SET value = $2::jsonb, updated_at = $3 WHERE id = $1", id, string(next), time.Now().UnixMilli())
	return id, err
}

func (m *Module) RefreshActionGroups(ctx context.Context, id int64) {
	m.invalidate(ctx, "update_action_groups", id)
}

func (m *Module) WriteActionGroup(ctx context.Context, change action.GroupChange) (int64, error) {
	source := change.Word
	var renames map[string]string
	if change.Operation == "update" {
		source = change.OriginalWord
		if source != change.Word {
			renames = map[string]string{source: change.Word}
		}
	}
	return m.editGroupDocument(ctx, renames, func(catalog *localization.Catalog, document map[string]any) error {
		keys := catalog.GroupKeys()
		if !slices.Contains(catalog.Scopes, strings.SplitN(change.Word, ":", 2)[0]) {
			return action.ErrInvalid
		}
		if change.Operation == "create" && keys[change.Word] || change.Operation != "create" && !keys[source] || source != change.Word && keys[change.Word] {
			return action.ErrGroupConflict
		}
		current := make(map[string]string, len(catalog.Locales))
		for _, locale := range catalog.Locales {
			labels, _ := catalog.Messages[locale.Tag].Frontend["groups"].(map[string]any)
			current[locale.Tag], _ = labels[source].(string)
		}
		if change.Operation != "create" && !maps.Equal(change.ExpectedNames, current) {
			return action.ErrGroupConflict
		}
		for tag, name := range change.Names {
			if _, registered := current[tag]; !registered {
				return action.ErrInvalid
			}
			current[tag] = strings.TrimSpace(name)
		}
		if change.Operation != "delete" && current[catalog.DefaultLocale] == "" {
			return action.ErrInvalid
		}
		messages := object(document, "messages")
		for tag, name := range current {
			labels := object(object(object(messages, tag), "frontend"), "groups")
			delete(labels, source)
			if change.Operation != "delete" && name != "" {
				labels[change.Word] = name
			}
		}
		order := slices.Clone(catalog.GroupOrder)
		if change.Operation == "create" && len(order) > 0 {
			order = append(order, change.Word)
		}
		if index := slices.Index(order, source); index >= 0 {
			if change.Operation == "delete" {
				order = slices.Delete(order, index, index+1)
			} else {
				order[index] = change.Word
			}
		}
		if order != nil {
			document["group_order"] = order
		}
		if flags, ok := document["group_defaults"].(map[string]any); ok && (source != change.Word || change.Operation == "delete") {
			flag, exists := flags[source]
			delete(flags, source)
			if exists && change.Operation == "update" {
				flags[change.Word] = flag
			}
			if len(flags) == 0 {
				delete(document, "group_defaults")
			}
		}
		return nil
	})
}

func (m *Module) WriteActionGroupOrder(ctx context.Context, input action.GroupOrder) (int64, error) {
	return m.editGroupDocument(ctx, nil, func(catalog *localization.Catalog, document map[string]any) error {
		if !slices.Equal(catalog.GroupOrder, input.ExpectedOrder) {
			return action.ErrGroupConflict
		}
		remaining := catalog.GroupKeys()
		for _, key := range input.Order {
			if !remaining[key] {
				return action.ErrInvalid
			}
			delete(remaining, key)
		}
		if len(remaining) != 0 {
			return action.ErrGroupConflict
		}
		document["group_order"] = input.Order
		return nil
	})
}
