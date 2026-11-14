package auth

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

type TOTPStatus struct {
	PasswordRequired   bool `json:"password_required"`
	Enabled            bool `json:"enabled"`
	EnrollmentRequired bool `json:"enrollment_required"`
}

type TOTPSetup struct {
	Secret    string `json:"secret"`
	URI       string `json:"uri"`
	ExpiredAt int64  `json:"expired_at"`
}

type TOTPEnrollment struct {
	*LoginResult
	RecoveryCodes []string `json:"recovery_codes"`
}

type TOTPTestResult struct {
	Verified bool `json:"verified"`
}

type enrollmentPending struct {
	Identity      authn.Identity `json:"identity"`
	Secret        string         `json:"secret,omitempty"`
	PeriodSeconds int64          `json:"period_seconds,omitempty"`
	ExpiredAt     int64          `json:"expired_at"`
}

// A bound account with a first-factor-only session must sign in again.
func (m *Module) totpEnrollmentRequired(ctx context.Context, identity authn.Identity) (bool, error) {
	required, _, err := m.totpEnrollmentPolicy(ctx, identity)
	return required, err
}

// Recovery of a previously enrolled factor cannot use the optional first-time skip.
func (m *Module) totpEnrollmentPolicy(ctx context.Context, identity authn.Identity) (required, canSkip bool, err error) {
	if !m.totpEnabled() {
		return false, false, nil
	}
	bound, err := m.verification.hasTOTP(ctx, identity.UserID)
	if err != nil {
		return false, false, err
	}
	if bound {
		if identity.Methods.SecondFactor != authn.TOTP {
			return false, false, ErrUnauthorized
		}
		return false, false, nil
	}
	if !m.verification.config.TOTPEnrollmentOptional {
		return true, false, nil
	}
	var skipped, everBound bool
	err = m.store.SQL(ctx).QueryRowContext(ctx, `SELECT metadata @> '{"totp_enrollment_skipped":true}'::jsonb, totp_ever_bound FROM t_user WHERE id=$1 AND code=$2 AND credential_version=$3`, identity.UserID, identity.Code, identity.CredentialVersion).Scan(&skipped, &everBound)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, ErrUnauthorized
	}
	if err != nil {
		return false, false, err
	}
	return !skipped || everBound, !everBound, nil
}

func (m *Module) TOTPStatus(ctx context.Context) (*TOTPStatus, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if m.verification == nil {
		return nil, ErrVerificationUnavailable
	}
	if !m.totpEnabled() {
		return &TOTPStatus{}, nil
	}
	enabled, err := m.verification.hasTOTP(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	canEnroll, err := m.canEnrollWithoutPassword(ctx, identity)
	if err != nil {
		return nil, err
	}
	result := &TOTPStatus{Enabled: enabled, EnrollmentRequired: !enabled, PasswordRequired: !canEnroll}
	return result, nil
}

// grantTOTPEnrollment is issued after password verification only when enrollment
// is mandatory. The grant has that session's absolute
// expiry; refresh cannot extend it, and credential changes invalidate it.
func (m *Module) grantTOTPEnrollment(ctx context.Context, identity authn.Identity, sessionID string) error {
	identity.SessionID = sessionID
	session, err := m.sessions.authenticatedSession(ctx, identity)
	if err != nil {
		return err
	}
	grant := enrollmentPending{Identity: identity, ExpiredAt: session.ExpiredAt}
	return m.verification.put(ctx, "enrollment-grant:"+sessionID, grant, time.Until(time.UnixMilli(session.ExpiredAt)))
}

func (m *Module) canEnrollWithoutPassword(ctx context.Context, identity authn.Identity) (bool, error) {
	if !m.totpEnabled() || identity.SessionID == "" {
		return false, nil
	}
	var grant enrollmentPending
	if err := m.verification.get(ctx, "enrollment-grant:"+identity.SessionID, &grant, false); err != nil {
		if errors.Is(err, ErrVerificationInvalid) {
			return false, nil
		}
		return false, err
	}
	grant.Identity.PasswordResetRequired = false
	identity.PasswordResetRequired = false
	return grant.Identity == identity && grant.ExpiredAt > time.Now().UnixMilli(), nil
}

func (m *Module) BeginTOTPEnrollment(ctx context.Context, password string) (*TOTPSetup, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if !m.totpEnabled() {
		return nil, ErrVerificationUnavailable
	}
	if err := m.verification.attempt(ctx, identity); err != nil {
		return nil, err
	}
	var hash string
	if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT password FROM t_user WHERE id = $1 AND credential_version = $2`, identity.UserID, identity.CredentialVersion).Scan(&hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnauthorized
		}
		return nil, err
	}
	if password == "" {
		allowed, err := m.canEnrollWithoutPassword(ctx, identity)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrIncorrectPassword
		}
	} else if !VerifyPassword(hash, password) {
		return nil, ErrIncorrectPassword
	}
	enabled, err := m.verification.hasTOTP(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	if enabled {
		return nil, ErrTOTPAlreadyBound
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return nil, err
	}
	pending := enrollmentPending{Identity: identity, Secret: secret, PeriodSeconds: int64(m.verification.config.TOTPPeriod / time.Second), ExpiredAt: time.Now().Add(m.verification.config.TOTPEnrollmentTTL).UnixMilli()}
	if err := m.verification.put(ctx, "enrollment:"+identity.SessionID, pending, m.verification.config.TOTPEnrollmentTTL); err != nil {
		return nil, err
	}
	accountName := identity.Username
	if prefix := strings.TrimSpace(m.verification.config.TOTPAccountNamePrefix); prefix != "" {
		accountName = prefix + " - " + identity.Username
	}
	return &TOTPSetup{Secret: secret, URI: totpURI(m.verification.config.Issuer, accountName, secret, m.verification.config.TOTPPeriod), ExpiredAt: pending.ExpiredAt}, nil
}

func (m *Module) ConfirmTOTPEnrollment(ctx context.Context, code string) (*TOTPEnrollment, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if !m.totpEnabled() {
		return nil, ErrVerificationUnavailable
	}
	if err := m.verification.attempt(ctx, identity); err != nil {
		return nil, err
	}
	var pending enrollmentPending
	if err := m.verification.get(ctx, "enrollment:"+identity.SessionID, &pending, false); err != nil {
		return nil, err
	}
	if pending.Identity != identity || pending.ExpiredAt <= time.Now().UnixMilli() {
		return nil, ErrVerificationInvalid
	}
	// The QR code fixes the period, even if another replica or a restart uses a new default.
	period, err := totpPeriod(pending.PeriodSeconds)
	if err != nil {
		return nil, err
	}
	step, err := m.verification.verifyTOTPCode(pending.Secret, strings.TrimSpace(code), time.Now(), -1, period)
	if err != nil {
		return nil, err
	}
	codes, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	ciphertext, err := sealVerification(m.verification.cipher, pending.Secret, "totp:"+strconv.FormatInt(identity.UserID, 10))
	if err != nil {
		return nil, err
	}
	session, err := m.sessions.authenticatedSession(ctx, identity)
	if err != nil {
		return nil, err
	}
	var result *LoginResult
	var newSessionID string
	err = m.store.Tx(ctx, func(txCtx context.Context) error {
		if err := m.verification.lockIdentity(txCtx, identity); err != nil {
			return err
		}
		enabled, err := m.verification.hasTOTP(txCtx, identity.UserID)
		if err != nil {
			return err
		}
		if enabled {
			return ErrTOTPAlreadyBound
		}
		now := time.Now().UnixMilli()
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `INSERT INTO t_user_totp (user_id, secret_ciphertext, last_used_step, period_seconds) VALUES ($1, $2, $3, $4)`, identity.UserID, ciphertext, step, pending.PeriodSeconds); err != nil {
			return err
		}
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `DELETE FROM t_user_recovery_code WHERE user_id = $1`, identity.UserID); err != nil {
			return err
		}
		for _, recovery := range codes {
			if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `INSERT INTO t_user_recovery_code (user_id, code_hash) VALUES ($1, $2)`, identity.UserID, recoveryDigest(recovery)); err != nil {
				return err
			}
		}
		if err := m.enrollmentNotice(txCtx, identity.UserID); err != nil {
			return err
		}
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `UPDATE t_user SET credential_version = credential_version + 1, totp_recovery_pending = FALSE, totp_ever_bound = TRUE, updated_at = $1 WHERE id = $2`, now, identity.UserID); err != nil {
			return err
		}
		identity.CredentialVersion++
		identity.Methods.SecondFactor = authn.TOTP
		result, newSessionID, err = m.issueLoginResult(txCtx, identity, session.RememberMe)
		return err
	})
	if err != nil {
		if newSessionID != "" {
			_ = m.sessions.Revoke(context.WithoutCancel(ctx), newSessionID)
		}
		return nil, err
	}
	// Enrollment proves possession but never creates a sensitive-operation grant.
	return &TOTPEnrollment{LoginResult: result, RecoveryCodes: codes}, nil
}

func (m *Module) ResetUserTOTP(ctx context.Context, userID int64) error {
	if !m.totpEnabled() {
		return ErrVerificationUnavailable
	}
	if userID <= 0 {
		return ErrInvalid
	}
	// Page access is checked explicitly, independent of the new route binding.
	allowed, err := m.canResetTOTP(ctx)
	if err != nil {
		return err
	}
	if !allowed {
		return apperror.Forbidden
	}
	return m.store.MutationTx(ctx, func(txCtx context.Context) error {
		var id int64
		err := m.store.SQL(txCtx).QueryRowContext(txCtx, `UPDATE t_user SET credential_version = credential_version + 1, totp_recovery_pending = FALSE, updated_at = $1 WHERE id = $2 RETURNING id`, time.Now().UnixMilli(), userID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTOTPUserNotFound
		}
		if err != nil {
			return err
		}
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `DELETE FROM t_user_recovery_code WHERE user_id = $1`, userID); err != nil {
			return err
		}
		_, err = m.store.SQL(txCtx).ExecContext(txCtx, `DELETE FROM t_user_totp WHERE user_id = $1`, userID)
		return err
	})
}

// Reset authority follows the management page grant explicitly requested by the
// product, rather than requiring permission to edit user assignments/passwords.
func (m *Module) canResetTOTP(ctx context.Context) (bool, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return false, ErrUnauthorized
	}
	permissions, err := m.permissionsFor(ctx, identity.UserID, authn.PermissionScope(ctx), report.Fields{"menus"})
	if err != nil {
		return false, err
	}
	if permissions.PermissionScope == "*" {
		return true, nil
	}
	for _, menu := range permissions.Menus {
		if menu.MenuPath == "/system/user" {
			return true, nil
		}
	}
	return false, nil
}
