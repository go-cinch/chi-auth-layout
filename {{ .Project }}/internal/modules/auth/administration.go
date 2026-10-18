package auth

import (
	"context"
	"fmt"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

// The transaction lock serializes changes which can affect a caller's current
// grants. Read committed checks happen after the lock, so a queued mutation
// cannot use a grant removed by the preceding transaction.
func (m *Module) LockPermissionMutation(ctx context.Context) error {
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.UserID <= 0 {
		return authn.ErrPermissionAdministrationRequired
	}
	if _, err := m.store.SQL(ctx).ExecContext(ctx, `SELECT pg_advisory_xact_lock(73482301991337)`); err != nil {
		return fmt.Errorf("lock permission mutation: %w", err)
	}
	return nil
}

func (m *Module) permissionAdministration(ctx context.Context) (bool, bool, error) {
	if err := m.LockPermissionMutation(ctx); err != nil {
		return false, false, err
	}
	identity, _ := authn.FromContext(ctx)
	const query = `WITH assigned AS (
 SELECT BTRIM(value) AS code FROM t_user u CROSS JOIN LATERAL UNNEST(STRING_TO_ARRAY(u.action,',')) value WHERE u.id=$1
 UNION
 SELECT BTRIM(value) FROM t_user u JOIN t_role r ON r.id=u.role_id CROSS JOIN LATERAL UNNEST(STRING_TO_ARRAY(r.action,',')) value WHERE u.id=$1
 UNION
 SELECT BTRIM(value) FROM t_user_user_group_relation relation JOIN t_user_group g ON g.id=relation.user_group_id CROSS JOIN LATERAL UNNEST(STRING_TO_ARRAY(g.action,',')) value WHERE relation.user_id=$1
 ) SELECT COALESCE(BOOL_OR(a.config->>'builtin'='all'),false),COALESCE(BOOL_OR(a.word=$2||':grant:system:permission-admin'),false)
 FROM t_action a JOIN assigned ON assigned.code=a.code WHERE a.scope=$2`
	var all, grant bool
	if err := m.store.SQL(ctx).QueryRowContext(ctx, query, identity.UserID, authn.PermissionScope(ctx)).Scan(&all, &grant); err != nil {
		return false, false, fmt.Errorf("load permission administration capability: %w", err)
	}
	return all, grant, nil
}

func (m *Module) RequireAllPermissions(ctx context.Context) error {
	all, _, err := m.permissionAdministration(ctx)
	if err != nil {
		return err
	}
	if !all {
		return authn.ErrPermissionAdministrationRequired
	}
	return nil
}

func (m *Module) RequirePermissionGrant(ctx context.Context) error {
	all, grant, err := m.permissionAdministration(ctx)
	if err != nil {
		return err
	}
	if !all && !grant {
		return authn.ErrPermissionGrantRequired
	}
	return nil
}

// Publishing the resource graph is delegated by its explicit developer feature.
// Merely receiving ordinary dictionary or permission-group endpoints is insufficient.
func (m *Module) RequireCatalogAdministration(ctx context.Context) error {
    all, _, err := m.permissionAdministration(ctx)
    if err != nil { return err }
    if all { return nil }
    identity, ok := authn.FromContext(ctx)
    if !ok { return authn.ErrPermissionAdministrationRequired }
    permission, err := m.permissions(ctx, identity.UserID, authn.PermissionScope(ctx))
    if err != nil { return err }
    if !permission.buttonGrants[authn.PermissionScope(ctx)+":btn:system:developer:publish"] { return authn.ErrPermissionAdministrationRequired }
    return nil
}
