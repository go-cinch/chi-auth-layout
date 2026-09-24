package tracing

import (
	"context"
	"net/http"
	"testing"
	"time"

	"{{ .Computed.module_name_final }}/internal/common/config"

	"go.opentelemetry.io/otel"
)

func TestTraceIDs(t *testing.T) {
	if traceID, spanID := IDsFromContext(context.Background()); traceID != "" || spanID != "" {
		t.Fatalf("empty IDs = %q, %q", traceID, spanID)
	}
	ctx, traceID, ok := ContextWithNewTraceID(context.Background())
	if !ok || traceID == "" || TraceIDFromContext(ctx) != traceID || SpanIDFromContext(ctx) == "" {
		t.Fatalf("generated trace context = %q, %v", traceID, ok)
	}
	if _, ok := SpanContextFromTraceID("invalid"); ok {
		t.Fatal("invalid trace ID was accepted")
	}
	spanContext, ok := SpanContextFromTraceID("00112233445566778899AABBCCDDEEFF")
	if !ok || !spanContext.IsRemote() || !spanContext.IsSampled() {
		t.Fatalf("span context = %#v, %v", spanContext, ok)
	}
	header := make(http.Header)
	header.Set(HeaderTraceID, "00112233445566778899aabbccddeeff")
	if _, ok := SpanContextFromTraceIDHeader(header); !ok {
		t.Fatal("trace ID header was rejected")
	}
}

func TestSampleRatio(t *testing.T) {
	for input, expected := range map[float64]float64{-1: 0, 0.25: 0.25, 2: 1} {
		if actual := sampleRatio(input); actual != expected {
			t.Fatalf("sampleRatio(%v) = %v", input, actual)
		}
	}
}

func TestNewProvider(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})
	var cfg config.Config
	cfg.Server.Name = "test"
	cfg.Tracer.Ratio = 0.5
	provider, err := NewProvider(t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	cfg.Tracer.OTLP.Endpoint = "127.0.0.1:4317"
	cfg.Tracer.OTLP.Insecure = true
	cfg.Tracer.Stdout = true
	provider, err = NewProvider(t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := provider.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
