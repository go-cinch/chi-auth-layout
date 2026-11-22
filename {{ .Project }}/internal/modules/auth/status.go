package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
)

// StatusQuery selects one verification flow. Public and authenticated callers
// use different entry points; a purpose never changes the authentication mode.
type StatusQuery struct {
	Purpose     string
	Fields      report.Fields
	BrowserUUID string
	Identifier  string
	IP          string
	MFATicket   string
}

type StatusRequirements struct {
	TOTPOnboardingHidden           *bool `json:"totp_onboarding_hidden,omitempty"`
	PasswordResetOnboardingHidden *bool `json:"password_reset_onboarding_hidden,omitempty"`
	EmailOnboardingHidden         *bool `json:"email_onboarding_hidden,omitempty"`
	TOTPSkipAllowed                *bool `json:"totp_skip_allowed,omitempty"`
	PasswordResetReason           string `json:"password_reset_reason,omitempty"`
	PasswordResetSkipAllowed      *bool `json:"password_reset_skip_allowed,omitempty"`
	PasswordResetRequired         *bool `json:"password_reset_required,omitempty"`
	TOTPEnrollmentRequired         *bool `json:"totp_enrollment_required,omitempty"`
	EmailBindingRequired          *bool `json:"email_binding_required,omitempty"`
	CaptchaRequired               *bool `json:"captcha_required,omitempty"`
	EnrollmentRequired            *bool `json:"enrollment_required,omitempty"`
	PasswordRequired              *bool `json:"password_required,omitempty"`
}

type StatusMethods struct {
	TOTP         bool   `json:"totp"`
	Email        bool   `json:"email"`
	RecoveryCode bool   `json:"recovery_code"`
	Address      string `json:"address,omitempty"`
	Fallback     string `json:"fallback,omitempty"`
}

type StatusEmailCode struct {
	RetryAt        int64  `json:"retry_at"`
	VerificationID string `json:"verification_id,omitempty"`
	ExpiredAt      int64  `json:"expired_at,omitempty"`
	Completed      bool   `json:"completed"`
}

type StatusGrant struct {
	ExpiredAt int64 `json:"expired_at"`
}

type StatusResult struct {
	Purpose      string              `json:"purpose"`
	ServerTime   int64               `json:"server_time"`
	Requirements *StatusRequirements `json:"requirements,omitempty"`
	Methods      *StatusMethods      `json:"methods,omitempty"`
	EmailCode    *StatusEmailCode    `json:"email_code,omitempty"`
	Grant        *StatusGrant        `json:"grant,omitempty"`
}

func statusFields(purpose string, public bool) []string {
	if public {
		if purpose == "login_totp_recovery" {
			return []string{"methods", "email_code"}
		}
		if purpose == "login" {
			return []string{"requirements", "email_code"}
		}
		return nil
	}
	switch purpose {
	case "onboarding", "totp_enrollment":
		return []string{"requirements"}
	case "email_bind":
		return []string{"email_code"}
	case "email_unbind":
		return []string{"methods", "email_code"}
	case "sensitive":
		return []string{"methods", "grant", "email_code"}
	case "totp_recovery":
		return []string{"methods", "email_code"}
	default:
		return nil
	}
}

func (q StatusQuery) validate(public bool) error {
	allowed := statusFields(q.Purpose, public)
	if len(allowed) == 0 || len(q.Fields) == 0 {
		return ErrInvalid
	}
	if err := q.Fields.Validate(allowed...); err != nil {
		return ErrInvalid
	}
	if q.Fields.Has("email_code") && !browserUUIDPattern.MatchString(q.BrowserUUID) {
		return ErrInvalid
	}
	if q.Purpose == "login" && q.Fields.Has("requirements") && strings.TrimSpace(q.Identifier) == "" {
		return ErrInvalid
	}
	if q.Purpose == "email_bind" && q.Identifier != "" {
		if _, err := normalizeEmail(q.Identifier); err != nil {
			return err
		}
	}
	return nil
}

// PublicStatus is read-only: point-selection challenges are issued by POST
// /pub/captcha, never by a status refresh.
func (m *Module) PublicStatus(ctx context.Context, q StatusQuery) (*StatusResult, error) {
	if err := q.validate(true); err != nil {
		return nil, err
	}
	result := &StatusResult{Purpose: q.Purpose, ServerTime: time.Now().UnixMilli()}
	if q.Purpose == "login_totp_recovery" {
		if err := m.loginRecoveryStatus(ctx, q, result); err != nil {
			return nil, err
		}
		return result, nil
	}
	if q.Fields.Has("requirements") {
		required, err := m.loginCaptchaRequired(ctx, q.Identifier)
		if err != nil {
			return nil, err
		}
		result.Requirements = &StatusRequirements{CaptchaRequired: &required}
	}
	if q.Fields.Has("email_code") {
		if m.emailCodes == nil {
			return nil, ErrEmailUnavailable
		}
		if strings.TrimSpace(q.Identifier) == "" {
			return result, m.setMailboxCooldown(ctx, "", q, result)
		}
		state, err := m.emailCodes.Status(ctx, q.BrowserUUID, q.IP, "login", strings.TrimSpace(q.Identifier), 0)
		if err != nil {
			return nil, err
		}
		result.setEmailCode(state)
	}
	return result, nil
}

func (r *StatusResult) setEmailCode(state *EmailCodeStatus) {
	r.ServerTime = state.ServerTime
	r.EmailCode = &StatusEmailCode{RetryAt: state.RetryAt, VerificationID: state.VerificationID, ExpiredAt: state.ExpiredAt}
}

func (m *Module) Status(ctx context.Context, q StatusQuery) (*StatusResult, error) {
	if err := q.validate(false); err != nil {
		return nil, err
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	// Enrollment may need proof from an already verified mailbox. This permits
	// only verification metadata; other onboarding restrictions remain in force.
	if identity.PasswordResetRequired && q.Purpose != "totp_enrollment" && q.Purpose != "onboarding" && q.Purpose != "sensitive" {
		return nil, authn.ErrPasswordResetRequired
	}
	required, totpCanSkip, err := m.totpEnrollmentPolicy(ctx, identity)
	if err != nil {
		return nil, err
	}
	if identity.PasswordResetRequired && q.Purpose != "totp_enrollment" && q.Purpose != "onboarding" && !(required && q.Purpose == "sensitive") {
		return nil, authn.ErrPasswordResetRequired
	}
	if required && q.Purpose != "totp_enrollment" && q.Purpose != "onboarding" && q.Purpose != "sensitive" {
		return nil, ErrTOTPEnrollmentRequired
	}
	result := &StatusResult{Purpose: q.Purpose, ServerTime: time.Now().UnixMilli()}
	switch q.Purpose {
	case "onboarding":
		emailRequired := false
		if m.requiresEmailBinding() {
			var email string
			err := m.store.SQL(ctx).QueryRowContext(ctx, "SELECT COALESCE(email, '') FROM t_user WHERE id=$1 AND code=$2", identity.UserID, identity.Code).Scan(&email)
			if err != nil {
				return nil, err
			}
			emailRequired = email == ""
		}
		passwordReason, passwordCanSkip, err := m.passwordResetPolicy(ctx, identity)
		if err != nil {
			return nil, err
		}
		totpHidden := m.onboardingVisibility.TOTP && totpCanSkip
		passwordHidden := m.onboardingVisibility.PasswordReset && passwordCanSkip
		emailHidden := m.onboardingVisibility.Email && !m.requiresEmailBinding()
		result.Requirements = &StatusRequirements{PasswordResetReason: passwordReason, TOTPOnboardingHidden: &totpHidden, PasswordResetOnboardingHidden: &passwordHidden, EmailOnboardingHidden: &emailHidden, TOTPSkipAllowed: &totpCanSkip, PasswordResetSkipAllowed: &passwordCanSkip, PasswordResetRequired: &identity.PasswordResetRequired, TOTPEnrollmentRequired: &required, EmailBindingRequired: &emailRequired}
	case "totp_enrollment":
		passwordRequired := false
		if m.totpEnabled() {
			canEnroll, err := m.canEnrollWithoutPassword(ctx, identity)
			if err != nil {
				return nil, err
			}
			passwordRequired = !canEnroll
		}
		result.Requirements = &StatusRequirements{EnrollmentRequired: &required, PasswordRequired: &passwordRequired, TOTPSkipAllowed: &totpCanSkip}
	case "email_bind":
		if m.emailCodes == nil {
			return nil, ErrEmailUnavailable
		}
		if q.Identifier == "" {
			return result, m.setMailboxCooldown(ctx, "", q, result)
		}
		email, _ := normalizeEmail(q.Identifier)
		state, err := m.emailCodes.Status(ctx, q.BrowserUUID, q.IP, "bind", email, identity.UserID)
		if err != nil {
			return nil, err
		}
		result.setEmailCode(state)
	case "email_unbind":
		if err := m.unlinkFlowStatus(ctx, identity, q, result); err != nil {
			return nil, err
		}
	case "sensitive":
		if q.Fields.Has("email_code") {
			if err := m.mailboxCooldown(ctx, identity, q, result); err != nil {
				return nil, err
			}
		}
		if q.Fields.Has("methods") {
			methods, err := m.sensitiveMethods(ctx, identity, required)
			if err != nil {
				return nil, err
			}
			result.Methods = methods
		}
		if q.Fields.Has("grant") {
			result.Grant = &StatusGrant{}
			if m.totpEnabled() || m.sensitiveEmailEnabled() {
				proof, err := m.verification.proof(ctx, identity)
				if err != nil && !errors.Is(err, ErrStepUpRequired) {
					return nil, err
				}
				if err == nil {
					valid := m.totpEnabled()
					if proof.EmailBinding != "" {
						valid = false
						if m.sensitiveEmailEnabled() {
							email, err := m.sensitiveEmail(ctx, identity, false)
							if err != nil {
								return nil, err
							}
							valid = email != "" && m.emailCodes.digest("sensitive-binding", email) == proof.EmailBinding
						}
					}
					if valid {
						result.Grant.ExpiredAt = proof.ExpiredAt
					}
				}
			}
		}
	case "totp_recovery":
		if _, err := m.accountResetSession(ctx); err != nil {
			return nil, err
		}
		bound, err := m.verification.hasTOTP(ctx, identity.UserID)
		if err != nil {
			return nil, err
		}
		if !bound {
			return nil, ErrVerificationUnavailable
		}
		if q.Fields.Has("methods") {
			methods, err := m.sensitiveMethods(ctx, identity)
			if err != nil {
				return nil, err
			}
			methods.TOTP, methods.RecoveryCode = false, true
			result.Methods = methods
		}
		if q.Fields.Has("email_code") {
			if err := m.mailboxCooldown(ctx, identity, q, result); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func maskedEmail(address string) string {
	at := strings.LastIndexByte(address, '@')
	if at < 1 {
		return ""
	}
	return address[:min(2, at)] + "***" + address[at:]
}

func (m *Module) sensitiveMethods(ctx context.Context, identity authn.Identity, enrollmentRequired ...bool) (*StatusMethods, error) {
	totp := false
	if len(enrollmentRequired) == 0 || !enrollmentRequired[0] {
		var err error
		totp, err = m.availableTOTP(ctx, identity)
		if err != nil {
			return nil, err
		}
	}
	methods := &StatusMethods{TOTP: totp}
	if m.sensitiveEmailEnabled() {
		address, err := m.sensitiveEmail(ctx, identity, false)
		if err != nil {
			return nil, err
		}
		methods.Email, methods.Address = address != "", maskedEmail(address)
	}
	return methods, nil
}

func (m *Module) unlinkFlowStatus(ctx context.Context, identity authn.Identity, q StatusQuery, result *StatusResult) error {
	// Account dependencies are shared when both groups are selected. Email-code
	// polling does not compute the authenticator/password fallback.
	var address, fallback string
	if q.Fields.Has("methods") {
		account, err := m.unlinkAccount(ctx, identity, false)
		if err != nil {
			return err
		}
		address, fallback = account.email, account.fallback
		canSend := address != "" && m.emailCodes != nil && m.emailCodes.config.UnlinkURL != "" && m.emailCodes.sender.Ready()
		result.Methods = &StatusMethods{Email: canSend, Fallback: fallback, TOTP: address != "" && fallback == "verification"}
	} else {
		var err error
		address, err = m.sensitiveEmail(ctx, identity, false)
		if err != nil {
			return err
		}
	}
	if !q.Fields.Has("email_code") {
		return nil
	}
	result.EmailCode = &StatusEmailCode{Completed: address == ""}
	if address == "" {
		return nil
	}
	e := m.emailCodes
	if e == nil {
		return ErrEmailUnavailable
	}
	retry, err := e.retryAt(ctx, e.cooldownKeys(q.BrowserUUID, q.IP, address))
	if err != nil {
		return err
	}
	result.EmailCode.RetryAt = retry
	err = e.store.SQL(ctx).QueryRowContext(ctx, `SELECT verification_id,expired_at FROM t_auth_email_code WHERE user_id=$1 AND credential_version=$2 AND recipient=$3 AND purpose='unlink' AND NOT consumed AND expired_at>$4 ORDER BY created_at DESC LIMIT 1`, identity.UserID, identity.CredentialVersion, address, result.ServerTime).Scan(&result.EmailCode.VerificationID, &result.EmailCode.ExpiredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

// loginRecoveryStatus reads a first-factor-verified ticket without consuming it,
// extending its lifetime, or charging the factor-verification attempt budget.
func (m *Module) loginRecoveryStatus(ctx context.Context, q StatusQuery, result *StatusResult) error {
	ticket := q.MFATicket
	if !m.totpEnabled() {
		return ErrVerificationUnavailable
	}
	if !validSessionID(ticket) {
		return ErrVerificationInvalid
	}
	var pending mfaPending
	if err := m.verification.get(ctx, loginTicketPrefix+ticket, &pending, false); err != nil {
		return err
	}
	if !pending.Identity.Methods.Valid() || pending.ExpiredAt <= time.Now().UnixMilli() {
		return ErrVerificationInvalid
	}
	// This also rejects deleted, disabled, or credential-revoked accounts even
	// when email delivery is disabled. Match the profile's verified-email rule.
	address, err := m.sensitiveEmail(ctx, pending.Identity, false)
	if err != nil {
		return err
	}
	bound, err := m.verification.hasTOTP(ctx, pending.Identity.UserID)
	if err != nil {
		return err
	}
	if !bound {
		return ErrVerificationUnavailable
	}
	methods := &StatusMethods{RecoveryCode: true}
	if m.sensitiveEmailEnabled() && address != "" {
		methods.Email, methods.Address = true, maskedEmail(address)
	}
	if q.Fields.Has("methods") {
		result.Methods = methods
	}
	if q.Fields.Has("email_code") {
		if !methods.Email {
			return ErrEmailUnavailable
		}
		return m.setMailboxCooldown(ctx, address, q, result)
	}
	return nil
}

// mailboxCooldown reads the shared browser/IP/recipient send budget without
// issuing a code, restoring a proof, or consuming an Verification verification attempt.
func (m *Module) mailboxCooldown(ctx context.Context, identity authn.Identity, q StatusQuery, result *StatusResult) error {
	if !m.sensitiveEmailEnabled() {
		return ErrEmailUnavailable
	}
	address, err := m.sensitiveEmail(ctx, identity, false)
	if err != nil {
		return err
	}
	if address == "" {
		return ErrEmailUnavailable
	}
	return m.setMailboxCooldown(ctx, address, q, result)
}

func (m *Module) setMailboxCooldown(ctx context.Context, address string, q StatusQuery, result *StatusResult) error {
	retry, err := m.emailCodes.retryAt(ctx, m.emailCodes.cooldownKeys(q.BrowserUUID, q.IP, address))
	if err != nil {
		return err
	}
	result.ServerTime = time.Now().UnixMilli()
	result.EmailCode = &StatusEmailCode{RetryAt: retry}
	return nil
}
