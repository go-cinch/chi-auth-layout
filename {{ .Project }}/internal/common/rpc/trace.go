package rpc

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"{{ .Computed.module_name_final }}/internal/common/tracing"
)

func startTrace(ctx context.Context, service, method string) (context.Context, func(error)) {
	propagator := otel.GetTextMapPropagator()
	if len(propagator.Fields()) == 0 {
		return ctx, func(error) {}
	}
	incoming, _ := metadata.FromIncomingContext(ctx)
	carrier := propagation.MapCarrier{}
	for key, values := range incoming {
		if len(values) > 0 {
			carrier[key] = values[0]
		}
	}
	ctx = propagator.Extract(ctx, carrier)
	if !oteltrace.SpanContextFromContext(ctx).IsValid() {
		if remote, ok := tracing.SpanContextFromTraceID(carrier.Get(strings.ToLower(tracing.HeaderTraceID))); ok {
			ctx = oteltrace.ContextWithRemoteSpanContext(ctx, remote)
		}
	}
	ctx, span := otel.Tracer(strings.TrimSpace(service)+"/grpc").Start(ctx, strings.TrimPrefix(method, "/"), oteltrace.WithSpanKind(oteltrace.SpanKindServer))
	if tracing.TraceIDFromContext(ctx) == "" {
		ctx, _, _ = tracing.ContextWithNewTraceID(ctx)
	}
	if id := tracing.TraceIDFromContext(ctx); id != "" {
		_ = grpc.SetHeader(ctx, metadata.Pairs(strings.ToLower(tracing.HeaderTraceID), id))
	}
	span.SetAttributes(attribute.String("rpc.system", "grpc"))
	return ctx, func(err error) {
		span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(status.Code(err))))
		if err != nil {
			span.SetStatus(otelcodes.Error, status.Code(err).String())
		}
		span.End()
	}
}
