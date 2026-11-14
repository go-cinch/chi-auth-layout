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
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

// RecoverTOTPLogin uses a password-verified ticket to replace lost Verification credentials.
// It never completes a login or grants business/sensitive-operation access.
func (m *Module) RecoverTOTPLogin(ctx context.Context, ticket, recoveryCode string, passwords ...string) (*LoginResult, error) {
	if !m.totpEnabled() {
		return nil, ErrVerificationUnavailable
	}
	if len(ticket) < 32 || len(ticket) > 128 {
		return nil, ErrVerificationInvalid
	}
	var pending mfaPending
	if err := m.verification.get(ctx, loginTicketPrefix+ticket, &pending, false); err != nil {
		return nil, err
	}
	if !pending.Identity.Methods.Valid() || pending.ExpiredAt <= time.Now().UnixMilli() {
		return nil, ErrVerificationInvalid
	}
	auditctx.Identity(ctx, pending.Identity.UserID, pending.Identity.Username)
	if err := m.verification.attempt(ctx, pending.Identity); err != nil {
		return nil, err
	}
	if pending.Identity.Methods.Primary == authn.EmailCode {
		if len(passwords) != 1 || strings.TrimSpace(passwords[0]) == "" {
			return nil, ErrPasswordVerificationRequired
		}
		user, err := m.loginUser(ctx, pending.Identity.Username)
		if err != nil {
			return nil, err
		}
		if user.identity.CredentialVersion != pending.Identity.CredentialVersion || user.status != userStatusActive {
			return nil, ErrUnauthorized
		}
		if !VerifyPassword(user.passwordHash, passwords[0]) {
			return nil, ErrIncorrectPassword
		}
	}

	if !validRecoveryCode(recoveryCode) {
		return nil, ErrVerificationInvalid
	}
	return m.replaceLostFactor(ctx, ticket, pending, true, func(tx context.Context, identity authn.Identity) (bool, error) {
		return m.verifyRecoveryCode(tx, identity, recoveryCode)
	})
}

func validRecoveryCode(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 128
}

func (m *Module) verifyRecoveryCode(txCtx context.Context, identity authn.Identity, recoveryCode string) (bool, error) {
	var recoveryID int64
	err := m.store.SQL(txCtx).QueryRowContext(txCtx, `SELECT recovery.id FROM t_user_recovery_code AS recovery JOIN t_user_totp AS factor ON factor.user_id = recovery.user_id WHERE recovery.user_id = $1 AND recovery.code_hash = $2 FOR UPDATE OF recovery`, identity.UserID, recoveryDigest(recoveryCode)).Scan(&recoveryID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrVerificationInvalid
	}
	return err == nil, err
}

func (m *Module) replaceLostFactor(ctx context.Context, ticket string, pending mfaPending, byRecoveryCode bool, verify func(context.Context, authn.Identity) (bool, error)) (*LoginResult, error) {
	return m.replaceVerifiedFactor(ctx, pending, byRecoveryCode, func(txCtx context.Context, identity authn.Identity) (bool, error) {
		valid, err := verify(txCtx, identity)
		if err != nil || !valid {
			return false, err
		}
		// Consume the ticket only after the code is valid. The user row lock and
		// atomic Take serialize recovery with TOTP login and other recovery tickets.
		var consumed mfaPending
		if err := m.verification.get(txCtx, loginTicketPrefix+ticket, &consumed, true); err != nil {
			return false, err
		}
		if consumed != pending || consumed.ExpiredAt <= time.Now().UnixMilli() {
			return false, ErrVerificationInvalid
		}
		return true, nil
	})
}

func (m *Module) replaceVerifiedFactor(ctx context.Context, pending mfaPending, byRecoveryCode bool, verify func(context.Context, authn.Identity) (bool, error)) (*LoginResult, error) {
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
		valid, err := verify(txCtx, identity)
		if err != nil || !valid {
			return err
		} // commit incorrect email-code attempts without changing the factor
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `DELETE FROM t_user_recovery_code WHERE user_id = $1`, identity.UserID); err != nil {
			return err
		}
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `DELETE FROM t_user_totp WHERE user_id = $1`, identity.UserID); err != nil {
			return err
		}
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `UPDATE t_user SET credential_version = credential_version + 1, totp_recovery_pending = $3, updated_at = $1 WHERE id = $2`, time.Now().UnixMilli(), identity.UserID, byRecoveryCode); err != nil {
			return err
		}
		identity.CredentialVersion++
		identity.Methods.SecondFactor = ""
		identity.PasswordResetRequired = loginCount == 0
		result, newSessionID, err = m.issueLoginResult(txCtx, identity, pending.RememberMe)
		if err != nil {
			return err
		}
		result.TOTPEnrollmentRequired = true
		return m.grantTOTPEnrollment(txCtx, identity, newSessionID)
	})
	if err != nil {
		if newSessionID != "" {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if revokeErr := m.sessions.Revoke(cleanupCtx, newSessionID); revokeErr != nil {
				err = errors.Join(err, fmt.Errorf("revoke provisional Verification recovery session: %w", revokeErr))
			}
			if _, grantErr := m.verification.state.Take(cleanupCtx, "enrollment-grant:"+newSessionID); grantErr != nil && !errors.Is(grantErr, ErrVerificationInvalid) {
				err = errors.Join(err, fmt.Errorf("remove provisional Verification enrollment grant: %w", grantErr))
			}
		}
		return nil, err
	}
	if result == nil {
		return nil, ErrEmailInvalid
	}
	// The committed credential version gives this session a fresh counter even
	// if Redis cleanup fails or an old request arrives after cleanup.
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if resetErr := m.verification.state.ResetAttempts(cleanupCtx, verificationAttemptKey(pending.Identity)); resetErr != nil {
		slog.WarnContext(ctx, fmt.Sprintf("clear previous verification attempt counter for user %d version %d after recovery failed: %v", pending.Identity.UserID, pending.Identity.CredentialVersion, resetErr), "operation", "clear_totp_recovery_attempts", "outcome", "failure", "user_id", pending.Identity.UserID, "credential_version", pending.Identity.CredentialVersion, "error", resetErr)
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
