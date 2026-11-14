package auth

import (
    "errors"
    "log/slog"
    "net/http"
    "strconv"
    "{{ .Computed.module_name_final }}/internal/common/apperror"
    "{{ .Computed.module_name_final }}/internal/common/authn"
    "{{ .Computed.module_name_final }}/internal/common/server"
{{- if eq .Computed.http_router_final "gin" }}
    "github.com/gin-gonic/gin"
{{- else }}
    "github.com/go-chi/chi/v5"
{{- end }}
)

// Empty input uses only the server-side grant from a successful password login.
// Personal enrollment continues to require both encrypted credential fields.
type TOTPSetupInput struct {
    ChallengeID string `json:"challenge_id,omitempty"`
    Credential string `json:"credential,omitempty"`
}

type TOTPVerificationInput struct {
    VerificationCode string `json:"verification_code"`
}

func writeVerificationError(w http.ResponseWriter, r *http.Request, err error) {
    server.SetVerificationErrorHeaders(w, err)
    status := http.StatusBadRequest
    switch {
    case errors.Is(err, ErrTOTPUserNotFound): status = http.StatusNotFound
    case errors.Is(err, ErrUnauthorized): status = http.StatusUnauthorized
    case errors.Is(err, ErrLoginMFARequired), errors.Is(err, ErrPasswordVerificationRequired), errors.Is(err, ErrStepUpRequired), errors.Is(err, ErrTOTPEnrollmentRequired), errors.Is(err, apperror.Forbidden): status = http.StatusForbidden
    case errors.Is(err, ErrVerificationRateLimited): status = http.StatusTooManyRequests
    case errors.Is(err, ErrVerificationUnavailable): status = http.StatusServiceUnavailable
    case errors.Is(err, ErrTOTPAlreadyBound): status = http.StatusConflict
    case errors.Is(err, ErrVerificationInvalid), errors.Is(err, ErrIncorrectPassword), errors.Is(err, ErrInvalidCredential), errors.Is(err, ErrInvalid):
    default:
        slog.ErrorContext(r.Context(), "verification operation failed: "+err.Error(), "operation", "verification", "outcome", "failure", "error", err)
        status, err = http.StatusInternalServerError, apperror.Internal
    }
    server.WriteError(w, r, status, err)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) totpStatus(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) totpStatus(w http.ResponseWriter, r *http.Request) {
{{ end }}
    w.Header().Set("Cache-Control", "no-store")
    result, err := m.TOTPStatus(r.Context())
    if err != nil { writeVerificationError(w, r, err); return }
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) totpChallenge(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) totpChallenge(w http.ResponseWriter, r *http.Request) {
{{ end }}
    w.Header().Set("Cache-Control", "no-store")
    if !m.totpEnabled() { writeVerificationError(w, r, ErrVerificationUnavailable); return }
    identity, _ := authn.FromContext(r.Context())
    result, err := m.credentials.TOTPSetupChallenge(r.Context(), identity.UserID)
    if err != nil { writeVerificationError(w, r, err); return }
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) totpSetup(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) totpSetup(w http.ResponseWriter, r *http.Request) {
{{ end }}
    var input TOTPSetupInput
    w.Header().Set("Cache-Control", "no-store")
    if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    var password string
    if input.ChallengeID != "" || input.Credential != "" {
        identity, _ := authn.FromContext(r.Context())
        var err error
        password, err = m.credentials.OpenTOTPSetup(r.Context(), identity.UserID, EncryptedLoginInput{ChallengeID: input.ChallengeID, Credential: input.Credential})
        if err != nil { writeVerificationError(w, r, err); return }
    }
    result, err := m.BeginTOTPEnrollment(r.Context(), password)
    if err != nil { writeVerificationError(w, r, err); return }
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) totpConfirm(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) totpConfirm(w http.ResponseWriter, r *http.Request) {
{{ end }}
    var input TOTPVerificationInput
    w.Header().Set("Cache-Control", "no-store")
    if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    result, err := m.ConfirmTOTPEnrollment(r.Context(), input.VerificationCode)
    if err != nil { writeVerificationError(w, r, err); return }
    writeLoginHeaders(w)
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) totpTest(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) totpTest(w http.ResponseWriter, r *http.Request) {
{{ end }}
    var input TOTPVerificationInput
    w.Header().Set("Cache-Control", "no-store")
    if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    result, err := m.TestTOTP(r.Context(), input.VerificationCode)
    if err != nil {
        switch {
        case errors.Is(err, ErrVerificationInvalid):
            // A failed test is not a request to authorize or replay a business operation.
            server.WriteError(w, r, http.StatusBadRequest, err)
        case errors.Is(err, ErrVerificationRateLimited):
            server.WriteError(w, r, http.StatusTooManyRequests, err)
        case errors.Is(err, ErrUnauthorized):
            server.WriteError(w, r, http.StatusUnauthorized, err)
        case errors.Is(err, authn.ErrPasswordResetRequired):
            server.WriteError(w, r, http.StatusForbidden, err)
        case errors.Is(err, ErrTOTPEnrollmentRequired):
            server.SetVerificationErrorHeaders(w, err)
            server.WriteError(w, r, http.StatusForbidden, err)
        case errors.Is(err, ErrVerificationUnavailable):
            server.WriteError(w, r, http.StatusServiceUnavailable, err)
        default:
            slog.ErrorContext(r.Context(), "authenticator test failed: "+err.Error(), "operation", "totp_test", "outcome", "failure", "error", err)
            server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
        }
        return
    }
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) totpReset(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) totpReset(w http.ResponseWriter, r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
    id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
    id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end -}}
    if err != nil || id <= 0 { writeVerificationError(w, r, ErrInvalid); return }
    if err := m.ResetUserTOTP(r.Context(), id); err != nil { writeVerificationError(w, r, err); return }
    server.WriteOK(w)
}
