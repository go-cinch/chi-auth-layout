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

func outgoingTrace(ctx context.Context) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	for key, value := range carrier {
		md.Set(key, value)
	}
	if id := tracing.TraceIDFromContext(ctx); id != "" {
		md.Set(strings.ToLower(tracing.HeaderTraceID), id)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func startClientTrace(ctx context.Context, name, method string) (context.Context, func(error)) {
	ctx, span := otel.Tracer(name+"/grpc").Start(ctx, strings.TrimPrefix(method, "/"), oteltrace.WithSpanKind(oteltrace.SpanKindClient))
	span.SetAttributes(attribute.String("rpc.system", "grpc"))
	return outgoingTrace(ctx), func(err error) {
		span.SetAttributes(attribute.Int("rpc.grpc.status_code", int(status.Code(err))))
		if err != nil {
			span.SetStatus(otelcodes.Error, status.Code(err).String())
		}
		span.End()
	}

}

// Propagate the caller's trace and deadline without limiting stream duration.
func clientStreamTrace() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, conn *grpc.ClientConn, method string, next grpc.Streamer, options ...grpc.CallOption) (grpc.ClientStream, error) {
		return next(outgoingTrace(ctx), desc, conn, method, options...)
	}
}
