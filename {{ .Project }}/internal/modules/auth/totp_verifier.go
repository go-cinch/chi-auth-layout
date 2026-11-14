package auth

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func (m *Module) totpEnabled() bool {
	return m.verification != nil && m.verification.config.TOTPEnabled
}

func (m *Verification) hasTOTP(ctx context.Context, userID int64) (bool, error) {
	var enabled bool
	err := m.db.SQL(ctx).QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM t_user_totp WHERE user_id = $1)`, userID).Scan(&enabled)
	return enabled, err
}

// consumeTOTP verifies only an authenticator code and records its TOTP counter
// atomically. Recovery codes reset enrollment through a separate password-login flow.
func (m *Verification) consumeTOTP(ctx context.Context, identity authn.Identity, code string) error {
	if err := m.attempt(ctx, identity); err != nil {
		return err
	}
	return m.consumeTOTPCode(ctx, identity, code)
}

// consumeTOTPCode owns the shared replay boundary, independently of rate limits
// and sensitive-operation grants. All TOTP consumers lock the same rows.
func (m *Verification) consumeTOTPCode(ctx context.Context, identity authn.Identity, code string) error {
	return m.db.Tx(ctx, func(txCtx context.Context) error {
		if err := m.lockIdentity(txCtx, identity); err != nil {
			return err
		}
		return m.consumeLockedTOTPCode(txCtx, identity, code)
	})
}

// The caller holds the user row in the transaction that performs the protected action.
func (m *Verification) consumeLockedTOTPCode(txCtx context.Context, identity authn.Identity, code string) error {
	var ciphertext string
	var lastStep, periodSeconds int64
	err := m.db.SQL(txCtx).QueryRowContext(txCtx, `SELECT secret_ciphertext, last_used_step, period_seconds FROM t_user_totp WHERE user_id = $1 FOR UPDATE`, identity.UserID).Scan(&ciphertext, &lastStep, &periodSeconds)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTOTPEnrollmentRequired
	}
	if err != nil {
		return err
	}
	now := time.Now()
	code = strings.TrimSpace(code)
	secret, err := openVerification(m.cipher, ciphertext, "totp:"+strconv.FormatInt(identity.UserID, 10))
	if err != nil {
		return err
	}
	period, err := totpPeriod(periodSeconds)
	if err != nil {
		return err
	}
	step, err := m.verifyTOTPCode(secret, code, now, lastStep, period)
	if err != nil {
		return err
	}
	_, err = m.db.SQL(txCtx).ExecContext(txCtx, `UPDATE t_user_totp SET last_used_step = $1, updated_at = $2 WHERE user_id = $3`, step, now.UnixMilli(), identity.UserID)
	return err
}

// Debug codes are reusable without advancing or resetting the real TOTP replay
// counter, so disabling debug restores the existing authenticator normally.
func (m *Verification) verifyTOTPCode(secret, code string, now time.Time, lastStep int64, period time.Duration) (int64, error) {
	if m.config.TOTPDebug {
		if code != "999999" {
			return 0, ErrVerificationInvalid
		}
		return max(lastStep, 0), nil
	}
	return verifyTOTP(secret, code, now, lastStep, period)
}

// TestTOTP uses the shared attempt budget and replay boundary without creating,
// reading, extending, or clearing a sensitive-operation grant.
func (m *Module) TestTOTP(ctx context.Context, code string) (*TOTPTestResult, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.UserID <= 0 || identity.CredentialVersion <= 0 || identity.SessionID == "" {
		return nil, ErrUnauthorized
	}
	if identity.PasswordResetRequired {
		return nil, authn.ErrPasswordResetRequired
	}
	if !m.totpEnabled() {
		return nil, ErrVerificationUnavailable
	}
	if err := m.verification.consumeTOTP(ctx, identity, code); err != nil {
		return nil, err
	}
	return &TOTPTestResult{Verified: true}, nil
}

// In optional enrollment mode an unbound account has no authenticator method.
func (m *Module) availableTOTP(ctx context.Context, identity authn.Identity) (bool, error) {
	if !m.totpEnabled() {
		return false, nil
	}
	if !m.verification.config.TOTPEnrollmentOptional {
		return true, nil
	}
	return m.verification.hasTOTP(ctx, identity.UserID)
}
