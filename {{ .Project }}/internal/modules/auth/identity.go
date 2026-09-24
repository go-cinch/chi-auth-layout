package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

type IdentityValidator struct {
	store    *db.Store
	sessions *Sessions
	passwordResetRequired bool
}

func NewIdentityValidator(store *db.Store, sessions *Sessions, switches Switches) *IdentityValidator {
	return &IdentityValidator{store: store, sessions: sessions, passwordResetRequired: switches.PasswordResetRequired}
}

func (v *IdentityValidator) ValidateIdentity(ctx context.Context, identity authn.Identity) error {
	_, err := v.ResolveIdentity(ctx, identity)
	return err
}

func (v *IdentityValidator) ResolveIdentity(ctx context.Context, identity authn.Identity) (authn.Identity, error) {
	identity.PasswordResetRequired = false
	if err := v.sessions.ValidateIdentity(ctx, identity); err != nil {
		return authn.Identity{}, err
	}
	current, status, err := currentCredentialIdentity(ctx, v.store, identity, v.passwordResetRequired)
	if err != nil {
		return authn.Identity{}, err
	}
	if status != userStatusActive {
		return authn.Identity{}, ErrUnauthorized
	}
	return current, nil
}

// Keep the issued version: refresh must never upgrade an old credential.
func currentCredentialIdentity(ctx context.Context, store *db.Store, identity authn.Identity, passwordResetRequired bool) (authn.Identity, int16, error) {
	if identity.CredentialVersion <= 0 {
		return authn.Identity{}, 0, ErrUnauthorized
	}
	const query = `SELECT username, code, status, credential_version, login_count FROM t_user WHERE id = $1 AND code = $2`
	current := identity
	var status int16
	var loginCount int64
	err := store.SQL(ctx).QueryRowContext(ctx, query, identity.UserID, identity.Code).Scan(
		&current.Username, &current.Code, &status, &current.CredentialVersion, &loginCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return authn.Identity{}, 0, ErrUnauthorized
	}
	if err != nil {
		return authn.Identity{}, 0, fmt.Errorf("query authentication user: %w", err)
	}
	if current != identity {
		return authn.Identity{}, 0, ErrUnauthorized
	}
	current.PasswordResetRequired = passwordResetRequired && loginCount == 0
	return current, status, nil
}
