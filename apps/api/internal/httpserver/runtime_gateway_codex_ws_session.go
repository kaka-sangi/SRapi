// Package httpserver: Codex Responses WebSocket idle-tracker.
//
// This is a verbatim port of the CLIProxyAPI codexWebsocketSessionStore idle
// pattern (internal/runtime/executor/codex_websockets_executor.go) adapted for
// the srapi gateway. The CLIProxyAPI executor enforced a 5-minute read deadline
// on the upstream websocket; here we mirror that at the gateway-relay layer so
// that an idle client/upstream pair is reaped within the same 5-minute window
// instead of pinning a slot indefinitely.
//
// Behaviour matches CLIProxyAPI:
//   - codexResponsesWebsocketIdleTimeout = 5 * time.Minute
//   - SetActive(...) updates lastActivity on every frame in either direction
//   - a janitor goroutine sweeps every 30s and closes idle sessions with a
//     structured "ws_idle_timeout" log event and a normal WebSocket close on
//     both sides.
package httpserver

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

// codexResponsesWebsocketIdleTimeout mirrors the upstream constant from
// CLIProxyAPI/internal/runtime/executor/codex_websockets_executor.go.
const codexResponsesWebsocketIdleTimeout = 5 * time.Minute

// codexResponsesWebsocketJanitorInterval is how often the janitor wakes to
// sweep idle sessions. The CLIProxyAPI executor enforced the timeout via
// SetReadDeadline; we approximate that resolution with a 30s sweep.
const codexResponsesWebsocketJanitorInterval = 30 * time.Second

// wsIdleSession tracks the last-activity timestamp for a single websocket
// relay. The store closes the session (both sides) once it has been idle for
// codexResponsesWebsocketIdleTimeout.
type wsIdleSession struct {
	id           string
	lastActivity atomic.Int64 // unix nanoseconds; updated by SetActive
	clientConn   *websocket.Conn
	upstreamConn *websocket.Conn // may be nil for gateway-captured (non-relay) sessions
	cancel       context.CancelFunc
	closeOnce    sync.Once
	closed       atomic.Bool
	now          func() time.Time // injected by the owning store; nil-safe via SetActive
}

// SetActive records that a frame was just sent or received on this session.
// Safe to call from multiple goroutines.
func (s *wsIdleSession) SetActive() {
	if s == nil {
		return
	}
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	s.lastActivity.Store(now().UnixNano())
}

// LastActivity returns the most recent SetActive timestamp.
func (s *wsIdleSession) LastActivity() time.Time {
	if s == nil {
		return time.Time{}
	}
	return time.Unix(0, s.lastActivity.Load())
}

// IsClosed reports whether the session has been reaped/closed.
func (s *wsIdleSession) IsClosed() bool {
	if s == nil {
		return true
	}
	return s.closed.Load()
}

// closeNow drives the idle-timeout teardown: emit a normal close frame to
// both sides, run the cancel hook, and mark the session closed exactly once.
func (s *wsIdleSession) closeNow(reason string) {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		if s.clientConn != nil {
			_ = s.clientConn.Close(websocket.StatusNormalClosure, reason)
		}
		if s.upstreamConn != nil {
			_ = s.upstreamConn.Close(websocket.StatusNormalClosure, reason)
		}
		if s.cancel != nil {
			s.cancel()
		}
	})
}

// wsIdleSessionStore is the gateway-wide registry of active websocket
// sessions. The janitor goroutine is started lazily the first time a session
// is registered.
type wsIdleSessionStore struct {
	mu          sync.Mutex
	sessions    map[string]*wsIdleSession
	janitorOnce sync.Once
	timeout     time.Duration
	interval    time.Duration
	now         func() time.Time
	logger      *slog.Logger
}

// globalWSIdleSessionStore is the package-level store used by the gateway
// WebSocket handlers. Tests construct their own stores via
// newWSIdleSessionStore for hermetic time control.
var globalWSIdleSessionStore = newWSIdleSessionStore(
	codexResponsesWebsocketIdleTimeout,
	codexResponsesWebsocketJanitorInterval,
	time.Now,
	nil,
)

func newWSIdleSessionStore(timeout, interval time.Duration, now func() time.Time, logger *slog.Logger) *wsIdleSessionStore {
	if now == nil {
		now = time.Now
	}
	return &wsIdleSessionStore{
		sessions: make(map[string]*wsIdleSession),
		timeout:  timeout,
		interval: interval,
		now:      now,
		logger:   logger,
	}
}

// SetLogger lets the http server wire its structured logger into the global
// store on first use. Safe to call repeatedly; the most recent non-nil logger
// wins.
func (st *wsIdleSessionStore) SetLogger(logger *slog.Logger) {
	if st == nil || logger == nil {
		return
	}
	st.mu.Lock()
	st.logger = logger
	st.mu.Unlock()
}

// Register adds a session to the store and ensures the janitor is running.
// The returned cancel func should be deferred by the caller; it removes the
// session from the store without driving the close frames (the WebSocket
// handlers run their own close paths on normal exit).
func (st *wsIdleSessionStore) Register(id string, clientConn, upstreamConn *websocket.Conn, cancel context.CancelFunc) *wsIdleSession {
	if st == nil || id == "" {
		return nil
	}
	sess := &wsIdleSession{
		id:           id,
		clientConn:   clientConn,
		upstreamConn: upstreamConn,
		cancel:       cancel,
		now:          st.now,
	}
	sess.SetActive()
	st.mu.Lock()
	st.sessions[id] = sess
	st.mu.Unlock()
	st.ensureJanitor()
	return sess
}

// Unregister removes a session from the store. Safe to call multiple times.
func (st *wsIdleSessionStore) Unregister(id string) {
	if st == nil || id == "" {
		return
	}
	st.mu.Lock()
	delete(st.sessions, id)
	st.mu.Unlock()
}

// ensureJanitor starts the sweep goroutine on first use.
func (st *wsIdleSessionStore) ensureJanitor() {
	st.janitorOnce.Do(func() {
		go st.runJanitor(context.Background())
	})
}

// runJanitor wakes every interval, sweeps the session map, and closes
// sessions that have been idle longer than the configured timeout. Closed
// sessions are removed eagerly so that callers that never call Unregister
// (e.g. on a panic) cannot leak entries.
func (st *wsIdleSessionStore) runJanitor(ctx context.Context) {
	ticker := time.NewTicker(st.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st.sweep()
		}
	}
}

// sweep is exported on the store for test-driven sweeps. It walks the
// session map once, snapshotting candidates under the store mutex, then
// drives the close paths outside the lock to avoid holding it across a
// network write.
func (st *wsIdleSessionStore) sweep() {
	if st == nil {
		return
	}
	now := st.now()
	timeout := st.timeout

	type expiredSession struct {
		sess    *wsIdleSession
		idleFor time.Duration
	}
	var expired []expiredSession

	st.mu.Lock()
	for id, sess := range st.sessions {
		if sess == nil {
			delete(st.sessions, id)
			continue
		}
		if sess.IsClosed() {
			delete(st.sessions, id)
			continue
		}
		idle := now.Sub(sess.LastActivity())
		if idle >= timeout {
			expired = append(expired, expiredSession{sess: sess, idleFor: idle})
			delete(st.sessions, id)
		}
	}
	logger := st.logger
	st.mu.Unlock()

	for _, ex := range expired {
		if logger != nil {
			logger.Info("ws_idle_timeout",
				"session_id", ex.sess.id,
				"idle_ms", ex.idleFor.Milliseconds(),
				"timeout_ms", timeout.Milliseconds(),
			)
		}
		ex.sess.closeNow("ws_idle_timeout")
	}
}
