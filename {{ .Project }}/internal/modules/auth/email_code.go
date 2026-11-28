package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/infra/db"
)

var (
	ErrEmailUnavailable             = apperror.New("AUTH_EMAIL_UNAVAILABLE", "email delivery is unavailable; use password login or try again later")
	ErrEmailInvalid                 = apperror.New("AUTH_EMAIL_CODE_INVALID", "verification code is invalid or expired")
	ErrEmailAddress                 = apperror.New("AUTH_EMAIL_ADDRESS_INVALID", "enter a valid email address")
	ErrEmailOccupied                = apperror.New("AUTH_EMAIL_OCCUPIED", "this email cannot be used for binding")
	ErrEmailCooldown                = apperror.New("AUTH_EMAIL_COOLDOWN", "wait before requesting another verification code")
	ErrEmailBindingRequired         = apperror.New("AUTH_EMAIL_BINDING_REQUIRED", "bind an email before continuing")
	ErrPasswordVerificationRequired = apperror.New("AUTH_PASSWORD_VERIFICATION_REQUIRED", "confirm your current password before recovering the authenticator")
	browserUUIDPattern              = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
)

type EmailSender interface {
	Ready() bool
	SendHTML(context.Context, string, string, string, string) error
}

type EmailConfig struct {
	HistoryRetentionDays int
	Templates            EmailTemplateIDs
	AuditURL             string
	Debug                bool
	TestingQueueEndpoint bool
	TestingQueueToken    string
	UnlinkURL            string
	RequireBinding       bool
	CodeTTL              time.Duration
	ResendInterval       time.Duration
	MaxAttempts          int
	SendLimit            int
	SensitiveLifetime    time.Duration
	AlertTiers           []AlertTier
}

type EmailCodes struct {
	store     *db.Store
	sender    EmailSender
	templates EmailTemplateRenderer
	config    EmailConfig
	key       []byte
	sentQueue *sentEmailQueue
}

type EmailCodeStatus struct {
	ServerTime     int64  `json:"server_time"`
	RetryAt        int64  `json:"retry_at"`
	VerificationID string `json:"verification_id,omitempty"`
	ExpiredAt      int64  `json:"expired_at,omitempty"`
}

func NewEmailCodes(store *db.Store, sender EmailSender, cfg EmailConfig, key string) (*EmailCodes, error) {
	if cfg.HistoryRetentionDays == 0 {
		cfg.HistoryRetentionDays = 30
	}
	// Bound conversion to time.Duration before multiplying by a day.
	if cfg.HistoryRetentionDays < 1 || cfg.HistoryRetentionDays > 106751 {
		return nil, errors.New("auth.history.retentionDays must be between 1 and 106751")
	}
	if store == nil || sender == nil || len(key) < 32 || cfg.CodeTTL <= 0 || cfg.CodeTTL > 30*time.Minute || cfg.ResendInterval < time.Second || cfg.MaxAttempts < 1 || cfg.MaxAttempts > 20 || cfg.SendLimit < 1 {
		return nil, errors.New("invalid email verification configuration")
	}
	if cfg.UnlinkURL != "" && !validUnlinkURL(cfg.UnlinkURL) {
		return nil, errors.New("invalid auth.email.unlinkURL")
	}
	if cfg.AuditURL != "" && !validUnlinkURL(cfg.AuditURL) {
		return nil, errors.New("invalid auth.email.auditURL")
	}
	if err := validateAlertTiers(cfg.AlertTiers); err != nil {
		return nil, err
	}
	var sentQueue *sentEmailQueue
	if cfg.TestingQueueEndpoint && sender.Ready() {
		sentQueue = newSentEmailQueue(cfg.TestingQueueToken)
	}
	cfg.TestingQueueToken = ""
	result := &EmailCodes{store: store, sender: sender, config: cfg, key: []byte(key), sentQueue: sentQueue}
	return result, nil
}

// AE001 is raised only when the ticket trigger cannot insert its River job.
func emailInsertError(err error) error {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) && pgErr.Code == "AE001" {
		return ErrEmailUnavailable
	}
	return err
}

func (m *Module) SetEmailCodes(codes *EmailCodes) { m.emailCodes = codes }

func normalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	_, domain, _ := strings.Cut(value, "@")
	dot := strings.LastIndexByte(domain, '.')
	if err != nil || address.Address != value || address.Name != "" || len(value) > 254 || dot <= 0 || dot == len(domain)-1 {
		return "", ErrEmailAddress
	}
	return value, nil
}

func validEmailContext(uuid, purpose, identifier string) bool {
	return browserUUIDPattern.MatchString(uuid) && (purpose == "login" || purpose == "bind" || purpose == "unlink" || purpose == "sensitive" || purpose == "totp_reset") && strings.TrimSpace(identifier) != ""
}

func (e *EmailCodes) clientIP(r *http.Request) string { return server.RequestClientIP(r) }

func (e *EmailCodes) mac(values ...string) []byte {
	mac := hmac.New(sha256.New, e.key)
	_, _ = mac.Write([]byte("auth-email\x00" + strings.Join(values, "\x00")))
	return mac.Sum(nil)
}

func (e *EmailCodes) digest(values ...string) string {
	return hex.EncodeToString(e.mac(values...))
}

func (e *EmailCodes) cooldownKeys(uuid, ip, recipient string) []string {
	keys := []string{"uuid:" + uuid, "ip:" + ip}
	if recipient != "" {
		keys = append(keys, "recipient:"+e.digest(recipient))
	}
	sort.Strings(keys)
	return keys
}

func (e *EmailCodes) retryAt(ctx context.Context, keys []string) (int64, error) {
	var deadline int64
	err := e.store.SQL(ctx).QueryRowContext(ctx, `SELECT COALESCE(MAX(retry_at), 0) FROM t_auth_email_cooldown WHERE cooldown_key = ANY($1)`, pq.Array(keys)).Scan(&deadline)
	return deadline, err
}

func (e *EmailCodes) Status(ctx context.Context, uuid, ip, purpose, identifier string, userID int64) (*EmailCodeStatus, error) {
	if !validEmailContext(uuid, purpose, identifier) {
		return nil, ErrInvalid
	}
	retryAt, err := e.retryAt(ctx, e.cooldownKeys(uuid, ip, ""))
	if err != nil {
		return nil, err
	}
	result := &EmailCodeStatus{ServerTime: time.Now().UnixMilli(), RetryAt: retryAt}
	// UUID restores only the pending UI context, never proves identity or code possession.
	err = e.store.SQL(ctx).QueryRowContext(ctx, `SELECT verification_id, expired_at FROM t_auth_email_code WHERE browser_uuid = $1 AND purpose = $2 AND identifier = $3 AND ($4 = 0 OR user_id = $4) AND (delivered OR delivery_attempts IS NOT NULL) AND consumed = FALSE AND expired_at > $5 ORDER BY created_at DESC LIMIT 1`, uuid, purpose, identifier, userID, result.ServerTime).Scan(&result.VerificationID, &result.ExpiredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	return result, err
}

func (e *EmailCodes) Send(ctx context.Context, uuid, ip, purpose, identifier, recipient string, userID, version int64) (*EmailCodeStatus, error) {
	if !validEmailContext(uuid, purpose, identifier) {
		return nil, ErrInvalid
	}
	if !e.sender.Ready() {
		return nil, ErrEmailUnavailable
	}
	token, err := randomSessionValue()
	if err != nil {
		return nil, err
	}
	result := &EmailCodeStatus{}
	keys := e.cooldownKeys(uuid, ip, recipient)
	err = e.store.Tx(ctx, func(txCtx context.Context) error {
		// Serialize link issuance with confirmation, fallback and address changes.
		if purpose == "unlink" || purpose == "sensitive" || purpose == "totp_reset" {
			var current string
			err := e.store.SQL(txCtx).QueryRowContext(txCtx, `SELECT email FROM t_user WHERE id=$1 AND credential_version=$2 AND email=$3 AND email_verified_at IS NOT NULL AND status=$4 FOR NO KEY UPDATE`, userID, version, recipient, userStatusActive).Scan(&current)
			if errors.Is(err, sql.ErrNoRows) {
				if purpose == "sensitive" {
					return ErrVerificationInvalid
				}
				return ErrUnlinkInvalid
			}
			if err != nil {
				return err
			}
		}
		// All instances and endpoints share the same locks and budgets.
		for _, key := range keys {
			if _, err := e.store.SQL(txCtx).ExecContext(txCtx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 39183))`, key); err != nil {
				return err
			}
		}
		now := time.Now().UnixMilli()
		result.ServerTime = now
		deadline, err := e.retryAt(txCtx, keys)
		if err != nil {
			return err
		}
		result.RetryAt = deadline
		if deadline > now {
			return ErrEmailCooldown
		}
		result.RetryAt = now + e.config.ResendInterval.Milliseconds()
		result.ExpiredAt = now + e.config.CodeTTL.Milliseconds()
		for _, key := range keys {
			if _, err := e.store.SQL(txCtx).ExecContext(txCtx, `INSERT INTO t_auth_email_cooldown (cooldown_key, updated_at, retry_at) VALUES ($1, $2, $3) ON CONFLICT (cooldown_key) DO UPDATE SET updated_at = EXCLUDED.updated_at, retry_at = EXCLUDED.retry_at`, key, now, result.RetryAt); err != nil {
				return err
			}
		}
		// A sensitive ticket belongs to its issuing session. Sending from a
		// second session (or browser sharing the UUID) must not cancel it.
		_, err = e.store.SQL(txCtx).ExecContext(txCtx, `UPDATE t_auth_email_code SET consumed = TRUE, updated_at = $1
			WHERE purpose = $2 AND NOT consumed AND (
				($2 = 'sensitive' AND user_id = $4 AND identifier = $5)
				OR ($2 <> 'sensitive' AND (($2 <> 'unlink' AND browser_uuid = $3) OR (user_id = $4 AND $4 <> 0)))
			)`, now, purpose, uuid, userID, identifier)
		if err != nil {
			return err
		}
		_, err = e.store.SQL(txCtx).ExecContext(txCtx, `INSERT INTO t_auth_email_code (verification_id, created_at, updated_at, browser_uuid, client_ip, purpose, identifier, user_id, recipient, credential_version, code_hash, expired_at, delivery_attempts) VALUES ($1, $2, $2, $3, $4, $5, $6, NULLIF($7, 0), $8, $9, $10, $11, 0)`, token, now, uuid, ip, purpose, identifier, userID, recipient, version, e.digest(token, e.verificationCode(token)), result.ExpiredAt)
		if err != nil {
			return emailInsertError(err)
		}
		return e.sealEmail(txCtx, token)
	})
	if errors.Is(err, ErrEmailCooldown) {
		return result, err
	}
	if err != nil {
		return nil, err
	}
	result.ServerTime = time.Now().UnixMilli()
	result.VerificationID = token
	return result, nil
}

// Consume joins the caller's transaction. Invalid attempts must be committed,
// so valid=false is a business outcome, not a rollback-producing error.
func (e *EmailCodes) Consume(ctx context.Context, id, uuid, purpose, identifier, recipient, code string, userID, version int64) (bool, error) {
	if !validEmailContext(uuid, purpose, identifier) || len(code) != 6 || len(id) > 128 {
		return false, nil
	}
	var hash string
	var attempts int
	var expires int64
	var delivered, consumed bool
	err := e.store.SQL(ctx).QueryRowContext(ctx, `SELECT code_hash, attempts, expired_at, delivered, consumed FROM t_auth_email_code WHERE verification_id = $1 AND browser_uuid = $2 AND purpose = $3 AND identifier = $4 AND recipient = $5 AND user_id = $6 AND credential_version = $7 FOR UPDATE`, id, uuid, purpose, identifier, recipient, userID, version).Scan(&hash, &attempts, &expires, &delivered, &consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if consumed || !delivered || expires <= time.Now().UnixMilli() || attempts >= e.config.MaxAttempts {
		return false, nil
	}
	valid := hmac.Equal([]byte(hash), []byte(e.digest(id, code)))
	_, err = e.store.SQL(ctx).ExecContext(ctx, `UPDATE t_auth_email_code SET attempts = attempts + 1, consumed = $1, updated_at = $2 WHERE verification_id = $3`, valid || attempts+1 >= e.config.MaxAttempts, time.Now().UnixMilli(), id)
	return valid, err
}
