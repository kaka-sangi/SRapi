package httpserver

import (
	"context"
	"time"
)

// gatewayUsageWriteTimeout caps how long a single asynchronous usage/billing
// write may run before its context is canceled. It is deliberately generous —
// the previous synchronous path inherited the gateway request timeout (minutes),
// so a tight bound here would be a regression that raised the chance of
// canceling a write mid-sequence and leaving billing rows partially applied. It
// exists only to stop a permanently-stuck write from pinning a semaphore slot or
// stalling graceful shutdown forever.
const gatewayUsageWriteTimeout = 2 * time.Minute

// startUsageWriters arms asynchronous gateway usage/billing processing. When
// maxConcurrency <= 0 the semaphore stays nil and recordGatewayUsage runs
// inline (fully synchronous, the historical behavior). Otherwise the semaphore
// bounds in-flight async writes to maxConcurrency; recordGatewayUsage spawns a
// tracked goroutine per write up to that bound and falls back to inline
// processing when saturated.
func (rt *runtimeState) startUsageWriters(maxConcurrency int) {
	if maxConcurrency <= 0 {
		return
	}
	rt.usageSem = make(chan struct{}, maxConcurrency)
}

// dispatchUsageWrite runs fn off the request critical path when an async slot is
// free, and inline otherwise. ctx has already been detached from the request
// (so a client disconnect cannot cancel the billing write) but still carries
// the request's values for logging/tracing. Returns true when the write was
// dispatched asynchronously.
func (rt *runtimeState) dispatchUsageWrite(ctx context.Context, fn func(context.Context)) bool {
	if rt.usageSem == nil {
		fn(ctx)
		return false
	}
	// Hold the read lock across the slot acquisition and WaitGroup.Add so the
	// Add is ordered before drainUsageWriters' Wait (which takes the write lock
	// first). Once draining has begun we never Add again — running inline
	// instead — which prevents a WaitGroup reuse panic if a handler is still in
	// flight after a timed-out graceful shutdown.
	rt.usageMu.RLock()
	if rt.usageDraining {
		rt.usageMu.RUnlock()
		fn(ctx)
		return false
	}
	select {
	case rt.usageSem <- struct{}{}:
		rt.usageWG.Add(1)
		rt.usageMu.RUnlock()
		go func() {
			defer rt.usageWG.Done()
			defer func() { <-rt.usageSem }()
			defer func() {
				if r := recover(); r != nil && rt.logger != nil {
					rt.logger.Error("panic in async gateway usage write", "panic", r)
				}
			}()
			jobCtx, cancel := context.WithTimeout(ctx, gatewayUsageWriteTimeout)
			defer cancel()
			fn(jobCtx)
		}()
		return true
	default:
		rt.usageMu.RUnlock()
		// All async slots busy: apply backpressure by processing inline rather
		// than dropping billing data or growing an unbounded queue.
		fn(ctx)
		return false
	}
}

// drainUsageWriters blocks until every outstanding asynchronous usage write has
// finished, or until ctx is done. It first flips usageDraining so no new async
// writes are dispatched (any concurrent request processes inline), guaranteeing
// the WaitGroup count cannot rise once Wait begins. It is invoked during
// graceful shutdown AFTER the HTTP server has stopped accepting connections and
// BEFORE the database connection is closed, so in-flight billing/feedback writes
// are durably persisted. It then drains the best-effort ops_error_logs recorder
// with whatever budget remains. Safe to call more than once and when async
// processing was never armed.
func (rt *runtimeState) drainUsageWriters(ctx context.Context) {
	if rt == nil {
		return
	}
	if rt.usageSem != nil {
		rt.usageMu.Lock()
		rt.usageDraining = true
		rt.usageMu.Unlock()
		done := make(chan struct{})
		go func() {
			rt.usageWG.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			if rt.logger != nil {
				rt.logger.Warn("timed out draining async gateway usage writes", "error", ctx.Err())
			}
		}
	}
	if rt.opsErrorLogRecorder != nil {
		rt.opsErrorLogRecorder.drain(ctx)
	}
}
