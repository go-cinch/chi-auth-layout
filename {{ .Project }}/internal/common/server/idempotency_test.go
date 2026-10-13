package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/idempotency"
)

func TestIdempotency(t *testing.T) {
	store := idempotency.NewMemoryStore()
	handled := 0
	handler := Idempotency(store, time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handled++
		WriteOK(w)
	}))
	request := func(method, key string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/resource", nil)
		if key != "" {
			req.Header.Set(IdempotencyHeader, key)
		}
		handler.ServeHTTP(recorder, req)
		return recorder
	}

	if recorder := request(http.MethodPost, "request-1"); recorder.Code != http.StatusOK {
		t.Fatalf("first status = %d", recorder.Code)
	}
	if recorder := request(http.MethodPost, "request-1"); recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "already been used") {
		t.Fatalf("duplicate response = %d %s", recorder.Code, recorder.Body.String())
	}
	if recorder := request(http.MethodPost, "bad key"); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d", recorder.Code)
	}
	if recorder := request(http.MethodPost, ""); recorder.Code != http.StatusOK {
		t.Fatalf("missing key status = %d", recorder.Code)
	}
	if recorder := request(http.MethodGet, "request-1"); recorder.Code != http.StatusOK {
		t.Fatalf("get status = %d", recorder.Code)
	}
	if handled != 3 {
		t.Fatalf("handled requests = %d", handled)
	}
}

type failingIdempotencyStore struct{}

func (failingIdempotencyStore) Claim(context.Context, string, time.Duration) (bool, error) {
	return false, errors.New("unavailable")
}

func TestIdempotencyStoreFailures(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler was called")
	})
	for name, store := range map[string]idempotency.Store{
		"missing": nil,
		"failure": failingIdempotencyStore{},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/resource", nil)
			request.Header.Set(IdempotencyHeader, "request-1")
			Idempotency(store, time.Hour)(next).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d", recorder.Code)
			}
		})
	}
}

func TestValidIdempotencyKey(t *testing.T) {
	for value, want := range map[string]bool{
		"550e8400-e29b-41d4-a716-446655440000": true,
		"request_1.example:value":              true,
		"":                                     false,
		"contains whitespace":                  false,
		strings.Repeat("a", 129):               false,
	} {
		if got := validIdempotencyKey(value); got != want {
			t.Fatalf("validIdempotencyKey(%q) = %v, want %v", value, got, want)
		}
	}
}
