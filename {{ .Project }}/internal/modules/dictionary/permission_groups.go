package dictionary

import (
	"context"
	"encoding/json"
	"errors"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
	"strings"
)

var ErrGroupNotEmpty = apperror.New("DICTIONARY_GROUP_NOT_EMPTY", "only empty permission groups can be deleted")

// PermissionGroups owns action SQL and joins the catalog mutation transaction.
type PermissionGroups interface {
	ReconcilePermissionGroups(context.Context, map[string]string, []string) (bool, error)
}

func (m *Module) SetPermissionGroups(groups PermissionGroups) { m.permissionGroups = groups }

func (m *Module) reconcilePermissionGroups(ctx context.Context, before json.RawMessage, value *Dictionary, moves map[string]string) error {
	if value.Key != LocalizationKey || len(before) == 0 {
		if len(moves) > 0 {
			return ErrInvalid
		}
		return nil
	}
	var previous localization.Catalog
	if err := json.Unmarshal(before, &previous); err != nil {
		if len(moves) > 0 {
			return ErrInvalid
		}
		return nil
	}
	next, err := localization.Parse(value.Value)
	if err != nil {
		return ErrInvalid
	}
	currentKeys := next.GroupKeys()
	previousKeys := previous.GroupKeys()
	for from, to := range moves {
		if !previousKeys[from] || currentKeys[from] || !currentKeys[to] || from == to || strings.Split(from, ":")[0] != strings.Split(to, ":")[0] {
			return ErrInvalid
		}
	}
	removed := []string{}
	for key := range previousKeys {
		if !currentKeys[key] && moves[key] == "" {
			removed = append(removed, key)
		}
	}
	if len(moves) == 0 && len(removed) == 0 {
		return nil
	}
	if m.permissionGroups == nil {
		return errors.New("permission group storage is unavailable")
	}
	empty, err := m.permissionGroups.ReconcilePermissionGroups(ctx, moves, removed)
	if err != nil {
		return err
	}
	if !empty {
		return ErrGroupNotEmpty
	}
	return nil
}
