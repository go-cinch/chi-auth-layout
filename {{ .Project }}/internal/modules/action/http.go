package action

import (
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
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
var _ modules.IdempotentHTTPModule = (*Module)(nil)

func (*Module) Name() string { return "/action" }

func (*Module) Idempotent() bool { return true }

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) HTTP(r *gin.RouterGroup) {
	r.GET("", m.list)
	r.GET("/group", m.listGroups)
	r.GET("/:id", m.get)
	r.POST("", m.create)
	r.PATCH("/:id", m.update)
	r.DELETE("/:id", m.delete)
}
{{ else -}}
func (m *Module) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Get("/", m.list)
	r.Get("/group", m.listGroups)
	r.Get("/{id}", m.get)
	r.Post("/", m.create)
	r.Patch("/{id}", m.update)
	r.Delete("/{id}", m.delete)
	return r
}
{{ end }}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) listGroups(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) listGroups(w http.ResponseWriter, r *http.Request) {
{{ end }}
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	query := r.URL.Query()
	if values, provided := query["keyword"]; provided && len(values) != 1 {
		server.WriteError(w, r, http.StatusBadRequest, apperror.KeywordQuery)
		return
	}
	value, err := m.ListGroups(r.Context(), query.Get("keyword"))
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "list action groups failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) get(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.ActionID)
		return
	}
	value, err := m.Get(r.Context(), id)
	if errors.Is(err, ErrInvalid) {
		server.WriteError(w, r, http.StatusBadRequest, ErrInvalid)
		return
	}
	if errors.Is(err, ErrNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), r.Method+" "+r.URL.Path+" failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
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
	var input CreateActionInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	value, err := m.Create(r.Context(), input)
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), r.Method+" "+r.URL.Path+" failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) update(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) update(w http.ResponseWriter, r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.ActionID)
		return
	}
	var input UpdateActionInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody)
		return
	}
	value, err := m.Update(r.Context(), id, input)
	if errors.Is(err, apperror.FeatureDisabled) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) {
		server.WriteError(w, r, http.StatusBadRequest, err)
		return
	}
	if errors.Is(err, ErrNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), r.Method+" "+r.URL.Path+" failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) delete(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
{{ end }}
{{ if eq .Computed.http_router_final "gin" -}}
	parts := strings.Split(c.Param("id"), ",")
{{ else -}}
	parts := strings.Split(chi.URLParam(r, "id"), ",")
{{ end }}
	if len(parts) > 100 {
		server.WriteError(w, r, http.StatusBadRequest, ErrIDs)
		return
	}
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id <= 0 {
			server.WriteError(w, r, http.StatusBadRequest, ErrIDs)
			return
		}
		ids = append(ids, id)
	}
	err := m.Delete(r.Context(), ids...)
	if errors.Is(err, apperror.FeatureDisabled) {
		server.WriteError(w, r, http.StatusForbidden, err)
		return
	}
	if errors.Is(err, ErrIDs) {
		server.WriteError(w, r, http.StatusBadRequest, ErrIDs)
		return
	}
	if errors.Is(err, ErrNotFound) {
		server.WriteError(w, r, http.StatusNotFound, ErrNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), r.Method+" "+r.URL.Path+" failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
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
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	query := r.URL.Query()
	input := ListActionsInput{
		Code: query.Get("code"), Name: query.Get("name"), Group: query.Get("group"), Word: query.Get("word"),
		Resource: query.Get("resource"), Menu: query.Get("menu"), Button: query.Get("button"),
	}
	if values, provided := query["p"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.Page)
			return
		}
		parsed, err := strconv.ParseInt(values[0], 10, 32)
		if err != nil {
			server.WriteError(w, r, http.StatusBadRequest, apperror.Page)
			return
		}
		value := int32(parsed)
		input.Page = &value
	}
	if values, provided := query["s"]; provided {
		if len(values) != 1 {
			server.WriteError(w, r, http.StatusBadRequest, apperror.PageSize)
			return
		}
		parsed, err := strconv.ParseInt(values[0], 10, 32)
		if err != nil {
			server.WriteError(w, r, http.StatusBadRequest, apperror.PageSize)
			return
		}
		value := int32(parsed)
		input.PageSize = &value
	}
	value, err := m.List(r.Context(), input)
	if err != nil {
		slog.ErrorContext(r.Context(), "list actions failed: "+err.Error())
		server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal)
		return
	}
	server.WriteOK(w, value)
}
