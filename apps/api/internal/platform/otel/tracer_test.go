package otel

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"

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

func assertAttr(t *testing.T, attrs []attribute.KeyValue, key attribute.Key, value string) {
	t.Helper()
	for _, attr := range attrs {
		if attr.Key == key && attr.Value.AsString() == value {
			return
		}
	}
	t.Fatalf("missing attribute %s=%q in %+v", key, value, attrs)
}
