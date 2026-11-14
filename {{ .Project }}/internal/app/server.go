package app

import (
{{- if and .Computed.enable_health_check_final .Computed.enable_redis_final }}
	"context"
{{- end }}
	"fmt"
	"net/http"

	"{{ .Computed.module_name_final }}/internal/common/config"
{{- if .Computed.enable_grpc_final }}
	"{{ .Computed.module_name_final }}/internal/common/rpc"
{{- end }}
	"{{ .Computed.module_name_final }}/internal/common/server"
	"{{ .Computed.module_name_final }}/internal/docs"
	"{{ .Computed.module_name_final }}/internal/modules"
)

func (a *Application) NewRouter(cfg *config.Config) (http.Handler, error) {
	mounted {{ if .Computed.enable_grpc_final }}, _ {{ end }} := a.generatedModules()
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

func (a *Application) configureTransports(cfg *config.Config, httpModules []modules.HTTPModule {{ if .Computed.enable_grpc_final }}, grpcModules []modules.GRPCModule {{ end }}) error {

{{- if .Computed.enable_grpc_final }}
	// Start HTTP for HTTP modules or an empty scaffold; helpers do not enable it.
	if len(httpModules) > 0 || len(grpcModules) == 0 {

{{- end }}
		handler, err := a.newRouter(cfg, httpModules)
		if err != nil {
			return fmt.Errorf("initialize http router: %w", err)
		}
		a.server = &http.Server{Addr: cfg.HTTP.Addr, Handler: handler, ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout, IdleTimeout: cfg.HTTP.IdleTimeout}

{{- if .Computed.enable_grpc_final }}
	}
	if len(grpcModules) > 0 {
		var err error
		a.grpcServer, err = rpc.NewServer(cfg, grpcModules...)
		if err != nil {
			return fmt.Errorf("initialize grpc server: %w", err)
		}
		a.grpcAddr = cfg.GRPC.Addr
	}
	a.shutdownTimeout = cfg.GRPC.ShutdownTimeout

{{- end }}
	if cfg.HTTP.Profiler.Enabled {
		a.profilerServer = &http.Server{
			Addr:              cfg.HTTP.Profiler.Addr,
			Handler:           server.NewProfilerHandler(),
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			IdleTimeout:       cfg.HTTP.IdleTimeout,
		}
	}

	return nil
}
