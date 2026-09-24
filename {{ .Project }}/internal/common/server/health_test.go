package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/config"
)

func TestHealthz(t *testing.T) {
	checks := 0
	deadlineSeen := false
	health := NewHealth(
		func(ctx context.Context) error {
			checks++
			deadline, ok := ctx.Deadline()
			deadlineSeen = ok && time.Until(deadline) <= healthCheckTimeout && time.Until(deadline) > healthCheckTimeout-time.Second
			return errors.New("unavailable")
		},
		func(context.Context) error { checks++; return nil },
	)
	if health.Name() != "/healthz" {
		t.Fatalf("name = %q", health.Name())
	}
	handler, err := NewRouter(&config.Config{}, nil, nil, health)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusServiceUnavailable || checks != 2 || !deadlineSeen {
		t.Fatalf("response = %d, checks = %d", recorder.Code, checks)
	}
	healthyRouter, err := NewRouter(&config.Config{}, nil, nil, NewHealth())
	if err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	healthyRouter.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("healthy response = %d %q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	healthyRouter.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method response = %d", recorder.Code)
	}
}
