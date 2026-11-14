package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

var (
	ErrInvalidPasswordChange = apperror.New("AUTH_INVALID_PASSWORD_CHANGE", "invalid password change")
	ErrIncorrectPassword     = apperror.New("AUTH_INCORRECT_PASSWORD", "current password is incorrect")
	ErrSamePassword          = apperror.New("AUTH_SAME_PASSWORD", "new password must differ from current password")
	ErrPasswordChangeLocked  = apperror.New("AUTH_PASSWORD_CHANGE_LOCKED", "self-service password change is locked; please contact an administrator")
)

const PasswordChangeFailuresMetadataKey = "password_change_failures"

type PasswordChangeInput struct {
	OldPassword   string         `json:"old_password" example:"current-password"`
	NewPassword   string         `json:"new_password" example:"new-password"`
	CaptchaID     string         `json:"captcha_id,omitempty"`
	CaptchaPoints []CaptchaPoint `json:"captcha_points,omitempty"`
}

type passwordChangeFailure struct {
	cause           error
	captchaRequired bool
}

func (e *passwordChangeFailure) Error() string { return e.cause.Error() }

func (e *passwordChangeFailure) Unwrap() error { return e.cause }

func requiresPasswordChangeCaptcha(err error) bool {
	var failure *passwordChangeFailure
	return errors.As(err, &failure) && failure.captchaRequired
}

func (m *Module) PasswordChangeChallenge(ctx context.Context) (*CredentialChallenge, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	return m.credentials.PasswordChangeChallenge(ctx, identity.UserID)
}

func (m *Module) OpenPasswordChange(ctx context.Context, encrypted EncryptedPasswordChangeInput) (PasswordChangeInput, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return PasswordChangeInput{}, ErrUnauthorized
	}
	return m.credentials.OpenPasswordChange(ctx, identity.UserID, encrypted)
}

func (m *Module) NewPasswordChangeCaptcha(ctx context.Context) (*PointCaptchaChallenge, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	return m.passwordGuard.NewChallenge(ctx, identity.UserID)
}

func (m *Module) RefreshPasswordChangeCaptcha(ctx context.Context, captchaID string) (*PointCaptchaChallenge, error) {
	if _, ok := authn.FromContext(ctx); !ok {
		return nil, ErrUnauthorized
	}
	return m.passwordGuard.RefreshChallenge(ctx, captchaID)
}

func (m *Module) VerifyPasswordChangeCaptcha(ctx context.Context, captchaID string, points []CaptchaPoint) (*PointCaptchaVerificationResult, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	verified, err := m.passwordGuard.Check(ctx, identity.UserID, captchaID, points)
	if err != nil {
		return nil, err
	}
	if verified {
		return &PointCaptchaVerificationResult{Verified: true}, nil
	}
	captcha, err := m.passwordGuard.NewChallenge(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	return &PointCaptchaVerificationResult{Captcha: captcha}, nil
}

func (m *Module) ChangePassword(ctx context.Context, input PasswordChangeInput) error {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if identity.PasswordResetRequired {
		return authn.ErrPasswordResetRequired
	}
	if strings.TrimSpace(input.OldPassword) == "" || strings.TrimSpace(input.NewPassword) == "" {
		return ErrInvalidPasswordChange
	}
	locked, err := m.passwordChangeLocked(ctx, identity)
	if err != nil {
		return err
	}
	if locked {
		return ErrPasswordChangeLocked
	}
	failureCount, err := m.passwordGuard.FailureCount(ctx, identity.UserID)
	if err != nil {
		return err
	}
	if m.passwordGuard.LockRequired(failureCount) {
		if err := m.persistPasswordChangeLock(ctx, identity); err != nil {
			return err
		}
		return ErrPasswordChangeLocked
	}
	captchaRequired := m.passwordGuard.CaptchaRequired(failureCount)
	if captchaRequired {
		verified, err := m.passwordGuard.Verify(ctx, identity.UserID, input.CaptchaID, input.CaptchaPoints)
		if err != nil {
			return err
		}
		if !verified {
			return &passwordChangeFailure{cause: ErrPointCaptchaRequired, captchaRequired: true}
		}
	}
	err = m.store.MutationTx(ctx, func(ctx context.Context) error {
		var currentPasswordHash string
		var credentialVersion int64
		err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT password, credential_version FROM t_user WHERE id = $1 AND code = $2 FOR UPDATE`, identity.UserID, identity.Code).Scan(&currentPasswordHash, &credentialVersion)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnauthorized
		}
		if err != nil {
			return fmt.Errorf("query current password: %w", err)
		}
		if credentialVersion != identity.CredentialVersion || credentialVersion <= 0 {
			return ErrUnauthorized
		}
		if !VerifyPassword(currentPasswordHash, input.OldPassword) {
			return ErrIncorrectPassword
		}
		if VerifyPassword(currentPasswordHash, input.NewPassword) {
			return ErrSamePassword
		}
		passwordHash, err := HashPassword(input.NewPassword)
		if err != nil {
			return fmt.Errorf("hash changed password: %w", err)
		}
		now := time.Now().UnixMilli()
		result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET password = $1, credential_version = credential_version + 1, wrong = 0, updated_at = $2 WHERE id = $3 AND code = $4`, string(passwordHash), now, identity.UserID, identity.Code)
		if err != nil {
			return fmt.Errorf("update current password: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("update current password result: %w", err)
		}
		if updated != 1 {
			return ErrUnauthorized
		}
		return nil
	})
	if errors.Is(err, ErrIncorrectPassword) {
		failureCount, guardErr := m.passwordGuard.RecordFailure(ctx, identity.UserID)
		if guardErr != nil {
			return guardErr
		}
		if m.passwordGuard.LockRequired(failureCount) {
			if err := m.persistPasswordChangeLock(ctx, identity); err != nil {
				return err
			}
			return ErrPasswordChangeLocked
		}
		return &passwordChangeFailure{cause: ErrIncorrectPassword, captchaRequired: m.passwordGuard.CaptchaRequired(failureCount)}
	}
	if errors.Is(err, ErrSamePassword) && captchaRequired {
		return &passwordChangeFailure{cause: ErrSamePassword, captchaRequired: true}
	}
	if err != nil {
		return err
	}
	if err := m.passwordGuard.Clear(ctx, identity.UserID); err != nil {
		slog.WarnContext(ctx, fmt.Sprintf("clear password change failures for user %d failed: %v", identity.UserID, err), "operation", "clear_password_change_failures", "outcome", "failure", "error", err, "user_id", identity.UserID)
	}
	return nil
}

func (m *Module) passwordChangeLocked(ctx context.Context, identity authn.Identity) (bool, error) {
	const query = `SELECT CASE
		WHEN metadata ? 'password_change_failures' AND jsonb_typeof(metadata -> 'password_change_failures') = 'number'
		THEN (metadata ->> 'password_change_failures')::numeric >= $3
		ELSE false
	END FROM t_user WHERE id = $1 AND code = $2`
	var locked bool
	err := m.store.SQL(ctx).QueryRowContext(ctx, query, identity.UserID, identity.Code, m.passwordGuard.LockThreshold()).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrUnauthorized
	}
	if err != nil {
		return false, fmt.Errorf("query password change lock: %w", err)
	}
	return locked, nil
}

func (m *Module) persistPasswordChangeLock(ctx context.Context, identity authn.Identity) error {
	now := time.Now().UnixMilli()
	result, err := m.store.SQL(ctx).ExecContext(ctx, `UPDATE t_user SET metadata = jsonb_set(metadata, '{password_change_failures}', to_jsonb($1::bigint), true), updated_at = $2 WHERE id = $3 AND code = $4`,
		m.passwordGuard.LockThreshold(), now, identity.UserID, identity.Code)
	if err != nil {
		return fmt.Errorf("persist password change lock: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("persist password change lock result: %w", err)
	}
	if updated != 1 {
		return ErrUnauthorized
	}
	if err := m.passwordGuard.Clear(ctx, identity.UserID); err != nil {
		slog.WarnContext(ctx, fmt.Sprintf("clear locked password change failures for user %d failed: %v", identity.UserID, err), "operation", "clear_locked_password_change_failures", "outcome", "failure", "error", err, "user_id", identity.UserID)
	}
	return nil
}
