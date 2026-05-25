package oteltest

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

// NewExporter installs an in-memory tracer provider for a single test.
func NewExporter(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tracerProvider)
	t.Cleanup(func() {
		_ = tracerProvider.Shutdown(t.Context())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})
	return exporter
}

// FindSpan returns the first exported span with the provided name.
func FindSpan(t *testing.T, spans tracetest.SpanStubs, name string) tracetest.SpanStub {
	t.Helper()
	for _, span := range spans {
		if span.Name == name {
			return span
		}
	}
	t.Fatalf("missing span %q in %+v", name, spans)
	return tracetest.SpanStub{}
}

// AssertStringAttr verifies that attrs contains key=value as a string attribute.
func AssertStringAttr(t *testing.T, attrs []attribute.KeyValue, key attribute.Key, value string) {
	t.Helper()
	for _, attr := range attrs {
		if attr.Key == key && attr.Value.AsString() == value {
			return
		}
	}
	t.Fatalf("missing span attribute %s=%q in %+v", key, value, attrs)
}

// AssertIntAttr verifies that attrs contains key=value as an int attribute.
func AssertIntAttr(t *testing.T, attrs []attribute.KeyValue, key attribute.Key, value int) {
	t.Helper()
	for _, attr := range attrs {
		if attr.Key == key && attr.Value.AsInt64() == int64(value) {
			return
		}
	}
	t.Fatalf("missing span attribute %s=%d in %+v", key, value, attrs)
}
