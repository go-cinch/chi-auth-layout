package auth

import (
	"context"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrMFAUserNotFound       = apperror.New("AUTH_MFA_USER_NOT_FOUND", "user not found")
	ErrMFAInvalid            = apperror.New("AUTH_MFA_INVALID", "invalid or already used verification code; use a new code")
	ErrMFARequired           = apperror.New("AUTH_STEP_UP_REQUIRED", "verify two-factor authentication before this operation")
	ErrMFAEnrollmentRequired = apperror.New("AUTH_MFA_ENROLLMENT_REQUIRED", "bind an authenticator before continuing")
	ErrMFAAlreadyEnabled     = apperror.New("AUTH_MFA_ALREADY_ENABLED", "two-factor authentication is already enabled")
	ErrMFARateLimited        = apperror.New("AUTH_MFA_RATE_LIMITED", "too many verification attempts; try again later")
	ErrMFAUnavailable        = apperror.New("AUTH_MFA_UNAVAILABLE", "two-factor authentication requires an available Redis store")
)

type MFAConfig struct {
	LoginTicketTTL       time.Duration
	Enabled              bool
	TOTPPeriod           time.Duration
	EnrollmentTTL        time.Duration
	SensitiveLifetime    time.Duration
	RateLimitWindow      time.Duration
	RateLimitMaxAttempts int64
	AccountNamePrefix    string
	Issuer               string
	EncryptionKeyFile    string
	// A domain-separated key derived from the persisted JWT key is the local
	// default. Deployments can supply an independent base64-encoded 32-byte key.
	SigningKey string
}

type MFA struct {
	db     *db.Store
	state  MFAStateStore
	cipher cipher.AEAD
	config MFAConfig
}

type MFAStatus struct {
	PasswordRequired   bool  `json:"password_required"`
	Enabled            bool  `json:"enabled"`
	EnrollmentRequired bool  `json:"enrollment_required"`
	SensitiveExpiredAt int64 `json:"sensitive_expired_at"`
}

type MFASetup struct {
	Secret    string `json:"secret"`
	URI       string `json:"uri"`
	ExpiredAt int64  `json:"expired_at"`
}

type MFAEnrollment struct {
	*LoginResult
	RecoveryCodes []string `json:"recovery_codes"`
}

type MFATestResult struct {
	Verified bool `json:"verified"`
}

type mfaPending struct {
	Identity   authn.Identity `json:"identity"`
	RememberMe bool           `json:"remember_me"`
	Secret     string         `json:"secret,omitempty"`
	ExpiredAt  int64          `json:"expired_at"`
}

type MFAProof struct {
	UserID            int64  `json:"user_id"`
	SessionID         string `json:"session_id"`
	CredentialVersion int64  `json:"credential_version"`
	Fingerprint       string `json:"fingerprint"`
	VerifiedAt        int64  `json:"verified_at"`
	ExpiredAt         int64  `json:"expired_at"`
}

func NewMFA(store *db.Store, state MFAStateStore, cfg MFAConfig) (*MFA, error) {
	if !cfg.Enabled {
		return &MFA{config: cfg}, nil
	}
	if store == nil || strings.TrimSpace(cfg.Issuer) == "" || len(cfg.SigningKey) < 32 {
		return nil, errors.New("invalid mfa configuration")
	}
	if cfg.LoginTicketTTL < time.Millisecond || cfg.LoginTicketTTL%time.Millisecond != 0 {
		return nil, errors.New("auth.mfa.loginTicketTTL must be a positive whole number of milliseconds")
	}
	if cfg.TOTPPeriod <= 0 || cfg.TOTPPeriod%time.Second != 0 {
		return nil, errors.New("auth.mfa.totpPeriod must be a positive whole number of seconds")
	}
	if cfg.EnrollmentTTL < time.Millisecond || cfg.EnrollmentTTL%time.Millisecond != 0 {
		return nil, errors.New("auth.mfa.enrollmentTTL must be a positive whole number of milliseconds")
	}
	if cfg.SensitiveLifetime < time.Millisecond || cfg.SensitiveLifetime%time.Millisecond != 0 {
		return nil, errors.New("auth.mfa.sensitiveLifetime must be a positive whole number of milliseconds")
	}
	if cfg.RateLimitWindow < time.Millisecond || cfg.RateLimitWindow%time.Millisecond != 0 {
		return nil, errors.New("auth.mfa.rateLimit.window must be a positive whole number of milliseconds")
	}
	if cfg.RateLimitMaxAttempts <= 0 {
		return nil, errors.New("auth.mfa.rateLimit.maxAttempts must be positive")
	}
	mac := hmac.New(sha256.New, []byte(cfg.SigningKey))
	_, _ = mac.Write([]byte("auth/mfa/encryption/v1\x00" + cfg.Issuer))
	key := mac.Sum(nil)
	if cfg.EncryptionKeyFile != "" {
		raw, err := os.ReadFile(cfg.EncryptionKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read mfa encryption key: %w", err)
		}
		key, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return nil, errors.New("mfa encryption key file must contain base64")
		}
	}
	aead, err := mfaCipher(key)
	if err != nil {
		return nil, err
	}
	return &MFA{db: store, state: state, cipher: aead, config: cfg}, nil
}

func (m *Module) mfaEnabled() bool { return m.mfa != nil && m.mfa.config.Enabled }

func (m *MFA) enabled(ctx context.Context, userID int64) (bool, error) {
	var enabled bool
	err := m.db.SQL(ctx).QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM t_user_totp WHERE user_id = $1)`, userID).Scan(&enabled)
	return enabled, err
}

func (m *MFA) put(ctx context.Context, key string, value any, ttl time.Duration) error {
	if m.state == nil {
		return ErrMFAUnavailable
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	sealed, err := sealMFA(m.cipher, string(raw), key)
	if err != nil {
		return err
	}
	return m.state.Put(ctx, key, sealed, ttl)
}

func (m *MFA) get(ctx context.Context, key string, value any, take bool) error {
	if m.state == nil {
		return ErrMFAUnavailable
	}
	var raw string
	var err error
	if take {
		raw, err = m.state.Take(ctx, key)
	} else {
		raw, err = m.state.Get(ctx, key)
	}
	if err != nil {
		return err
	}
	raw, err = openMFA(m.cipher, raw, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), value); err != nil {
		return ErrMFAInvalid
	}
	return nil
}

func mfaAttemptKey(identity authn.Identity) string {
	return strconv.FormatInt(identity.UserID, 10) + ":" + strconv.FormatInt(identity.CredentialVersion, 10)
}

func (m *MFA) attempt(ctx context.Context, identity authn.Identity) error {
	if m.state == nil {
		return ErrMFAUnavailable
	}
	if identity.UserID <= 0 || identity.CredentialVersion <= 0 {
		return ErrUnauthorized
	}
	allowed, err := m.state.Attempt(ctx, mfaAttemptKey(identity), m.config.RateLimitWindow, m.config.RateLimitMaxAttempts)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrMFARateLimited
	}
	return nil
}

// lockIdentity serializes enrollment, reset, login verification and code
// consumption against credential changes. Call only inside the owning Tx.
func (m *MFA) lockIdentity(ctx context.Context, identity authn.Identity) error {
	var version int64
	var status int16
	err := m.db.SQL(ctx).QueryRowContext(ctx, `SELECT credential_version, status FROM t_user WHERE id = $1 AND code = $2 FOR UPDATE`, identity.UserID, identity.Code).Scan(&version, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnauthorized
	}
	if err != nil {
		return err
	}
	if version != identity.CredentialVersion || status != userStatusActive {
		return ErrUnauthorized
	}
	return nil
}

// consume verifies only an authenticator code and records its TOTP counter
// atomically. Recovery codes reset enrollment through a separate password-login flow.
func (m *MFA) consume(ctx context.Context, identity authn.Identity, code string) error {
	if err := m.attempt(ctx, identity); err != nil {
		return err
	}
	return m.consumeCode(ctx, identity, code)
}

// consumeCode owns the shared replay boundary, independently of rate limits
// and sensitive-operation grants. All TOTP consumers lock the same rows.
func (m *MFA) consumeCode(ctx context.Context, identity authn.Identity, code string) error {
	return m.db.Tx(ctx, func(txCtx context.Context) error {
		if err := m.lockIdentity(txCtx, identity); err != nil {
			return err
		}
		var ciphertext string
		var lastStep int64
		err := m.db.SQL(txCtx).QueryRowContext(txCtx, `SELECT secret_ciphertext, last_used_step FROM t_user_totp WHERE user_id = $1 FOR UPDATE`, identity.UserID).Scan(&ciphertext, &lastStep)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMFAEnrollmentRequired
		}
		if err != nil {
			return err
		}
		now := time.Now()
		code = strings.TrimSpace(code)
		secret, err := openMFA(m.cipher, ciphertext, "totp:"+strconv.FormatInt(identity.UserID, 10))
		if err != nil {
			return err
		}
		step, err := verifyTOTP(secret, code, now, lastStep, m.config.TOTPPeriod)
		if err != nil {
			return err
		}
		_, err = m.db.SQL(txCtx).ExecContext(txCtx, `UPDATE t_user_totp SET last_used_step = $1, updated_at = $2 WHERE user_id = $3`, step, now.UnixMilli(), identity.UserID)
		return err
	})
}

// TestMFA uses the shared attempt budget and replay boundary without creating,
// reading, extending, or clearing a sensitive-operation grant.
func (m *Module) TestMFA(ctx context.Context, code string) (*MFATestResult, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.UserID <= 0 || identity.CredentialVersion <= 0 || identity.SessionID == "" {
		return nil, ErrUnauthorized
	}
	if identity.PasswordResetRequired {
		return nil, authn.ErrPasswordResetRequired
	}
	if !m.mfaEnabled() {
		return nil, ErrMFAUnavailable
	}
	if err := m.mfa.consume(ctx, identity, code); err != nil {
		return nil, err
	}
	return &MFATestResult{Verified: true}, nil
}

func (m *Module) mfaEnrollmentRequired(ctx context.Context, identity authn.Identity) (bool, error) {
	if !m.mfaEnabled() {
		return false, nil
	}
	enabled, err := m.mfa.enabled(ctx, identity.UserID)
	return !enabled, err
}

func (m *Module) MFAStatus(ctx context.Context) (*MFAStatus, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if m.mfa == nil {
		return nil, ErrMFAUnavailable
	}
	if !m.mfaEnabled() {
		return &MFAStatus{}, nil
	}
	enabled, err := m.mfa.enabled(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	canEnroll, err := m.canEnrollWithoutPassword(ctx, identity)
	if err != nil {
		return nil, err
	}
	result := &MFAStatus{Enabled: enabled, EnrollmentRequired: !enabled, PasswordRequired: !canEnroll}
	proof, err := m.mfa.proof(ctx, identity)
	if err == nil {
		result.SensitiveExpiredAt = proof.ExpiredAt
	} else if !errors.Is(err, ErrMFARequired) {
		return nil, err
	}
	return result, nil
}

// grantMFAEnrollment is issued after password verification only when enrollment
// is mandatory. The grant has that session's absolute
// expiry; refresh cannot extend it, and credential changes invalidate it.
func (m *Module) grantMFAEnrollment(ctx context.Context, identity authn.Identity, sessionID string) error {
	identity.SessionID = sessionID
	session, err := m.sessions.authenticatedSession(ctx, identity)
	if err != nil {
		return err
	}
	grant := mfaPending{Identity: identity, ExpiredAt: session.ExpiredAt}
	return m.mfa.put(ctx, "enrollment-grant:"+sessionID, grant, time.Until(time.UnixMilli(session.ExpiredAt)))
}

func (m *Module) canEnrollWithoutPassword(ctx context.Context, identity authn.Identity) (bool, error) {
	if !m.mfaEnabled() || identity.SessionID == "" {
		return false, nil
	}
	var grant mfaPending
	if err := m.mfa.get(ctx, "enrollment-grant:"+identity.SessionID, &grant, false); err != nil {
		if errors.Is(err, ErrMFAInvalid) {
			return false, nil
		}
		return false, err
	}
	grant.Identity.PasswordResetRequired = false
	identity.PasswordResetRequired = false
	return grant.Identity == identity && grant.ExpiredAt > time.Now().UnixMilli(), nil
}

func (m *Module) BeginMFAEnrollment(ctx context.Context, password string) (*MFASetup, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if !m.mfaEnabled() {
		return nil, ErrMFAUnavailable
	}
	if err := m.mfa.attempt(ctx, identity); err != nil {
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
	enabled, err := m.mfa.enabled(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}
	if enabled {
		return nil, ErrMFAAlreadyEnabled
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return nil, err
	}
	pending := mfaPending{Identity: identity, Secret: secret, ExpiredAt: time.Now().Add(m.mfa.config.EnrollmentTTL).UnixMilli()}
	if err := m.mfa.put(ctx, "enrollment:"+identity.SessionID, pending, m.mfa.config.EnrollmentTTL); err != nil {
		return nil, err
	}
	accountName := identity.Username
	if prefix := strings.TrimSpace(m.mfa.config.AccountNamePrefix); prefix != "" {
		accountName = prefix + " - " + identity.Username
	}
	return &MFASetup{Secret: secret, URI: totpURI(m.mfa.config.Issuer, accountName, secret, m.mfa.config.TOTPPeriod), ExpiredAt: pending.ExpiredAt}, nil
}

func (m *Module) ConfirmMFAEnrollment(ctx context.Context, code string) (*MFAEnrollment, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	if !m.mfaEnabled() {
		return nil, ErrMFAUnavailable
	}
	if err := m.mfa.attempt(ctx, identity); err != nil {
		return nil, err
	}
	var pending mfaPending
	if err := m.mfa.get(ctx, "enrollment:"+identity.SessionID, &pending, false); err != nil {
		return nil, err
	}
	if pending.Identity != identity || pending.ExpiredAt <= time.Now().UnixMilli() {
		return nil, ErrMFAInvalid
	}
	step, err := verifyTOTP(pending.Secret, strings.TrimSpace(code), time.Now(), -1, m.mfa.config.TOTPPeriod)
	if err != nil {
		return nil, err
	}
	codes, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}
	ciphertext, err := sealMFA(m.mfa.cipher, pending.Secret, "totp:"+strconv.FormatInt(identity.UserID, 10))
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
		if err := m.mfa.lockIdentity(txCtx, identity); err != nil {
			return err
		}
		enabled, err := m.mfa.enabled(txCtx, identity.UserID)
		if err != nil {
			return err
		}
		if enabled {
			return ErrMFAAlreadyEnabled
		}
		now := time.Now().UnixMilli()
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `INSERT INTO t_user_totp (user_id, secret_ciphertext, last_used_step) VALUES ($1, $2, $3)`, identity.UserID, ciphertext, step); err != nil {
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
		if _, err := m.store.SQL(txCtx).ExecContext(txCtx, `UPDATE t_user SET credential_version = credential_version + 1, updated_at = $1 WHERE id = $2`, now, identity.UserID); err != nil {
			return err
		}
		identity.CredentialVersion++
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
	return &MFAEnrollment{LoginResult: result, RecoveryCodes: codes}, nil
}

func (m *Module) beginMFALogin(ctx context.Context, identity authn.Identity, remember bool) (*LoginResult, error) {
	ticket, err := randomSessionValue()
	if err != nil {
		return nil, err
	}
	pending := mfaPending{Identity: identity, RememberMe: remember, ExpiredAt: time.Now().Add(m.mfa.config.LoginTicketTTL).UnixMilli()}
	if err := m.mfa.put(ctx, "login:"+ticket, pending, m.mfa.config.LoginTicketTTL); err != nil {
		return nil, err
	}
	return &LoginResult{MFARequired: true, MFATicket: ticket, ExpiredAt: pending.ExpiredAt}, nil
}

func (m *Module) CompleteMFALogin(ctx context.Context, ticket, code string) (*LoginResult, error) {
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
	// The continuation has no username in its HTTP body. Attribute every
	// verification outcome to the identity from the validated server-side ticket.
	auditctx.Identity(ctx, pending.Identity.UserID, pending.Identity.Username)
	if strings.TrimSpace(code) == "" {
		return nil, ErrMFARequired
	}
	if err := m.mfa.consume(ctx, pending.Identity, code); err != nil {
		return nil, err
	}
	// Only the winner may issue a session, even with concurrent factor verification.
	var consumed mfaPending
	if err := m.mfa.get(ctx, "login:"+ticket, &consumed, true); err != nil {
		return nil, err
	}
	return m.finishLogin(ctx, pending.Identity, pending.RememberMe, false)
}

func (m *MFA) proof(ctx context.Context, identity authn.Identity) (*MFAProof, error) {
	var proof MFAProof
	err := m.get(ctx, "sensitive:"+identity.SessionID, &proof, false)
	if errors.Is(err, ErrMFAInvalid) {
		return nil, ErrMFARequired
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if proof.UserID != identity.UserID || proof.SessionID != identity.SessionID || proof.CredentialVersion != identity.CredentialVersion || proof.Fingerprint == "" || proof.VerifiedAt > now || proof.ExpiredAt <= now || proof.ExpiredAt-proof.VerifiedAt != m.config.SensitiveLifetime.Milliseconds() {
		return nil, ErrMFARequired
	}
	return &proof, nil
}

func (m *Module) clearSensitiveMFA(ctx context.Context, sessionID string) error {
	if m.mfa == nil || m.mfa.state == nil || !validSessionID(sessionID) {
		return nil
	}
	_, err := m.mfa.state.Take(ctx, "sensitive:"+sessionID)
	if err == nil || errors.Is(err, ErrMFAInvalid) {
		return nil
	}
	return fmt.Errorf("clear sensitive verification: %w", err)
}

func (m *Module) validateMFASession(ctx context.Context, identity authn.Identity) error {
	_, err := m.sessions.authenticatedSession(ctx, identity)
	if errors.Is(err, ErrSessionNotFound) {
		return ErrUnauthorized
	}
	return err
}

func (m *Module) VerifySensitiveMFA(ctx context.Context, code string) (*MFAProof, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.PasswordResetRequired {
		return nil, ErrUnauthorized
	}
	if !m.mfaEnabled() {
		return nil, ErrMFAUnavailable
	}
	if err := m.validateMFASession(ctx, identity); err != nil {
		return nil, err
	}
	if err := m.mfa.consume(ctx, identity, code); err != nil {
		return nil, err
	}
	fingerprint, err := randomSessionValue()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	proof := &MFAProof{UserID: identity.UserID, SessionID: identity.SessionID, CredentialVersion: identity.CredentialVersion, Fingerprint: fingerprint, VerifiedAt: now.UnixMilli(), ExpiredAt: now.Add(m.mfa.config.SensitiveLifetime).UnixMilli()}
	if err := m.mfa.put(ctx, "sensitive:"+identity.SessionID, proof, m.mfa.config.SensitiveLifetime); err != nil {
		return nil, err
	}
	// An already authenticated verification can finish after logout. Recheck
	// after publishing so that a late writer withdraws the revoked session's
	// grant, while a later logout removes a grant published before revocation.
	if err := m.validateMFASession(ctx, identity); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return nil, errors.Join(err, m.clearSensitiveMFA(cleanupCtx, identity.SessionID))
	}
	auditctx.MFA(ctx, proof.Fingerprint, proof.VerifiedAt)
	return proof, nil
}

func (m *Module) ResetUserMFA(ctx context.Context, userID int64) error {
	if !m.mfaEnabled() {
		return ErrMFAUnavailable
	}
	if userID <= 0 {
		return ErrInvalid
	}
	// Page access is checked explicitly, independent of the new route binding.
	allowed, err := m.canResetMFA(ctx)
	if err != nil {
		return err
	}
	if !allowed {
		return apperror.Forbidden
	}
	return m.store.Tx(ctx, func(txCtx context.Context) error {
		var id int64
		err := m.store.SQL(txCtx).QueryRowContext(txCtx, `UPDATE t_user SET credential_version = credential_version + 1, updated_at = $1 WHERE id = $2 RETURNING id`, time.Now().UnixMilli(), userID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMFAUserNotFound
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
func (m *Module) canResetMFA(ctx context.Context) (bool, error) {
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
