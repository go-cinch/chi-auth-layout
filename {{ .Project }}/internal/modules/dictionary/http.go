package dictionary

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
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

func (*Module) Name() string { return "/dictionary" }

func (*Module) Idempotent() bool { return true }

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) HTTP(r *gin.RouterGroup) {
	r.GET("", m.list)
	r.GET("/:id", m.get)
	r.POST("", m.create)
	r.PATCH("/:id", m.update)
	r.DELETE("/:id", m.delete)
}
{{ else -}}
func (m *Module) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Get("/", m.list)
	r.Get("/{id}", m.get)
	r.Post("/", m.create)
	r.Patch("/{id}", m.update)
	r.Delete("/{id}", m.delete)
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
	if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.DictionaryID); return }
	value, cacheHit, err := m.getCached(r.Context(), id)
	if errors.Is(err, ErrInvalid) { server.WriteError(w, r, http.StatusBadRequest, ErrInvalid); return }
	if errors.Is(err, ErrNotFound) { server.WriteError(w, r, http.StatusNotFound, ErrNotFound); return }
	if err != nil { slog.ErrorContext(r.Context(), "get dictionary failed: "+err.Error()); server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal); return }
	if cacheHit { w.Header().Set("X-Cache-Hit", "1") }
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) create(c *gin.Context) { w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) create(w http.ResponseWriter, r *http.Request) {
{{ end }}
	var input CreateDictionaryInput
	if !decodeDictionaryInput(w, r, &input) { return }
	value, err := m.Create(r.Context(), input)
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) { server.WriteError(w, r, http.StatusBadRequest, err); return }
	if err != nil { slog.ErrorContext(r.Context(), "create dictionary failed: "+err.Error()); server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal); return }
	server.WriteOK(w, value)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) update(c *gin.Context) { w, r := c.Writer, c.Request; id, err := strconv.ParseInt(c.Param("id"), 10, 64)
{{ else -}}
func (m *Module) update(w http.ResponseWriter, r *http.Request) { id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
{{ end }}
	if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.DictionaryID); return }
	var input UpdateDictionaryInput
	if !decodeDictionaryInput(w, r, &input) { return }
	value, err := m.Update(r.Context(), id, input)
	if errors.Is(err, apperror.FeatureDisabled) { server.WriteError(w, r, http.StatusForbidden, err); return }
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrConflict) { server.WriteError(w, r, http.StatusBadRequest, err); return }
	if errors.Is(err, ErrNotFound) { server.WriteError(w, r, http.StatusNotFound, ErrNotFound); return }
	if err != nil { slog.ErrorContext(r.Context(), "update dictionary failed: "+err.Error()); server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal); return }
	server.WriteOK(w, value)
}

func decodeDictionaryInput(w http.ResponseWriter, r *http.Request, input any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody); return false }
	if err := decoder.Decode(new(any)); err != io.EOF { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidBody); return false }
	return true
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) delete(c *gin.Context) { w, r := c.Writer, c.Request; parts := strings.Split(c.Param("id"), ",")
{{ else -}}
func (m *Module) delete(w http.ResponseWriter, r *http.Request) { parts := strings.Split(chi.URLParam(r, "id"), ",")
{{ end }}
	if len(parts) > 100 { server.WriteError(w, r, http.StatusBadRequest, ErrIDs); return }
	ids := make([]int64, 0, len(parts))
	for _, part := range parts { id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); if err != nil || id <= 0 { server.WriteError(w, r, http.StatusBadRequest, ErrIDs); return }; ids = append(ids, id) }
	err := m.Delete(r.Context(), ids...)
	if errors.Is(err, apperror.FeatureDisabled) { server.WriteError(w, r, http.StatusForbidden, err); return }
	if errors.Is(err, ErrIDs) { server.WriteError(w, r, http.StatusBadRequest, ErrIDs); return }
	if errors.Is(err, ErrNotFound) { server.WriteError(w, r, http.StatusNotFound, ErrNotFound); return }
	if err != nil { slog.ErrorContext(r.Context(), "delete dictionaries failed: "+err.Error()); server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal); return }
	server.WriteOK(w)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) list(c *gin.Context) { w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) list(w http.ResponseWriter, r *http.Request) {
{{ end }}
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery); return }
	query := r.URL.Query()
	input := ListDictionariesInput{Key: query.Get("key"), Name: query.Get("name")}
	if values, provided := query["enabled"]; provided {
		if len(values) != 1 { server.WriteError(w, r, http.StatusBadRequest, apperror.DictionaryEnabled); return }
		parsed, err := strconv.ParseBool(values[0]); if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.DictionaryEnabled); return }; input.Enabled = &parsed
	}
	if values, provided := query["p"]; provided { if len(values) != 1 { server.WriteError(w, r, http.StatusBadRequest, apperror.Page); return }; parsed, err := strconv.ParseInt(values[0], 10, 32); if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.Page); return }; value := int32(parsed); input.Page = &value }
	if values, provided := query["s"]; provided { if len(values) != 1 { server.WriteError(w, r, http.StatusBadRequest, apperror.PageSize); return }; parsed, err := strconv.ParseInt(values[0], 10, 32); if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.PageSize); return }; value := int32(parsed); input.PageSize = &value }
	value, err := m.List(r.Context(), input)
	if err != nil { slog.ErrorContext(r.Context(), "list dictionaries failed: "+err.Error()); server.WriteError(w, r, http.StatusInternalServerError, apperror.Internal); return }
	server.WriteOK(w, value)
}
