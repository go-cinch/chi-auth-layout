package dictionary

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/lib/pq"
	"reflect"
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

// The resource catalog is authorization policy, even when edited through the
// dictionary screen. Recheck privilege inside the mutation transaction.
func (m *Module) guardCatalogMutation(ctx context.Context, ids []int64, nextKey string, groupsOnly ...bool) error {
	if len(ids) == 0 {
		if nextKey == LocalizationKey {
			return authn.RequireCatalogAdministration(ctx, m.administration)
		}
		return nil
	}
	if !m.permissionContext(ctx) {
		return nil
	}
	if err := authn.LockPermissionMutation(ctx, m.administration); err != nil {
		return err
	}
	var catalog bool
	if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM t_dictionary WHERE id = ANY($1) AND dictionary_key = $2)`, pq.Array(ids), LocalizationKey).Scan(&catalog); err != nil {
		return err
	}
	if catalog || nextKey == LocalizationKey {
		if len(groupsOnly) > 0 && groupsOnly[0] {
			return authn.RequireAdministrationGrant(ctx, m.administration)
		}
		return authn.RequireCatalogAdministration(ctx, m.administration)
	}
	return nil
}

// expected_value is compared with the locked database row before any write.
// A grant manager may edit group names/order, but cannot smuggle resource,
// feature, language or ordinary message changes through the same endpoint.
func permissionGroupsOnly(before, after json.RawMessage) bool {
	strip := func(raw json.RawMessage) (map[string]any, bool) {
		var doc map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&doc) != nil {
			return nil, false
		}
		delete(doc, "group_order")
		messages, ok := doc["messages"].(map[string]any)
		if !ok {
			return nil, false
		}
		for _, value := range messages {
			locale, ok := value.(map[string]any)
			if !ok {
				return nil, false
			}
			frontend, ok := locale["frontend"].(map[string]any)
			if !ok {
				return nil, false
			}
			delete(frontend, "groups")
		}
		return doc, true
	}
	a, validBefore := strip(before)
	b, validAfter := strip(after)
	return validBefore && validAfter && reflect.DeepEqual(a, b)
}
