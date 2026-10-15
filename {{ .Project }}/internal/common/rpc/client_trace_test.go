package rpc

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"{{ .Computed.module_name_final }}/internal/common/logging"
)

func TestClientTracePropagation(t *testing.T) {
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
	parent, parentSpan := provider.Tracer("test").Start(t.Context(), "parent")
	defer parentSpan.End()
	original := metadata.Pairs("authorization", "explicit-token")
	parent = metadata.NewOutgoingContext(parent, original)
	ctx, end := startClientTrace(parent, "remote", "/remote.v1.UserService/GetUser")
	md, _ := metadata.FromOutgoingContext(ctx)
	if len(md.Get("traceparent")) != 1 || md.Get("authorization")[0] != "explicit-token" || len(original.Get("traceparent")) != 0 {
		t.Fatalf("metadata: %v", md)
	}
	extracted := otel.GetTextMapPropagator().Extract(context.Background(), propagation.MapCarrier{"traceparent": md.Get("traceparent")[0]})
	if oteltrace.SpanContextFromContext(extracted).TraceID() != parentSpan.SpanContext().TraceID() {
		t.Fatal("trace chain broken")
	}
	end(status.Error(codes.NotFound, "missing"))
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].SpanKind() != oteltrace.SpanKindClient || spans[0].Parent().SpanID() != parentSpan.SpanContext().SpanID() {
		t.Fatalf("spans: %v", spans)
	}
	_, err := clientStreamTrace()(parent, nil, nil, "/remote/Watch", func(ctx context.Context, desc *grpc.StreamDesc, conn *grpc.ClientConn, method string, options ...grpc.CallOption) (grpc.ClientStream, error) {
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("stream gained a unary deadline")
		}
		md, _ := metadata.FromOutgoingContext(ctx)
		if len(md.Get("traceparent")) != 1 {
			t.Fatal("stream trace missing")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	previousLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	if err := logging.Init(&output, "info"); err != nil {
		t.Fatal(err)
	}
	err = clientUnary("remote", time.Second)(parent, "/remote.v1.UserService/GetUser", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return status.Error(codes.Unavailable, "offline")
		})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("client error: %v", err)
	}
	entries := readRPCLogs(t, &output)
	if len(entries) != 1 || entries[0]["trace_id"] != parentSpan.SpanContext().TraceID().String() {
		t.Fatalf("client log trace: %#v", entries)
	}
	assertRPCLogFields(t, entries[0], "/remote.v1.UserService/GetUser", codes.Unavailable)
	if spanID, ok := entries[0]["span_id"].(string); !ok || spanID == "" || spanID == parentSpan.SpanContext().SpanID().String() {
		t.Fatalf("client log span: %#v", entries[0])
	}
}
