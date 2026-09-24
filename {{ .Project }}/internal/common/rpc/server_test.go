package rpc

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
	"{{ .Computed.module_name_final }}/internal/common/config"
)

type testModule struct{}

func (testModule) GRPC(r grpc.ServiceRegistrar) {
	r.RegisterService(&grpc.ServiceDesc{ServiceName: "test.Service", HandlerType: (*interface{})(nil)}, struct{}{})
}

func TestNewServerAndShutdown(t *testing.T) {
	cfg := &config.Config{}
	cfg.GRPC.Reflection = true
	server, err := NewServer(cfg, testModule{})
	if err != nil {
		t.Fatal(err)
	}
	services := server.GetServiceInfo()
	for _, name := range []string{"test.Service", "grpc.health.v1.Health", "grpc.reflection.v1.ServerReflection"} {
		if _, ok := services[name]; !ok {
			t.Fatalf("missing service %s", name)
		}
	}
	if err := Shutdown(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	if _, err := NewServer(cfg, nil); err == nil {
		t.Fatal("nil module accepted")
	}
}

func TestShutdownDeadline(t *testing.T) {
	server, err := NewServer(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///health", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	watch, err := healthpb.NewHealthClient(conn).Watch(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := watch.Recv(); err != nil {
		t.Fatal(err)
	}
	stopping, stop := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer stop()
	if err := Shutdown(stopping, server); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown: %v", err)
	}
	if _, err := watch.Recv(); err == nil {
		t.Fatal("stream remained open after forced shutdown")
	}
}
