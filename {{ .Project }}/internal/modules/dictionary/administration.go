package dictionary

import (
	"context"
	"strings"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

type PermissionAdministration interface {
	authn.AllPermissionsPolicy
	authn.PermissionMutationPolicy
}

func (m *Module) SetPermissionAdministration(policy PermissionAdministration) {
	m.administration = policy
}

func (m *Module) permissionContext(ctx context.Context) bool {
	_, authenticated := authn.FromContext(ctx)
	return m.administration != nil || authenticated || authn.IsHTTPPermissionRequest(ctx)
}

// Both built-ins need stable keys, values and enabled states. Only I18N_CATALOG
// additionally requires catalog administration to edit its name or description.
func (m *Module) guardDictionaryMutation(ctx context.Context, ids []int64, input *UpdateDictionaryInput) error {
	if input != nil && input.Key != nil && strings.TrimSpace(*input.Key) == LocalizationKey {
		return ErrCatalogProtected
	}
	protectedFields := input == nil || input.Key != nil || input.Value != nil || input.Enabled != nil
	if !protectedFields && !m.permissionContext(ctx) {
		return nil
	}
	if err := authn.LockPermissionMutation(ctx, m.administration); err != nil {
		return err
	}
	keys := []string{LocalizationKey}
	if protectedFields {
		keys = append(keys, UIPreferenceDictionaryKey)
	}
	var protected bool
	if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM t_dictionary WHERE id = ANY($1) AND dictionary_key = ANY($2) FOR UPDATE)`, pq.Array(ids), pq.Array(keys)).Scan(&protected); err != nil {
		return err
	}
	if !protected {
		return nil
	}
	if protectedFields {
		return ErrCatalogProtected
	}
	return authn.RequireCatalogAdministration(ctx, m.administration)
}
