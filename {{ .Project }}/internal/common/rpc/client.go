package rpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// ClientConfig describes one downstream service. Timeout bounds unary calls and
// the optional startup health check; streams use the caller's context deadline.
type ClientConfig struct {
	Addr        string
	Timeout     time.Duration
	HealthCheck bool
	Insecure    bool
}

// NewClient creates a reusable typed client and its application-owned cleanup.
// Without HealthCheck, initialization is lazy and does not prove reachability.
func NewClient[T any](ctx context.Context, name string, cfg ClientConfig, factory func(grpc.ClientConnInterface) T, options ...grpc.DialOption) (T, func(), error) {
	var zero T
	if strings.TrimSpace(cfg.Addr) == "" || factory == nil || cfg.Timeout < 0 {
		return zero, nil, fmt.Errorf("initialize %s grpc client: address, constructor and non-negative timeout required", name)
	}
	if err := ctx.Err(); err != nil {
		return zero, nil, err
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	var transport credentials.TransportCredentials = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	if cfg.Insecure {
		transport = insecure.NewCredentials()
	}
	base := []grpc.DialOption{
		grpc.WithTransportCredentials(transport),
		grpc.WithChainUnaryInterceptor(clientUnary(name, cfg.Timeout)),
	}
{{- if .Computed.enable_trace_final }}
	base = append(base, grpc.WithChainStreamInterceptor(clientStreamTrace()))
{{- end }}
	conn, err := grpc.NewClient(cfg.Addr, append(base, options...)...)
	if err != nil {
		return zero, nil, fmt.Errorf("initialize %s grpc client: %w", name, err)
	}
	cleanup := func() { _ = conn.Close() }
	if cfg.HealthCheck {
		checkCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		response, err := healthpb.NewHealthClient(conn).Check(checkCtx, &healthpb.HealthCheckRequest{}, grpc.WaitForReady(true))
		if err == nil && response.GetStatus() != healthpb.HealthCheckResponse_SERVING {
			err = status.Error(codes.Unavailable, "service is not serving")
		}
		if err != nil {
			cleanup()
			return zero, nil, fmt.Errorf("check %s grpc health: %w", name, err)
		}
	}
	slog.InfoContext(ctx, "initialize "+name+" grpc client: "+cfg.Addr)
	return factory(conn), cleanup, nil
}

func clientUnary(name string, timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, request, response any, conn *grpc.ClientConn, next grpc.UnaryInvoker, options ...grpc.CallOption) (err error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
{{- if .Computed.enable_trace_final }}
		ctx, end := startClientTrace(ctx, name, method)
		defer func() { end(err) }()
{{- end }}
		started := time.Now()
		err = next(ctx, method, request, response, conn, options...)
		slog.InfoContext(ctx, fmt.Sprintf("call %s %s %s %dms", name, method, status.Code(err), time.Since(started).Milliseconds()))
		return err
	}
}
