package httpserver

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	opserrorlogsservice "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/service"
)

const (
	defaultOpsErrorLogQueueSize    = 256
	defaultOpsErrorLogWriteTimeout = 5 * time.Second
)

type opsErrorLogRecorderConfig struct {
	QueueSize    int
	WriteTimeout time.Duration
}

type opsErrorLogRecorder struct {
	service      *opserrorlogsservice.Service
	logger       *slog.Logger
	queue        chan opserrorlogscontract.RecordRequest
	writeTimeout time.Duration
	mu           sync.RWMutex
	draining     bool
	startOnce    sync.Once
	started      atomic.Bool
	stopOnce     sync.Once
	done         chan struct{}
	enqueued     atomic.Int64
	processed    atomic.Int64
	dropped      atomic.Int64
	writeFailed  atomic.Int64
}

func newOpsErrorLogRecorder(service *opserrorlogsservice.Service, logger *slog.Logger, cfg opsErrorLogRecorderConfig) *opsErrorLogRecorder {
	if service == nil {
		return nil
	}
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = defaultOpsErrorLogQueueSize
	}
	writeTimeout := cfg.WriteTimeout
	if writeTimeout <= 0 {
		writeTimeout = defaultOpsErrorLogWriteTimeout
	}
	r := &opsErrorLogRecorder{
		service:      service,
		logger:       logger,
		queue:        make(chan opserrorlogscontract.RecordRequest, queueSize),
		writeTimeout: writeTimeout,
		done:         make(chan struct{}),
	}
	return r
}

func (r *opsErrorLogRecorder) enqueue(req opserrorlogscontract.RecordRequest) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	if r.draining {
		r.mu.RUnlock()
		return false
	}
	r.startOnce.Do(func() {
		r.started.Store(true)
		go r.run()
	})
	select {
	case r.queue <- req:
		r.enqueued.Add(1)
		r.mu.RUnlock()
		return true
	default:
		r.mu.RUnlock()
		dropped := r.dropped.Add(1)
		if r.logger != nil && (dropped == 1 || dropped%100 == 0) {
			r.logger.Warn("ops_error_logs async queue full; dropping error evidence", "queued", len(r.queue), "capacity", cap(r.queue), "dropped_total", dropped)
		}
		return false
	}
}

func (r *opsErrorLogRecorder) drain(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.draining = true
	started := r.started.Load()
	r.mu.Unlock()
	if !started {
		return
	}
	r.stopOnce.Do(func() {
		close(r.queue)
	})
	select {
	case <-r.done:
	case <-ctx.Done():
		if r.logger != nil {
			r.logger.Warn("timed out draining ops_error_logs async recorder", "error", ctx.Err(), "queued", len(r.queue), "dropped_total", r.dropped.Load())
		}
	}
}

func (r *opsErrorLogRecorder) run() {
	defer close(r.done)
	for req := range r.queue {
		r.record(req)
	}
}

func (r *opsErrorLogRecorder) record(req opserrorlogscontract.RecordRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), r.writeTimeout)
	defer cancel()
	if err := r.service.RecordError(ctx, req); err != nil {
		r.writeFailed.Add(1)
		if r.logger != nil {
			r.logger.Warn("ops_error_logs RecordError failed", "request_id", req.RequestID, "error", err)
		}
	}
	r.processed.Add(1)
}

type opsErrorLogRecorderSnapshot struct {
	Queued      int
	Capacity    int
	Enqueued    int64
	Processed   int64
	Dropped     int64
	WriteFailed int64
	Started     bool
	Draining    bool
}

func (r *opsErrorLogRecorder) snapshot() opsErrorLogRecorderSnapshot {
	if r == nil {
		return opsErrorLogRecorderSnapshot{}
	}
	r.mu.RLock()
	draining := r.draining
	r.mu.RUnlock()
	return opsErrorLogRecorderSnapshot{
		Queued:      len(r.queue),
		Capacity:    cap(r.queue),
		Enqueued:    r.enqueued.Load(),
		Processed:   r.processed.Load(),
		Dropped:     r.dropped.Load(),
		WriteFailed: r.writeFailed.Load(),
		Started:     r.started.Load(),
		Draining:    draining,
	}
}
