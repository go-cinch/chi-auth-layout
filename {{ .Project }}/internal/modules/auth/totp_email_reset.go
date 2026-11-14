package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/server"
{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ end }}
)

func (m *Module) resetTicket(ctx context.Context, ticket string) (mfaPending, error) {
	var p mfaPending
	if !m.totpEnabled() || !m.sensitiveEmailEnabled() {
		return p, ErrEmailUnavailable
	}
	if !validSessionID(ticket) {
		return p, ErrVerificationInvalid
	}
	if err := m.verification.get(ctx, loginTicketPrefix+ticket, &p, false); err != nil {
		return p, err
	}
	if !p.Identity.Methods.Valid() || p.ExpiredAt <= time.Now().UnixMilli() {
		return p, ErrVerificationInvalid
	}
	return p, m.verification.attempt(ctx, p.Identity)
}

func (m *Module) readEmailReset(w http.ResponseWriter, r *http.Request) (string, mfaPending, LoginInput, error) {
	w.Header().Set("Cache-Control", "no-store")
	tickets := r.Header.Values(authn.MFATicketHeader)
	if len(tickets) != 1 {
		return "", mfaPending{}, LoginInput{}, ErrVerificationInvalid
	}
	p, err := m.resetTicket(r.Context(), tickets[0])
	if err != nil {
		return "", p, LoginInput{}, err
	}
	var body EncryptedLoginInput
	if err := server.DecodeJSON(w, r, &body, server.BodyAuth); err != nil {
		return "", p, LoginInput{}, ErrInvalidCredential
	}
	input, err := m.credentials.Open(r.Context(), body)
	return tickets[0], p, input, err
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) sendResetCode(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) sendResetCode(w http.ResponseWriter, r *http.Request) {
{{ end }}

	ticket, p, input, err := m.readEmailReset(w, r)
	var state *EmailCodeStatus
	if err == nil {
		state, err = m.requestResetCode(r.Context(), ticket, p, input, m.emailCodes.clientIP(r))
	}
	if err != nil {
		writeEmailError(w, r, err, state)
		return
	}
	server.WriteOK(w, state)
}

func (m *Module) requestResetCode(ctx context.Context, ticket string, p mfaPending, input LoginInput, ip string) (*EmailCodeStatus, error) {
	if input.Password == "" || input.Method == "email_code" || !browserUUIDPattern.MatchString(input.BrowserUUID) {
		return nil, ErrInvalidCredential
	}
	var hash, address string
	err := m.store.SQL(ctx).QueryRowContext(ctx, `SELECT u.password,u.email FROM t_user u JOIN t_user_totp f ON f.user_id=u.id
        WHERE u.id=$1 AND u.credential_version=$2 AND u.status=$3 AND u.email_verified_at IS NOT NULL AND u.email IS NOT NULL`, p.Identity.UserID, p.Identity.CredentialVersion, userStatusActive).Scan(&hash, &address)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEmailUnavailable
	}
	if err != nil {
		return nil, err
	}
	if !VerifyPassword(hash, input.Password) {
		return nil, ErrIncorrectPassword
	}
	return m.emailCodes.Send(ctx, input.BrowserUUID, ip, "totp_reset", m.emailCodes.digest("reset-ticket", ticket), address, p.Identity.UserID, p.Identity.CredentialVersion)
}

// The caller resolves the login ticket and decrypts the credential before confirmation.
func (m *Module) confirmEmailResetProof(ctx context.Context, ticket string, pending mfaPending, input LoginInput) (*LoginResult, error) {
	if input.Method != "email_code" || !validSensitiveCode(input.VerificationCode) {
		return nil, ErrEmailInvalid
	}
	return m.replaceLostFactor(ctx, ticket, pending, false, func(tx context.Context, identity authn.Identity) (bool, error) {
		email, err := m.sensitiveEmail(tx, identity, false)
		if err != nil || email == "" {
			return false, err
		}
		return m.emailCodes.Consume(tx, input.VerificationID, input.BrowserUUID, "totp_reset", m.emailCodes.digest("reset-ticket", ticket), email, input.VerificationCode, identity.UserID, identity.CredentialVersion)
	})
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) confirmEmailReset(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) confirmEmailReset(w http.ResponseWriter, r *http.Request) {
{{ end }}

	ticket, pending, input, err := m.readEmailReset(w, r)
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	result, err := m.confirmEmailResetProof(r.Context(), ticket, pending, input)
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	writeLoginHeaders(w)
	server.WriteOK(w, result)
}
