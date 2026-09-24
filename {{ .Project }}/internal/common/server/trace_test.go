{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/redact"
	"{{ .Computed.module_name_final }}/internal/common/tracing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTraceIDUsesRoutePattern(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})
	router := gin.New()
	router.Use(TraceID("test", redact.New()))
	router.GET("/user/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/user/42", nil))
	if len(recorder.Ended()) != 1 || recorder.Ended()[0].Name() != "GET /user/:id" {
		t.Fatalf("spans = %#v", recorder.Ended())
	}

	const traceID = "0123456789abcdef0123456789abcdef"
	router.GET("/reset/:token", func(c *gin.Context) {
		if tracing.TraceIDFromContext(c.Request.Context()) != traceID {
			t.Error("request trace context lost")
		}
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/reset/abcdef", nil)
	request.Header.Set("traceparent", "00-"+traceID+"-0123456789abcdef-01")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Header().Get(tracing.HeaderTraceID) != traceID {
		t.Fatalf("trace header: %v", response.Header())
	}
	spans := recorder.Ended()
	if len(spans) != 2 || spans[1].Name() != "GET /reset/:token" {
		t.Fatalf("spans: %#v", spans)
	}
	attrs := map[string]string{}
	for _, attr := range spans[1].Attributes() {
		attrs[string(attr.Key)] = attr.Value.AsString()
	}
	if attrs[routeAttributeKey] != "/reset/:token" || attrs[routeParamAttributePrefix+"token"] != "abc***" {
		t.Fatalf("attributes: %#v", attrs)
	}
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))
	if len(recorder.Ended()) != 3 {
		t.Fatal("unmatched request span missing")
	}
}

func TestTraceIDPassesThroughWithoutPropagator(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
	called := false
	handler := gin.New()
	handler.Use(TraceID("test", redact.New()))
	handler.GET("/", func(c *gin.Context) { called = true })
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatal("handler was not called")
	}
}
{{ else -}}
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"{{ .Computed.module_name_final }}/internal/common/redact"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTraceIDUsesRoutePattern(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})
	router := chi.NewRouter()
	router.Use(TraceID("test", redact.New()))
	router.Get("/user/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/user/42", nil))
	if len(recorder.Ended()) != 1 || recorder.Ended()[0].Name() != "GET /user/{id}" {
		t.Fatalf("spans = %#v", recorder.Ended())
	}
}

func TestTraceIDPassesThroughWithoutPropagator(t *testing.T) {
	previous := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	t.Cleanup(func() { otel.SetTextMapPropagator(previous) })
	called := false
	handler := TraceID("test", redact.New())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called { t.Fatal("handler was not called") }
}
{{ end -}}
