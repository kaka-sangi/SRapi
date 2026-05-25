package httpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	"github.com/srapi/srapi/apps/api/internal/testsupport/oteltest"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
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

func TestTracingMiddlewareP99OverheadBudget(t *testing.T) {
	if os.Getenv("SRAPI_OTEL_P99_GUARD") != "1" {
		t.Skip("set SRAPI_OTEL_P99_GUARD=1 to run the OTel p99 overhead guard")
	}

	samples := envInt(t, "SRAPI_OTEL_P99_SAMPLES", 2000)
	warmup := envInt(t, "SRAPI_OTEL_P99_WARMUP", 200)
	budget := time.Duration(envInt(t, "SRAPI_OTEL_P99_BUDGET_MS", 5)) * time.Millisecond
	if samples < 100 {
		t.Fatalf("SRAPI_OTEL_P99_SAMPLES must be at least 100, got %d", samples)
	}
	if warmup < 0 {
		t.Fatalf("SRAPI_OTEL_P99_WARMUP must be non-negative, got %d", warmup)
	}

	baseline := buildObservedHandler(t, noop.NewTracerProvider())
	tracedProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			tracetest.NewInMemoryExporter(),
			sdktrace.WithMaxQueueSize(samples+warmup+100),
			sdktrace.WithMaxExportBatchSize(samples+warmup+100),
			sdktrace.WithBatchTimeout(time.Hour),
		),
	)
	traced := buildObservedHandler(t, tracedProvider)
	t.Cleanup(func() {
		_ = tracedProvider.Shutdown(t.Context())
		otel.SetTracerProvider(noop.NewTracerProvider())
	})

	for i := 0; i < warmup; i++ {
		serveObservedRequest(t, baseline, i)
		serveObservedRequest(t, traced, i)
	}
	runtime.GC()

	baselineDurations := make([]time.Duration, 0, samples)
	tracedDurations := make([]time.Duration, 0, samples)
	for i := 0; i < samples; i++ {
		baselineDurations = append(baselineDurations, measureObservedRequest(t, baseline, i))
		tracedDurations = append(tracedDurations, measureObservedRequest(t, traced, i))
	}

	baselineP99 := percentileDuration(baselineDurations, 99)
	tracedP99 := percentileDuration(tracedDurations, 99)
	overhead := tracedP99 - baselineP99
	if overhead < 0 {
		overhead = 0
	}
	t.Logf("otel p99 overhead: baseline=%s traced=%s overhead=%s budget=%s samples=%d warmup=%d", baselineP99, tracedP99, overhead, budget, samples, warmup)
	if overhead > budget {
		t.Fatalf("OTel p99 overhead %s exceeds budget %s", overhead, budget)
	}
}

func buildObservedHandler(t *testing.T, provider trace.TracerProvider) http.Handler {
	t.Helper()
	otel.SetTracerProvider(provider)
	return New(config.Load(), nil)
}

func measureObservedRequest(t *testing.T, handler http.Handler, index int) time.Duration {
	t.Helper()
	started := time.Now()
	serveObservedRequest(t, handler, index)
	return time.Since(started)
}

func serveObservedRequest(t *testing.T, handler http.Handler, index int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	request.Header.Set("X-Request-ID", "req_otel_"+strconv.Itoa(index))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected /livez 200, got %d body=%s", response.Code, response.Body.String())
	}
}

func percentileDuration(values []time.Duration, percentile int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	index := (len(sorted)*percentile + 99) / 100
	if index <= 0 {
		return sorted[0]
	}
	if index > len(sorted) {
		return sorted[len(sorted)-1]
	}
	return sorted[index-1]
}

func envInt(t *testing.T, key string, fallback int) int {
	t.Helper()
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("%s must be an integer, got %q", key, raw)
	}
	return value
}
