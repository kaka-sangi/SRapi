package httpserver

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

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
