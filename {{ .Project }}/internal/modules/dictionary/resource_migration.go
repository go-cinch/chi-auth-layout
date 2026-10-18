package dictionary

import (
	"context"
	"encoding/json"
	"errors"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
)

// ResourcePermissions joins the caller's transaction and owns all action SQL.
type ResourcePermissions interface {
	ReconcileResourcePermissions(context.Context, json.RawMessage, json.RawMessage, map[string]string, map[string]string) (bool, error)
}

func (m *Module) SetResourcePermissions(permissions ResourcePermissions) {
	m.resourcePermissions = permissions
}

func (m *Module) migrateResourcePermissions(ctx context.Context, before json.RawMessage, value *Dictionary, resources, features map[string]string) error {
	if value.Key != LocalizationKey {
		if len(resources) > 0 || len(features) > 0 {
			return ErrInvalid
		}
		return nil
	}
	next, err := localization.Parse(value.Value)
	if err != nil {
		return ErrInvalid
	}
	var previous localization.Catalog
	if len(before) > 0 {
		if json.Unmarshal(before, &previous) != nil {
			return ErrInvalid
		}
	}
	if len(resources) == 0 && len(features) == 0 && previous.ResourceCatalog.Equal(next.ResourceCatalog) {
		return nil
	}
	if m.resourcePermissions == nil {
		return errors.New("resource permission storage is unavailable")
	}
	valid, err := m.resourcePermissions.ReconcileResourcePermissions(ctx, before, value.Value, resources, features)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalid
	}
	return nil
}
