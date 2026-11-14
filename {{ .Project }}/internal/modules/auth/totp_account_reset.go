package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/server"
{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ end }}
)

// A reset is scoped to the caller's current session and an enrolled factor.
// Login tickets, admin reset permissions and ordinary step-up grants are independent.
func (m *Module) accountResetSession(ctx context.Context) (mfaPending, error) {
	identity, ok := authn.FromContext(ctx)
	if !ok || identity.SessionID == "" || identity.PasswordResetRequired {
		return mfaPending{}, ErrUnauthorized
	}
	if !m.totpEnabled() {
		return mfaPending{}, ErrVerificationUnavailable
	}
	session, err := m.sessions.authenticatedSession(ctx, identity)
	if errors.Is(err, ErrSessionNotFound) {
		return mfaPending{}, ErrUnauthorized
	}
	return mfaPending{Identity: identity, RememberMe: session.RememberMe}, err
}

func (m *Module) accountResetMailbox(ctx context.Context, identity authn.Identity) (string, error) {
	if !m.sensitiveEmailEnabled() {
		return "", ErrEmailUnavailable
	}
	var address string
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT u.email FROM t_user u JOIN t_user_totp f ON f.user_id=u.id
		WHERE u.id=$1 AND u.credential_version=$2 AND u.status=$3 AND u.email_verified_at IS NOT NULL AND u.email IS NOT NULL`, identity.UserID, identity.CredentialVersion, userStatusActive).Scan(&address)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && address == "") {
		return "", ErrEmailUnavailable
	}
	return address, err
}

func (m *Module) requestAccountResetCode(ctx context.Context, pending mfaPending, browserUUID, ip string) (*EmailCodeStatus, error) {
	identity := pending.Identity
	address, err := m.accountResetMailbox(ctx, identity)
	if err != nil {
		return nil, err
	}
	binding := m.emailCodes.digest("reset-session", identity.SessionID)
	return m.emailCodes.Send(ctx, browserUUID, ip, "totp_reset", binding, address, identity.UserID, identity.CredentialVersion)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) sendAccountResetCode(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) sendAccountResetCode(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	pending, err := m.accountResetSession(ctx)
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	var input SensitiveEmailRequest
	if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	var ip string
	if m.emailCodes != nil {
		ip = m.emailCodes.clientIP(r)
	}
	state, err := m.requestAccountResetCode(ctx, pending, input.BrowserUUID, ip)
	if err != nil {
		writeEmailError(w, r, err, state)
		return
	}
	server.WriteOK(w, state)
}

type accountResetProof struct {
	BrowserUUID      string `json:"browser_uuid"`
	VerificationID   string `json:"verification_id"`
	VerificationCode string `json:"verification_code"`
}

// pending comes from accountResetSession; a supplied recovery code excludes an email proof.
func (m *Module) confirmAccountResetProof(ctx context.Context, pending mfaPending, input accountResetProof, recoveryCode *string) (*LoginResult, error) {
	if (recoveryCode != nil && (input != (accountResetProof{}) || !validRecoveryCode(*recoveryCode))) ||
		(recoveryCode == nil && (!validSensitiveCode(input.VerificationCode) || !browserUUIDPattern.MatchString(input.BrowserUUID) || !validSessionID(input.VerificationID))) {
		return nil, ErrVerificationInvalid
	}
	if err := m.verification.attempt(ctx, pending.Identity); err != nil {
		return nil, err
	}
	return m.replaceVerifiedFactor(ctx, pending, recoveryCode != nil, func(tx context.Context, identity authn.Identity) (bool, error) {
		if err := m.validateVerificationSession(tx, identity); err != nil {
			return false, err
		}
		if recoveryCode != nil {
			return m.verifyRecoveryCode(tx, identity, *recoveryCode)
		}
		address, err := m.accountResetMailbox(tx, identity)
		if err != nil {
			return false, err
		}
		binding := m.emailCodes.digest("reset-session", identity.SessionID)
		return m.emailCodes.Consume(tx, input.VerificationID, input.BrowserUUID, "totp_reset", binding, address, input.VerificationCode, identity.UserID, identity.CredentialVersion)
	})
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) confirmAccountReset(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) confirmAccountReset(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	pending, err := m.accountResetSession(ctx)
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	var input accountResetProof
	if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	codes := r.Header.Values(authn.RecoveryCodeHeader)
	if len(codes) > 1 {
		writeEmailError(w, r, ErrVerificationInvalid, nil)
		return
	}
	var recoveryCode *string
	if len(codes) == 1 {
		recoveryCode = &codes[0]
	}
	result, err := m.confirmAccountResetProof(ctx, pending, input, recoveryCode)
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	writeLoginHeaders(w)
	server.WriteOK(w, result)
}
