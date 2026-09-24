package server

import (
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"context"
	"log/slog"
	"net/http"
{{- if eq .Computed.http_router_final "gin" }}
	"github.com/gin-gonic/gin"
{{- end }}
	"time"
)

const healthCheckTimeout = 5 * time.Second

type HealthCheck func(context.Context) error

type Health struct {
	checks []HealthCheck
}

func NewHealth(checks ...HealthCheck) *Health {
	return &Health{checks: checks}
}

func (*Health) Name() string {
	return "/healthz"
}

func (*Health) Public() bool { return true }

{{ if eq .Computed.http_router_final "gin" -}}
func (h *Health) HTTP(r *gin.RouterGroup) {
	r.GET("", gin.WrapH(h.handler()))
}

func (h *Health) handler() http.Handler {
{{ else -}}
func (h *Health) HTTP() http.Handler {
{{ end -}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteError(w, r, http.StatusMethodNotAllowed, apperror.MethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		healthy := true
		for _, check := range h.checks {
			if err := check(ctx); err != nil {
				slog.WarnContext(ctx, "health check failed: "+err.Error())
				healthy = false
			}
		}
		if !healthy {
			WriteError(w, r, http.StatusServiceUnavailable, apperror.ServiceUnavailable)
			return
		}
		WriteOK(w)
	})
}
