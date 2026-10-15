package audit

import (
	"encoding/csv"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/auditctx"
	"{{ .Computed.module_name_final }}/internal/common/report"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/modules"

{{ if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{ else }}
	"github.com/go-chi/chi/v5"
{{ end }}
)

var _ modules.HTTPModule = (*Module)(nil)
var _ modules.HTTPAuditRecorder = (*Module)(nil)

func (*Module) Name() string { return "/audit" }

{{ if eq .Computed.http_router_final "gin" }}
func (m *Module) HTTP(r *gin.RouterGroup) {
	r.GET("/login", m.list)
	r.GET("/login/summary", m.summary)
	r.GET("/login/option", m.options)
	r.GET("/login/export", m.export)
	r.GET("/login/detail/:id", m.get)
	r.GET("/operation", m.list)
	r.GET("/operation/summary", m.summary)
	r.GET("/operation/option", m.options)
	r.GET("/operation/export", m.export)
	r.GET("/operation/detail/:id", m.get)
{{ else }}
func (m *Module) HTTP() http.Handler {
	r := chi.NewRouter()
	r.Get("/login", m.list)
	r.Get("/login/summary", m.summary)
	r.Get("/login/option", m.options)
	r.Get("/login/export", m.export)
	r.Get("/login/detail/{id}", m.get)
	r.Get("/operation", m.list)
	r.Get("/operation/summary", m.summary)
	r.Get("/operation/option", m.options)
	r.Get("/operation/export", m.export)
	r.Get("/operation/detail/{id}", m.get)
	return r
{{ end }}
}

func kind(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/audit/login") {
		return "login"
	}
	return "operation"
}

func parse(r *http.Request) (Filter, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return Filter{}, ErrFilter
	}
	return parseQuery(r, query)
}

func parseQuery(r *http.Request, q url.Values) (Filter, error) {
	f := Filter{Kind: kind(r)}
	for _, key := range []string{"q", "module", "result", "from", "to", "time_zone", "p", "s", "weekday", "hour", "username", "ip", "path", "sort"} {
		if v, ok := q[key]; ok && (len(v) != 1 || v[0] == "") {
			return f, ErrFilter
		}
	}
	f.Username = q.Get("username")
	f.IP = q.Get("ip")
	f.Path = q.Get("path")
	f.Sort = q.Get("sort")
	f.Query = q.Get("q")
	f.Module = q.Get("module")
	f.Result = q.Get("result")
	f.Timezone = q.Get("time_zone")
	for key, target := range map[string]*int64{"from": &f.From, "to": &f.To} {
		if v, ok := q[key]; ok {
			n, e := strconv.ParseInt(v[0], 10, 64)
			if e != nil || n <= 0 {
				return f, ErrFilter
			}
			*target = n
		}
	}
	for key, target := range map[string]**int32{"p": &f.Page, "s": &f.Size, "weekday": &f.Weekday, "hour": &f.Hour} {
		if v, ok := q[key]; ok {
			n, e := strconv.ParseInt(v[0], 10, 32)
			if e != nil {
				return f, ErrFilter
			}
			i := int32(n)
			*target = &i
		}
	}
	return normalize(f)
}

func replyError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	var public error = apperror.Internal
	switch {
	case errors.Is(err, ErrFilter), errors.Is(err, ErrExportLimit):
		status = http.StatusBadRequest
		public = err
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
		public = err
	default:
		slog.ErrorContext(r.Context(), "query audit events failed: "+err.Error(), "operation", "query_audit_events", "outcome", "failure", "error", err)
	}
	server.WriteError(w, r, status, public)
}

{{ if eq .Computed.http_router_final "gin" }}
func (m *Module) list(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else }}
func (m *Module) list(w http.ResponseWriter, r *http.Request) {
{{ end }}

	f, err := parse(r)
	if err != nil {
		replyError(w, r, err)
		return
	}
 f.Fields, err = report.ParseFields(r.URL.Query(), listFields...)
 if err != nil { server.WriteError(w,r,http.StatusBadRequest,apperror.InvalidQuery); return }
	result, err := m.List(r.Context(), f)
	if err != nil {
		replyError(w, r, err)
		return
	}
	server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" }}
func (m *Module) summary(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else }}
func (m *Module) summary(w http.ResponseWriter, r *http.Request) {
{{ end }}

	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		replyError(w, r, ErrFilter)
		return
	}
	fields, err := report.ParseFields(query, "metrics", "trend", "heatmap", "modules", "accounts", "sources", "repeated_failures")
	if err != nil {
		replyError(w, r, ErrFilter)
		return
	}
	f, err := parseQuery(r, query)
	if err != nil {
		replyError(w, r, err)
		return
	}
	result, err := m.Summary(r.Context(), f, fields...)
	if err != nil {
		replyError(w, r, err)
		return
	}
	server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" }}
func (m *Module) options(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else }}
func (m *Module) options(w http.ResponseWriter, r *http.Request) {
{{ end }}

	f, err := parse(r)
	if err != nil {
		replyError(w, r, err)
		return
	}
	result, err := m.Options(r.Context(), f)
	if err != nil {
		replyError(w, r, err)
		return
	}
	server.WriteOK(w, result)
}

{{ if eq .Computed.http_router_final "gin" }}
func (m *Module) get(c *gin.Context) {
	w, r := c.Writer, c.Request
	raw := c.Param("id")
{{ else }}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	raw := chi.URLParam(r, "id")
{{ end }}

	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		replyError(w, r, ErrFilter)
		return
	}
	result, err := m.Get(r.Context(), kind(r), id)
	if err != nil {
		replyError(w, r, err)
		return
	}
	server.WriteOK(w, result)
}

func csvSafe(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "@") {
		return "'" + value
	}
	return value
}

{{ if eq .Computed.http_router_final "gin" }}
func (m *Module) export(c *gin.Context) {
	w, r := c.Writer, c.Request
{{ else }}
func (m *Module) export(w http.ResponseWriter, r *http.Request) {
{{ end }}

	f, err := parse(r)
	if err != nil {
		replyError(w, r, err)
		return
	}
	items, err := m.Export(r.Context(), f)
	if err != nil {
		replyError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-`+f.Kind+`.csv"`)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"id", "occurred_at_unix_ms", "username", "user_id", "module", "method", "path", "ip", "status", "duration_ms", "error_code", "trace_id"})
	for _, e := range items {
		_ = writer.Write([]string{strconv.FormatInt(e.ID, 10), strconv.FormatInt(e.OccurredAt, 10), csvSafe(e.Username), strconv.FormatInt(e.UserID, 10), e.Module, e.Method, csvSafe(e.Path), csvSafe(e.IP), strconv.Itoa(e.Status), strconv.FormatInt(e.DurationMS, 10), e.ErrorCode, csvSafe(e.TraceID)})
	}
	writer.Flush()
}

// AuditTarget keeps business scope and operation semantics in the owning capability.
func (*Module) AuditTarget(method, path string) auditctx.Target {
	target := auditctx.Target{}
	if method != http.MethodPost && method != http.MethodPatch && method != http.MethodPut && method != http.MethodDelete {
		return target
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch parts[0] {
	case "user", "role", "action", "user-group", "dictionary", "whitelist", "msg":
		target = auditctx.Target{Kind: "operation", Module: parts[0], ResourceType: parts[0]}
	case "auth":
		switch path {
		case "/auth/pub/login":
			if method == http.MethodPost {
				return auditctx.Target{Kind: "login", Module: "auth", Operation: "login", ResourceType: "session"}
			}
		case "/auth/pub/register":
			target = auditctx.Target{Kind: "operation", Module: "auth", Operation: "register", ResourceType: "user"}
		case "/auth/change/pwd":
			target = auditctx.Target{Kind: "operation", Module: "auth", Operation: "change_password", ResourceType: "user"}
		case "/auth/reset/pwd":
			target = auditctx.Target{Kind: "operation", Module: "auth", Operation: "reset_password", ResourceType: "user"}
		case "/auth/logout":
			target = auditctx.Target{Kind: "operation", Module: "auth", Operation: "logout", ResourceType: "session"}
		}
	}
	if target.Kind == "" {
		return target
	}
	if target.Operation == "" {
		switch method {
		case http.MethodPost:
			target.Operation = "create"
		case http.MethodPatch, http.MethodPut:
			target.Operation = "update"
		case http.MethodDelete:
			target.Operation = "delete"
		}
	}
	if len(parts) > 1 {
		last := parts[len(parts)-1]
		if _, err := strconv.ParseInt(strings.Split(last, ",")[0], 10, 64); err == nil {
			target.ResourceID = last
		}
	}
	if parts[0] == "msg" && len(parts) > 1 && parts[1] == "inbox" {
		if method == http.MethodPatch {
			target.Operation = "read"
		}
		if method == http.MethodDelete {
			target.Operation = "delete_personal"
		}
		if path == "/msg/inbox/read-all" {
			target.Operation = "read_all"
		}
	}
	return target
}
