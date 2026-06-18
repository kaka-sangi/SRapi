package httpserver

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	opserrorlogsservice "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/service"
)

type blockingOpsErrorLogStore struct {
	release chan struct{}
	entered chan struct{}
	once    sync.Once
}

func newBlockingOpsErrorLogStore() *blockingOpsErrorLogStore {
	return &blockingOpsErrorLogStore{
		release: make(chan struct{}),
		entered: make(chan struct{}),
	}
}

func (s *blockingOpsErrorLogStore) Insert(ctx context.Context, entry opserrorlogscontract.Entry) (opserrorlogscontract.Entry, error) {
	s.once.Do(func() {
		close(s.entered)
	})
	select {
	case <-s.release:
	case <-ctx.Done():
		return opserrorlogscontract.Entry{}, ctx.Err()
	}
	return entry, nil
}

func (s *blockingOpsErrorLogStore) List(context.Context, opserrorlogscontract.ListFilter) (opserrorlogscontract.ListResult, error) {
	return opserrorlogscontract.ListResult{}, nil
}

func (s *blockingOpsErrorLogStore) Get(context.Context, int64) (opserrorlogscontract.Entry, error) {
	return opserrorlogscontract.Entry{}, opserrorlogscontract.ErrNotFound
}

func (s *blockingOpsErrorLogStore) UpdateResolution(context.Context, opserrorlogscontract.UpdateResolutionRequest) (opserrorlogscontract.Entry, error) {
	return opserrorlogscontract.Entry{}, opserrorlogscontract.ErrNotFound
}

func (s *blockingOpsErrorLogStore) DeleteOlderThan(context.Context, time.Time) (int, error) {
	return 0, nil
}

type failingOpsErrorLogStore struct {
	err error
}

func (s failingOpsErrorLogStore) Insert(context.Context, opserrorlogscontract.Entry) (opserrorlogscontract.Entry, error) {
	return opserrorlogscontract.Entry{}, s.err
}

func (s failingOpsErrorLogStore) List(context.Context, opserrorlogscontract.ListFilter) (opserrorlogscontract.ListResult, error) {
	return opserrorlogscontract.ListResult{}, nil
}

func (s failingOpsErrorLogStore) Get(context.Context, int64) (opserrorlogscontract.Entry, error) {
	return opserrorlogscontract.Entry{}, opserrorlogscontract.ErrNotFound
}

func (s failingOpsErrorLogStore) UpdateResolution(context.Context, opserrorlogscontract.UpdateResolutionRequest) (opserrorlogscontract.Entry, error) {
	return opserrorlogscontract.Entry{}, opserrorlogscontract.ErrNotFound
}

func (s failingOpsErrorLogStore) DeleteOlderThan(context.Context, time.Time) (int, error) {
	return 0, nil
}

func TestRecordOpsErrorLogDoesNotBlockOnSlowStore(t *testing.T) {
	store := newBlockingOpsErrorLogStore()
	svc, err := opserrorlogsservice.New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	rt := &runtimeState{
		logger:              slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		opsErrorLogs:        svc,
		opsErrorLogRecorder: newOpsErrorLogRecorder(svc, nil, opsErrorLogRecorderConfig{QueueSize: 1, WriteTimeout: time.Second}),
	}
	defer func() {
		close(store.release)
		rt.opsErrorLogRecorder.drain(context.Background())
	}()
	server := &Server{runtime: rt}
	rec := gatewayUsageRecord{
		RequestID:                "req_async_ops",
		SourceProtocol:           "openai-compatible",
		SourceEndpoint:           "/v1/chat/completions",
		TargetProtocol:           "openai-compatible",
		Model:                    "ops-model",
		StatusCode:               ptrInt(502),
		ErrorClass:               ptrStringValue("server_bad"),
		ProviderErrorMessage:     "upstream failed",
		ProviderErrorBodyExcerpt: `{"error":"bad gateway"}`,
	}

	done := make(chan struct{})
	go func() {
		server.recordOpsErrorLog(context.Background(), rec)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("recordOpsErrorLog blocked on ops error log persistence")
	}

	select {
	case <-store.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("async ops error log write did not reach the store")
	}
}

func TestOpsErrorLogRecorderLogsWriteFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	svc, err := opserrorlogsservice.New(failingOpsErrorLogStore{err: errors.New("store unavailable")}, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	recorder := newOpsErrorLogRecorder(svc, logger, opsErrorLogRecorderConfig{QueueSize: 1, WriteTimeout: time.Second})

	if !recorder.enqueue(opserrorlogscontract.RecordRequest{RequestID: "req_failed_ops", ErrorMessage: "provider failed"}) {
		t.Fatal("expected enqueue to succeed")
	}
	recorder.drain(context.Background())

	if got := buf.String(); !strings.Contains(got, "ops_error_logs RecordError failed") || !strings.Contains(got, "req_failed_ops") {
		t.Fatalf("expected write failure log with request id, got %q", got)
	}
}

func TestOpsErrorLogRecorderLogsQueueDrops(t *testing.T) {
	store := newBlockingOpsErrorLogStore()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	svc, err := opserrorlogsservice.New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	recorder := newOpsErrorLogRecorder(svc, logger, opsErrorLogRecorderConfig{QueueSize: 1, WriteTimeout: time.Second})
	defer func() {
		close(store.release)
		recorder.drain(context.Background())
	}()

	if !recorder.enqueue(opserrorlogscontract.RecordRequest{RequestID: "req_blocking", ErrorMessage: "provider failed"}) {
		t.Fatal("expected first enqueue to start worker")
	}
	select {
	case <-store.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("async worker did not enter blocking store")
	}
	if !recorder.enqueue(opserrorlogscontract.RecordRequest{RequestID: "req_buffered", ErrorMessage: "provider failed"}) {
		t.Fatal("expected second enqueue to fill queue buffer")
	}
	if recorder.enqueue(opserrorlogscontract.RecordRequest{RequestID: "req_dropped", ErrorMessage: "provider failed"}) {
		t.Fatal("expected third enqueue to be dropped when queue is full")
	}

	if got := buf.String(); !strings.Contains(got, "ops_error_logs async queue full; dropping error evidence") {
		t.Fatalf("expected queue drop warning, got %q", got)
	}
}

func TestOpsErrorLogRecorderMetricsExposeDropsAndFailures(t *testing.T) {
	store := newBlockingOpsErrorLogStore()
	svc, err := opserrorlogsservice.New(store, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	recorder := newOpsErrorLogRecorder(svc, nil, opsErrorLogRecorderConfig{QueueSize: 1, WriteTimeout: time.Second})
	defer func() {
		close(store.release)
		recorder.drain(context.Background())
	}()

	if !recorder.enqueue(opserrorlogscontract.RecordRequest{RequestID: "req_blocking", ErrorMessage: "provider failed"}) {
		t.Fatal("expected first enqueue to start worker")
	}
	select {
	case <-store.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("async worker did not enter blocking store")
	}
	if !recorder.enqueue(opserrorlogscontract.RecordRequest{RequestID: "req_buffered", ErrorMessage: "provider failed"}) {
		t.Fatal("expected second enqueue to fill queue buffer")
	}
	if recorder.enqueue(opserrorlogscontract.RecordRequest{RequestID: "req_dropped", ErrorMessage: "provider failed"}) {
		t.Fatal("expected third enqueue to be dropped when queue is full")
	}

	collector := newOpsErrorLogRecorderMetricsTestCollector(recorder)
	metrics := gatherRecorderMetrics(t, collector)
	for _, expected := range []string{
		"srapi_ops_error_log_dropped_total 1",
		"srapi_ops_error_log_enqueued_total 2",
		"srapi_ops_error_log_processed_total 0",
		"srapi_ops_error_log_queue_capacity 1",
		"srapi_ops_error_log_queue_depth 1",
		"srapi_ops_error_log_write_failures_total 0",
	} {
		if !strings.Contains(metrics, expected) {
			t.Fatalf("expected metrics to contain %q, got:\n%s", expected, metrics)
		}
	}
}

func TestOpsErrorLogRecorderMetricsExposeWriteFailures(t *testing.T) {
	svc, err := opserrorlogsservice.New(failingOpsErrorLogStore{err: errors.New("store unavailable")}, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	recorder := newOpsErrorLogRecorder(svc, nil, opsErrorLogRecorderConfig{QueueSize: 1, WriteTimeout: time.Second})
	if !recorder.enqueue(opserrorlogscontract.RecordRequest{RequestID: "req_failed_ops", ErrorMessage: "provider failed"}) {
		t.Fatal("expected enqueue to succeed")
	}
	recorder.drain(context.Background())

	metrics := gatherRecorderMetrics(t, newOpsErrorLogRecorderMetricsTestCollector(recorder))
	if expected := "srapi_ops_error_log_write_failures_total 1"; !strings.Contains(metrics, expected) {
		t.Fatalf("expected metrics to contain %q, got:\n%s", expected, metrics)
	}
}

type opsErrorLogRecorderMetricsTestCollector struct {
	inner *runtimeMetricsCollector
}

func newOpsErrorLogRecorderMetricsTestCollector(recorder *opsErrorLogRecorder) opsErrorLogRecorderMetricsTestCollector {
	return opsErrorLogRecorderMetricsTestCollector{
		inner: newRuntimeMetricsCollector(context.Background(), &runtimeState{opsErrorLogRecorder: recorder}),
	}
}

func (c opsErrorLogRecorderMetricsTestCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.inner.descs.opsErrorLogQueueDepth
	ch <- c.inner.descs.opsErrorLogQueueCapacity
	ch <- c.inner.descs.opsErrorLogEnqueued
	ch <- c.inner.descs.opsErrorLogProcessed
	ch <- c.inner.descs.opsErrorLogDropped
	ch <- c.inner.descs.opsErrorLogWriteFailures
}

func (c opsErrorLogRecorderMetricsTestCollector) Collect(ch chan<- prometheus.Metric) {
	c.inner.collectWorkerMetrics(ch, map[string]bool{})
}

func gatherRecorderMetrics(t *testing.T, collector prometheus.Collector) string {
	t.Helper()
	registry := prometheus.NewPedanticRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatalf("register collector: %v", err)
	}
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{ErrorHandling: promhttp.HTTPErrorOnError}).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected metrics 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}
