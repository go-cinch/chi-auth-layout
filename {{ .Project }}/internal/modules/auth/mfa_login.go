package auth

import (
	"context"
	"strings"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
)

// MFA login tickets continue a verified primary factor, including recovery.
const loginTicketPrefix = "login:v2:"

type mfaPending struct {
	Identity   authn.Identity `json:"identity"`
	RememberMe bool           `json:"remember_me"`
	ExpiredAt  int64          `json:"expired_at"`
}

func (m *Module) beginMFALogin(ctx context.Context, identity authn.Identity, remember bool) (*LoginResult, error) {
	ticket, err := randomSessionValue()
	if err != nil {
		return nil, err
	}
	pending := mfaPending{Identity: identity, RememberMe: remember, ExpiredAt: time.Now().Add(m.verification.config.LoginTicketTTL).UnixMilli()}
	if err := m.verification.put(ctx, loginTicketPrefix+ticket, pending, m.verification.config.LoginTicketTTL); err != nil {
		return nil, err
	}
	return &LoginResult{MFARequired: true, MFATicket: ticket, ExpiredAt: pending.ExpiredAt}, nil
}

func (m *Module) CompleteTOTPLogin(ctx context.Context, ticket, code string) (*LoginResult, error) {
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
	// The continuation has no username in its HTTP body. Attribute every
	// verification outcome to the identity from the validated server-side ticket.
	auditctx.Identity(ctx, pending.Identity.UserID, pending.Identity.Username)
	if strings.TrimSpace(code) == "" {
		return nil, ErrStepUpRequired
	}
	if err := m.verification.consumeTOTP(ctx, pending.Identity, code); err != nil {
		return nil, err
	}
	// Only the winner may issue a session, even with concurrent factor verification.
	var consumed mfaPending
	if err := m.verification.get(ctx, loginTicketPrefix+ticket, &consumed, true); err != nil {
		return nil, err
	}
	pending.Identity.Methods.SecondFactor = authn.TOTP
	return m.finishLogin(ctx, pending.Identity, pending.RememberMe, false)
}
