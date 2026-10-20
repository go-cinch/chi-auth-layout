package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/pagination"
	"{{ .Computed.module_name_final }}/internal/infra/db"
	"{{ .Computed.module_name_final }}/internal/modules/msg"
{{- if .Computed.enable_grpc_final }}
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/rpc"
{{- end }}
)

func TestRunAndClose(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
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
	decoder := json.NewDecoder(&output)
	for _, name := range []string{"http", "profiler"} {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		address, ok := record["address"].(string)
		if !ok || !strings.HasPrefix(address, "127.0.0.1:") || strings.HasSuffix(address, ":0") {
			t.Fatalf("listening address = %#v", record["address"])
		}
		if record["operation"] != "server_start" || record["outcome"] != "success" || record["server"] != name || record["msg"] != name+" server running at "+address {
			t.Fatalf("startup log = %#v", record)
		}
	}
	var stopped map[string]any
	if err := decoder.Decode(&stopped); err != nil {
		t.Fatal(err)
	}
	if stopped["operation"] != "server_stop" || stopped["outcome"] != "success" || stopped["msg"] != "servers stopped" {
		t.Fatalf("shutdown log = %#v", stopped)
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

func TestRunLogsShutdownFailure(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	addresses := make(chan string, 1)
	application := &Application{
		shutdownTimeout: time.Millisecond,
		server: &http.Server{
			Addr: "127.0.0.1:0",
			BaseContext: func(listener net.Listener) context.Context {
				addresses <- listener.Addr().String()
				return t.Context()
			},
			Handler: http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				cancel()
				<-request.Context().Done()
			}),
		},
	}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		select {
		case address := <-addresses:
			client := &http.Client{Timeout: time.Second}
			response, err := client.Get("http://" + address)
			if err == nil {
				_ = response.Body.Close()
			}
		case <-ctx.Done():
		}
	}()
	if err := application.Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v", err)
	}
	<-requestDone
	decoder := json.NewDecoder(&output)
	var record map[string]any
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record["operation"] != "server_stop" || record["outcome"] != "failure" || record["level"] != "ERROR" || record["error"] != context.DeadlineExceeded.Error() || record["msg"] != "servers stopped with errors: "+context.DeadlineExceeded.Error() {
		t.Fatalf("shutdown failure log = %#v", record)
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
func TestRunStopsMessageExpiryWorker(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mock.ExpectQuery("SELECT user_id FROM t_user_msg_status.*LIMIT 100").WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	limits, _ := pagination.New(100, 100)
	application := &Application{server: &http.Server{Addr: "127.0.0.1:0", Handler: http.NotFoundHandler()}, msgModule: msg.New(&db.Store{DB: database}, limits)}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- application.Run(ctx) }()
	deadline := time.Now().Add(time.Second)
	for mock.ExpectationsWereMet() != nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("expiry worker did not stop")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
