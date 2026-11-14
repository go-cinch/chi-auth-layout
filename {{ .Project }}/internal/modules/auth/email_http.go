package auth

import (
    "errors"
    "log/slog"
    "net/http"
    "net/url"
    "strconv"
    "strings"
    "time"

{{- if eq .Computed.http_router_final "gin" }}
    "github.com/gin-gonic/gin"
{{- end }}
    "{{ .Computed.module_name_final }}/internal/common/apperror"
    "{{ .Computed.module_name_final }}/internal/common/auditctx"
    "{{ .Computed.module_name_final }}/internal/common/authn"
    "{{ .Computed.module_name_final }}/internal/common/server"
)

type SensitiveEmailRequest struct {
    BrowserUUID string `json:"browser_uuid"`
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) sendSensitiveCode(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) sendSensitiveCode(w http.ResponseWriter, r *http.Request) {
{{ end }}
    w.Header().Set("Cache-Control", "no-store")
    identity, ok := authn.FromContext(r.Context())
    if !ok { writeEmailError(w,r,ErrUnauthorized,nil); return }
    if err := m.allowEnrollmentSensitive(r.Context(), identity); err != nil { writeEmailError(w,r,err,nil); return }
    if !m.sensitiveEmailEnabled() { writeEmailError(w,r,ErrEmailUnavailable,nil); return }
    var input SensitiveEmailRequest
    if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    email, err := m.sensitiveEmail(r.Context(), identity, false)
    if err != nil { writeEmailError(w,r,err,nil); return }
    if email == "" { writeEmailError(w,r,ErrVerificationInvalid,nil); return }
    state, err := m.emailCodes.Send(r.Context(), input.BrowserUUID, m.emailCodes.clientIP(r), "sensitive", identity.SessionID, email, identity.UserID, identity.CredentialVersion)
    if err != nil { writeEmailError(w,r,err,state); return }
    server.WriteOK(w,state)
}

type EmailCodeError struct {
    ErrorCode string `json:"error_code"`
    Msg string `json:"msg"`
    *EmailCodeStatus
}

func writeEmailError(w http.ResponseWriter, r *http.Request, err error, state *EmailCodeStatus) {
    status := http.StatusBadRequest
    switch {
    case errors.Is(err, ErrEmailCooldown):
        status = http.StatusTooManyRequests
        if state != nil { w.Header().Set("Retry-After", strconv.FormatInt(max(1, (state.RetryAt-time.Now().UnixMilli()+999)/1000), 10)) }
    case errors.Is(err, ErrUnlinkThrottled), errors.Is(err, ErrVerificationRateLimited): status = http.StatusTooManyRequests
    case errors.Is(err, ErrUnlinkFactor), errors.Is(err, ErrUnlinkInvalid): status = http.StatusConflict
    case errors.Is(err, ErrVerificationInvalid), errors.Is(err, ErrIncorrectPassword): status = http.StatusBadRequest
    case errors.Is(err, ErrVerificationUnavailable), errors.Is(err, ErrEmailUnavailable): status = http.StatusServiceUnavailable
    case errors.Is(err, ErrEmailOccupied): status = http.StatusConflict
    case errors.Is(err, ErrUnauthorized): status = http.StatusUnauthorized
    case errors.Is(err, ErrEmailBindingRequired), errors.Is(err, ErrPasswordVerificationRequired): status = http.StatusForbidden
    case errors.Is(err, authn.ErrPasswordResetRequired):
        server.WriteError(w, r, http.StatusForbidden, err)
        return
    case errors.Is(err, ErrTOTPEnrollmentRequired):
        server.WriteHTTPSecurityError(w, r, err)
        return
    case errors.Is(err, ErrEmailInvalid), errors.Is(err, ErrInvalid), errors.Is(err, ErrEmailAddress), errors.Is(err, ErrInvalidCredential), errors.Is(err, ErrSliderCaptchaRequired):
    default:
        slog.ErrorContext(r.Context(), "email authentication failed", "operation", "email_authentication", "outcome", "failure", "error", err)
        err = apperror.Internal
        status = http.StatusInternalServerError
    }
    auditctx.Error(r.Context(), apperror.Code(err))
    server.WriteJSON(w, status, EmailCodeError{ErrorCode: apperror.Code(err), Msg: server.ErrorMessage(r, err), EmailCodeStatus: state})
}

func (m *Module) emailCodeStatus(w http.ResponseWriter, r *http.Request, purpose string) {
    w.Header().Set("Cache-Control", "no-store")
    if m.emailCodes == nil { writeEmailError(w, r, ErrEmailUnavailable, nil); return }
    query, err := url.ParseQuery(r.URL.RawQuery)
    if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery); return }
    field := "username"
    var userID int64
    if purpose == "bind" {
        field = "email"
        identity, ok := authn.FromContext(r.Context())
        if !ok { writeEmailError(w, r, ErrUnauthorized, nil); return }
        userID = identity.UserID
    }
    if len(query["browser_uuid"]) != 1 || len(query[field]) != 1 { writeEmailError(w, r, ErrInvalid, nil); return }
    identifier := strings.TrimSpace(query.Get(field))
    if purpose == "bind" { identifier, err = normalizeEmail(identifier); if err != nil { writeEmailError(w, r, err, nil); return } }
    state, err := m.emailCodes.Status(r.Context(), query.Get("browser_uuid"), m.emailCodes.clientIP(r), purpose, identifier, userID)
    if err != nil { writeEmailError(w, r, err, state); return }
    server.WriteOK(w, state)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) loginEmailCodeStatus(c *gin.Context) { m.emailCodeStatus(c.Writer, c.Request, "login") }
func (m *Module) bindEmailCodeStatus(c *gin.Context) { m.emailCodeStatus(c.Writer, c.Request, "bind") }
func (m *Module) sendLoginEmailCode(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) loginEmailCodeStatus(w http.ResponseWriter, r *http.Request) { m.emailCodeStatus(w, r, "login") }
func (m *Module) bindEmailCodeStatus(w http.ResponseWriter, r *http.Request) { m.emailCodeStatus(w, r, "bind") }
func (m *Module) sendLoginEmailCode(w http.ResponseWriter, r *http.Request) {
{{ end }}
    w.Header().Set("Cache-Control", "no-store")
    if m.emailCodes == nil { writeEmailError(w, r, ErrEmailUnavailable, nil); return }
    var input EmailCodeInput
    if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    state, captcha, err := m.SendLoginEmailCode(r.Context(), input, m.emailCodes.clientIP(r))
    if errors.Is(err, ErrPointCaptchaRequired) {
        server.WriteJSON(w, http.StatusBadRequest, LoginFailureResponse{ErrorCode: apperror.Code(err), Msg: server.ErrorMessage(r, err), CaptchaRequired: true, Captcha: captcha})
        return
    }
    if err != nil { writeEmailError(w, r, err, state); return }
    server.WriteOK(w, state)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) sendBindEmailCode(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) sendBindEmailCode(w http.ResponseWriter, r *http.Request) {
{{ end }}
    w.Header().Set("Cache-Control", "no-store")
    if m.emailCodes == nil { writeEmailError(w, r, ErrEmailUnavailable, nil); return }
    if _, ok := authn.FromContext(r.Context()); !ok { writeEmailError(w, r, ErrUnauthorized, nil); return }
    var input EmailCodeInput
    if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    state, err := m.SendBindEmailCode(r.Context(), input, m.emailCodes.clientIP(r))
    if err != nil { writeEmailError(w, r, err, state); return }
    server.WriteOK(w, state)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) bindEmail(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) bindEmail(w http.ResponseWriter, r *http.Request) {
{{ end }}
    w.Header().Set("Cache-Control", "no-store")
    identity, ok := authn.FromContext(r.Context())
    if !ok { writeEmailError(w, r, ErrUnauthorized, nil); return }
    var encrypted EncryptedLoginInput
    if err := server.DecodeJSON(w, r, &encrypted, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    input, err := m.credentials.OpenEmailBind(r.Context(), identity.UserID, encrypted)
    if err != nil { writeEmailError(w, r, err, nil); return }
    result, err := m.BindEmail(r.Context(), input)
    if err != nil { writeEmailError(w, r, err, nil); return }
    writeLoginHeaders(w)
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) unlinkStatus(c *gin.Context) {
    w,r:=c.Writer,c.Request
{{ else -}}
func (m *Module) unlinkStatus(w http.ResponseWriter,r *http.Request) {
{{ end }}

	w.Header().Set("Cache-Control", "no-store")
	identity, ok := authn.FromContext(r.Context())
	if !ok {
		writeEmailError(w, r, ErrUnauthorized, nil)
		return
	}
	if m.emailCodes == nil {
		writeEmailError(w, r, ErrEmailUnavailable, nil)
		return
	}
	state, err := m.emailUnlinkStatus(r.Context(), identity, r.URL.Query().Get("browser_uuid"), m.emailCodes.clientIP(r))
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	server.WriteOK(w, state)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) requestUnlink(c *gin.Context) {
    w,r:=c.Writer,c.Request
{{ else -}}
func (m *Module) requestUnlink(w http.ResponseWriter,r *http.Request) {
{{ end }}

	w.Header().Set("Cache-Control", "no-store")
	identity, ok := authn.FromContext(r.Context())
	if !ok {
		writeEmailError(w, r, ErrUnauthorized, nil)
		return
	}
	var input EmailUnlinkRequest
	if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	e := m.emailCodes
	if e == nil || e.config.UnlinkURL == "" {
		writeEmailError(w, r, ErrEmailUnavailable, nil)
		return
	}
	a, err := m.unlinkAccount(r.Context(), identity, false)
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	if a.email == "" {
		writeEmailError(w, r, ErrUnlinkInvalid, nil)
		return
	}
	state, err := e.Send(r.Context(), input.BrowserUUID, e.clientIP(r), "unlink", a.email, a.email, identity.UserID, identity.CredentialVersion)
	if err != nil {
		writeEmailError(w, r, err, state)
		return
	}
	server.WriteOK(w, state)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) verifyUnlink(c *gin.Context) {
    w,r:=c.Writer,c.Request
{{ else -}}
func (m *Module) verifyUnlink(w http.ResponseWriter,r *http.Request) {
{{ end }}

	w.Header().Set("Cache-Control", "no-store")
	identity, ok := authn.FromContext(r.Context())
	if !ok {
		writeEmailError(w, r, ErrUnauthorized, nil)
		return
	}
	var input *EmailUnlinkVerificationInput
	if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	if input == nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	proof, err := m.credentials.openUnlink(r.Context(), identity.UserID, *input, r.Header.Values(authn.TOTPCodeHeader))
	if err == nil {
		err = m.verifyUnlinkProof(r.Context(), identity, proof)
	}
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) confirmUnlink(c *gin.Context) {
    w,r:=c.Writer,c.Request
{{ else -}}
func (m *Module) confirmUnlink(w http.ResponseWriter,r *http.Request) {
{{ end }}

	w.Header().Set("Cache-Control", "no-store")
	var input EmailUnlinkConfirmation
	if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	result, err := m.confirmUnlinkToken(r.Context(), input.Token)
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	server.WriteOK(w, result)
}

type EmailUnlinkRequest struct {
	BrowserUUID string `json:"browser_uuid"`
}

type EmailUnlinkConfirmation struct {
	Token string `json:"token"`
}
