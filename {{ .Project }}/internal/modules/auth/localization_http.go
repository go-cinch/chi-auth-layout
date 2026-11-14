package auth

import (
	"net/http"
	"net/url"

	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules/localization"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
)

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) localization(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) localization(w http.ResponseWriter, r *http.Request) {
{{ end }}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	for key := range query {
		if key != "f" {
			server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
			return
		}
	}
	fields, err := report.ParseFields(query, "default_locale", "locales", "scopes", "operations", "group_order", "messages", "frontend_messages")
	if err != nil || (fields != nil && fields.Has("messages") && fields.Has("frontend_messages")) {
		server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery)
		return
	}
	if fields == nil {
		fields = report.Fields{"default_locale", "locales"}
	}
	w.Header().Set("Cache-Control", "no-cache")
	var value localization.PublicCatalog = m.catalogSource.PublicCatalog(fields...)
	server.WriteOK(w, value)
}
