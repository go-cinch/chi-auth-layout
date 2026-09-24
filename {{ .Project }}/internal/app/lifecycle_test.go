package app

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
{{- if .Computed.enable_grpc_final }}
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/rpc"
{{- end }}
)

func TestRunAndClose(t *testing.T) {
	application := &Application{
		server:         &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second},
		profilerServer: &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second},
	}
	closed := false
	application.cleanups = []func(){func() { closed = true }}
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(20*time.Millisecond, cancel)
	if err := application.Run(ctx); err != nil {
		t.Fatal(err)
	}
	application.Close()
	if !closed {
		t.Fatal("cleanup was not called")
	}
	application = &Application{server: &http.Server{Addr: "127.0.0.1:-1", Handler: http.NotFoundHandler()}}
	if err := application.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "http server") {
		t.Fatalf("listener error = %v", err)
	}
}

func TestRunDoesNotLogBeforeListenSucceeds(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	application := &Application{server: &http.Server{Addr: listener.Addr().String(), Handler: http.NotFoundHandler()}}
	if err := application.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.Contains(output.String(), "http server running") {
		t.Fatalf("startup log emitted before listen succeeded: %q", output.String())
	}

	application = &Application{
		server:         &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()},
		profilerServer: &http.Server{Addr: listener.Addr().String(), Handler: http.NotFoundHandler()},
	}
	if err := application.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "listen profiler server") {
		t.Fatalf("profiler Run() error = %v", err)
	}
	if strings.Contains(output.String(), "server running") {
		t.Fatalf("startup log emitted before all listeners succeeded: %q", output.String())
	}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (&Application{}).Run(canceled); err != nil {
		t.Fatalf("canceled Run() error = %v", err)
	}
}

{{ if .Computed.enable_grpc_final -}}
func TestGRPCLifecycle(t *testing.T) {
	server, err := rpc.NewServer(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{grpcServer: server, grpcAddr: "127.0.0.1:0"}
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(20*time.Millisecond, cancel)
	if err := application.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err := (&Application{}).Run(t.Context()); err == nil {
		t.Fatal("empty application accepted")
	}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	httpProbe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpAddr := httpProbe.Addr().String()
	httpProbe.Close()
	server, err = rpc.NewServer(&config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	application = &Application{server: &http.Server{Addr: httpAddr}, grpcServer: server, grpcAddr: occupied.Addr().String()}
	if err := application.Run(t.Context()); err == nil || !strings.Contains(err.Error(), "listen grpc server") {
		t.Fatalf("grpc listen: %v", err)
	}
	probe, err := net.Listen("tcp", httpAddr)
	if err != nil {
		t.Fatalf("http listener leaked: %v", err)
	}
	probe.Close()
}
{{ end -}}
