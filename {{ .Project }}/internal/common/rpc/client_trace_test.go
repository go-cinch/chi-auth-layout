package rpc

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
}
