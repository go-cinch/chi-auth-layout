package tracing

import (
	"context"
	"crypto/rand"
	"net/http"
	"strings"

	oteltrace "go.opentelemetry.io/otel/trace"
)

const HeaderTraceID = "X-Trace-Id"

func IDsFromContext(ctx context.Context) (traceID string, spanID string) {
	spanContext := oteltrace.SpanContextFromContext(ctx)
	if spanContext.HasTraceID() {
		traceID = spanContext.TraceID().String()
	}
	if spanContext.HasSpanID() {
		spanID = spanContext.SpanID().String()
	}
	return traceID, spanID
}

func TraceIDFromContext(ctx context.Context) string {
	traceID, _ := IDsFromContext(ctx)
	return traceID
}

func SpanIDFromContext(ctx context.Context) string {
	_, spanID := IDsFromContext(ctx)
	return spanID
}

func SpanContextFromTraceIDHeader(header http.Header) (oteltrace.SpanContext, bool) {
	return SpanContextFromTraceID(strings.TrimSpace(header.Get(HeaderTraceID)))
}

func SpanContextFromTraceID(value string) (oteltrace.SpanContext, bool) {
	traceID, err := oteltrace.TraceIDFromHex(strings.ToLower(strings.TrimSpace(value)))
	if err != nil || !traceID.IsValid() {
		return oteltrace.SpanContext{}, false
	}
	spanID, err := newSpanID()
	if err != nil {
		return oteltrace.SpanContext{}, false
	}
	return oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: oteltrace.FlagsSampled,
		Remote:     true,
	}), true
}

func ContextWithNewTraceID(ctx context.Context) (context.Context, string, bool) {
	traceID, err := newTraceID()
	if err != nil {
		return ctx, "", false
	}
	spanID, err := newSpanID()
	if err != nil {
		return ctx, "", false
	}
	spanContext := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})
	value := traceID.String()
	return oteltrace.ContextWithSpanContext(ctx, spanContext), value, true
}

func newTraceID() (oteltrace.TraceID, error) {
	for {
		var traceID oteltrace.TraceID
		if _, err := rand.Read(traceID[:]); err != nil {
			return oteltrace.TraceID{}, err
		}
		if traceID.IsValid() {
			return traceID, nil
		}
	}
}

func newSpanID() (oteltrace.SpanID, error) {
	for {
		var spanID oteltrace.SpanID
		if _, err := rand.Read(spanID[:]); err != nil {
			return oteltrace.SpanID{}, err
		}
		if spanID.IsValid() {
			return spanID, nil
		}
	}
}
