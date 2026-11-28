package dictionary

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

const LocalizationKey = "I18N_CATALOG"

// LoadLocalization starts runtime localization after migrations and before serving.
// Refresh reads the authoritative row, independently of best-effort value caches.
func (m *Module) LoadLocalization(ctx context.Context) error {
	return m.withLocalization(ctx, true)
}

func (m *Module) RefreshLocalization(ctx context.Context) error {
	return m.withLocalization(ctx, false)
}

func (m *Module) withLocalization(ctx context.Context, initialize bool) error {
	// Serialize authoritative reads without letting a slow poll hold a committed
	// write beyond its refresh deadline. A failed refresh keeps the last snapshot.
	select {
	case m.localizationGate <- struct{}{}:
		defer func() { <-m.localizationGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if initialize {
		m.localizationEnabled = true
	}
	if !m.localizationEnabled {
		return nil
	}
	return m.refreshLocalization(ctx)
}

func (m *Module) refreshLocalization(ctx context.Context) error {
	var raw []byte
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT value FROM t_dictionary WHERE dictionary_key = $1 AND enabled`, LocalizationKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		m.catalogSource.Reset()
		m.localizationRaw = nil
		return nil
	}
	if err != nil {
		return fmt.Errorf("load localization dictionary: %w", err)
	}
	if bytes.Equal(m.localizationRaw, raw) {
		return nil
	}
	if err := m.catalogSource.Install(raw); err != nil {
		return fmt.Errorf("load localization dictionary: %w", err)
	}
	m.localizationRaw = append([]byte(nil), raw...)
	return nil
}

func (m *Module) prepareLocalization(value *Dictionary) error {
	normalized, err := m.catalogSource.CanonicalCatalog(value.Value)
	if err != nil {
		return ErrInvalid
	}
	value.Value = normalized
	catalog, err := localization.Parse(value.Value)
	if err != nil {
		return ErrInvalid
	}
	// Validate the submitted snapshot here so legacy catalogs remain readable and repairable.
	type scopedPath struct{ scope, path string }
	owners := make(map[scopedPath]string)
	for resource, node := range catalog.ResourceCatalog.Resources {
		scope, _, _ := strings.Cut(resource, ":")
		for key, entry := range node.Entries {
			route := scopedPath{scope, entry.Path}
			if previous, exists := owners[route]; exists {
				return fmt.Errorf("%w: menu path %q in scope %q is used by %s and %s", ErrInvalid, entry.Path, scope, previous, key)
			}
			owners[route] = key
		}
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(value.Value, &document); err != nil {
		return ErrInvalid
	}
	var original map[string]localization.Messages
	if err := json.Unmarshal(document["messages"], &original); err != nil {
		return ErrInvalid
	}
	_, hadAliases := document["group_aliases"]
	delete(document, "group_aliases")
	var resources map[string]json.RawMessage
	if err := json.Unmarshal(document["resource_catalog"], &resources); err == nil {
		for _, key := range []string{"resource_aliases", "feature_aliases"} {
			if _, exists := resources[key]; exists {
				delete(resources, key)
				hadAliases = true
			}
		}
		if hadAliases {
			document["resource_catalog"], err = json.Marshal(resources)
			if err != nil {
				return ErrInvalid
			}
		}
	}
	// Validate first, then persist only enabled default groups. Missing means false.
	defaultsChanged := false
	for group, enabled := range catalog.GroupDefaults {
		if !enabled {
			delete(catalog.GroupDefaults, group)
			defaultsChanged = true
		}
	}
	if _, exists := document["group_defaults"]; exists && len(catalog.GroupDefaults) == 0 {
		delete(document, "group_defaults")
		defaultsChanged = true
	} else if defaultsChanged {
		document["group_defaults"], err = json.Marshal(catalog.GroupDefaults)
		if err != nil {
			return ErrInvalid
		}
	}
	if reflect.DeepEqual(original, catalog.Messages) && !hadAliases && !defaultsChanged {
		return nil
	}
	document["messages"], err = json.Marshal(catalog.Messages)
	if err != nil {
		return ErrInvalid
	}
	value.Value, err = json.Marshal(document)
	if err != nil {
		return ErrInvalid
	}
	return nil
}

func (m *Module) refreshLocalizationAfterWrite(ctx context.Context) {
	if err := m.RefreshLocalization(ctx); err != nil {
		slog.ErrorContext(ctx, "dictionary saved but localization refresh failed: "+err.Error(), "operation", "refresh_localization", "outcome", "failure", "db_committed", true, "error", err)
	}
}

type PublishCatalogInput struct {
	Value           json.RawMessage   `json:"value"`
	ExpectedValue   json.RawMessage   `json:"expected_value"`
	GroupRenames    map[string]string `json:"group_renames,omitempty"`
	ResourceRenames map[string]string `json:"resource_renames,omitempty"`
	FeatureRenames  map[string]string `json:"feature_renames,omitempty"`
}

func (m *Module) PublishCatalog(ctx context.Context, input PublishCatalogInput) (*Dictionary, error) {
	if !validValue(input.Value) || !validValue(input.ExpectedValue) {
		return nil, ErrInvalid
	}
	var saved *Dictionary
	err := m.store.MutationTx(ctx, func(ctx context.Context) error {
		if err := authn.RequireCatalogAdministration(ctx, m.administration); err != nil {
			return err
		}
		action.MarkPermissionMutation(ctx)
		executor := m.store.SQL(ctx)
		var id int64
		var matches bool
		err := executor.QueryRowContext(ctx, `SELECT id, value = $2::jsonb FROM t_dictionary WHERE dictionary_key = $1 FOR UPDATE`, LocalizationKey, string(input.ExpectedValue)).Scan(&id, &matches)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if !matches {
			return ErrVersionConflict
		}
		catalog := &Dictionary{ID: id, Key: LocalizationKey, Value: input.Value}
		if err = m.prepareLocalization(catalog); err != nil {
			return err
		}
		if _, err = executor.ExecContext(ctx, `UPDATE t_dictionary SET value = $1::jsonb, updated_at = $2 WHERE id = $3`, string(catalog.Value), time.Now().UnixMilli(), id); err != nil {
			return err
		}
		if err = m.migrateResourcePermissions(ctx, input.ExpectedValue, catalog, input.ResourceRenames, input.FeatureRenames); err != nil {
			return err
		}
		if err = m.reconcilePermissionGroups(ctx, input.ExpectedValue, catalog, input.GroupRenames); err != nil {
			return err
		}
		saved, err = get(ctx, executor, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	m.invalidate(ctx, "update", saved.ID)
	return saved, nil
}
