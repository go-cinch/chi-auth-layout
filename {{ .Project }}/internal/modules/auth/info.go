package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/action"
)

var infoFields = []string{"id", "username", "code", "role", "scope", "permission"}

func (value UserInfo) MarshalJSON() ([]byte, error) {
	type plain UserInfo
	return report.MarshalFields(plain(value), value.selectedFields)
}

func (m *Module) InfoSelected(ctx context.Context, scope string, fields report.Fields) (*UserInfo, error) {
	if err := fields.Validate(infoFields...); err != nil {
		return nil, err
	}
	if fields == nil {
		return m.Info(ctx, scope)
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return nil, ErrInfoScopeRequired
	}
	if !action.ValidScope(scope) {
		return nil, ErrInfoScopeInvalid
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	value := &UserInfo{selectedFields: fields, Scope: scope}
	projection := report.Projection{Fields: fields}
	projection.Add("id", "u.id", &value.ID)
	projection.Add("username", "u.username", &value.Username)
	projection.Add("code", "u.code", &value.Code)
	// Always verify the record exists, even for a context-only scope projection.
	if !fields.Has("id") && !fields.Has("username") && !fields.Has("code") {
		projection.Require("u.id", &value.ID)
	}
	from := "t_user AS u"
	var roleID sql.NullInt64
	var roleName, roleWord sql.NullString
	if fields.Has("role") {
		from += " LEFT JOIN t_role AS r ON r.id = u.role_id"
		projection.Require("r.id", &roleID)
		projection.Require("r.name", &roleName)
		projection.Require("r.word", &roleWord)
	}
	err := m.store.SQL(ctx).QueryRowContext(ctx, projection.SQL(from)+" WHERE u.id = $1 AND u.code = $2", identity.UserID, identity.Code).Scan(projection.Targets...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, fmt.Errorf("query current user: %w", err)
	}
	if roleID.Valid {
		value.Role = &UserRole{ID: roleID.Int64, Name: roleName.String, Word: roleWord.String}
	}
	if fields.Has("permission") {
		catalog := m.catalogSource.Select([]string{"resource_catalog"})
		permission, err := m.permissionsFromCatalog(ctx, identity.UserID, scope, nil, catalog)
		if err != nil {
			return nil, err
		}
		permission.Navigation = permissionNavigation(catalog.ResourceCatalog, scope, permission.Menus)
		value.Permission = permission
	}
	return value, nil
}
