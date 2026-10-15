package rpc

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"{{ .Computed.module_name_final }}/internal/common/config"
	"{{ .Computed.module_name_final }}/internal/common/logging"
	"{{ .Computed.module_name_final }}/internal/common/tracing"
)

type traceTransport struct{ header metadata.MD }

func (*traceTransport) Method() string { return "/test.Service/Call" }

func (s *traceTransport) SetHeader(md metadata.MD) error {
	s.header = metadata.Join(s.header, md)
	return nil
}

func (s *traceTransport) SendHeader(md metadata.MD) error { return s.SetHeader(md) }

func (*traceTransport) SetTrailer(metadata.MD) error { return nil }

func TestTrace(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	if err := logging.Init(&output, "info"); err != nil {
		t.Fatal(err)
	}
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	oldProvider, oldPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(oldProvider)
		otel.SetTextMapPropagator(oldPropagator)
		_ = provider.Shutdown(context.Background())
	})
	const id = "0123456789abcdef0123456789abcdef"
	for _, md := range []metadata.MD{
		metadata.Pairs("traceparent", "00-"+id+"-0123456789abcdef-01"),
		metadata.Pairs("x-trace-id", id),
	} {
		transport := &traceTransport{}
		incoming := grpc.NewContextWithServerTransportStream(metadata.NewIncomingContext(t.Context(), md), transport)
		_, err := Unary(&config.Config{})(incoming, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Call"}, func(ctx context.Context, _ any) (any, error) {
			if tracing.TraceIDFromContext(ctx) != id || transport.header.Get("x-trace-id")[0] != id {
				t.Fatalf("trace context or response header lost: %v", transport.header)
			}
			return nil, status.Error(codes.NotFound, "missing")
		})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("server error: %v", err)
		}
		entries := readRPCLogs(t, &output)
		if len(entries) != 1 || entries[0]["trace_id"] != id {
			t.Fatalf("server log trace: %#v", entries)
		}
		assertRPCLogFields(t, entries[0], "/test.Service/Call", codes.NotFound)
		if spanID, ok := entries[0]["span_id"].(string); !ok || spanID == "" {
			t.Fatalf("server log span: %#v", entries[0])
		}
	}
	if len(recorder.Ended()) != 2 || recorder.Ended()[0].Name() != "test.Service/Call" {
		t.Fatalf("spans: %#v", recorder.Ended())
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	original := t.Context()
	ctx, end := startTrace(original, "test", "/test.Service/Call")
	end(nil)
	if ctx != original {
		t.Fatal("disabled tracing replaced context")
	}
}
