package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestTracingMiddlewareRecordsHTTPServerSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tracerProvider)
	t.Cleanup(func() {
		_ = tracerProvider.Shutdown(t.Context())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	handler := New(config.Load(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	request.Header.Set("X-Request-ID", "req_trace")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one HTTP span, got %+v", spans)
	}
	span := spans[0]
	if span.Name != "GET /api/v1/health" || span.SpanKind != trace.SpanKindServer {
		t.Fatalf("unexpected span shape: %+v", span)
	}
	assertSpanAttr(t, span.Attributes, "srapi.request_id", "req_trace")
	assertSpanAttr(t, span.Attributes, "http.request.method", http.MethodGet)
	assertSpanAttr(t, span.Attributes, "http.route", "/api/v1/health")
	assertSpanIntAttr(t, span.Attributes, "http.response.status_code", http.StatusOK)
}

func assertSpanAttr(t *testing.T, attrs []attribute.KeyValue, key attribute.Key, value string) {
	t.Helper()
	for _, attr := range attrs {
		if attr.Key == key && attr.Value.AsString() == value {
			return
		}
	}
	t.Fatalf("missing span attribute %s=%q in %+v", key, value, attrs)
}

func assertSpanIntAttr(t *testing.T, attrs []attribute.KeyValue, key attribute.Key, value int) {
	t.Helper()
	for _, attr := range attrs {
		if attr.Key == key && attr.Value.AsInt64() == int64(value) {
			return
		}
	}
	t.Fatalf("missing span attribute %s=%d in %+v", key, value, attrs)
}
