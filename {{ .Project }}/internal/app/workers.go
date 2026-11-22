package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// startWorkers starts background tasks after all listeners have been bound.
// The returned function stops and joins them in reverse startup order.
func (a *Application) startWorkers(ctx context.Context) (func(), error) {
	var stops []func()
	if a.emailCodes != nil {
		stop, err := a.emailCodes.StartWorkers(ctx)
		if err != nil {
			return nil, fmt.Errorf("start River email workers: %w", err)
		}
		stops = append(stops, stop)
	}
{{- if .Computed.enable_redis_final }}
	if a.auditModule != nil && a.rds != nil {
		workerCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			a.auditModule.ConsumeStream(workerCtx)
		}()
		stops = append(stops, func() { cancel(); <-done })
	}
{{- end }}
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
		stops = append(stops, func() { stopWorker(); <-workerDone })
	}
	if a.dictionaryModule != nil && a.localizationRefreshInterval > 0 {
		workerCtx, stopWorker := context.WithCancel(ctx)
		workerDone := make(chan struct{})
		go func() {
			defer close(workerDone)
			ticker := time.NewTicker(a.localizationRefreshInterval)
			defer ticker.Stop()
			for {
				select {
				case <-workerCtx.Done():
					return
				case <-ticker.C:
					refreshCtx, cancel := context.WithTimeout(workerCtx, 4*time.Second)
					err := a.dictionaryModule.RefreshLocalization(refreshCtx)
					cancel()
					if err != nil && workerCtx.Err() == nil {
						slog.ErrorContext(workerCtx, "refresh localization failed: "+err.Error(), "operation", "refresh_localization", "outcome", "failure", "error", err)
					}
				}
			}
		}()
		stops = append(stops, func() { stopWorker(); <-workerDone })
	}
	return func() {
		for index := len(stops) - 1; index >= 0; index-- {
			stops[index]()
		}
	}, nil
}
