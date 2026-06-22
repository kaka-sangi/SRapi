package httpserver

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// gatewayDedupTTL is the default lifetime of a cached completed response.
// Matches chatgpt2api's DEFAULT_CHAT_COMPLETION_CACHE.ttl_seconds in
// services/config.py:43 verbatim (60s). PR-2 originally landed this at 30s
// with a misattributed comment; the user directive is "完全按那三个项目来"
// so the value is now restored to the reference default. Operators who
// want a tighter window can override via the (future) config knob; until
// that lands, ship the reference value as-is.
const gatewayDedupTTL = 60 * time.Second

// gatewayDedupMaxEntries bounds both the completed-result cache and the in-flight
// map. chatgpt2api uses a single max_entries setting for the completed cache and
// no explicit cap for in-flight calls — we add a matching cap on in-flight to
// keep memory bounded under a pathological fan-in (optimization on top).
const gatewayDedupMaxEntries = 1024

// gatewayDedupCacheableKeys mirror chatgpt2api's CACHEABLE_TEXT_KEYS
// (services/protocol/chat_completion_cache.py). Keep verbatim so a request
// hashed by chatgpt2api and a request hashed by srapi for the same payload
// produce the same key — this enables cross-process audit replay.
var gatewayDedupCacheableKeys = []string{
	"frequency_penalty",
	"max_completion_tokens",
	"max_tokens",
	"metadata",
	"model",
	"presence_penalty",
	"reasoning_effort",
	"response_format",
	"seed",
	"stop",
	"temperature",
	"tool_choice",
	"tools",
	"top_p",
	"user",
}

// gatewayDedupResult is the cached/computed payload. We carry an opaque []byte
// so the dedup layer stays format-agnostic — the gateway hands in the JSON
// response body it would have written, and other awaiters get the same bytes.
type gatewayDedupResult struct {
	Body    []byte
	Headers map[string]string
}

// gatewayDedupEntry is a completed cache entry.
type gatewayDedupEntry struct {
	key       string
	result    gatewayDedupResult
	expiresAt time.Time
}

// gatewayDedupInflightCall is shared by an owner goroutine and its followers.
// done is closed when result/err is set; followers select on ctx.Done() vs done
// so a canceled follower returns immediately without leaking.
type gatewayDedupInflightCall struct {
	done   chan struct{}
	result gatewayDedupResult
	err    error
}

// gatewayCompletionDedup is the in-flight + completed-result deduplicator.
// Mirrors chatgpt2api's ChatCompletionCache.get_or_compute_response with
// idiomatic Go primitives (chan close vs threading.Condition) and a bounded
// LRU on top of the entries dict.
type gatewayCompletionDedup struct {
	ttl        time.Duration
	maxEntries int

	mu         sync.Mutex
	entries    map[string]*list.Element // key -> *list.Element (value: *gatewayDedupEntry)
	entryOrder *list.List               // LRU; front = most recent
	inflight   map[string]*gatewayDedupInflightCall
}

func newGatewayCompletionDedup(ttl time.Duration, maxEntries int) *gatewayCompletionDedup {
	if ttl <= 0 {
		ttl = gatewayDedupTTL
	}
	if maxEntries <= 0 {
		maxEntries = gatewayDedupMaxEntries
	}
	return &gatewayCompletionDedup{
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    make(map[string]*list.Element, maxEntries),
		entryOrder: list.New(),
		inflight:   make(map[string]*gatewayDedupInflightCall, maxEntries),
	}
}

// GatewayDedupKey computes a stable hash for a non-streaming chat-completion
// request body. Mirrors chatgpt2api's cache_key:
//
//	sha256(json.dumps(_json_safe(canonical_body(body, messages, stream)),
//	       sort_keys=True, separators=",:"))
//
// stream=true is reflected in the key so a stream-vs-buffered request never
// collides. body is the parsed JSON request body (as a map). When body lacks a
// "messages" array, messages must be passed explicitly (mirrors chatgpt2api's
// API where messages are extracted prior to hashing).
func GatewayDedupKey(body map[string]any, messages []any, stream bool) string {
	canon := gatewayDedupCanonicalBody(body, messages, stream)
	enc, err := json.Marshal(canon)
	if err != nil {
		// json.Marshal can only fail on unsupported types — fall back to a
		// best-effort key derived from the format error so identical errors
		// still dedup. Realistic payloads (JSON-source maps) won't hit this.
		enc = []byte("__dedup_marshal_error__:" + err.Error())
	}
	sum := sha256.Sum256(enc)
	return hex.EncodeToString(sum[:])
}

// gatewayDedupCanonicalBody mirrors chatgpt2api's canonical_body: only
// cacheable text keys + messages + stream. Sort key order is enforced by the
// stable encoder used downstream.
func gatewayDedupCanonicalBody(body map[string]any, messages []any, stream bool) map[string]any {
	canon := make(map[string]any, len(gatewayDedupCacheableKeys)+2)
	for _, key := range gatewayDedupCacheableKeys {
		if v, ok := body[key]; ok {
			canon[key] = gatewayDedupJSONSafe(v)
		}
	}
	canon["messages"] = gatewayDedupJSONSafe(messages)
	canon["stream"] = stream
	return canon
}

// gatewayDedupJSONSafe matches chatgpt2api's _json_safe: bytes become a sha256
// hash + length record so binary payloads have a stable string form. Maps are
// recursively rewritten with string keys; lists/tuples are normalized.
func gatewayDedupJSONSafe(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		sum := sha256.Sum256(x)
		return map[string]any{
			"__bytes_sha256__": hex.EncodeToString(sum[:]),
			"length":           len(x),
		}
	case map[string]any:
		// Walk in sorted key order so the marshaled bytes are deterministic.
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(x))
		for _, k := range keys {
			out[k] = gatewayDedupJSONSafe(x[k])
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = gatewayDedupJSONSafe(item)
		}
		return out
	default:
		return x
	}
}

// GetOrCompute returns a cached/in-flight result if one exists; otherwise it
// invokes compute() once and shares the result with all parallel callers.
// stream=true must skip dedup (chatgpt2api gates streaming through a separate
// path); the caller is responsible for that branch — this method assumes
// non-streaming.
func (d *gatewayCompletionDedup) GetOrCompute(ctx context.Context, key string, compute func() (gatewayDedupResult, error)) (gatewayDedupResult, error) {
	if d == nil {
		return compute()
	}
	d.mu.Lock()
	d.evictExpiredLocked(time.Now())
	if elem, ok := d.entries[key]; ok {
		entry := elem.Value.(*gatewayDedupEntry)
		if entry.expiresAt.After(time.Now()) {
			d.entryOrder.MoveToFront(elem)
			result := entry.result
			d.mu.Unlock()
			return result, nil
		}
		// Expired entry — drop and fall through to in-flight check.
		d.entryOrder.Remove(elem)
		delete(d.entries, key)
	}
	if call, ok := d.inflight[key]; ok {
		d.mu.Unlock()
		select {
		case <-ctx.Done():
			return gatewayDedupResult{}, ctx.Err()
		case <-call.done:
			return call.result, call.err
		}
	}
	call := &gatewayDedupInflightCall{done: make(chan struct{})}
	d.inflight[key] = call
	d.mu.Unlock()

	result, err := compute()

	d.mu.Lock()
	// Always release the in-flight slot, even on error.
	delete(d.inflight, key)
	if err == nil {
		entry := &gatewayDedupEntry{key: key, result: result, expiresAt: time.Now().Add(d.ttl)}
		elem := d.entryOrder.PushFront(entry)
		d.entries[key] = elem
		d.enforceCapLocked()
	}
	d.mu.Unlock()

	// Publish to followers — set fields BEFORE closing the channel so a follower
	// that selects on done sees a fully-populated call (Go's happens-before on a
	// channel close covers this).
	call.result = result
	call.err = err
	close(call.done)

	return result, err
}

// evictExpiredLocked drops expired entries. Cheap because LRU iteration stops
// at the first non-expired entry from the back (oldest).
func (d *gatewayCompletionDedup) evictExpiredLocked(now time.Time) {
	for {
		back := d.entryOrder.Back()
		if back == nil {
			return
		}
		entry := back.Value.(*gatewayDedupEntry)
		if entry.expiresAt.After(now) {
			return
		}
		d.entryOrder.Remove(back)
		delete(d.entries, entry.key)
	}
}

// enforceCapLocked evicts oldest entries when the LRU is over the cap.
func (d *gatewayCompletionDedup) enforceCapLocked() {
	for d.entryOrder.Len() > d.maxEntries {
		back := d.entryOrder.Back()
		if back == nil {
			return
		}
		entry := back.Value.(*gatewayDedupEntry)
		d.entryOrder.Remove(back)
		delete(d.entries, entry.key)
	}
}

// Clear empties both the completed cache and the in-flight map. Intended for
// tests and graceful shutdown. In-flight callers receive ctx.Err()/their own
// completion via the existing done channel — Clear does NOT cancel them.
func (d *gatewayCompletionDedup) Clear() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries = make(map[string]*list.Element, d.maxEntries)
	d.entryOrder = list.New()
	d.inflight = make(map[string]*gatewayDedupInflightCall, d.maxEntries)
}

// ShouldDedupChatCompletion reports whether a chat-completion request payload
// is eligible for dedup. Streaming requests are NOT eligible (chatgpt2api's
// stream cache is a separate path and SSE responses have a fundamentally
// different shape). Empty messages disqualify too — there's nothing to hash.
func ShouldDedupChatCompletion(body map[string]any) bool {
	if body == nil {
		return false
	}
	if stream, ok := body["stream"].(bool); ok && stream {
		return false
	}
	switch m := body["messages"].(type) {
	case []any:
		return len(m) > 0
	default:
		return false
	}
}

