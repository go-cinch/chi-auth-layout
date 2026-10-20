package dictionary

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

// WriteActionNames updates only action labels, using the caller's transaction.
// The permission mutation lock is already held by the action module.
func (m *Module) WriteActionNames(ctx context.Context, oldWord, word string, names map[string]string) (int64, error) {
	var id int64
	var raw []byte
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT id, value FROM t_dictionary WHERE dictionary_key = $1 AND enabled FOR UPDATE`, LocalizationKey).Scan(&id, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, apperror.InvalidBody
	}
	if err != nil {
		return 0, err
	}
	catalog, err := localization.Parse(raw)
	if err != nil {
		return 0, err
	}
	registered := map[string]bool{}
	for _, locale := range catalog.Locales {
		registered[locale.Tag] = true
	}
	for tag := range names {
		if !registered[tag] {
			return 0, apperror.InvalidBody
		}
	}
	// Preserve unrelated namespaces and catalog extension fields.
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return 0, err
	}
	messages := object(document, "messages")
	for _, locale := range catalog.Locales {
		name, supplied := names[locale.Tag]
		if !supplied && (oldWord == "" || oldWord == word) {
			continue
		}
		frontend := object(object(messages, locale.Tag), "frontend")
		actions := object(frontend, "actions")
		if oldWord != "" && oldWord != word {
			if previous, exists := actions[oldWord]; exists {
				actions[word] = previous
				delete(actions, oldWord)
			}
		}
		if supplied {
			name = strings.TrimSpace(name)
			if name == "" {
				delete(actions, word)
			} else {
				actions[word] = name
			}
		}
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return 0, err
	}
	if _, err := localization.Parse(encoded); err != nil {
		return 0, err
	}
	_, err = m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_dictionary SET value = $2::jsonb, updated_at = $3 WHERE id = $1`, id, string(encoded), time.Now().UnixMilli())
	return id, err
}

func object(parent map[string]any, key string) map[string]any {
	value, ok := parent[key].(map[string]any)
	if !ok {
		value = map[string]any{}
		parent[key] = value
	}
	return value
}

func (m *Module) RefreshActionNames(ctx context.Context, id int64) {
	m.invalidate(ctx, "update_action_names", id)
}

// ActionCatalog participates in action writes through their transaction context.
func (m *Module) ActionCatalog(ctx context.Context) (localization.Catalog, error) {
	var raw []byte
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT value FROM t_dictionary WHERE dictionary_key=$1 AND enabled`, LocalizationKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return localization.Catalog{}, apperror.InvalidBody
	}
	if err != nil {
		return localization.Catalog{}, err
	}
	catalog, err := localization.Parse(raw)
	if err != nil {
		return localization.Catalog{}, err
	}
	return *catalog, nil
}
