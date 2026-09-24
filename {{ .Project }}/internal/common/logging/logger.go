package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"{{ .Computed.module_name_final }}/internal/common/config"
{{- if .Computed.enable_trace_final }}
	"{{ .Computed.module_name_final }}/internal/common/tracing"
{{- end }}
)

const (
	versionKey = "v"
	traceIDKey = "trace_id"
	spanIDKey  = "span_id"
)

func Init(out io.Writer, level string) error {
	var parsed slog.Level
	if err := parsed.UnmarshalText([]byte(strings.TrimSpace(level))); err != nil {
		return fmt.Errorf("parse log level: %w", err)
	}
	var handler slog.Handler = slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: parsed, AddSource: true, ReplaceAttr: callerAttribute(),
	})
	handler = handler.WithAttrs([]slog.Attr{slog.String(versionKey, config.Version)})
	slog.SetDefault(slog.New(contextHandler{next: handler}))
{{- if eq .Computed.http_router_final "gin" }}
	initGinLogging()
{{- end }}
	return nil
}

type contextHandler struct {
	next slog.Handler
}

func (h contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h contextHandler) Handle(ctx context.Context, record slog.Record) error {
{{- if .Computed.enable_trace_final }}
	traceID, spanID := tracing.IDsFromContext(ctx)
	var attrs [2]slog.Attr
	count := 0
	if traceID != "" {
		attrs[count] = slog.String(traceIDKey, traceID)
		count++
	}
	if spanID != "" {
		attrs[count] = slog.String(spanIDKey, spanID)
		count++
	}
	if count > 0 {
		record.AddAttrs(attrs[:count]...)
	}
{{- end }}
	return h.next.Handle(ctx, record)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{next: h.next.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{next: h.next.WithGroup(name)}
}
