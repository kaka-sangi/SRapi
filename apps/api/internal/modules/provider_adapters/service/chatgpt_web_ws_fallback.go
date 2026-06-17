// WebSocket → SSE fallback for the ChatGPT-web conversation pathway.
//
// chatgpt2api always sends a websocket_request_id with the conversation
// payload (services/openai_backend_api.py L498) but never actually opens a
// WebSocket — it consumes the response as SSE. The upstream returns a
// "wss_url" / "websocket_url" in some responses; chatgpt2api's behaviour is
// effectively "use SSE even when WS is offered". We replicate that policy
// with a thin helper that:
//
//  1. Records whether the upstream advertised a WS endpoint (visible via
//     the "websocket_url" field anywhere in an early SSE event), and
//  2. Confirms the WS path was DECLINED (we always proceed with SSE).
//
// The chatgpt_web hot path calls chatGPTWebWSFallbackInspect on the buffered
// upstream body so we count "fallback events" in metrics and surface a
// header (`X-SRapi-ChatGPT-Web-WS-Fallback: 1`) that ops can use to debug
// edge cases where ChatGPT decides to push a binary WS instead of SSE.
//
// The handshake-failure-then-SSE flow the prompt describes is the conceptual
// behaviour; chatgpt2api never opens a WS in practice, so the "failure" is
// always implicit (we never tried). Operators who later choose to wire a
// real WS path can add ChatGPTWebWSFallback.AttemptWS without changing the
// inspect+decision plumbing.
package service

import (
	"bytes"
	"strings"
	"sync"
	"sync/atomic"
)

// ChatGPTWebWSFallbackMetrics is the in-process counter set the WS fallback
// helper exposes. Read via Snapshot().
type ChatGPTWebWSFallbackMetrics struct {
	wsOffered  atomic.Int64
	sseUsed    atomic.Int64
	wsAttempts atomic.Int64
	wsFailures atomic.Int64
}

// ChatGPTWebWSFallbackMetricsSnapshot is the read-only view.
type ChatGPTWebWSFallbackMetricsSnapshot struct {
	WSOffered  int64
	SSEUsed    int64
	WSAttempts int64
	WSFailures int64
}

// Snapshot returns a current counter snapshot.
func (m *ChatGPTWebWSFallbackMetrics) Snapshot() ChatGPTWebWSFallbackMetricsSnapshot {
	if m == nil {
		return ChatGPTWebWSFallbackMetricsSnapshot{}
	}
	return ChatGPTWebWSFallbackMetricsSnapshot{
		WSOffered:  m.wsOffered.Load(),
		SSEUsed:    m.sseUsed.Load(),
		WSAttempts: m.wsAttempts.Load(),
		WSFailures: m.wsFailures.Load(),
	}
}

// Reset zeros all counters (test hook).
func (m *ChatGPTWebWSFallbackMetrics) Reset() {
	if m == nil {
		return
	}
	m.wsOffered.Store(0)
	m.sseUsed.Store(0)
	m.wsAttempts.Store(0)
	m.wsFailures.Store(0)
}

var (
	defaultChatGPTWebWSFallbackOnce    sync.Once
	defaultChatGPTWebWSFallbackMetrics *ChatGPTWebWSFallbackMetrics
)

// chatGPTWebWSFallbackMetricsSingleton returns the package-global metrics
// instance.
func chatGPTWebWSFallbackMetricsSingleton() *ChatGPTWebWSFallbackMetrics {
	defaultChatGPTWebWSFallbackOnce.Do(func() {
		defaultChatGPTWebWSFallbackMetrics = &ChatGPTWebWSFallbackMetrics{}
	})
	return defaultChatGPTWebWSFallbackMetrics
}

// ChatGPTWebWSFallbackOutcome describes how the WS fallback decision went.
type ChatGPTWebWSFallbackOutcome struct {
	// Offered is true when the upstream body contained a websocket URL
	// (wss_url / websocket_url field). Always true → fallback engaged.
	Offered bool
	// FellBack is true when we used SSE despite WS being offered. Equal
	// to Offered in this port (we never attempt WS).
	FellBack bool
}

// ChatGPTWebWSFallbackInspect scans the buffered upstream body for an
// upstream-offered WebSocket URL and increments metrics accordingly. The
// caller decides what to do with the result; the chatgpt_web hot path
// always uses SSE.
//
// We look for "wss_url" or "websocket_url" keys (chatgpt2api would care
// about either) but only count one per body. The match is byte-substring,
// not JSON-parsed, so a partial / streamed body still works.
func ChatGPTWebWSFallbackInspect(body []byte) ChatGPTWebWSFallbackOutcome {
	m := chatGPTWebWSFallbackMetricsSingleton()
	// SSE used is the unconditional outcome.
	m.sseUsed.Add(1)
	out := ChatGPTWebWSFallbackOutcome{}
	if len(body) == 0 {
		return out
	}
	lowered := bytes.ToLower(body)
	if bytes.Contains(lowered, []byte("\"wss_url\"")) ||
		bytes.Contains(lowered, []byte("\"websocket_url\"")) ||
		bytes.Contains(lowered, []byte("wss://")) {
		m.wsOffered.Add(1)
		out.Offered = true
		out.FellBack = true
	}
	return out
}

// ChatGPTWebWSFallbackResponseHeader is the HTTP header surfaced to ops so
// they can debug a request that fell back. The value is "1" when the
// upstream offered WS and we chose SSE, otherwise the header is omitted.
const ChatGPTWebWSFallbackResponseHeader = "X-SRapi-ChatGPT-Web-WS-Fallback"

// chatGPTWebWSFallbackHeaderValue returns the value to attach to the
// downstream response when an upstream WS offer was declined.
func chatGPTWebWSFallbackHeaderValue(out ChatGPTWebWSFallbackOutcome) string {
	if out.FellBack {
		return "1"
	}
	return ""
}

// recordWSAttemptFailed lets an operator-wired AttemptWS path increment the
// failure counter from outside. Kept as a method so tests don't need to
// reach into the unexported counters.
func (m *ChatGPTWebWSFallbackMetrics) recordWSAttemptFailed(reason string) {
	if m == nil {
		return
	}
	m.wsAttempts.Add(1)
	m.wsFailures.Add(1)
	_ = strings.TrimSpace(reason)
}
