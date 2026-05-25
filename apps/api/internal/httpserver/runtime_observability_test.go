package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	"github.com/srapi/srapi/apps/api/internal/testsupport/oteltest"
	"go.opentelemetry.io/otel/trace"
)

func TestTracingMiddlewareRecordsHTTPServerSpan(t *testing.T) {
	exporter := oteltest.NewExporter(t)

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
	oteltest.AssertStringAttr(t, span.Attributes, "srapi.request_id", "req_trace")
	oteltest.AssertStringAttr(t, span.Attributes, "http.request.method", http.MethodGet)
	oteltest.AssertStringAttr(t, span.Attributes, "http.route", "/api/v1/health")
	oteltest.AssertIntAttr(t, span.Attributes, "http.response.status_code", http.StatusOK)
}
