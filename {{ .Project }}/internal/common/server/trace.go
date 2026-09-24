{{ if eq .Computed.http_router_final "gin" -}}
package server

import (
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/redact"
	"{{ .Computed.module_name_final }}/internal/common/tracing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	routeAttributeKey         = "http.route"
	routeParamAttributePrefix = "http.route.param."
)

func TraceID(serviceName string, policy redact.Policy) gin.HandlerFunc {
	propagator := otel.GetTextMapPropagator()
	if len(propagator.Fields()) == 0 {
		return func(c *gin.Context) { c.Next() }
	}
	serviceName = strings.TrimSpace(serviceName)
	tracer := otel.Tracer(serviceName + "/http")

	return func(c *gin.Context) {
		r := c.Request
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		if !oteltrace.SpanContextFromContext(ctx).IsValid() {
			if spanContext, ok := tracing.SpanContextFromTraceIDHeader(r.Header); ok {
				ctx = oteltrace.ContextWithRemoteSpanContext(ctx, spanContext)
			}
		}
		ctx, span := tracer.Start(ctx, r.Method, oteltrace.WithSpanKind(oteltrace.SpanKindServer))
		defer span.End()

		traceID := tracing.TraceIDFromContext(ctx)
		if traceID == "" {
			var ok bool
			ctx, traceID, ok = tracing.ContextWithNewTraceID(ctx)
			if !ok {
				traceID = ""
			}
		}
		if traceID != "" {
			c.Writer.Header().Set(tracing.HeaderTraceID, traceID)
		}
		c.Request = r.WithContext(ctx)
		c.Next()

		route := c.FullPath()
		if route == "" {
			return
		}
		span.SetName(r.Method + " " + route)
		attrs := make([]attribute.KeyValue, 1, 1+len(c.Params))
		attrs[0] = attribute.String(routeAttributeKey, route)
		for _, param := range c.Params {
			if param.Key != "" {
				attrs = append(attrs, attribute.String(routeParamAttributePrefix+param.Key, policy.Value(param.Key, param.Value)))
			}
		}
		span.SetAttributes(attrs...)
	}
}
{{ else -}}
package server

import (
	"net/http"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/redact"
	"{{ .Computed.module_name_final }}/internal/common/tracing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	routeAttributeKey         = "http.route"
	routeParamAttributePrefix = "http.route.param."
)

func TraceID(serviceName string, policy redact.Policy) func(http.Handler) http.Handler {
	propagator := otel.GetTextMapPropagator()
	if len(propagator.Fields()) == 0 {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	serviceName = strings.TrimSpace(serviceName)
	tracer := otel.Tracer(serviceName + "/http")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			if !oteltrace.SpanContextFromContext(ctx).IsValid() {
				if spanContext, ok := tracing.SpanContextFromTraceIDHeader(r.Header); ok {
					ctx = oteltrace.ContextWithRemoteSpanContext(ctx, spanContext)
				}
			}
			ctx, span := tracer.Start(ctx, r.Method, oteltrace.WithSpanKind(oteltrace.SpanKindServer))
			defer span.End()

			traceID := tracing.TraceIDFromContext(ctx)
			if traceID == "" {
				var ok bool
				ctx, traceID, ok = tracing.ContextWithNewTraceID(ctx)
				if !ok {
					traceID = ""
				}
			}
			if traceID != "" {
				w.Header().Set(tracing.HeaderTraceID, traceID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))

			routeContext := chi.RouteContext(ctx)
			route := routeContext.RoutePattern()
			if route == "" {
				return
			}
			span.SetName(r.Method + " " + route)
			attrs := make([]attribute.KeyValue, 1, 1+len(routeContext.URLParams.Keys))
			attrs[0] = attribute.String(routeAttributeKey, route)
			for index, name := range routeContext.URLParams.Keys {
				if name != "" && index < len(routeContext.URLParams.Values) {
					value := policy.Value(name, routeContext.URLParams.Values[index])
					attrs = append(attrs, attribute.String(routeParamAttributePrefix+name, value))
				}
			}
			span.SetAttributes(attrs...)
		})
	}
}
{{ end -}}
