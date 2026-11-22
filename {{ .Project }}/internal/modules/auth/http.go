package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
    "strconv"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"

{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- else }}
	"github.com/go-chi/chi/v5"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules"
"{{ .Computed.module_name_final }}/internal/modules/localization"
"{{ .Computed.module_name_final }}/internal/modules/dictionary"
)

var _ modules.HTTPAuthenticationPolicy = (*Module)(nil)
var _ modules.HTTPModule = (*Module)(nil)
var _ modules.ConfigurableHTTPModule = (*Module)(nil)
var _ modules.HTTPPermissionAuthorizer = (*Module)(nil)
var _ modules.AnonymousHTTPModule = (*Module)(nil)

func (m *Module) Name() string { return m.httpPath }

func (*Module) HTTPPathConfig() string { return "auth.path" }

func (*Module) PermissionEndpoint() string { return "/permission" }

const (
	permissionHeaderMethod         = "X-Original-Method"
	permissionHeaderURI            = "X-Original-URI"
	permissionHeaderPath           = "X-Permission-URI"
	permissionHeaderResource       = "X-Original-Resource"
	permissionResponseCode         = "X-Code"
	challengePurposeLogin          = "login"
	challengePurposeRegister       = "register"
	challengePurposePasswordChange = "password_change"
)

type ChallengeInput struct {
	Purpose string `json:"purpose"`
}

type RefreshTokenInput struct {
	RefreshToken string `json:"refresh_token"`
}

type SliderChallengeInput struct {
	Purpose  string `json:"purpose"`
	Username string `json:"username"`
}

// SystemProperties contains deployment settings that anonymous clients may display.
type SystemProperties struct {
	RegistrationEnabled bool `json:"registration_enabled"`
 UIPreferences map[string]dictionary.UIPreferences `json:"ui_preferences,omitempty"`
	DefaultLocale string `json:"default_locale"`
	Locales []localization.Language `json:"locales"`
	EmailEnabled bool `json:"email_enabled"`
	TOTPEnabled   bool `json:"totp_enabled"`
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) systemProperties(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) systemProperties(w http.ResponseWriter, r *http.Request) {
{{ end -}}
	w.Header().Set("Cache-Control", "no-store")
    query, err := url.ParseQuery(r.URL.RawQuery)
    if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery); return }
    client, err := preferenceClient(query)
    if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery); return }
    var ui map[string]dictionary.UIPreferences
    if client != "" {
        defaults, err := m.uiDefaults(r.Context(), client)
        if err != nil { server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal); return }
        ui = map[string]dictionary.UIPreferences{client: defaults}
    }
	catalog := m.catalogSource.PublicCatalog("default_locale", "locales")
	locales := make([]localization.Language, 0, len(catalog.Locales))
	for _, locale := range catalog.Locales {
		if locale.Enabled { locales = append(locales, locale) }
	}
	server.WriteOK(w, SystemProperties{
		RegistrationEnabled: m.registrationEnabled,
 UIPreferences: ui,
		DefaultLocale: catalog.DefaultLocale, Locales: locales,
		EmailEnabled: m.emailCodes != nil && m.emailCodes.sender.Ready(),
		TOTPEnabled:   m.totpEnabled(),
	})
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) timezoneOptions(c *gin.Context) {
    w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) timezoneOptions(w http.ResponseWriter, r *http.Request) {
{{ end -}}
    w.Header().Set("Cache-Control", "no-store")
    value, err := m.TimezoneOptions(r.Context())
    if err != nil { server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal); return }
    server.WriteOK(w, value)
}

func PermissionTargetFromRequest(r *http.Request) PermissionTarget {
	requestPath := strings.TrimSpace(r.Header.Get(permissionHeaderPath))
	if requestPath == "" {
		requestPath = strings.TrimSpace(r.Header.Get(permissionHeaderURI))
	}
	return normalizePermissionTarget(PermissionTarget{
		Method:   r.Header.Get(permissionHeaderMethod),
		Path:     requestPath,
		Resource: r.Header.Get(permissionHeaderResource),
	})
}

func (m *Module) AllowAnonymousHTTP(r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != m.Name()+m.PermissionEndpoint() {
		return false
	}
	target := PermissionTargetFromRequest(r)
	if target.Resource != "" || target.Method == "" || !strings.HasPrefix(target.Path, "/") || strings.HasPrefix(target.Path, "//") {
		return false
	}
	parsed, err := url.ParseRequestURI(target.Path)
	if err != nil {
		return false
	}
	requestPath := path.Clean(parsed.Path)
	return target.Method == http.MethodHead || target.Method == http.MethodOptions ||
		strings.Contains(requestPath, "/pub/") || strings.HasSuffix(requestPath, "/pub")
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) HTTP(r *gin.RouterGroup) {
	r.GET("/pub/i18n", m.localization)
	r.GET("/pub/opt", m.systemProperties)
 r.GET("/pub/timezones", m.timezoneOptions)
	r.GET("/pub/status", m.publicStatus)
	r.GET("/status", m.status)
	r.GET("/pub/register/username", m.usernameAvailability)
	r.POST("/pub/challenge", m.publicChallenge)
	r.POST("/pub/register", m.register)
	r.POST("/pub/slider/challenge", m.sliderChallenge)
	r.POST("/pub/slider/verify", m.verifySlider)
	r.GET("/pub/login/verification", m.loginVerification)
	r.POST("/pub/captcha", m.refreshLoginCaptcha)
	r.POST("/pub/captcha/verify", m.verifyLoginCaptcha)
	r.POST("/pub/login/email-code", m.sendLoginEmailCode)
    r.POST("/pub/totp/reset/email-code", m.sendResetCode)
    r.POST("/pub/totp/reset/confirm", m.confirmEmailReset)
    r.POST("/email/code", m.sendBindEmailCode)
    r.POST("/email/sensitive-code", m.sendSensitiveCode)
    r.POST("/email/bind", m.bindEmail)
    r.GET("/email/unbind/status", m.unlinkStatus)
    r.POST("/email/unbind/request", m.requestUnlink)
    r.POST("/email/unbind", m.verifyUnlink)
    r.POST("/pub/email/unbind", m.confirmUnlink)
    r.POST("/pub/login", m.login)
	r.POST("/pub/refresh", m.refresh)
	r.POST("/pub/logout", m.logout)
	r.GET("/permission", m.checkPermission)
	r.POST("/logout", m.logout)
	r.POST("/challenge", m.passwordChangeChallenge)
	r.POST("/captcha", m.refreshPasswordChangeCaptcha)
	r.POST("/captcha/verify", m.verifyPasswordChangeCaptcha)
	r.PATCH("/change/pwd", m.changePassword)
	r.PATCH("/reset/pwd", m.resetPassword)
	r.GET("/totp", m.totpStatus)
	r.POST("/totp/challenge", m.totpChallenge)
	r.POST("/totp/setup", m.totpSetup)
	r.POST("/totp/confirm", m.totpConfirm)
	r.POST("/totp/test", m.totpTest)
	r.POST("/totp/recovery/email-code", m.sendAccountResetCode)
	r.POST("/totp/recovery/confirm", m.confirmAccountReset)
	r.POST("/totp/reset/:id", m.totpReset)
	r.GET("/pub/login/email-code/status", m.loginEmailCodeStatus)
    r.GET("/email/code/status", m.bindEmailCodeStatus)
    r.GET("/info", m.info)
	r.PATCH("/info", m.updateInfo)
}
{{ else -}}
func (m *Module) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Get("/pub/i18n", m.localization)
	r.Get("/pub/opt", m.systemProperties)
 r.Get("/pub/timezones", m.timezoneOptions)
	r.Get("/pub/status", m.publicStatus)
	r.Get("/status", m.status)
	r.Get("/pub/register/username", m.usernameAvailability)
	r.Post("/pub/challenge", m.publicChallenge)
	r.Post("/pub/register", m.register)
	r.Post("/pub/slider/challenge", m.sliderChallenge)
	r.Post("/pub/slider/verify", m.verifySlider)
	r.Get("/pub/login/verification", m.loginVerification)
	r.Post("/pub/captcha", m.refreshLoginCaptcha)
	r.Post("/pub/captcha/verify", m.verifyLoginCaptcha)
	r.Post("/pub/login/email-code", m.sendLoginEmailCode)
    r.Post("/pub/totp/reset/email-code", m.sendResetCode)
    r.Post("/pub/totp/reset/confirm", m.confirmEmailReset)
    r.Post("/email/code", m.sendBindEmailCode)
    r.Post("/email/sensitive-code", m.sendSensitiveCode)
    r.Post("/email/bind", m.bindEmail)
    r.Get("/email/unbind/status", m.unlinkStatus)
    r.Post("/email/unbind/request", m.requestUnlink)
    r.Post("/email/unbind", m.verifyUnlink)
    r.Post("/pub/email/unbind", m.confirmUnlink)
    r.Post("/pub/login", m.login)
	r.Post("/pub/refresh", m.refresh)
	r.Post("/pub/logout", m.logout)
	r.Get("/permission", m.checkPermission)
	r.Post("/logout", m.logout)
	r.Post("/challenge", m.passwordChangeChallenge)
	r.Post("/captcha", m.refreshPasswordChangeCaptcha)
	r.Post("/captcha/verify", m.verifyPasswordChangeCaptcha)
	r.Patch("/change/pwd", m.changePassword)
	r.Patch("/reset/pwd", m.resetPassword)
	r.Get("/totp", m.totpStatus)
	r.Post("/totp/challenge", m.totpChallenge)
	r.Post("/totp/setup", m.totpSetup)
	r.Post("/totp/confirm", m.totpConfirm)
	r.Post("/totp/test", m.totpTest)
	r.Post("/totp/recovery/email-code", m.sendAccountResetCode)
	r.Post("/totp/recovery/confirm", m.confirmAccountReset)
	r.Post("/totp/reset/{id}", m.totpReset)
	r.Get("/pub/login/email-code/status", m.loginEmailCodeStatus)
    r.Get("/email/code/status", m.bindEmailCodeStatus)
    r.Get("/info", m.info)
	r.Patch("/info", m.updateInfo)
	return r
}
{{ end }}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) checkPermission(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) checkPermission(w http.ResponseWriter, r *http.Request) {
{{ end }}
	if m.AllowAnonymousHTTP(r) {
		server.WriteOK(w)
		return
	}
	identity, allowed, err := m.CheckPermission(r.Context(), PermissionTargetFromRequest(r))
	if errors.Is(err, ErrEmailBindingRequired) || errors.Is(err, ErrTOTPEnrollmentRequired) { server.WriteHTTPSecurityError(w, r, err); return }
    if errors.Is(err, authn.ErrPasswordResetRequired) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "check permission failed: "+err.Error(), "operation", "check_permission", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	if !allowed {
		server.WriteError(w, r, http.StatusForbidden, apperror.Forbidden)
		return
	}
    target := PermissionTargetFromRequest(r)
    if err := m.checkEmailTarget(r.Context(), target.Method, target.Path); err != nil { server.WriteHTTPSecurityError(w, r, err); return }
    if target.Resource == "" {
        if err := m.checkSensitiveRequest(r, target.Method, target.Path); err != nil {
            server.WriteHTTPSecurityError(w, r, err)
            return
        }
    }
	w.Header().Set(permissionResponseCode, identity.Code)
	server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) usernameAvailability(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) usernameAvailability(w http.ResponseWriter, r *http.Request) {
{{ end }}
	if !m.registrationEnabled {
		server.WriteError(w, r, http.StatusNotFound, apperror.NotFound)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	usernames, provided := query["username"]
	if !provided || len(usernames) != 1 {
		server.WriteError(w, r, http.StatusBadRequest, apperror.UsernameQuery)
		return
	}
	availability, err := m.UsernameAvailability(r.Context(), usernames[0])
	if errors.Is(err, ErrInvalidRegistration) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalidRegistration)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "check username availability failed: "+err.Error(), "operation", "check_username_availability", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, availability)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) publicChallenge(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) publicChallenge(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input ChallengeInput
	if err := server.DecodeJSON(w, r, &input, server.BodyAuthChallenge); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	var (
		challenge *CredentialChallenge
		err       error
	)
	switch input.Purpose {
	case challengePurposeLogin:
		challenge, err = m.credentials.Challenge(r.Context())
	case challengePurposeRegister:
		challenge, err = m.credentials.RegistrationChallenge(r.Context())
	default:
		server.WriteError(w, r, http.StatusBadRequest, apperror.PublicPurpose)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "create "+input.Purpose+" credential challenge failed: "+err.Error(), "operation", "create_credential_challenge", "outcome", "failure", "error", err, "purpose", input.Purpose)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, challenge)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) sliderChallenge(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) sliderChallenge(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input SliderChallengeInput
	if err := server.DecodeJSON(w, r, &input, server.BodySliderChallenge); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	sliderChallenge, err := m.sliderCaptcha.IssueSliderChallenge(r.Context(), input.Purpose, input.Username)
	if errors.Is(err, ErrEmailInvalid) || errors.Is(err, ErrEmailUnavailable) { writeEmailError(w, r, err, nil); return }
    if errors.Is(err, ErrSliderCaptchaRequired) {
		server.WriteError(w, r, http.StatusBadRequest, ErrSliderCaptchaRequired)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "create slider captcha challenge failed: "+err.Error(), "operation", "create_slider_captcha_challenge", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, sliderChallenge)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) verifySlider(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) verifySlider(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input SliderCaptchaVerification
	if err := server.DecodeJSON(w, r, &input, server.BodyAuth); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	result, err := m.sliderCaptcha.VerifySlider(r.Context(), input)
	if errors.Is(err, ErrEmailInvalid) || errors.Is(err, ErrEmailUnavailable) { writeEmailError(w, r, err, nil); return }
    if errors.Is(err, ErrSliderCaptchaRequired) {
		server.WriteError(w, r, http.StatusBadRequest, ErrSliderCaptchaRequired)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "verify slider captcha failed: "+err.Error(), "operation", "verify_slider_captcha", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) register(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) register(w http.ResponseWriter, r *http.Request) {
{{ end }}
	if !m.registrationEnabled {
		server.WriteError(w, r, http.StatusNotFound, apperror.NotFound)
		return
	}
	var encrypted EncryptedRegisterInput
	if err := server.DecodeJSON(w, r, &encrypted, server.BodyAuth); err != nil {
		server.WriteJSON(w, http.StatusBadRequest, PasswordChangeFailureResponse{ErrorCode: apperror.Code(err), Msg: server.ErrorMessage(r, err)})
		return
	}
	input, err := m.credentials.OpenRegistration(r.Context(), encrypted)
	if errors.Is(err, ErrInvalidRegistrationCredential) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalidRegistrationCredential)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "decrypt registration credential failed: "+err.Error(), "operation", "decrypt_registration_credential", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	if err := m.sliderCaptcha.ConsumeProof(r.Context(), sliderCaptchaPurposeRegister, input.Username, input.SliderProof); errors.Is(err, ErrSliderCaptchaRequired) {
		server.WriteError(w, r, http.StatusBadRequest, ErrSliderCaptchaRequired)
		return
	} else if err != nil {
		slog.ErrorContext(r.Context(), "consume registration slider proof failed: "+err.Error(), "operation", "consume_registration_slider_proof", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	if err := m.Register(r.Context(), input); errors.Is(err, ErrInvalidRegistration) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalidRegistration)
		return
	} else if errors.Is(err, ErrUsernameExists) {
		server.WriteError(w, r, http.StatusConflict, ErrUsernameExists)
		return
	} else if err != nil {
		slog.ErrorContext(r.Context(), "register user failed: "+err.Error(), "operation", "register_user", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) passwordChangeChallenge(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) passwordChangeChallenge(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input ChallengeInput
	if err := server.DecodeJSON(w, r, &input, server.BodyAuthChallenge); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
    identity, _ := authn.FromContext(r.Context())
    if input.Purpose == "totp_setup" {
        if !m.totpEnabled() { writeVerificationError(w, r, ErrVerificationUnavailable); return }
        challenge, err := m.credentials.TOTPSetupChallenge(r.Context(), identity.UserID)
        if err != nil { writeVerificationError(w, r, err); return }
        server.WriteOK(w, challenge)
        return
    }
    required, enrollmentErr := m.totpEnrollmentRequired(r.Context(), identity)
    if enrollmentErr != nil { writeVerificationError(w, r, enrollmentErr); return }
    if required && input.Purpose != "sensitive" { server.WriteHTTPSecurityError(w, r, ErrTOTPEnrollmentRequired); return }
	if input.Purpose == "sensitive" {
        identity, ok := authn.FromContext(r.Context())
        if !ok || identity.SessionID == "" { writeEmailError(w, r, ErrUnauthorized, nil); return }
        challenge, err := m.credentials.challenge(r.Context(), sensitiveScope(identity), identity.UserID)
        if err != nil { writeEmailError(w, r, err, nil); return }
        server.WriteOK(w, challenge)
        return
    }
    if input.Purpose == "email_bind" || input.Purpose == "email_unbind" {
        identity, _ := authn.FromContext(r.Context())
        var challenge *CredentialChallenge
        var err error
        if input.Purpose == "email_unbind" { challenge, err = m.credentials.challenge(r.Context(), unlinkCredentialType, identity.UserID) } else { challenge, err = m.credentials.EmailBindChallenge(r.Context(), identity.UserID) }
        if err != nil { writeEmailError(w, r, err, nil); return }
        server.WriteOK(w, challenge)
        return
    }
    if input.Purpose != challengePurposePasswordChange && input.Purpose != "password_reset" {
		server.WriteError(w, r, http.StatusBadRequest, apperror.PasswordPurpose)
		return
	}
	var challenge *CredentialChallenge
	var err error
	if input.Purpose == "password_reset" {
		challenge, err = m.PasswordResetChallenge(r.Context())
	} else {
		identity, _ := authn.FromContext(r.Context())
		if identity.PasswordResetRequired {
			server.WriteError(w, r, http.StatusForbidden, authn.ErrPasswordResetRequired)
			return
		}
		challenge, err = m.PasswordChangeChallenge(r.Context())
	}
	if errors.Is(err, ErrPasswordResetNotRequired) {
		server.WriteError(w, r, http.StatusConflict, err)
		return
	}
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "create password change challenge failed: "+err.Error(), "operation", "create_password_change_challenge", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, challenge)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) changePassword(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) changePassword(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var encrypted EncryptedPasswordChangeInput
	if err := server.DecodeJSON(w, r, &encrypted, server.BodyAuth); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	input, err := m.OpenPasswordChange(r.Context(), encrypted)
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if errors.Is(err, ErrInvalidPasswordCredential) {
		server.WriteJSON(w, http.StatusBadRequest, PasswordChangeFailureResponse{ErrorCode: apperror.Code(ErrInvalidPasswordCredential), Msg: server.ErrorMessage(r, ErrInvalidPasswordCredential)})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "decrypt password change credential failed: "+err.Error(), "operation", "decrypt_password_change_credential", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	changeErr := m.ChangePassword(r.Context(), input)
	if errors.Is(changeErr, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if errors.Is(changeErr, ErrPasswordChangeLocked) {
		server.WriteJSON(w, http.StatusForbidden, PasswordChangeFailureResponse{ErrorCode: apperror.Code(ErrPasswordChangeLocked), Msg: server.ErrorMessage(r, ErrPasswordChangeLocked)})
		return
	}
	if requiresPasswordChangeCaptcha(changeErr) {
		captcha, err := m.NewPasswordChangeCaptcha(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "create password change captcha failed: "+err.Error(), "operation", "create_password_change_captcha", "outcome", "failure", "error", err)
			server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
			return
		}
		server.WriteJSON(w, http.StatusBadRequest, PasswordChangeFailureResponse{
			ErrorCode: apperror.Code(changeErr), Msg: server.ErrorMessage(r, changeErr), CaptchaRequired: true, Captcha: captcha,
		})
		return
	}
	if errors.Is(changeErr, ErrInvalidPasswordChange) || errors.Is(changeErr, ErrIncorrectPassword) || errors.Is(changeErr, ErrSamePassword) || errors.Is(changeErr, ErrPointCaptchaRequired) {
		server.WriteJSON(w, http.StatusBadRequest, PasswordChangeFailureResponse{ErrorCode: apperror.Code(changeErr), Msg: server.ErrorMessage(r, changeErr)})
		return
	}
	if changeErr != nil {
		slog.ErrorContext(r.Context(), "change current password failed: "+changeErr.Error(), "operation", "change_current_password", "outcome", "failure", "error", changeErr)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) loginVerification(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) loginVerification(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	usernames, provided := query["username"]
	if !provided || len(usernames) != 1 {
		server.WriteError(w, r, http.StatusBadRequest, apperror.UsernameQuery)
		return
	}
	verification, err := m.LoginVerification(r.Context(), usernames[0])
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "check login verification failed: "+err.Error(), "operation", "check_login_verification", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, verification)
}

type LoginPointCaptchaInput struct {
 Username string `json:"username,omitempty"`
 CaptchaID string `json:"captcha_id,omitempty"`
}

type RefreshPointCaptchaInput struct {
	CaptchaID string `json:"captcha_id"`
}

type VerifyPointCaptchaInput struct {
	CaptchaID     string         `json:"captcha_id"`
	CaptchaPoints []CaptchaPoint `json:"captcha_points"`
}

type VerifyLoginPointCaptchaInput struct {
	Username      string         `json:"username"`
	CaptchaID     string         `json:"captcha_id"`
	CaptchaPoints []CaptchaPoint `json:"captcha_points"`
}

type LoginFailureResponse struct {
	ErrorCode       string                 `json:"error_code"`
	Msg             string                 `json:"msg"`
	CaptchaRequired bool                   `json:"captcha_required"`
	Captcha         *PointCaptchaChallenge `json:"captcha,omitempty"`
}

type PasswordChangeFailureResponse struct {
	ErrorCode       string                 `json:"error_code"`
	Msg             string                 `json:"msg"`
	CaptchaRequired bool                   `json:"captcha_required"`
	Captcha         *PointCaptchaChallenge `json:"captcha,omitempty"`
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) verifyPasswordChangeCaptcha(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) verifyPasswordChangeCaptcha(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input VerifyPointCaptchaInput
	if err := server.DecodeJSON(w, r, &input, server.BodyAuthVerification); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	result, err := m.VerifyPasswordChangeCaptcha(r.Context(), input.CaptchaID, input.CaptchaPoints)
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "verify password change captcha failed: "+err.Error(), "operation", "verify_password_change_captcha", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) verifyLoginCaptcha(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) verifyLoginCaptcha(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input VerifyLoginPointCaptchaInput
	if err := server.DecodeJSON(w, r, &input, server.BodyAuthVerification); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	result, err := m.VerifyLoginCaptcha(r.Context(), input.Username, input.CaptchaID, input.CaptchaPoints)
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "verify login captcha failed: "+err.Error(), "operation", "verify_login_captcha", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) refreshPasswordChangeCaptcha(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) refreshPasswordChangeCaptcha(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input RefreshPointCaptchaInput
	if err := server.DecodeJSON(w, r, &input, server.BodyAuthChallenge); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	challenge, err := m.RefreshPasswordChangeCaptcha(r.Context(), input.CaptchaID)
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if errors.Is(err, ErrPointCaptchaNotFound) {
		server.WriteError(w, r, http.StatusBadRequest, apperror.CaptchaExpired)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "refresh password change captcha failed: "+err.Error(), "operation", "refresh_password_change_captcha", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, challenge)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) refreshLoginCaptcha(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) refreshLoginCaptcha(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input LoginPointCaptchaInput
	if err := server.DecodeJSON(w, r, &input, server.BodyAuthChallenge); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
    if input.Username != "" {
        if input.CaptchaID != "" { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody); return }
        verification, err := m.LoginVerification(r.Context(), input.Username)
        if err != nil { writeEmailError(w, r, err, nil); return }
        server.WriteOK(w, verification)
        return
    }
	pointCaptchaChallenge, err := m.captcha.RefreshChallenge(r.Context(), input.CaptchaID)
	if errors.Is(err, ErrPointCaptchaNotFound) {
		server.WriteError(w, r, http.StatusBadRequest, apperror.CaptchaExpired)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "refresh point captcha failed: "+err.Error(), "operation", "refresh_point_captcha", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, pointCaptchaChallenge)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) refresh(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) refresh(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input RefreshTokenInput
	if err := server.DecodeJSON(w, r, &input, server.BodyAuthVerification); err != nil || strings.TrimSpace(input.RefreshToken) == "" {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	refreshResult, err := m.Refresh(r.Context(), input.RefreshToken)
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "refresh login failed: "+err.Error(), "operation", "refresh_login", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
    writeLoginHeaders(w)
	server.WriteOK(w, refreshResult)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) login(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) login(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
    ticket := strings.TrimSpace(r.Header.Get(authn.MFATicketHeader))
    recoveryHeaders := r.Header.Values(authn.RecoveryCodeHeader)
	if len(recoveryHeaders) > 0 {
		auditctx.AlsoRecordUserOperation(r.Context(), "recover_totp_recovery_code")
	}
    if len(recoveryHeaders) > 0 && (len(recoveryHeaders) != 1 || ticket == "" || len(r.Header.Values(authn.MFATicketHeader)) != 1 || len(r.Header.Values(authn.TOTPCodeHeader)) > 0) {
        writeVerificationError(w, r, ErrVerificationInvalid)
        return
    }
    if ticket != "" {
        var continuation *EncryptedLoginInput
        if err := server.DecodeJSON(w, r, &continuation, server.BodyAuth); err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
        if continuation == nil { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody); return }
        var result *LoginResult
        var err error
        if len(recoveryHeaders) > 0 {
            var password string
            if continuation.ChallengeID != "" || continuation.Credential != "" {
                credential, openErr := m.credentials.Open(r.Context(), *continuation)
                if openErr != nil || credential.Method == "email_code" { writeEmailError(w, r, ErrInvalidCredential, nil); return }
                password = credential.Password
            }
            result, err = m.RecoverTOTPLogin(r.Context(), ticket, recoveryHeaders[0], password)
        } else {
            if continuation.ChallengeID != "" || continuation.Credential != "" { writeEmailError(w, r, ErrInvalidCredential, nil); return }
            result, err = m.CompleteTOTPLogin(r.Context(), ticket, r.Header.Get(authn.TOTPCodeHeader))
        }
        if err != nil { writeVerificationError(w, r, err); return }
        writeLoginHeaders(w)
        server.WriteOK(w, result)
        return
    }

	var encrypted EncryptedLoginInput
	if err := server.DecodeJSON(w, r, &encrypted, server.BodyAuth); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	input, err := m.credentials.Open(r.Context(), encrypted)
	if errors.Is(err, ErrInvalidCredential) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalidCredential)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "decrypt login credential failed: "+err.Error(), "operation", "decrypt_login_credential", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	auditctx.Identity(r.Context(), 0, strings.TrimSpace(input.Username))
	loginResult, err := m.Login(r.Context(), input)
	if err != nil {
		auditctx.Error(r.Context(), apperror.Code(err))
	}
	if errors.Is(err, ErrEmailInvalid) || errors.Is(err, ErrEmailUnavailable) { writeEmailError(w, r, err, nil); return }
    if errors.Is(err, ErrSliderCaptchaRequired) {
		server.WriteError(w, r, http.StatusBadRequest, ErrSliderCaptchaRequired)
		return
	}
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if errors.Is(err, ErrLoginFailed) || errors.Is(err, ErrPointCaptchaRequired) {
		if !requiresPointCaptcha(err) {
			server.WriteJSON(w, http.StatusUnauthorized, LoginFailureResponse{ErrorCode: apperror.Code(err), Msg: server.ErrorMessage(r, err)})
			return
		}
		captcha, challengeErr := m.captcha.NewChallenge(r.Context(), input.Username)
		if challengeErr != nil {
			slog.ErrorContext(r.Context(), "create point captcha failed: "+challengeErr.Error(), "operation", "create_point_captcha", "outcome", "failure", "error", challengeErr)
			server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
			return
		}
		server.WriteJSON(w, http.StatusUnauthorized, LoginFailureResponse{
			ErrorCode: apperror.Code(err), Msg: server.ErrorMessage(r, err), CaptchaRequired: true, Captcha: captcha,
		})
		return
	}
	if errors.Is(err, ErrPendingApproval) || errors.Is(err, ErrLocked) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "login failed: "+err.Error(), "operation", "login", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
    if loginResult.MFARequired {
        if code := strings.TrimSpace(r.Header.Get(authn.TOTPCodeHeader)); code != "" {
            verified, verifyErr := m.CompleteTOTPLogin(r.Context(), loginResult.MFATicket, code)
            if verifyErr == nil {
                writeLoginHeaders(w)
                server.WriteOK(w, verified)
                return
            }
            w.Header().Set(authn.MFATicketHeader, loginResult.MFATicket)
            w.Header().Set(authn.MFAExpiresAtHeader, strconv.FormatInt(loginResult.ExpiredAt, 10))
            writeVerificationError(w, r, verifyErr)
            return
        }
        w.Header().Set(authn.MFATicketHeader, loginResult.MFATicket)
        w.Header().Set(authn.MFAExpiresAtHeader, strconv.FormatInt(loginResult.ExpiredAt, 10))
        writeVerificationError(w, r, ErrLoginMFARequired)
        return
    }
    writeLoginHeaders(w)
	server.WriteOK(w, loginResult)
}

type SelfPreferences struct {
	OnboardingPreferences
	Language *string `json:"language,omitempty"`
 Timezone *string `json:"timezone,omitempty"`
 UIPreferences map[string]dictionary.UIPreferences `json:"ui_preferences,omitempty"`
}

func (p SelfPreferences) valid() bool {
    if p.UIPreferences != nil {
        _, ok := p.UIPreferences[dictionary.UIPreferenceClient]
        return ok && len(p.UIPreferences)==1 && p.Language==nil && p.Timezone==nil && !p.EmailBindingSkipped && !p.TOTPEnrollmentSkipped && !p.PasswordResetSkipped
    }
    if p.Language != nil || p.Timezone != nil {
        if p.EmailBindingSkipped || p.TOTPEnrollmentSkipped || p.PasswordResetSkipped { return false }
        return (p.Language != nil && *p.Language != "" && p.Timezone == nil) || (p.Timezone != nil && *p.Timezone != "" && p.Language == nil)
    }
    return p.OnboardingPreferences.valid()
}

type UpdateInfoInput struct {
	Metadata SelfPreferences `json:"metadata"`
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) updateInfo(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) updateInfo(w http.ResponseWriter, r *http.Request) {
{{ end }}
	var input UpdateInfoInput
	if err := server.DecodeJSON(w, r, &input, server.BodyBusiness); err != nil || !input.Metadata.valid() {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
    if input.Metadata.UIPreferences != nil {
        result, err := m.UpdateUIPreferences(r.Context(), input.Metadata.UIPreferences)
        if errors.Is(err, ErrInvalid) { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody); return }
        if err != nil { writeEmailError(w, r, err, nil); return }
        server.WriteOK(w, result)
        return
    }
	var err error
	if input.Metadata.Language != nil {
		err = m.UpdateLanguage(r.Context(), *input.Metadata.Language)
	} else if input.Metadata.Timezone != nil {
 err = m.UpdateTimezone(r.Context(), *input.Metadata.Timezone)
	} else {
		err = m.SkipOnboarding(r.Context(), input.Metadata.OnboardingPreferences)
	}
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err != nil {
		writeEmailError(w, r, err, nil)
		return
	}
	server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) info(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) info(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	fields, err := report.ParseFields(query, infoFields...)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	client, err := preferenceClient(query)
    if err != nil || (client != "" && (fields == nil || !fields.Has("ui_preferences"))) { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery); return }
    info, err := m.InfoSelected(r.Context(), authn.PermissionScope(r.Context()), fields, client)
    if errors.Is(err, ErrInvalid) { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery); return }
	if errors.Is(err, ErrInfoScopeInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}

	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get current user failed: "+err.Error(), "operation", "get_current_user", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, info)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) logout(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) logout(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var input RefreshTokenInput
	if err := server.DecodeJSON(w, r, &input, server.BodyAuthVerification); err != nil || strings.TrimSpace(input.RefreshToken) == "" {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := m.Logout(r.Context(), input.RefreshToken); err != nil {
		slog.ErrorContext(r.Context(), "logout failed: "+err.Error(), "operation", "logout", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) resetPassword(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) resetPassword(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var encrypted EncryptedPasswordResetInput
	if err := server.DecodeJSON(w, r, &encrypted, server.BodyAuth); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	input, err := m.OpenPasswordReset(r.Context(), encrypted)
	var result *LoginResult
	if err == nil {
		result, err = m.ResetPassword(r.Context(), input)
	}
	switch {
	case errors.Is(err, ErrUnauthorized):
		server.WriteError(w, r, http.StatusUnauthorized, err)
	case errors.Is(err, ErrTOTPEnrollmentRequired):
        server.SetVerificationErrorHeaders(w, err)
		server.WriteError(w, r, http.StatusForbidden, err)
	case errors.Is(err, ErrPasswordResetNotRequired):
		server.WriteError(w, r, http.StatusConflict, err)
	case errors.Is(err, ErrInvalidPasswordCredential), errors.Is(err, ErrInvalidPasswordReset), errors.Is(err, ErrSamePassword):
		server.WriteError(w, r, http.StatusBadRequest, err)
	case err != nil:
		slog.ErrorContext(r.Context(), "reset current password failed: "+err.Error(), "operation", "reset_current_password", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
	default:
		writeLoginHeaders(w)
	server.WriteOK(w, result)
	}
}

// Session restrictions are read through GET /status?purpose=onboarding&f=requirements.
// Session responses contain tokens only and are never cacheable.
func writeLoginHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}
