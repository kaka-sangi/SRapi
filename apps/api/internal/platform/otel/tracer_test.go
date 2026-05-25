package otel

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"

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

func TestNewTracerProviderExportsSpansToOTLPCollector(t *testing.T) {
	endpoint, collector := startTraceCollector(t)
	tp, shutdown, err := NewTracerProvider(context.Background(), config.ObservabilityConfig{
		ServiceName:      "srapi-api",
		ServiceVersion:   "2026.5",
		Environment:      "test",
		TracesEnabled:    true,
		OTLPEndpoint:     endpoint,
		OTLPInsecure:     true,
		TraceSampleRatio: 1,
		BatchTimeout:     10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new tracer provider: %v", err)
	}

	_, span := tp.Tracer(tracerName).Start(context.Background(), "collector.smoke",
		trace.WithAttributes(attribute.String("srapi.test.outcome", "exported")),
	)
	span.End()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown tracer provider: %v", err)
	}

	spans := collector.spans()
	if len(spans) != 1 {
		t.Fatalf("expected one exported span, got %+v", spans)
	}
	if spans[0].Name != "collector.smoke" {
		t.Fatalf("expected collector smoke span, got %+v", spans[0])
	}
	assertProtoAttr(t, spans[0].Attributes, "srapi.test.outcome", "exported")
	assertProtoAttr(t, collector.resourceAttributes(), "service.name", "srapi-api")
	assertProtoAttr(t, collector.resourceAttributes(), "service.version", "2026.5")
	assertProtoAttr(t, collector.resourceAttributes(), "deployment.environment.name", "test")
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

type traceCollector struct {
	collectortracepb.UnimplementedTraceServiceServer

	mu       sync.Mutex
	requests []*collectortracepb.ExportTraceServiceRequest
}

func startTraceCollector(t *testing.T) (string, *traceCollector) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for OTLP trace collector: %v", err)
	}
	server := grpc.NewServer()
	collector := &traceCollector{}
	collectortracepb.RegisterTraceServiceServer(server, collector)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
	})
	return listener.Addr().String(), collector
}

func (c *traceCollector) Export(_ context.Context, req *collectortracepb.ExportTraceServiceRequest) (*collectortracepb.ExportTraceServiceResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	return &collectortracepb.ExportTraceServiceResponse{}, nil
}

func (c *traceCollector) spans() []*tracepb.Span {
	c.mu.Lock()
	defer c.mu.Unlock()
	var spans []*tracepb.Span
	for _, req := range c.requests {
		for _, resourceSpan := range req.GetResourceSpans() {
			for _, scopeSpan := range resourceSpan.GetScopeSpans() {
				spans = append(spans, scopeSpan.GetSpans()...)
			}
		}
	}
	return spans
}

func (c *traceCollector) resourceAttributes() []*commonpb.KeyValue {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, req := range c.requests {
		for _, resourceSpan := range req.GetResourceSpans() {
			if resource := resourceSpan.GetResource(); resource != nil {
				return resource.GetAttributes()
			}
		}
	}
	return nil
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

func assertProtoAttr(t *testing.T, attrs []*commonpb.KeyValue, key, value string) {
	t.Helper()
	for _, attr := range attrs {
		if attr.GetKey() == key && attr.GetValue().GetStringValue() == value {
			return
		}
	}
	t.Fatalf("missing proto attribute %s=%q in %+v", key, value, attrs)
}
