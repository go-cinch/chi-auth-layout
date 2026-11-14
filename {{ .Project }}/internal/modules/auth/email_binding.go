package auth

import (
	"context"
	"errors"
	"github.com/lib/pq"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

func (m *Module) requiresEmailBinding() bool {
	return m.emailCodes != nil && m.emailCodes.config.RequireBinding && m.emailCodes.sender.Ready()
}

func (m *Module) populateEmailInfo(ctx context.Context, value *UserInfo) error {
	if m.emailCodes == nil {
		return nil
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return ErrUnauthorized
	}
	if err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COALESCE(email, ''), email_verified_at FROM t_user WHERE id = $1 AND code = $2`, identity.UserID, identity.Code).Scan(&value.Email, &value.EmailVerifiedAt); err != nil {
		return err
	}
	value.EmailBindingRequired = value.Email == "" && m.requiresEmailBinding()
	return nil
}

func (m *Module) checkEmailTarget(ctx context.Context, method, target string) error {
	if !m.requiresEmailBinding() {
		return nil
	}
	relative, owned := RelativeAuthHTTPPath(target, m.httpPath)
	if owned && ownsAuthHTTPMethod(relative, method) {
		switch relative {
		case "/status", "/info", "/email/unbind/status", "/email/code/status", "/email/code", "/email/bind", "/challenge", "/logout", "/pub/logout", "/pub/refresh", "/totp", "/totp/challenge", "/totp/setup", "/totp/confirm", "/totp/test", "/reset/pwd":
			return nil
		}
	}
	var info UserInfo
	if err := m.populateEmailInfo(ctx, &info); err != nil {
		return err
	}
	if info.EmailBindingRequired {
		return ErrEmailBindingRequired
	}
	return nil
}

type EmailBindInput struct {
	ChallengeID      string `json:"challenge_id"`
	Email            string `json:"email"`
	BrowserUUID      string `json:"browser_uuid"`
	VerificationID   string `json:"verification_id"`
	VerificationCode string `json:"verification_code"`
}

func (m *Module) BindEmail(ctx context.Context, input EmailBindInput) (*LoginResult, error) {
	if m.emailCodes == nil {
		return nil, ErrEmailUnavailable
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return nil, err
	}
	session, err := m.sessions.authenticatedSession(ctx, identity)
	if err != nil {
		return nil, err
	}
	var result *LoginResult
	var sessionID string
	valid := false
	err = m.store.Tx(ctx, func(txCtx context.Context) error {
		// Keep foreign-key checks used by concurrent code delivery compatible.
		var current string
		var version, count int64
		var status int16
		err := m.store.SQL(txCtx).QueryRowContext(txCtx, `SELECT COALESCE(email, ''), credential_version, login_count, status FROM t_user WHERE id = $1 AND code = $2 FOR NO KEY UPDATE`, identity.UserID, identity.Code).Scan(&current, &version, &count, &status)
		if err != nil {
			return err
		}
		if current != "" {
			return ErrEmailOccupied
		}
		if version != identity.CredentialVersion || status != userStatusActive {
			return ErrUnauthorized
		}
		if count == 0 {
			return authn.ErrPasswordResetRequired
		}
		if required, err := m.totpEnrollmentRequired(txCtx, identity); err != nil {
			return err
		} else if required {
			return ErrTOTPEnrollmentRequired
		}
		valid, err = m.emailCodes.Consume(txCtx, input.VerificationID, input.BrowserUUID, "bind", email, email, input.VerificationCode, identity.UserID, version)
		if err != nil || !valid {
			return err
		}
		now := time.Now().UnixMilli()
		_, err = m.store.SQL(txCtx).ExecContext(txCtx, `UPDATE t_user SET email = $1, email_verified_at = $2, updated_at = $2, credential_version = credential_version + 1 WHERE id = $3`, email, now, identity.UserID)
		if err != nil {
			var constraint *pq.Error
			if errors.As(err, &constraint) && constraint.Code == "23505" {
				return ErrEmailOccupied
			}
			return err
		}
		if err = m.emailCodes.emailLinkedNotice(txCtx, identity.UserID); err != nil {
			return err
		}
		identity.CredentialVersion++
		result, sessionID, err = m.issueLoginResult(txCtx, identity, session.RememberMe)
		return err
	})
	if err != nil {
		if sessionID != "" {
			_ = m.sessions.Revoke(context.WithoutCancel(ctx), sessionID)
		}
		return nil, err
	}
	if !valid {
		return nil, ErrEmailInvalid
	}
	return result, nil
}

func (m *Module) SendBindEmailCode(ctx context.Context, input EmailCodeInput, ip string) (*EmailCodeStatus, error) {
	if m.emailCodes == nil {
		return nil, ErrEmailUnavailable
	}
	identity, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return nil, err
	}
	var unavailable bool
	err = m.store.SQL(ctx).QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM t_user WHERE (id = $1 AND email IS NOT NULL) OR email = $2 OR (id <> $1 AND LOWER(TRIM(username)) = $2))`, identity.UserID, email).Scan(&unavailable)
	if err != nil {
		return nil, err
	}
	if unavailable {
		return nil, ErrEmailOccupied
	}
	return m.emailCodes.Send(ctx, input.BrowserUUID, ip, "bind", email, email, identity.UserID, identity.CredentialVersion)
}
