package rpc

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func clientHealthServer(t *testing.T, interceptor grpc.UnaryServerInterceptor) (*health.Server, grpc.DialOption) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.UnaryInterceptor(interceptor))
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	return healthServer, grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() })
}

func TestClientHealthCallsAndCleanup(t *testing.T) {
	_, dialer := clientHealthServer(t, func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if md.Get("x-test")[0] != "preserved" {
			t.Error("outgoing metadata lost")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Error("missing deadline")
		}
		return next(ctx, req)
	})
	ctx := metadata.AppendToOutgoingContext(t.Context(), "x-test", "preserved")
	client, cleanup, err := NewClient(ctx, "remote", ClientConfig{Addr: "passthrough:///remote", Insecure: true, HealthCheck: true}, healthpb.NewHealthClient, dialer)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{}); status.Code(err) != codes.Canceled {
		t.Fatalf("closed connection: %v", err)
	}
}

func TestClientHealthRejectsNotServing(t *testing.T) {
	checks, dialer := clientHealthServer(t, nil)
	checks.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	client, cleanup, err := NewClient(t.Context(), "remote", ClientConfig{Addr: "passthrough:///remote", Insecure: true, HealthCheck: true}, healthpb.NewHealthClient, dialer)
	if status.Code(err) != codes.Unavailable || client != nil || cleanup != nil {
		t.Fatalf("unhealthy: %v %v", client, err)
	}
}

func TestClientDeadlinesAndCancellation(t *testing.T) {
	_, dialer := clientHealthServer(t, func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	})
	cfg := ClientConfig{Addr: "passthrough:///remote", Insecure: true, Timeout: 100 * time.Millisecond}
	client, cleanup, err := NewClient(t.Context(), "remote", cfg, healthpb.NewHealthClient, dialer)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := client.Check(t.Context(), &healthpb.HealthCheckRequest{}); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("default deadline: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{}); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("caller deadline: %v", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	cancel()
	if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{}); status.Code(err) != codes.Canceled {
		t.Fatalf("canceled: %v", err)
	}
	cfg.HealthCheck = true
	if _, cleanup, err := NewClient(t.Context(), "remote", cfg, healthpb.NewHealthClient, dialer); status.Code(err) != codes.DeadlineExceeded || cleanup != nil {
		t.Fatalf("health timeout: %v", err)
	}
}

func TestClientValidationAndLazyInitialization(t *testing.T) {
	for _, cfg := range []ClientConfig{
		{}, {Addr: "localhost:9090", Timeout: -time.Second},
	} {
		if _, _, err := NewClient(t.Context(), "remote", cfg, healthpb.NewHealthClient); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	cfg := ClientConfig{Addr: "localhost:1"}
	if _, _, err := NewClient[healthpb.HealthClient](t.Context(), "remote", cfg, nil); err == nil {
		t.Fatal("nil factory accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := NewClient(ctx, "remote", cfg, healthpb.NewHealthClient); err != context.Canceled {
		t.Fatalf("canceled initialization: %v", err)
	}
	client, cleanup, err := NewClient(t.Context(), "remote", cfg, healthpb.NewHealthClient)
	if err != nil || client == nil {
		t.Fatalf("lazy initialization: %v", err)
	}
	cleanup()
	if _, _, err := NewClient(t.Context(), "remote", cfg, healthpb.NewHealthClient, grpc.WithTransportCredentials(nil)); err == nil {
		t.Fatal("invalid credentials accepted")
	}
}
