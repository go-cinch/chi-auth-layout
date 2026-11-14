package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

type UIPreferenceSource interface {
	UIPreferenceDefaults(context.Context, string) (dictionary.UIPreferences, error)
}

type UIPreferencesResult struct {
	UIPreferences map[string]dictionary.UIPreferences `json:"ui_preferences"`
}

func (m *Module) SetUIPreferenceSource(source UIPreferenceSource) { m.uiPreferenceSource = source }

func preferenceClient(query url.Values) (string, error) {
	clients, exists := query["ui"]
	if !exists {
		return "", nil
	}
	if len(clients) != 1 || clients[0] != dictionary.UIPreferenceClient {
		return "", ErrInvalid
	}
	return clients[0], nil
}

func (m *Module) uiDefaults(ctx context.Context, client string) (dictionary.UIPreferences, error) {
	if client != dictionary.UIPreferenceClient {
		return nil, ErrInvalid
	}
	if m.uiPreferenceSource == nil {
		return nil, errors.New("UI preference source is unavailable")
	}
	return m.uiPreferenceSource.UIPreferenceDefaults(ctx, client)
}

func (m *Module) UpdateUIPreferences(ctx context.Context, changes map[string]dictionary.UIPreferences) (*UIPreferencesResult, error) {
	patch, exists := changes[dictionary.UIPreferenceClient]
	if !exists || len(changes) != 1 {
		return nil, ErrInvalid
	}
	if patch != nil {
		if err := dictionary.ValidateUIPreferencePatch(patch); err != nil {
			return nil, ErrInvalid
		}
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if identity.PasswordResetRequired {
		return nil, authn.ErrPasswordResetRequired
	}
	required, err := m.totpEnrollmentRequired(ctx, identity)
	if err != nil {
		return nil, err
	}
	if required {
		return nil, ErrTOTPEnrollmentRequired
	}
	var effective dictionary.UIPreferences
	err = m.store.Tx(ctx, func(txCtx context.Context) error {
		defaults, err := m.uiDefaults(txCtx, dictionary.UIPreferenceClient)
		if err != nil {
			return err
		}
		var raw []byte
		err = m.store.SQL(txCtx).QueryRowContext(txCtx, `SELECT COALESCE(metadata #> '{ui_preferences,vben_pc}', '{}'::jsonb)
            FROM t_user WHERE id=$1 AND code=$2 AND credential_version=$3 AND status=$4 FOR UPDATE`, identity.UserID, identity.Code, identity.CredentialVersion, userStatusActive).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnauthorized
		}
		if err != nil {
			return err
		}
		var overrides dictionary.UIPreferences
		_ = json.Unmarshal(raw, &overrides)
		updated, resolved, err := dictionary.ApplyUIPreferences(defaults, overrides, patch)
		if err != nil {
			return ErrInvalid
		}
		effective = resolved
		parent := map[string]dictionary.UIPreferences{}
		if len(updated) != 0 {
			parent[dictionary.UIPreferenceClient] = updated
		}
		encoded, err := json.Marshal(parent)
		if err != nil {
			return err
		}
		_, err = m.store.SQL(txCtx).ExecContext(txCtx, `UPDATE t_user SET metadata = jsonb_set(metadata, '{ui_preferences}',
            (CASE WHEN jsonb_typeof(metadata->'ui_preferences')='object' THEN metadata->'ui_preferences' ELSE '{}'::jsonb END - 'vben_pc') || $1::jsonb), updated_at=$2
            WHERE id=$3 AND code=$4`, string(encoded), time.Now().UnixMilli(), identity.UserID, identity.Code)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &UIPreferencesResult{UIPreferences: map[string]dictionary.UIPreferences{dictionary.UIPreferenceClient: effective}}, nil
}
