package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/auditctx"
)

// RecoverMFALogin uses a password-verified ticket to replace lost MFA credentials.
// It never completes a login or grants business/sensitive-operation access.
func (m *Module) RecoverMFALogin(ctx context.Context, ticket, recoveryCode string) (*LoginResult, error) {
	if !m.mfaEnabled() {
		return nil, ErrMFAUnavailable
	}
	if len(ticket) < 32 || len(ticket) > 128 {
		return nil, ErrMFAInvalid
	}
	var pending mfaPending
	if err := m.mfa.get(ctx, "login:"+ticket, &pending, false); err != nil {
		return nil, err
	}
	if pending.ExpiredAt <= time.Now().UnixMilli() {
		return nil, ErrMFAInvalid
	}
	auditctx.Identity(ctx, pending.Identity.UserID, pending.Identity.Username)
	if err := m.mfa.attempt(ctx, pending.Identity); err != nil {
		return nil, err
	}
	recoveryCode = strings.TrimSpace(recoveryCode)
	if recoveryCode == "" || len(recoveryCode) > 128 {
		return nil, ErrMFAInvalid
	}
	var result *LoginResult
	var newSessionID string
	err := m.store.Tx(ctx, func(txCtx context.Context) error {
		identity := pending.Identity
		var version, loginCount int64
		var status int16
		err := m.store.SQL(txCtx).QueryRowContext(txCtx, `SELECT credential_version, status, login_count FROM t_user WHERE id = $1 AND code = $2 FOR UPDATE`, identity.UserID, identity.Code).Scan(&version, &status, &loginCount)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if version != identity.CredentialVersion || status != userStatusActive {
			return ErrUnauthorized
		}
		var recoveryID int64
		err = m.store.SQL(txCtx).QueryRowContext(txCtx, `SELECT recovery.id FROM t_user_recovery_code AS recovery JOIN t_user_totp AS factor ON factor.user_id = recovery.user_id WHERE recovery.user_id = $1 AND recovery.code_hash = $2 FOR UPDATE OF recovery`, identity.UserID, recoveryDigest(recoveryCode)).Scan(&recoveryID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMFAInvalid
		}
		if err != nil {
			return err
		}
		// Consume the ticket only after the code is valid. The user row lock and
		// atomic Take serialize recovery with TOTP login and other recovery tickets.
		var consumed mfaPending
		if err := m.mfa.get(txCtx, "login:"+ticket, &consumed, true); err != nil {
			return err
		}
		if consumed != pending || consumed.ExpiredAt <= time.Now().UnixMilli() {
			return ErrMFAInvalid
		}
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `DELETE FROM t_user_recovery_code WHERE user_id = $1`, identity.UserID); err != nil {
			return err
		}
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `DELETE FROM t_user_totp WHERE user_id = $1`, identity.UserID); err != nil {
			return err
		}
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `UPDATE t_user SET credential_version = credential_version + 1, updated_at = $1 WHERE id = $2`, time.Now().UnixMilli(), identity.UserID); err != nil {
			return err
		}
		identity.CredentialVersion++
		identity.PasswordResetRequired = loginCount == 0
		result, newSessionID, err = m.issueLoginResult(txCtx, identity, pending.RememberMe)
		if err != nil {
			return err
		}
		result.MFAEnrollmentRequired = true
		return m.grantMFAEnrollment(txCtx, identity, newSessionID)
	})
	if err != nil {
		if newSessionID != "" {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if revokeErr := m.sessions.Revoke(cleanupCtx, newSessionID); revokeErr != nil {
				err = errors.Join(err, fmt.Errorf("revoke provisional MFA recovery session: %w", revokeErr))
			}
			if _, grantErr := m.mfa.state.Take(cleanupCtx, "enrollment-grant:"+newSessionID); grantErr != nil && !errors.Is(grantErr, ErrMFAInvalid) {
				err = errors.Join(err, fmt.Errorf("remove provisional MFA enrollment grant: %w", grantErr))
			}
		}
		return nil, err
	}
	// The committed credential version gives this session a fresh counter even
	// if Redis cleanup fails or an old request arrives after cleanup.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if resetErr := m.mfa.state.ResetAttempts(cleanupCtx, mfaAttemptKey(pending.Identity)); resetErr != nil {
		slog.WarnContext(ctx, fmt.Sprintf("clear previous mfa attempt counter for user %d version %d after recovery failed: %v", pending.Identity.UserID, pending.Identity.CredentialVersion, resetErr), "operation", "clear_mfa_recovery_attempts", "outcome", "failure", "user_id", pending.Identity.UserID, "credential_version", pending.Identity.CredentialVersion, "error", resetErr)
	}
	return result, nil
}

func newRecoveryCodes() ([]string, error) {
	codes := make([]string, 10)
	for i := range codes {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		encoded := hex.EncodeToString(raw)
		codes[i] = encoded[:8] + "-" + encoded[8:16] + "-" + encoded[16:24] + "-" + encoded[24:]
	}
	return codes, nil
}

func recoveryDigest(value string) string {
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
