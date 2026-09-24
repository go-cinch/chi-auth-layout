package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

{{- if .Computed.enable_grpc_final }}
	"google.golang.org/grpc"
	"{{ .Computed.module_name_final }}/internal/common/rpc"
{{- end }}
)

type endpoint struct {
	name, addr string
	serve      func(net.Listener) error
	shutdown   func(context.Context) error
}

func (a *Application) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return nil
	}
	var endpoints []endpoint
	for _, item := range []struct {
		name   string
		server *http.Server
	}{
		{"http", a.server},
		{"profiler", a.profilerServer},
	} {
		if item.server == nil {
			continue
		}
		srv := item.server
		address := srv.Addr
		if address == "" {
			address = ":http"
		}
		endpoints = append(endpoints, endpoint{name: item.name, addr: address, serve: srv.Serve, shutdown: func(ctx context.Context) error {
			if err := srv.Shutdown(ctx); err != nil {
				return errors.Join(err, srv.Close())
			}
			return nil
		}})
	}
{{- if .Computed.enable_grpc_final }}
	if a.grpcServer != nil {
		address := a.grpcAddr
		if address == "" {
			address = ":9090"
		}
		endpoints = append(endpoints, endpoint{name: "grpc", addr: address, serve: a.grpcServer.Serve, shutdown: func(ctx context.Context) error { return rpc.Shutdown(ctx, a.grpcServer) }})
	}
{{- end }}
	if len(endpoints) == 0 {
		return errors.New("no servers configured")
	}
	listeners := make([]net.Listener, 0, len(endpoints))
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	for _, item := range endpoints {
		listener, err := net.Listen("tcp", item.addr)
		if err != nil {
			return fmt.Errorf("listen %s server: %w", item.name, err)
		}
		listeners = append(listeners, listener)
	}
	results := make(chan error, len(endpoints))
	for index, item := range endpoints {
		listener := listeners[index]
		slog.InfoContext(ctx, item.name+" server running at "+listener.Addr().String())
		go func() {
			err := item.serve(listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed){{ if .Computed.enable_grpc_final }} && !errors.Is(err, grpc.ErrServerStopped){{ end }} {
				err = fmt.Errorf("%s server: %w", item.name, err)
			} else {
				err = nil
			}
			results <- err
		}()
	}
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-results:
	}
	timeout := a.shutdownTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stopped := make(chan error, len(endpoints))
	for _, item := range endpoints {
		go func() { stopped <- item.shutdown(stopCtx) }()
	}
	for range endpoints {
		serveErr = errors.Join(serveErr, <-stopped)
	}
	slog.InfoContext(ctx, "servers stopped")
	return serveErr
}
