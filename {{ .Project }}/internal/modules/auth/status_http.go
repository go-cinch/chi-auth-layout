package auth

import (
	"net/http"
	"net/url"
	"strings"

{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/authn"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/common/server"
)

func parseStatusQuery(query url.Values, public bool) (StatusQuery, error) {
	q := StatusQuery{}
	for key, values := range query {
		switch key {
		case "purpose", "f", "browser_uuid", "username", "email":
		default: return q, apperror.InvalidQuery
		}
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" { return q, apperror.InvalidQuery }
	}
	q.Purpose = query.Get("purpose")
	if q.Purpose == "login_totp_recovery" {
		for key := range query {
			if key != "purpose" && key != "f" && key != "browser_uuid" { return q, apperror.InvalidQuery }
		}
	}
	allowed := statusFields(q.Purpose, public)
	if len(allowed) == 0 || len(query["f"]) != 1 { return q, apperror.InvalidQuery }
	var err error
	q.Fields, err = report.ParseFields(query, allowed...)
	if err != nil { return q, err }
	q.BrowserUUID = query.Get("browser_uuid")
	if q.Purpose == "login" { q.Identifier = strings.TrimSpace(query.Get("username")) }
	if q.Purpose == "email_bind" { q.Identifier = query.Get("email") }
	if err := q.validate(public); err != nil { return q, apperror.InvalidQuery }
	return q, nil
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) publicStatus(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) publicStatus(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery); return }
	q, err := parseStatusQuery(query, true)
	if err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
	if q.Purpose == "login_totp_recovery" {
		tickets := r.Header.Values(authn.MFATicketHeader)
		if len(tickets) != 1 { writeEmailError(w, r, ErrVerificationInvalid, nil); return }
		q.MFATicket = tickets[0]
	}
	if m.emailCodes != nil { q.IP = m.emailCodes.clientIP(r) }
	result, err := m.PublicStatus(r.Context(), q)
	if err != nil { writeEmailError(w, r, err, nil); return }
	server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" -}}
func (m *Module) status(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else -}}
func (m *Module) status(w http.ResponseWriter, r *http.Request) {
{{ end }}
	w.Header().Set("Cache-Control", "no-store")
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil { server.WriteError(w, r, http.StatusBadRequest, apperror.InvalidQuery); return }
	q, err := parseStatusQuery(query, false)
	if err != nil { server.WriteError(w, r, http.StatusBadRequest, err); return }
	if m.emailCodes != nil { q.IP = m.emailCodes.clientIP(r) }
	result, err := m.Status(r.Context(), q)
	if err != nil { writeEmailError(w, r, err, nil); return }
	server.WriteOK(w, result)
}
