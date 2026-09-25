package auth

import (
 "{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- else }}
	"github.com/go-chi/chi/v5"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules"
)

var _ modules.HTTPModule = (*Module)(nil)
var _ modules.HTTPPermissionAuthorizer = (*Module)(nil)
var _ modules.AnonymousHTTPModule = (*Module)(nil)

func (*Module) Name() string { return "/auth" }

func (*Module) PermissionEndpoint() string { return "/permission" }

const (
	permissionHeaderMethod = "X-Original-Method"
	permissionHeaderURI = "X-Original-URI"
	permissionHeaderPath = "X-Permission-URI"
	permissionHeaderResource = "X-Original-Resource"
	permissionResponseCode = "X-Code"
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

func PermissionTargetFromRequest(r *http.Request) PermissionTarget {
	requestPath := strings.TrimSpace(r.Header.Get(permissionHeaderPath))
	if requestPath == "" {
		requestPath = strings.TrimSpace(r.Header.Get(permissionHeaderURI))
	}
	return normalizePermissionTarget(PermissionTarget{
		Method: r.Header.Get(permissionHeaderMethod),
		Path: requestPath,
		Resource: r.Header.Get(permissionHeaderResource),
	})
}

func (m *Module) AllowAnonymousHTTP(r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != m.Name()+m.PermissionEndpoint() {
		return false
	}
	target := PermissionTargetFromRequest(r)
	if target.Method == "" || !strings.HasPrefix(target.Path, "/") || strings.HasPrefix(target.Path, "//") {
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
	r.GET("/pub/register/username", m.usernameAvailability)
	r.POST("/pub/challenge", m.publicChallenge)
	r.POST("/pub/register", m.register)
	r.POST("/pub/slider/challenge", m.sliderChallenge)
	r.POST("/pub/slider/verify", m.verifySlider)
	r.GET("/pub/login/verification", m.loginVerification)
	r.POST("/pub/captcha", m.refreshLoginCaptcha)
	r.POST("/pub/captcha/verify", m.verifyLoginCaptcha)
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
	r.GET("/info", m.info)
}
{{ else -}}
func (m *Module) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Get("/pub/register/username", m.usernameAvailability)
	r.Post("/pub/challenge", m.publicChallenge)
	r.Post("/pub/register", m.register)
	r.Post("/pub/slider/challenge", m.sliderChallenge)
	r.Post("/pub/slider/verify", m.verifySlider)
	r.Get("/pub/login/verification", m.loginVerification)
	r.Post("/pub/captcha", m.refreshLoginCaptcha)
	r.Post("/pub/captcha/verify", m.verifyLoginCaptcha)
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
	r.Get("/info", m.info)
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
    if errors.Is(err, authn.ErrPasswordResetRequired) {
        server.WriteError(w, r, http.StatusForbidden, err)
        return
    }
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "check permission failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	if !allowed {
		server.WriteError(w, r, http.StatusForbidden, apperror.Forbidden)
		return
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
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	usernames, provided := r.URL.Query()["username"]
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
		slog.ErrorContext(r.Context(), "check username availability failed: "+err.Error())
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
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
		slog.ErrorContext(r.Context(), "create "+input.Purpose+" credential challenge failed: "+err.Error())
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	sliderChallenge, err := m.sliderCaptcha.IssueSliderChallenge(r.Context(), input.Purpose, input.Username)
	if errors.Is(err, ErrSliderCaptchaRequired) {
		server.WriteError(w, r, http.StatusBadRequest, ErrSliderCaptchaRequired)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "create slider captcha challenge failed: "+err.Error())
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	result, err := m.sliderCaptcha.VerifySlider(r.Context(), input)
	if errors.Is(err, ErrSliderCaptchaRequired) {
		server.WriteError(w, r, http.StatusBadRequest, ErrSliderCaptchaRequired)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "verify slider captcha failed: "+err.Error())
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
	var encrypted EncryptedRegisterInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encrypted); err != nil {
		server.WriteJSON(w, http.StatusBadRequest, PasswordChangeFailureResponse{ErrorCode: apperror.Code(apperror.InvalidBody), Msg: server.ErrorMessage(r, apperror.InvalidBody)})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteJSON(w, http.StatusBadRequest, PasswordChangeFailureResponse{ErrorCode: apperror.Code(apperror.InvalidBody), Msg: server.ErrorMessage(r, apperror.InvalidBody)})
		return
	}
	input, err := m.credentials.OpenRegistration(r.Context(), encrypted)
	if errors.Is(err, ErrInvalidRegistrationCredential) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalidRegistrationCredential)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "decrypt registration credential failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	if err := m.sliderCaptcha.ConsumeProof(r.Context(), sliderCaptchaPurposeRegister, input.Username, input.SliderProof); errors.Is(err, ErrSliderCaptchaRequired) {
		server.WriteError(w, r, http.StatusBadRequest, ErrSliderCaptchaRequired)
		return
	} else if err != nil {
		slog.ErrorContext(r.Context(), "consume registration slider proof failed: "+err.Error())
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
		slog.ErrorContext(r.Context(), "register user failed: "+err.Error())
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
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
	if errors.Is(err, apperror.FeatureDisabled) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "create password change challenge failed: "+err.Error())
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encrypted); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	input, err := m.OpenPasswordChange(r.Context(), encrypted)
	if errors.Is(err, apperror.FeatureDisabled) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if errors.Is(err, ErrInvalidPasswordCredential) {
		server.WriteJSON(w, http.StatusBadRequest, PasswordChangeFailureResponse{ErrorCode: apperror.Code(ErrInvalidPasswordCredential), Msg: server.ErrorMessage(r, ErrInvalidPasswordCredential)})
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "decrypt password change credential failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	changeErr := m.ChangePassword(r.Context(), input)
	if errors.Is(changeErr, apperror.FeatureDisabled) {
		server.WriteError(w, r, http.StatusForbidden, changeErr)
		return
	}
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
			slog.ErrorContext(r.Context(), "create password change captcha failed: "+err.Error())
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
		slog.ErrorContext(r.Context(), "change current password failed: "+changeErr.Error())
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
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	usernames, provided := r.URL.Query()["username"]
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
		slog.ErrorContext(r.Context(), "check login verification failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, verification)
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
	ErrorCode string `json:"error_code"`
	Msg string `json:"msg"`
	CaptchaRequired bool                   `json:"captcha_required"`
	Captcha         *PointCaptchaChallenge `json:"captcha,omitempty"`
}

type PasswordChangeFailureResponse struct {
	ErrorCode string `json:"error_code"`
	Msg string `json:"msg"`
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	result, err := m.VerifyPasswordChangeCaptcha(r.Context(), input.CaptchaID, input.CaptchaPoints)
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "verify password change captcha failed: "+err.Error())
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	result, err := m.VerifyLoginCaptcha(r.Context(), input.Username, input.CaptchaID, input.CaptchaPoints)
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "verify login captcha failed: "+err.Error())
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
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
		slog.ErrorContext(r.Context(), "refresh password change captcha failed: "+err.Error())
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
	var input RefreshPointCaptchaInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	pointCaptchaChallenge, err := m.captcha.RefreshChallenge(r.Context(), input.CaptchaID)
	if errors.Is(err, ErrPointCaptchaNotFound) {
		server.WriteError(w, r, http.StatusBadRequest, apperror.CaptchaExpired)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "refresh point captcha failed: "+err.Error())
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF || strings.TrimSpace(input.RefreshToken) == "" {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	refreshResult, err := m.Refresh(r.Context(), input.RefreshToken)
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "refresh login failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, refreshResult)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) login(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) login(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	var encrypted EncryptedLoginInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encrypted); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	input, err := m.credentials.Open(r.Context(), encrypted)
	if errors.Is(err, ErrInvalidCredential) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalidCredential)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "decrypt login credential failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	loginResult, err := m.Login(r.Context(), input)
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
			slog.ErrorContext(r.Context(), "create point captcha failed: "+challengeErr.Error())
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
		slog.ErrorContext(r.Context(), "login failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, loginResult)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) info(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) info(w http.ResponseWriter, r *http.Request) {
{{ end }}
	info, err := m.Info(r.Context())
	if errors.Is(err, ErrUnauthorized) {
		server.WriteError(w, r, http.StatusUnauthorized, ErrUnauthorized)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "get current user failed: "+err.Error())
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
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF || strings.TrimSpace(input.RefreshToken) == "" {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := m.Logout(r.Context(), input.RefreshToken); err != nil {
		slog.ErrorContext(r.Context(), "logout failed: "+err.Error())
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
    decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
    decoder.DisallowUnknownFields()
    if decoder.Decode(&encrypted) != nil || decoder.Decode(new(any)) != io.EOF {
        server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
        return
    }
    input, err := m.OpenPasswordReset(r.Context(), encrypted)
    var result *LoginResult
    if err == nil { result, err = m.ResetPassword(r.Context(), input) }
    switch {
    case errors.Is(err, ErrUnauthorized):
        server.WriteError(w, r, http.StatusUnauthorized, err)
    case errors.Is(err, ErrPasswordResetNotRequired):
        server.WriteError(w, r, http.StatusConflict, err)
    case errors.Is(err, ErrInvalidPasswordCredential), errors.Is(err, ErrInvalidPasswordReset), errors.Is(err, ErrSamePassword):
        server.WriteError(w, r, http.StatusBadRequest, err)
    case err != nil:
        slog.ErrorContext(r.Context(), "reset current password failed: "+err.Error())
        server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
    default:
        server.WriteOK(w, result)
    }
}
