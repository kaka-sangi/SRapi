package otel

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/srapi/srapi/apps/api/internal/config"
)

func TestResourceForConfigUsesStableServiceAttributes(t *testing.T) {
	resource := resourceForConfig(config.ObservabilityConfig{
		ServiceName:    "srapi-api",
		ServiceVersion: "1.2.3",
		Environment:    "test",
	})
	attrs := resource.Attributes()

	assertAttr(t, attrs, "service.name", "srapi-api")
	assertAttr(t, attrs, "service.version", "1.2.3")
	assertAttr(t, attrs, "deployment.environment.name", "test")
}

func TestNewTracerProviderDisabledDoesNotRequireCollector(t *testing.T) {
	tp, shutdown, err := NewTracerProvider(context.Background(), config.ObservabilityConfig{
		ServiceName:      "srapi",
		ServiceVersion:   "test",
		Environment:      "local",
		TraceSampleRatio: 1,
		BatchTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("new tracer provider: %v", err)
	}
	if tp == nil || shutdown == nil {
		t.Fatal("expected tracer provider and shutdown function")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown tracer provider: %v", err)
	}
}

func TestEndSpanRecordsErrorTypeAndStatus(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tracerProvider)
	t.Cleanup(func() {
		_ = tracerProvider.Shutdown(t.Context())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	ctx, span := StartSpan(context.Background(), "test.operation")
	EndSpan(span, errors.New("operation failed"), "classified_error", attribute.String("srapi.test.outcome", "failed"))
	_ = ctx

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %+v", spans)
	}
	if spans[0].Status.Code != codes.Error || spans[0].Status.Description != "classified_error" {
		t.Fatalf("expected error status, got %+v", spans[0].Status)
	}
	assertAttr(t, spans[0].Attributes, "error.type", "classified_error")
	assertAttr(t, spans[0].Attributes, "srapi.test.outcome", "failed")
}

func assertAttr(t *testing.T, attrs []attribute.KeyValue, key attribute.Key, value string) {
	t.Helper()
	for _, attr := range attrs {
		if attr.Key == key && attr.Value.AsString() == value {
			return
		}
	}
	t.Fatalf("missing attribute %s=%q in %+v", key, value, attrs)
}
