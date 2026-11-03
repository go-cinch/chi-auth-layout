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
type MFASetupInput struct {
    ChallengeID string `json:"challenge_id,omitempty"`
    Credential string `json:"credential,omitempty"`
}

type MFAVerificationInput struct {
    VerificationCode string `json:"verification_code"`
}

func writeMFAError(w http.ResponseWriter, r *http.Request, err error) {
    server.SetMFAErrorHeaders(w, err)
    status := http.StatusBadRequest
    switch {
    case errors.Is(err, ErrMFAUserNotFound): status = http.StatusNotFound
    case errors.Is(err, ErrUnauthorized): status = http.StatusUnauthorized
    case errors.Is(err, ErrMFARequired), errors.Is(err, ErrMFAEnrollmentRequired), errors.Is(err, apperror.Forbidden): status = http.StatusForbidden
    case errors.Is(err, ErrMFARateLimited): status = http.StatusTooManyRequests
    case errors.Is(err, ErrMFAUnavailable): status = http.StatusServiceUnavailable
    case errors.Is(err, ErrMFAAlreadyEnabled): status = http.StatusConflict
    case errors.Is(err, ErrMFAInvalid), errors.Is(err, ErrIncorrectPassword), errors.Is(err, ErrInvalidCredential), errors.Is(err, ErrInvalid):
    default:
        slog.ErrorContext(r.Context(), "mfa operation failed: "+err.Error(), "operation", "mfa", "outcome", "failure", "error", err)
        status, err = http.StatusInternalServerError, apperror.Internal
    }
    server.WriteError(w, r, status, err)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) mfaStatus(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) mfaStatus(w http.ResponseWriter, r *http.Request) {
{{ end }}
    w.Header().Set("Cache-Control", "no-store")
    result, err := m.MFAStatus(r.Context())
    if err != nil { writeMFAError(w, r, err); return }
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) mfaChallenge(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) mfaChallenge(w http.ResponseWriter, r *http.Request) {
{{ end }}
    w.Header().Set("Cache-Control", "no-store")
    if !m.mfaEnabled() { writeMFAError(w, r, ErrMFAUnavailable); return }
    identity, _ := authn.FromContext(r.Context())
    result, err := m.credentials.MFASetupChallenge(r.Context(), identity.UserID)
    if err != nil { writeMFAError(w, r, err); return }
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) mfaSetup(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) mfaSetup(w http.ResponseWriter, r *http.Request) {
{{ end }}
    var input MFASetupInput
    w.Header().Set("Cache-Control", "no-store")
    if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    var password string
    if input.ChallengeID != "" || input.Credential != "" {
        identity, _ := authn.FromContext(r.Context())
        var err error
        password, err = m.credentials.OpenMFASetup(r.Context(), identity.UserID, EncryptedLoginInput{ChallengeID: input.ChallengeID, Credential: input.Credential})
        if err != nil { writeMFAError(w, r, err); return }
    }
    result, err := m.BeginMFAEnrollment(r.Context(), password)
    if err != nil { writeMFAError(w, r, err); return }
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) mfaConfirm(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) mfaConfirm(w http.ResponseWriter, r *http.Request) {
{{ end }}
    var input MFAVerificationInput
    w.Header().Set("Cache-Control", "no-store")
    if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    result, err := m.ConfirmMFAEnrollment(r.Context(), input.VerificationCode)
    if err != nil { writeMFAError(w, r, err); return }
    writeLoginHeaders(w, result.LoginResult)
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) mfaTest(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) mfaTest(w http.ResponseWriter, r *http.Request) {
{{ end }}
    var input MFAVerificationInput
    w.Header().Set("Cache-Control", "no-store")
    if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
    result, err := m.TestMFA(r.Context(), input.VerificationCode)
    if err != nil {
        switch {
        case errors.Is(err, ErrMFAInvalid):
            // A failed test is not a request to authorize or replay a business operation.
            server.WriteError(w, r, http.StatusBadRequest, err)
        case errors.Is(err, ErrMFARateLimited):
            server.WriteError(w, r, http.StatusTooManyRequests, err)
        case errors.Is(err, ErrUnauthorized):
            server.WriteError(w, r, http.StatusUnauthorized, err)
        case errors.Is(err, authn.ErrPasswordResetRequired):
            server.WriteError(w, r, http.StatusForbidden, err)
        case errors.Is(err, ErrMFAEnrollmentRequired):
            server.SetMFAErrorHeaders(w, err)
            server.WriteError(w, r, http.StatusForbidden, err)
        case errors.Is(err, ErrMFAUnavailable):
            server.WriteError(w, r, http.StatusServiceUnavailable, err)
        default:
            slog.ErrorContext(r.Context(), "authenticator test failed: "+err.Error(), "operation", "mfa_test", "outcome", "failure", "error", err)
            server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
        }
        return
    }
    server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) mfaReset(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) mfaReset(w http.ResponseWriter, r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
    id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
    id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end -}}
    if err != nil || id <= 0 { writeMFAError(w, r, ErrInvalid); return }
    if err := m.ResetUserMFA(r.Context(), id); err != nil { writeMFAError(w, r, err); return }
    server.WriteOK(w)
}
