// Package taskqueue provides a bounded, worker-pool-backed task queue with
// configurable overflow policies. It is modeled after the bounded FIFO channel
// pattern used in sub2api for background processing (usage recording,
// subscription maintenance, etc.) and gives callers explicit control over what
// happens when the queue is full: drop the task, block the caller, or run the
// task synchronously in the caller's goroutine.
//
// Usage:
//
//	q := taskqueue.New(taskqueue.Config{
//	    Name:      "billing",
//	    Workers:   4,
//	    QueueSize: 512,
//	    Overflow:  taskqueue.OverflowRunSync,
//	    Logger:    slog.Default(),
//	})
//	q.Start(ctx)
//	q.Submit(func(ctx context.Context) { recordUsage(ctx) })
//	// On shutdown:
//	q.Shutdown(shutdownCtx)
package taskqueue

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// OverflowPolicy determines what happens when the internal channel is full and
// a new task is submitted.
type OverflowPolicy int

const (
	// OverflowDrop silently drops the task and increments the dropped counter.
	OverflowDrop OverflowPolicy = iota
	// OverflowBlock blocks the caller until a slot becomes available.
	OverflowBlock
	// OverflowRunSync runs the task inline in the caller's goroutine, providing
	// backpressure without data loss — the same fallback used by the existing
	// gateway usage writer's semaphore path.
	OverflowRunSync
)

// Task is the unit of work processed by workers.
type Task func(ctx context.Context)

// Queue is a bounded, worker-pool-backed task queue. Use [New] to create one
// and [Queue.Start] to launch the worker goroutines. Once started, tasks are
// submitted via [Queue.Submit] and processed by the pool until [Queue.Shutdown]
// is called.
type Queue struct {
	name     string
	ch       chan Task
	wg       sync.WaitGroup
	overflow OverflowPolicy
	logger   *slog.Logger
	stopped  atomic.Bool
	dropped  atomic.Int64
	executed atomic.Int64
	workers  int
}

// Config parameterizes a [Queue].
type Config struct {
	// Name is a human-readable label used in log messages.
	Name string
	// Workers is the number of concurrent consumer goroutines. Defaults to 1.
	Workers int
	// QueueSize is the capacity of the internal buffered channel. Defaults to 256.
	QueueSize int
	// Overflow determines what happens when the channel is full. Defaults to
	// OverflowDrop.
	Overflow OverflowPolicy
	// Logger receives panic and overflow log entries. May be nil.
	Logger *slog.Logger
}

// New creates a Queue. Call [Queue.Start] to launch its worker goroutines.
func New(cfg Config) *Queue {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 256
	}
	return &Queue{
		name:     cfg.Name,
		ch:       make(chan Task, cfg.QueueSize),
		overflow: cfg.Overflow,
		logger:   cfg.Logger,
		workers:  cfg.Workers,
	}
}

// Start launches the worker goroutines that drain the internal channel. It
// should be called exactly once. The provided context controls the lifetime of
// the workers: when it is canceled the workers exit after finishing the current
// task.
func (q *Queue) Start(ctx context.Context) {
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(ctx)
	}
}

// worker is the main loop for a single consumer goroutine. It processes tasks
// from the channel until the channel is closed or the context is canceled.
func (q *Queue) worker(ctx context.Context) {
	defer q.wg.Done()
	for {
		select {
		case task, ok := <-q.ch:
			if !ok {
				return
			}
			q.runTask(ctx, task)
		case <-ctx.Done():
			return
		}
	}
}

// runTask executes a single task with panic recovery.
func (q *Queue) runTask(ctx context.Context, task Task) {
	defer func() {
		if r := recover(); r != nil {
			if q.logger != nil {
				q.logger.Error("task queue worker panic",
					"queue", q.name,
					"panic", r,
				)
			}
		}
	}()
	task(ctx)
	q.executed.Add(1)
}

// Submit enqueues a task for asynchronous processing. It returns true when the
// task was accepted (either enqueued or, for OverflowRunSync, executed inline)
// and false when it was dropped or the queue has been stopped.
func (q *Queue) Submit(task Task) bool {
	if q.stopped.Load() {
		return false
	}
	select {
	case q.ch <- task:
		return true
	default:
		return q.handleOverflow(task)
	}
}

// handleOverflow applies the configured overflow policy when the channel is
// full.
func (q *Queue) handleOverflow(task Task) bool {
	switch q.overflow {
	case OverflowDrop:
		q.dropped.Add(1)
		if q.logger != nil {
			q.logger.Warn("task queue overflow: task dropped",
				"queue", q.name,
				"dropped_total", q.dropped.Load(),
			)
		}
		return false
	case OverflowRunSync:
		q.runTask(context.Background(), task)
		return true
	case OverflowBlock:
		q.ch <- task
		return true
	default:
		q.dropped.Add(1)
		return false
	}
}

// Shutdown stops accepting new tasks, closes the internal channel, and waits
// for all workers to finish processing queued work. If ctx is canceled before
// workers finish, the remaining workers are abandoned and ctx.Err() is
// returned.
func (q *Queue) Shutdown(ctx context.Context) error {
	q.stopped.Store(true)
	close(q.ch)
	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stats returns the lifetime count of tasks executed and dropped.
func (q *Queue) Stats() (executed, dropped int64) {
	return q.executed.Load(), q.dropped.Load()
}

// Len returns the number of tasks currently buffered in the channel.
func (q *Queue) Len() int {
	return len(q.ch)
}
