package app

import (
{{- if and .Computed.enable_health_check_final .Computed.enable_redis_final }}
	"context"
{{- end }}
	"net/http"

	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/docs"
	"{{ .Computed.module_name_final }}/internal/modules"
)

func (a *Application) NewRouter(cfg *config.Config) (http.Handler, error) {
	mounted{{ if .Computed.enable_grpc_final }}, _{{ end }} := a.generatedModules()
	return a.newRouter(cfg, mounted)
}

func (a *Application) newRouter(cfg *config.Config, mounted []modules.HTTPModule) (http.Handler, error) {
{{- if .Computed.enable_health_check_final }}
	healthChecks := make([]server.HealthCheck, 0, 2)

	healthChecks = append(healthChecks, a.db.DB.PingContext)

{{- if .Computed.enable_redis_final }}
	healthChecks = append(healthChecks, func(ctx context.Context) error {
		return a.rds.Ping(ctx).Err()
	})
{{- end }}
{{- end }}
{{- if .Computed.enable_health_check_final }}
	mounted = append([]modules.Module{server.NewHealth(healthChecks...)}, mounted...)
{{- end }}
	if cfg.HTTP.Docs.Enabled {
		documentation, err := docs.New(cfg.HTTP.Docs.Servers, a.pagination)
		if err != nil {
			return nil, err
		}
		mounted = append(mounted, documentation)
	}
	return server.NewRouter(cfg, a.authenticator, a.idempotencyKeys, mounted...)
}
