package httpserver

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWSIdleSessionSetActiveResetsClock verifies that SetActive updates the
// LastActivity timestamp and that the store does not consider a session idle
// when it has been kept active.
func TestWSIdleSessionSetActiveResetsClock(t *testing.T) {
	t.Parallel()

	var fakeNow atomic.Int64
	fakeNow.Store(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	now := func() time.Time { return time.Unix(0, fakeNow.Load()) }

	store := newWSIdleSessionStore(5*time.Minute, time.Hour, now, slog.Default())
	sess := store.Register("session-active", nil, nil, func() {})
	if sess == nil {
		t.Fatalf("Register returned nil")
	}
	if got := sess.LastActivity().UnixNano(); got == 0 {
		t.Fatalf("expected lastActivity to be set on Register, got 0")
	}

	// 4 minutes pass, then a frame arrives; the session must NOT be reaped.
	fakeNow.Add(int64(4 * time.Minute))
	sess.SetActive()

	fakeNow.Add(int64(4 * time.Minute))
	store.sweep()

	if sess.IsClosed() {
		t.Fatalf("session was reaped despite SetActive at 4min mark; idle window should restart")
	}
}

// TestWSIdleSessionReapsAfterTimeout verifies the janitor sweep closes a
// session that has been idle longer than the configured timeout, emits the
// structured ws_idle_timeout log, and runs the cancel hook.
func TestWSIdleSessionReapsAfterTimeout(t *testing.T) {
	t.Parallel()

	var fakeNow atomic.Int64
	fakeNow.Store(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	now := func() time.Time { return time.Unix(0, fakeNow.Load()) }

	var logBuf bytes.Buffer
	var bufMu sync.Mutex
	logger := slog.New(slog.NewJSONHandler(syncWriter{w: &logBuf, mu: &bufMu}, &slog.HandlerOptions{Level: slog.LevelInfo}))
	store := newWSIdleSessionStore(5*time.Minute, time.Hour, now, logger)

	cancelCalled := make(chan struct{}, 1)
	sess := store.Register("session-expired", nil, nil, func() {
		select {
		case cancelCalled <- struct{}{}:
		default:
		}
	})

	// Jump the clock past the idle window.
	fakeNow.Add(int64(6 * time.Minute))
	store.sweep()

	if !sess.IsClosed() {
		t.Fatalf("session was not reaped after sweep past timeout")
	}
	select {
	case <-cancelCalled:
	case <-time.After(time.Second):
		t.Fatalf("cancel hook was not invoked on idle reap")
	}

	bufMu.Lock()
	logOutput := logBuf.String()
	bufMu.Unlock()
	if !strings.Contains(logOutput, "ws_idle_timeout") {
		t.Fatalf("expected ws_idle_timeout log event, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "session-expired") {
		t.Fatalf("expected session_id in log, got: %s", logOutput)
	}

	// A second sweep must be a no-op.
	store.sweep()
}

// TestWSIdleSessionUnregisterRemovesFromStore verifies that callers can
// unregister cleanly and that a subsequent sweep does not panic.
func TestWSIdleSessionUnregisterRemovesFromStore(t *testing.T) {
	t.Parallel()

	store := newWSIdleSessionStore(5*time.Minute, time.Hour, time.Now, slog.Default())
	store.Register("session-unregister", nil, nil, func() {})
	store.Unregister("session-unregister")

	store.mu.Lock()
	if _, ok := store.sessions["session-unregister"]; ok {
		store.mu.Unlock()
		t.Fatalf("session still present after Unregister")
	}
	store.mu.Unlock()

	store.sweep()
}

// TestWSIdleSessionNilSafe guards the nil-receiver contract used by callers
// that may not have created a session (e.g. tests with synthetic flows).
func TestWSIdleSessionNilSafe(t *testing.T) {
	t.Parallel()

	var sess *wsIdleSession
	sess.SetActive()
	if !sess.IsClosed() {
		t.Fatalf("nil session must report IsClosed == true")
	}
	if got := sess.LastActivity(); !got.IsZero() {
		t.Fatalf("nil session LastActivity must be zero, got %v", got)
	}
	sess.closeNow("noop")
}

// TestWSIdleSessionStoreNilSafe guards the nil-receiver contract on the
// store so that defensive call sites do not need to nil-check.
func TestWSIdleSessionStoreNilSafe(t *testing.T) {
	t.Parallel()

	var store *wsIdleSessionStore
	if got := store.Register("x", nil, nil, nil); got != nil {
		t.Fatalf("nil store Register must return nil")
	}
	store.Unregister("x")
	store.sweep()
	store.SetLogger(slog.Default())
}

// syncWriter is a tiny adapter so multiple goroutines can write to the same
// bytes.Buffer in tests without racing the slog handler.
type syncWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (sw syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// TestWSIdleSessionJanitorTickerStops ensures starting the janitor in a
// canceled context returns quickly. This protects the lazily-started
// goroutine from leaking in tests that exercise the global store.
func TestWSIdleSessionJanitorTickerStops(t *testing.T) {
	t.Parallel()

	store := newWSIdleSessionStore(time.Second, 10*time.Millisecond, time.Now, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		store.runJanitor(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("janitor did not stop after context cancel")
	}
}
