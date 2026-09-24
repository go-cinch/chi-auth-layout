package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

var (
	ErrPasswordResetNotRequired = apperror.New("AUTH_PASSWORD_RESET_NOT_REQUIRED", "password reset is not required")
	ErrInvalidPasswordReset     = apperror.New("AUTH_INVALID_PASSWORD_RESET", "new password must be 6-72 bytes")
)

type PasswordResetInput struct {
	NewPassword string `json:"new_password"`
}

func (m *Module) PasswordResetChallenge(ctx context.Context) (*CredentialChallenge, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if !identity.PasswordResetRequired {
		return nil, ErrPasswordResetNotRequired
	}
	return m.credentials.PasswordResetChallenge(ctx, identity.UserID)
}

func (m *Module) OpenPasswordReset(ctx context.Context, encrypted EncryptedPasswordResetInput) (PasswordResetInput, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return PasswordResetInput{}, ErrUnauthorized
	}
	return m.credentials.OpenPasswordReset(ctx, identity.UserID, encrypted)
}

// ResetPassword completes the first login. The row lock serializes resets with
// other resets, ordinary logins, and administrator password updates.
func (m *Module) ResetPassword(ctx context.Context, input PasswordResetInput) (*LoginResult, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if len(input.NewPassword) < 6 || len(input.NewPassword) > 72 {
		return nil, ErrInvalidPasswordReset
	}
	session, err := m.sessions.authenticatedSession(ctx, identity)
	if err != nil {
		return nil, ErrUnauthorized
	}
	var result *LoginResult
	var newSessionID string
	err = m.store.Tx(ctx, func(ctx context.Context) error {
		var currentHash string
		var version, count int64
		var status int16
		err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT password, credential_version, login_count, status FROM t_user WHERE id = $1 AND code = $2 FOR UPDATE`, identity.UserID, identity.Code).Scan(&currentHash, &version, &count, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnauthorized
		}
		if err != nil {
			return fmt.Errorf("query password reset user: %w", err)
		}
		if version != identity.CredentialVersion || status != userStatusActive {
			return ErrUnauthorized
		}
		if count != 0 {
			return ErrPasswordResetNotRequired
		}
		if bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(input.NewPassword)) == nil {
			return ErrSamePassword
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("hash reset password: %w", err)
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		_, err = m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET password = $1, credential_version = credential_version + 1, login_count = 1, last_logged_in_at = $2, wrong = 0, metadata = metadata - 'password_change_failures', updated_at = $2 WHERE id = $3 AND code = $4`, string(hash), now, identity.UserID, identity.Code)
		if err != nil {
			return fmt.Errorf("reset password: %w", err)
		}
		identity.CredentialVersion++
		identity.PasswordResetRequired = false
		result, newSessionID, err = m.issueLoginResult(ctx, identity, session.RememberMe)
		return err
	})
	if err != nil {
		if newSessionID != "" {
			_ = m.sessions.Revoke(context.WithoutCancel(ctx), newSessionID)
		}
		return nil, err
	}
	if err := m.passwordGuard.Clear(ctx, identity.UserID); err != nil {
		slog.WarnContext(ctx, "clear password reset failures failed: "+err.Error())
	}
	return result, nil
}

func (m *Module) issueLoginResult(ctx context.Context, identity authn.Identity, rememberMe bool) (*LoginResult, string, error) {
	session, err := m.sessions.Issue(ctx, identity, rememberMe)
	if err != nil {
		return nil, "", fmt.Errorf("issue login session: %w", err)
	}
	identity.SessionID = session.SessionID
	token, expiredAt, err := m.authenticator.Issue(identity)
	if err != nil {
		_ = m.sessions.Revoke(context.WithoutCancel(ctx), session.SessionID)
		return nil, "", fmt.Errorf("issue login token: %w", err)
	}
	return &LoginResult{AccessToken: token, RefreshToken: session.RefreshToken, ExpiredAt: expiredAt.UnixMilli(), PasswordResetRequired: identity.PasswordResetRequired}, session.SessionID, nil
}
