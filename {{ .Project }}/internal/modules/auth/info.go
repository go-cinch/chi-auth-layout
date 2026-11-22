package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/modules/action"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

var infoFields = []string{"id", "username", "code", "role", "scope", "permission", "email", "email_verified_at", "email_binding_required", "metadata", "totp", "ui_preferences"}

func (value UserInfo) MarshalJSON() ([]byte, error) {
	type plain UserInfo
	return report.MarshalFields(plain(value), value.selectedFields)
}

func (m *Module) InfoSelected(ctx context.Context, scope string, fields report.Fields, clients ...string) (*UserInfo, error) {
	if err := fields.Validate(infoFields...); err != nil {
		return nil, err
	}
	client := ""
	if len(clients) > 0 {
		client = clients[0]
	}
	if fields != nil && fields.Has("ui_preferences") && client != dictionary.UIPreferenceClient {
		return nil, ErrInvalid
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
	var uiOverrides []byte
	projection.Add("id", "u.id", &value.ID)
	projection.Add("username", "u.username", &value.Username)
	projection.Add("code", "u.code", &value.Code)
	projection.Add("email_verified_at", "u.email_verified_at", &value.EmailVerifiedAt)
	if fields.Has("email") || fields.Has("email_binding_required") {
		projection.Require("COALESCE(u.email, '')", &value.Email)
	}
	// Always verify the record exists, even for a context-only scope projection.
	if !fields.Has("id") && !fields.Has("username") && !fields.Has("code") {
		projection.Require("u.id", &value.ID)
	}
	if fields.Has("metadata") {
		value.Metadata = &SelfMetadata{}
		projection.Require("COALESCE(u.metadata->>'language', '')", &value.Metadata.Language)
		projection.Require("COALESCE(u.metadata->>'timezone', '')", &value.Metadata.Timezone)
		projection.Require(`u.metadata @> '{"email_binding_skipped":true}'::jsonb`, &value.Metadata.EmailBindingSkipped)
		projection.Require(`u.metadata @> '{"totp_enrollment_skipped":true}'::jsonb`, &value.Metadata.TOTPEnrollmentSkipped)
		projection.Require(`u.metadata @> '{"password_reset_skipped":true}'::jsonb`, &value.Metadata.PasswordResetSkipped)
	}
	if fields.Has("ui_preferences") {
		projection.Require(`COALESCE(u.metadata #> '{ui_preferences,vben_pc}', '{}'::jsonb)`, &uiOverrides)
	}
	if fields.Has("totp") {
		value.TOTP = &TOTPInfo{}
		projection.Require("EXISTS (SELECT 1 FROM t_user_totp f WHERE f.user_id = u.id)", &value.TOTP.Bound)
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
	if fields.Has("ui_preferences") {
		defaults, err := m.uiDefaults(ctx, client)
		if err != nil {
			return nil, err
		}
		var overrides dictionary.UIPreferences
		_ = json.Unmarshal(uiOverrides, &overrides)
		value.UIPreferences = map[string]dictionary.UIPreferences{client: dictionary.MergeUIPreferences(defaults, overrides)}
	}
	if roleID.Valid {
		value.Role = &UserRole{ID: roleID.Int64, Name: roleName.String, Word: roleWord.String}
	}
	if fields.Has("permission") {
		permission, err := m.userInfoPermission(ctx, identity.UserID, scope)
		if err != nil {
			return nil, err
		}
		value.Permission = permission
	}
	if fields.Has("email_binding_required") {
		value.EmailBindingRequired = value.Email == "" && m.requiresEmailBinding()
	}
	return value, nil
}

// Project UI actions from the same scoped HTTP grants used when an action is submitted.
func (m *Module) userInfoPermission(ctx context.Context, id int64, scope string) (Permission, error) {
	catalog := m.catalogSource.Select(report.Fields{"resource_catalog"})
	p, err := m.permissionsFromCatalog(ctx, id, scope, nil, catalog)
	if err != nil {
		return Permission{}, err
	}
	p.projectNavigation(catalog.ResourceCatalog, scope)
	if scope == "admin" {
		post := func(path string) bool {
			return m.allowsTarget(p, PermissionTarget{Method: "POST", Path: m.httpPath + path}, scope, catalog.ResourceCatalog)
		}
		add := func(action string, allowed bool) {
			if allowed {
				p.ProfileActions = append(p.ProfileActions, action)
			}
		}
		challenge := post("/challenge")
		add("change_password", challenge && m.allowsTarget(p, PermissionTarget{Method: "PATCH", Path: m.httpPath + "/change/pwd"}, scope, catalog.ResourceCatalog))
		add("bind_email", challenge && post("/email/code") && post("/email/bind"))
		add("unlink_by_email", post("/email/unbind/request"))
		add("unlink_by_factor", post("/email/unbind"))
		add("unlink_by_password", challenge && post("/email/unbind"))
		enroll := challenge && post("/totp/setup") && post("/totp/confirm")
		add("enroll_totp", enroll)
		add("test_totp", post("/totp/test"))
		recover := enroll && post("/totp/recovery/confirm")
		add("recover_totp", recover)
		add("recover_totp_by_email", recover && post("/totp/recovery/email-code"))
	}
	return p, nil
}

func (m *Module) SkipEmailBinding(ctx context.Context) error {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if m.requiresEmailBinding() {
		return ErrEmailBindingRequired
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user
		SET metadata = jsonb_set(metadata, '{email_binding_skipped}', 'true'::jsonb), updated_at = $1
		WHERE id = $2 AND code = $3`, time.Now().UnixMilli(), identity.UserID, identity.Code)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		return ErrUnauthorized
	}
	return err
}

// UpdateLanguage changes only the current user's language, preserving other metadata.
func (m *Module) UpdateLanguage(ctx context.Context, language string) error {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if identity.PasswordResetRequired {
		return authn.ErrPasswordResetRequired
	}
	required, err := m.totpEnrollmentRequired(ctx, identity)
	if err != nil {
		return err
	}
	if required {
		return ErrTOTPEnrollmentRequired
	}
	enabled := false
	for _, locale := range m.catalogSource.PublicCatalog("locales").Locales {
		if locale.Enabled && locale.Tag == language {
			enabled = true
			break
		}
	}
	if !enabled {
		return ErrInvalid
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user
        SET metadata = jsonb_set(metadata, '{language}', to_jsonb($1::text)), updated_at = $2
        WHERE id = $3 AND code = $4 AND credential_version = $5 AND status = $6`,
		language, time.Now().UnixMilli(), identity.UserID, identity.Code, identity.CredentialVersion, userStatusActive)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		return ErrUnauthorized
	}
	return err
}

// UpdateTimezone stores an IANA timezone independently of client UI overrides.
func (m *Module) UpdateTimezone(ctx context.Context, timezone string) error {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if identity.PasswordResetRequired {
		return authn.ErrPasswordResetRequired
	}
	required, err := m.totpEnrollmentRequired(ctx, identity)
	if err != nil {
		return err
	}
	if required {
		return ErrTOTPEnrollmentRequired
	}
	if timezone == "" || timezone == "Local" {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return ErrInvalid
	}
	result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user
        SET metadata = jsonb_set(metadata, '{timezone}', to_jsonb($1::text)), updated_at = $2
        WHERE id = $3 AND code = $4 AND credential_version = $5 AND status = $6`,
		timezone, time.Now().UnixMilli(), identity.UserID, identity.Code, identity.CredentialVersion, userStatusActive)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err == nil && rows == 0 {
		return ErrUnauthorized
	}
	return err
}
