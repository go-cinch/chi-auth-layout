package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

// Both password and email-code login resolve aliases identically. Never choose a
// candidate by trying its password; an ambiguous legacy alias fails closed.
func (m *Module) resolveLoginUsername(ctx context.Context, identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	if m.emailCodes == nil {
		return identifier, nil
	}
	email, _ := normalizeEmail(identifier)
	rows, err := m.store.SQL(ctx).QueryContext(ctx, `SELECT username FROM t_user WHERE username = $1 OR (email = NULLIF($2, '') AND email_verified_at IS NOT NULL) LIMIT 2`, identifier, email)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var username string
	count := 0
	for rows.Next() {
		if err := rows.Scan(&username); err != nil {
			return "", err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if count != 1 {
		return "", ErrLoginFailed
	}
	return username, nil
}

func (m *Module) loginEmail(ctx context.Context, input LoginInput) (*LoginResult, error) {
	if m.emailCodes == nil {
		return nil, ErrEmailUnavailable
	}
	user, err := m.loginUser(ctx, input.Username)
	if errors.Is(err, ErrLoginFailed) {
		return nil, ErrEmailInvalid
	}
	if err != nil {
		return nil, err
	}
	if user.status == userStatusLocked {
		unlocked, err := m.unlockExpiredUser(ctx, user.identity.UserID)
		if err != nil {
			return nil, err
		}
		if unlocked {
			user.status = userStatusActive
		}
	}
	if user.status != userStatusActive {
		return nil, ErrEmailInvalid
	}
	var email string
	err = m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COALESCE(email, '') FROM t_user WHERE id = $1 AND email_verified_at IS NOT NULL`, user.identity.UserID).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) || email == "" && err == nil {
		return nil, ErrEmailInvalid
	}
	if err != nil {
		return nil, err
	}
	valid := false
	err = m.store.Tx(ctx, func(txCtx context.Context) error {
		var consumeErr error
		valid, consumeErr = m.emailCodes.Consume(txCtx, input.VerificationID, input.BrowserUUID, "login", input.Username, email, input.VerificationCode, user.identity.UserID, user.identity.CredentialVersion)
		return consumeErr
	})
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrEmailInvalid
	}
	return m.completeFirstFactorLogin(ctx, user.identity, input.RememberMe, authn.EmailCode)
}

type EmailCodeInput struct {
	BrowserUUID   string         `json:"browser_uuid"`
	Username      string         `json:"username,omitempty"`
	Email         string         `json:"email,omitempty"`
	SliderProof   string         `json:"slider_proof,omitempty"`
	CaptchaID     string         `json:"captcha_id,omitempty"`
	CaptchaPoints []CaptchaPoint `json:"captcha_points,omitempty"`
}

// SendLoginEmailCode applies account, captcha and delivery rules independently of HTTP.
func (m *Module) SendLoginEmailCode(ctx context.Context, input EmailCodeInput, ip string) (*EmailCodeStatus, *PointCaptchaChallenge, error) {
	if m.emailCodes == nil {
		return nil, nil, ErrEmailUnavailable
	}
	input.Username = strings.TrimSpace(input.Username)
	if _, err := normalizeEmail(input.Username); err != nil {
		return nil, nil, err
	}
	if !validEmailContext(input.BrowserUUID, "login", input.Username) || input.Email != "" {
		return nil, nil, ErrInvalid
	}
	state, err := m.emailCodes.Status(ctx, input.BrowserUUID, ip, "login", input.Username, 0)
	if err != nil {
		return nil, nil, err
	}
	if state.RetryAt > state.ServerTime {
		return state, nil, ErrEmailCooldown
	}
	user, err := m.loginUser(ctx, input.Username)
	if err != nil && !errors.Is(err, ErrLoginFailed) {
		return nil, nil, err
	}
	if user != nil && m.captcha.Required(user.wrong) {
		verified, err := m.captcha.Verify(ctx, input.Username, input.CaptchaID, input.CaptchaPoints)
		if err != nil {
			return nil, nil, err
		}
		if !verified {
			captcha, err := m.captcha.NewChallenge(ctx, input.Username)
			if err != nil {
				return nil, nil, err
			}
			return nil, captcha, ErrPointCaptchaRequired
		}
	} else if err := m.sliderCaptcha.ConsumeProof(ctx, "email_login", input.Username, input.SliderProof); err != nil {
		return nil, nil, err
	}
	var recipient string
	var userID, version int64
	if user != nil {
		active := user.status == userStatusActive
		if user.status == userStatusLocked {
			active, err = m.unlockExpiredUser(ctx, user.identity.UserID)
			if err != nil {
				return nil, nil, err
			}
		}
		if active {
			err = m.store.SQL(ctx).QueryRowContext(ctx, `SELECT COALESCE(email, '') FROM t_user WHERE id = $1 AND email_verified_at IS NOT NULL`, user.identity.UserID).Scan(&recipient)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, nil, err
			}
			if recipient != "" {
				userID, version = user.identity.UserID, user.identity.CredentialVersion
			}
		}
	}
	state, err = m.emailCodes.Send(ctx, input.BrowserUUID, ip, "login", input.Username, recipient, userID, version)
	return state, nil, err
}
