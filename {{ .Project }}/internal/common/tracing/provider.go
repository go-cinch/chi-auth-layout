package tracing

import (
	"context"
	"fmt"
	"strings"

	"{{ .Computed.module_name_final }}/internal/common/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
)

func NewProvider(ctx context.Context, cfg *config.Config) (*sdktrace.TracerProvider, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	serviceName := strings.TrimSpace(cfg.Server.Name)
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(config.Version),
		)),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(sampleRatio(cfg.Tracer.Ratio)),
		)),
	}

	endpoint := strings.TrimSpace(cfg.Tracer.OTLP.Endpoint)
	if endpoint != "" {
		exporterOptions := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
		if cfg.Tracer.OTLP.Insecure {
			exporterOptions = append(exporterOptions, otlptracegrpc.WithInsecure())
		}
		exporter, err := otlptracegrpc.New(ctx, exporterOptions...)
		if err != nil {
			return nil, fmt.Errorf("initialize otlp trace exporter: %w", err)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	if cfg.Tracer.Stdout || endpoint == "" {
		exporter, err := stdouttrace.New()
		if err != nil {
			return nil, fmt.Errorf("initialize stdout trace exporter: %w", err)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}

	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	return provider, nil
}

func sampleRatio(ratio float64) float64 {
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}
