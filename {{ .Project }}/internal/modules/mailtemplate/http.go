package mailtemplate

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- else }}
	"github.com/go-chi/chi/v5"
{{- end }}
)

var _ modules.HTTPModule = (*Module)(nil)
var _ modules.IdempotentHTTPModule = (*Module)(nil)

func (*Module) Name() string { return "/mail-template" }

func (*Module) Idempotent() bool { return true }

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) HTTP(r *gin.RouterGroup) {
	r.GET("", m.list)
	r.POST("", m.create)
	r.GET("/:id", m.get)
	r.PATCH("/:id", m.update)
	r.DELETE("/:id", m.remove)
	r.POST("/:id/publish", m.publish)
	r.POST("/:id/test-send", m.testSend)
	r.GET("/:id/version", m.versions)
	r.GET("/:id/version/:version", m.version)
}

{{ else -}}
func (m *Module) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Get("/", m.list)
	r.Post("/", m.create)
	r.Get("/{id}", m.get)
	r.Patch("/{id}", m.update)
	r.Delete("/{id}", m.remove)
	r.Post("/{id}/publish", m.publish)
	r.Post("/{id}/test-send", m.testSend)
	r.Get("/{id}/version", m.versions)
	r.Get("/{id}/version/{version}", m.version)
	return r
}

{{ end }}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) get(c *gin.Context) {
	w, r := c.Writer, c.Request
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil || id <= 0 {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	value, err := m.Get(r.Context(), id)
	if writeFailure(w, r, err) {
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) versions(c *gin.Context) {
	w, r := c.Writer, c.Request
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
func (m *Module) versions(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil || id <= 0 {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	value, err := m.Versions(r.Context(), id)
	if writeFailure(w, r, err) {
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) version(c *gin.Context) {
	w, r := c.Writer, c.Request
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	v, versionErr := strconv.ParseInt(c.Param("version"), 10, 64)
{{ else -}}
func (m *Module) version(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	v, versionErr := strconv.ParseInt(chi.URLParam(r, "version"), 10, 64)
{{ end }}
	if err != nil || id <= 0 {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if versionErr != nil || v <= 0 {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	value, err := m.GetVersion(r.Context(), id, v)
	if writeFailure(w, r, err) {
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) create(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) create(w http.ResponseWriter, r *http.Request) {
{{ end }}
	var input CreateInput
	if err := server.DecodeJSON(w, r, &input, server.BodyBusiness); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	value, err := m.Create(r.Context(), input)
	if writeFailure(w, r, err) {
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) update(c *gin.Context) {
	w, r := c.Writer, c.Request
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
func (m *Module) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil || id <= 0 {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	var input UpdateInput
	if err := server.DecodeJSON(w, r, &input, server.BodyBusiness); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	value, err := m.Update(r.Context(), id, input)
	if writeFailure(w, r, err) {
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) publish(c *gin.Context) {
	w, r := c.Writer, c.Request
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
func (m *Module) publish(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil || id <= 0 {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	var input PublishInput
	if err := server.DecodeJSON(w, r, &input, server.BodyBusiness); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	value, err := m.Publish(r.Context(), id, input)
	if writeFailure(w, r, err) {
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) remove(c *gin.Context) {
	w, r := c.Writer, c.Request
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
func (m *Module) remove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil || id <= 0 {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if writeFailure(w, r, m.Delete(r.Context(), id)) {
		return
	}
	server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) list(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) list(w http.ResponseWriter, r *http.Request) {
{{ end }}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	var input ListInput
	for key, target := range map[string]**int32{"p": &input.P, "s": &input.S} {
		if values, ok := query[key]; ok {
			if len(values) != 1 {
				server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
				return
			}
			value, err := strconv.ParseInt(values[0], 10, 32)
			if err != nil {
				server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
				return
			}
			parsed := int32(value)
			*target = &parsed
		}
	}
	for key, target := range map[string]*string{"q": &input.Q, "category": &input.Category} {
		if values, ok := query[key]; ok {
			if len(values) != 1 {
				server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
				return
			}
			*target = values[0]
		}
	}
	value, err := m.List(r.Context(), input)
	if writeFailure(w, r, err) {
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) testSend(c *gin.Context) {
	w, r := c.Writer, c.Request
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
func (m *Module) testSend(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil || id <= 0 {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	var input TestSendInput
	if err := server.DecodeJSON(w, r, &input, server.BodyBusiness); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	if writeFailure(w, r, m.TestSend(r.Context(), id, input)) {
		return
	}
	server.WriteOK(w)
}

func writeFailure(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, authn.ErrUnauthorized):
		server.WriteError(w, r, http.StatusUnauthorized, apperror.Unauthorized)
	case errors.Is(err, ErrEmailRequired):
		server.WriteError(w, r, http.StatusForbidden, err)
	case errors.Is(err, ErrNotFound):
		server.WriteError(w, r, http.StatusNotFound, err)
	case errors.Is(err, ErrConflict), errors.Is(err, ErrDuplicate):
		server.WriteError(w, r, http.StatusConflict, err)
	case errors.Is(err, ErrDeliveryUnavailable):
		server.WriteError(w, r, http.StatusServiceUnavailable, err)
	case errors.Is(err, ErrInvalid):
		server.WriteError(w, r, http.StatusBadRequest, err)
	default:
		slog.ErrorContext(r.Context(), "email template operation failed: "+err.Error(), "operation", "mail_template", "outcome", "failure", "error", err)
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
	}
	return true
}
