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
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrLoginMFARequired        = apperror.New("AUTH_LOGIN_MFA_REQUIRED", "complete login verification before continuing")
	ErrTOTPUserNotFound        = apperror.New("AUTH_TOTP_USER_NOT_FOUND", "user not found")
	ErrVerificationInvalid     = apperror.New("AUTH_VERIFICATION_INVALID", "invalid or already used verification code; use a new code")
	ErrStepUpRequired          = apperror.New("AUTH_STEP_UP_REQUIRED", "verify your identity before this sensitive operation")
	ErrTOTPEnrollmentRequired  = apperror.New("AUTH_TOTP_ENROLLMENT_REQUIRED", "bind an authenticator before continuing")
	ErrTOTPAlreadyBound        = apperror.New("AUTH_TOTP_ALREADY_BOUND", "a TOTP authenticator is already bound")
	ErrVerificationRateLimited = apperror.New("AUTH_VERIFICATION_RATE_LIMITED", "too many verification attempts; try again later")
	ErrVerificationUnavailable = apperror.New("AUTH_VERIFICATION_UNAVAILABLE", "verification requires an available Redis store")
)

type VerificationConfig struct {
	TOTPEnrollmentOptional bool
	LoginTicketTTL         time.Duration
	TOTPEnabled            bool
	TOTPDebug              bool
	EmailEnabled           bool
	TOTPPeriod             time.Duration
	TOTPEnrollmentTTL      time.Duration
	SensitiveLifetime      time.Duration
	RateLimitWindow        time.Duration
	RateLimitMaxAttempts   int64
	TOTPAccountNamePrefix  string
	Issuer                 string
	EncryptionKeyFile      string
	// A domain-separated key derived from the persisted JWT key is the local
	// default. Deployments can supply an independent base64-encoded 32-byte key.
	SigningKey string
}

type Verification struct {
	db     *db.Store
	state  VerificationStateStore
	cipher cipher.AEAD
	config VerificationConfig
}

func NewVerification(store *db.Store, state VerificationStateStore, cfg VerificationConfig) (*Verification, error) {
	if !cfg.TOTPEnabled && !cfg.EmailEnabled {
		return &Verification{db: store, state: state, config: cfg}, nil
	}
	return newVerification(store, state, cfg)
}

func newVerification(store *db.Store, state VerificationStateStore, cfg VerificationConfig) (*Verification, error) {
	if store == nil || strings.TrimSpace(cfg.Issuer) == "" || len(cfg.SigningKey) < 32 {
		return nil, errors.New("invalid verification configuration")
	}
	if cfg.LoginTicketTTL < time.Millisecond || cfg.LoginTicketTTL%time.Millisecond != 0 {
		return nil, errors.New("auth.mfa.loginTicketTTL must be a positive whole number of milliseconds")
	}
	if cfg.TOTPPeriod <= 0 || cfg.TOTPPeriod%time.Second != 0 {
		return nil, errors.New("auth.totp.period must be a positive whole number of seconds")
	}
	if cfg.TOTPEnrollmentTTL < time.Millisecond || cfg.TOTPEnrollmentTTL%time.Millisecond != 0 {
		return nil, errors.New("auth.totp.enrollmentTTL must be a positive whole number of milliseconds")
	}
	if cfg.SensitiveLifetime < time.Millisecond || cfg.SensitiveLifetime%time.Millisecond != 0 {
		return nil, errors.New("auth.verification.sensitiveLifetime must be a positive whole number of milliseconds")
	}
	if cfg.RateLimitWindow < time.Millisecond || cfg.RateLimitWindow%time.Millisecond != 0 {
		return nil, errors.New("auth.verification.rateLimit.window must be a positive whole number of milliseconds")
	}
	if cfg.RateLimitMaxAttempts <= 0 {
		return nil, errors.New("auth.verification.rateLimit.maxAttempts must be positive")
	}
	// Keep this cryptographic domain stable: changing it would strand stored TOTP secrets.
	mac := hmac.New(sha256.New, []byte(cfg.SigningKey))
	_, _ = mac.Write([]byte("auth/mfa/encryption/v1\x00" + cfg.Issuer))
	key := mac.Sum(nil)
	if cfg.EncryptionKeyFile != "" {
		raw, err := os.ReadFile(cfg.EncryptionKeyFile)
		if err != nil {
			return nil, fmt.Errorf("read verification encryption key: %w", err)
		}
		key, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil {
			return nil, errors.New("verification encryption key file must contain base64")
		}
	}
	aead, err := verificationCipher(key)
	if err != nil {
		return nil, err
	}
	return &Verification{db: store, state: state, cipher: aead, config: cfg}, nil
}

func (m *Verification) put(ctx context.Context, key string, value any, ttl time.Duration) error {
	if m.state == nil {
		return ErrVerificationUnavailable
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	sealed, err := sealVerification(m.cipher, string(raw), key)
	if err != nil {
		return err
	}
	return m.state.Put(ctx, key, sealed, ttl)
}

func (m *Verification) get(ctx context.Context, key string, value any, take bool) error {
	if m.state == nil {
		return ErrVerificationUnavailable
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
	raw, err = openVerification(m.cipher, raw, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), value); err != nil {
		return ErrVerificationInvalid
	}
	return nil
}

func verificationAttemptKey(identity authn.Identity) string {
	return strconv.FormatInt(identity.UserID, 10) + ":" + strconv.FormatInt(identity.CredentialVersion, 10)
}

func (m *Verification) attempt(ctx context.Context, identity authn.Identity) error {
	if m.state == nil {
		return ErrVerificationUnavailable
	}
	if identity.UserID <= 0 || identity.CredentialVersion <= 0 {
		return ErrUnauthorized
	}
	allowed, err := m.state.Attempt(ctx, verificationAttemptKey(identity), m.config.RateLimitWindow, m.config.RateLimitMaxAttempts)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrVerificationRateLimited
	}
	return nil
}

// lockIdentity serializes enrollment, reset, login verification and code
// consumption against credential changes. Call only inside the owning Tx.
func (m *Verification) lockIdentity(ctx context.Context, identity authn.Identity) error {
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
