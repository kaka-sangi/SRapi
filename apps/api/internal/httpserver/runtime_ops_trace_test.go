package httpserver

import (
	"context"
	"testing"

	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsservice "github.com/srapi/srapi/apps/api/internal/modules/operations/service"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	opserrorlogsservice "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/service"
	opserrorlogsmemory "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/store/memory"
	"go.opentelemetry.io/otel/trace"
)

func TestGatewayOperationalEvidenceUsesTraceIDNotRequestID(t *testing.T) {
	const requestID = "req_trace_evidence"
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	ctx := contextWithRequestAndTraceForTest(t, requestID, traceID)

	opsErrorStore := opserrorlogsmemory.New()
	opsErrorService, err := opserrorlogsservice.New(opsErrorStore, nil)
	if err != nil {
		t.Fatalf("new ops error logs service: %v", err)
	}
	operationsStore := operationsmemory.New()
	operationsService, err := operationsservice.NewWithStores(operationsStore, operationsStore, nil)
	if err != nil {
		t.Fatalf("new operations service: %v", err)
	}
	rt := &runtimeState{
		opsErrorLogs: opsErrorService,
		operations:   operationsService,
	}
	server := &Server{runtime: rt}
	rec := gatewayUsageRecord{
		RequestID:            requestID,
		SourceProtocol:       "openai-compatible",
		SourceEndpoint:       "/v1/responses",
		TargetProtocol:       "openai-compatible",
		Model:                "ops-model",
		StatusCode:           ptrInt(502),
		ErrorClass:           ptrStringValue("server_bad"),
		ProviderErrorMessage: "upstream failed",
	}

	server.recordOpsErrorLog(ctx, rec)
	server.recordGatewaySystemLog(ctx, rec)

	errorLogs, err := opsErrorStore.List(context.Background(), opserrorlogscontract.ListFilter{})
	if err != nil {
		t.Fatalf("list ops error logs: %v", err)
	}
	if len(errorLogs.Items) != 1 {
		t.Fatalf("expected one ops error log, got %+v", errorLogs.Items)
	}
	if got := errorLogs.Items[0].TraceID; got != traceID {
		t.Fatalf("ops error log trace_id = %q, want %q", got, traceID)
	}
	if errorLogs.Items[0].TraceID == requestID {
		t.Fatalf("ops error log trace_id must not be request_id")
	}

	systemLogs, err := operationsStore.ListSystemLogs(context.Background(), operationscontract.SystemLogListOptions{})
	if err != nil {
		t.Fatalf("list system logs: %v", err)
	}
	if len(systemLogs.Items) != 1 {
		t.Fatalf("expected one system log, got %+v", systemLogs.Items)
	}
	if got := systemLogs.Items[0].TraceID; got != traceID {
		t.Fatalf("system log trace_id = %q, want %q", got, traceID)
	}
	if systemLogs.Items[0].TraceID == requestID {
		t.Fatalf("system log trace_id must not be request_id")
	}
}

func contextWithRequestAndTraceForTest(t *testing.T, requestID string, traceIDHex string) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex(traceIDHex)
	if err != nil {
		t.Fatalf("parse trace id: %v", err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("parse span id: %v", err)
	}
	ctx := context.WithValue(context.Background(), requestIDContextKey{}, requestID)
	return trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
}
