package otel

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/srapi/srapi/apps/api/internal/config"
)

// ShutdownFunc flushes and releases tracing resources.
type ShutdownFunc func(context.Context) error

// SetupTracerProvider configures the process-wide OpenTelemetry tracer provider.
func SetupTracerProvider(ctx context.Context, cfg config.ObservabilityConfig) (ShutdownFunc, error) {
	tp, shutdown, err := NewTracerProvider(ctx, cfg)
	if err != nil {
		return nil, err
	}
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return shutdown, nil
}

// NewTracerProvider builds a tracer provider without installing it globally.
func NewTracerProvider(ctx context.Context, cfg config.ObservabilityConfig) (*sdktrace.TracerProvider, ShutdownFunc, error) {
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceSampleRatio))
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(resourceForConfig(cfg)),
	}
	if cfg.TracesEnabled {
		exporter, err := otlpTraceExporter(ctx, cfg)
		if err != nil {
			return nil, nil, err
		}
		opts = append(opts, sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(cfg.BatchTimeout)))
	}
	tp := sdktrace.NewTracerProvider(opts...)
	return tp, tp.Shutdown, nil
}

func resourceForConfig(cfg config.ObservabilityConfig) *resource.Resource {
	return resource.NewSchemaless(
		attribute.String("service.name", strings.TrimSpace(cfg.ServiceName)),
		attribute.String("service.version", strings.TrimSpace(cfg.ServiceVersion)),
		attribute.String("deployment.environment.name", strings.TrimSpace(cfg.Environment)),
	)
}

func otlpTraceExporter(ctx context.Context, cfg config.ObservabilityConfig) (sdktrace.SpanExporter, error) {
	endpoint := strings.TrimSpace(cfg.OTLPEndpoint)
	if endpoint == "" {
		return nil, errors.New("OTLP endpoint is required when traces are enabled")
	}
	options := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	if cfg.OTLPInsecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	return otlptracegrpc.New(ctx, options...)
}
