package rpc

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/modules"
)

func NewServer(cfg *config.Config, mounted ...modules.GRPCModule) (*grpc.Server, error) {
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(Unary(cfg)), grpc.ChainStreamInterceptor(Stream(cfg)))
	for _, module := range mounted {
		if module == nil {
			return nil, errors.New("grpc module is required")
		}
		module.GRPC(server)
	}
	checks := health.NewServer()
	checks.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	for name := range server.GetServiceInfo() {
		checks.SetServingStatus(name, healthpb.HealthCheckResponse_SERVING)
	}
	healthpb.RegisterHealthServer(server, checks)
	if cfg.GRPC.Reflection {
		reflection.Register(server)
	}
	return server, nil
}

func Shutdown(ctx context.Context, server *grpc.Server) error {
	done := make(chan struct{})
	go func() { server.GracefulStop(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		server.Stop()
		return ctx.Err()
	}
}
