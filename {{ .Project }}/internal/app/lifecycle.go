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
	if a.msgModule != nil {
		workerCtx, stopWorker := context.WithCancel(ctx)
		workerDone := make(chan struct{})
		go func() {
			defer close(workerDone)
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				sweepCtx, cancel := context.WithTimeout(workerCtx, 4*time.Second)
				err := a.msgModule.ExpireStatus(sweepCtx)
				cancel()
				if err != nil && workerCtx.Err() == nil {
					slog.ErrorContext(workerCtx, "refresh expired message status: "+err.Error(), slog.String("operation", "message_expire_status"), slog.String("outcome", "failure"), slog.Any("error", err))
				}
				select {
				case <-workerCtx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
		defer func() { stopWorker(); <-workerDone }()
	}
 if a.dictionaryModule != nil {
  workerCtx, stopWorker := context.WithCancel(ctx)
  workerDone := make(chan struct{})
  go func() {
   defer close(workerDone)
   ticker := time.NewTicker(5 * time.Second)
   defer ticker.Stop()
   for {
    select {
    case <-workerCtx.Done(): return
    case <-ticker.C:
     refreshCtx, cancel := context.WithTimeout(workerCtx, 4*time.Second)
     err := a.dictionaryModule.RefreshLocalization(refreshCtx)
     cancel()
     if err != nil && workerCtx.Err() == nil { slog.ErrorContext(workerCtx, "refresh localization failed: "+err.Error(), "operation", "refresh_localization", "outcome", "failure", "error", err) }
    }
   }
  }()
  defer func() { stopWorker(); <-workerDone }()
 }
	results := make(chan error, len(endpoints))
	for index, item := range endpoints {
		listener := listeners[index]
		address := listener.Addr().String()
		slog.InfoContext(ctx, item.name+" server running at "+address, slog.String("operation", "server_start"), slog.String("outcome", "success"), slog.String("server", item.name), slog.String("address", address))
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
	if serveErr != nil {
		slog.ErrorContext(ctx, "servers stopped with errors: "+serveErr.Error(), slog.String("operation", "server_stop"), slog.String("outcome", "failure"), slog.Any("error", serveErr))
	} else {
		slog.InfoContext(ctx, "servers stopped", slog.String("operation", "server_stop"), slog.String("outcome", "success"))
	}
	return serveErr
}
