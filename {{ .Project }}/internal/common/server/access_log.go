{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"{{ .Computed.module_name_final }}/internal/common/redact"
)

func AccessLog(policy redact.Policy) gin.HandlerFunc {
	logger := slog.Default()
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "-"
		}
		message := fmt.Sprintf("%s %d %dms %s %s",
			c.Request.Method, c.Writer.Status(), time.Since(start).Milliseconds(),
			route, redactedPath(c.Request, c.Params, policy))
		logger.InfoContext(c.Request.Context(), message,
			"remote_addr", c.Request.RemoteAddr, "user_agent", c.Request.UserAgent())
	}
}

func redactedPath(r *http.Request, params gin.Params, policy redact.Policy) string {
	replacements := make([]string, 0, len(params)*2)
	for _, param := range params {
		if param.Key != "" && param.Value != "" && policy.IsSensitive(param.Key) {
			replacements = append(replacements, param.Value, policy.Value(param.Key, param.Value))
		}
	}
	if len(replacements) == 0 {
		return r.URL.Path
	}
	return strings.NewReplacer(replacements...).Replace(r.URL.Path)
}
{{ else -}}
package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/redact"

	"github.com/go-chi/chi/v5"
)

func AccessLog(policy redact.Policy) func(http.Handler) http.Handler {
	logger := slog.Default()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			writer := wrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(writer, r)

			status := writer.Status()
			if status == 0 {
				status = http.StatusOK
			}
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "-"
			}
			message := fmt.Sprintf("%s %d %dms %s %s",
				r.Method, status, time.Since(start).Milliseconds(), route, redactedPath(r, policy))
			logger.InfoContext(r.Context(), message,
				"remote_addr", r.RemoteAddr, "user_agent", r.UserAgent())
		})
	}
}

func redactedPath(r *http.Request, policy redact.Policy) string {
	routeContext := chi.RouteContext(r.Context())
	if routeContext == nil {
		return r.URL.Path
	}
	replacements := make([]string, 0, len(routeContext.URLParams.Keys)*2)
	for index, name := range routeContext.URLParams.Keys {
		if name == "" || index >= len(routeContext.URLParams.Values) || !policy.IsSensitive(name) {
			continue
		}
		value := routeContext.URLParams.Values[index]
		if value != "" {
			replacements = append(replacements, value, policy.Value(name, value))
		}
	}
	if len(replacements) == 0 {
		return r.URL.Path
	}
	return strings.NewReplacer(replacements...).Replace(r.URL.Path)
}
{{ end -}}
