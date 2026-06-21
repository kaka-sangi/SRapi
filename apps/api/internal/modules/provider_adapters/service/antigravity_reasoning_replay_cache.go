// Ported from CLIProxyAPI internal/cache/antigravity_reasoning_replay_cache.go.
// The reference implementation stores the per-turn `thoughtSignature` blocks
// and `function_call_part` items the Antigravity (Gemini-shaped) upstream
// emits, keyed by (model_name, session_key). On the next request the cached
// items get spliced back into `request.contents` so the upstream accepts the
// continuation byte-identically. Without the cache, every turn re-generates
// fresh signatures, the upstream rejects them with `400 thoughtSignature
// invalid`, and `gemini_signature_retry.go` silently downgrades the thinking
// blocks to plain text — observable as quality regressions on multi-turn
// Antigravity-on-Gemini traffic.
//
// Deviations from the reference (called out per the port directive):
//
//  1. SRapi does not depend on tidwall/gjson + tidwall/sjson. We use
//     encoding/json + map[string]any. The on-the-wire bytes stay equivalent
//     for the two documented item shapes (`thought_signature`,
//     `function_call_part`).
//
//  2. CLIProxyAPI evicts via sort.Slice over the whole map. We reuse the
//     container/list LRU pattern from CodexReasoningReplayCache so overflow
//     eviction stays O(1) under load. Defaults match the reference verbatim
//     (10k entries, 1h TTL, 128-batch evict). Sliding TTL on Get preserved.
//
//  3. The KV-backed mode (homekv) is omitted — SRapi runs in-process; a
//     future shared-store backend can be plugged behind the same exported
//     surface without changing call sites. The CodexReasoningReplayCache
//     made the same call.
package service

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const (
	// AntigravityReasoningReplayCacheTTL limits how long replay items stay in
	// process memory. Matches CLIProxyAPI verbatim.
	AntigravityReasoningReplayCacheTTL = 1 * time.Hour
	// AntigravityReasoningReplayCacheMaxEntries bounds process memory for
	// replay continuity. Matches CLIProxyAPI verbatim.
	AntigravityReasoningReplayCacheMaxEntries = 10240
	// AntigravityReasoningReplayCacheEvictBatchSize leaves headroom after the
	// cache reaches capacity. Matches CLIProxyAPI verbatim.
	AntigravityReasoningReplayCacheEvictBatchSize = 128

	// minAntigravityThoughtSignatureReplayLen rejects clearly truncated
	// signatures. Matches CLIProxyAPI verbatim.
	minAntigravityThoughtSignatureReplayLen = 16
)

// AntigravityReasoningReplayCache is the bounded LRU+TTL cache.
type AntigravityReasoningReplayCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	max     int
	evict   int
	ttl     time.Duration
	now     func() time.Time
}

type antigravityReplayCacheValue struct {
	key       string
	items     [][]byte
	timestamp time.Time
}

// NewAntigravityReasoningReplayCache builds a cache with the reference
// defaults. A nil clock uses time.Now.
func NewAntigravityReasoningReplayCache(max int, evictBatch int, ttl time.Duration, now func() time.Time) *AntigravityReasoningReplayCache {
	if max <= 0 {
		max = AntigravityReasoningReplayCacheMaxEntries
	}
	if evictBatch <= 0 {
		evictBatch = AntigravityReasoningReplayCacheEvictBatchSize
	}
	if ttl <= 0 {
		ttl = AntigravityReasoningReplayCacheTTL
	}
	if now == nil {
		now = time.Now
	}
	return &AntigravityReasoningReplayCache{
		entries: make(map[string]*list.Element),
		order:   list.New(),
		max:     max,
		evict:   evictBatch,
		ttl:     ttl,
		now:     now,
	}
}

// DefaultAntigravityReasoningReplayCache is the package-global instance used
// by request handlers. Tests can swap it out by re-assigning the var.
var DefaultAntigravityReasoningReplayCache = NewAntigravityReasoningReplayCache(
	AntigravityReasoningReplayCacheMaxEntries,
	AntigravityReasoningReplayCacheEvictBatchSize,
	AntigravityReasoningReplayCacheTTL,
	nil,
)

// PutItems stores a batch of items after normalization. Returns true when at
// least one normalized item was cached.
func (c *AntigravityReasoningReplayCache) PutItems(modelName, sessionKey string, items [][]byte) bool {
	return c.PutItemsCtx(context.Background(), modelName, sessionKey, items)
}

// PutItemsCtx is the context-aware variant; the in-memory backend ignores
// ctx but the signature lines up with a future KV backend.
func (c *AntigravityReasoningReplayCache) PutItemsCtx(_ context.Context, modelName, sessionKey string, items [][]byte) bool {
	key := antigravityReasoningReplayCacheKey(modelName, sessionKey)
	if key == "" {
		return false
	}
	normalized, ok := normalizeAntigravityReasoningReplayItems(items)
	if !ok {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if elem, exists := c.entries[key]; exists {
		val := elem.Value.(*antigravityReplayCacheValue)
		val.items = normalized
		val.timestamp = now
		c.order.MoveToFront(elem)
		return true
	}
	val := &antigravityReplayCacheValue{key: key, items: normalized, timestamp: now}
	elem := c.order.PushFront(val)
	c.entries[key] = elem
	if c.order.Len() > c.max {
		c.evictBatchLocked(c.evict)
	}
	return true
}

// GetItems returns the cached batch and refreshes the timestamp (sliding TTL).
func (c *AntigravityReasoningReplayCache) GetItems(modelName, sessionKey string) ([][]byte, bool) {
	items, ok, err := c.GetItemsCtx(context.Background(), modelName, sessionKey)
	if err != nil {
		return nil, false
	}
	return items, ok
}

// GetItemsCtx is the context-aware variant; see PutItemsCtx.
func (c *AntigravityReasoningReplayCache) GetItemsCtx(_ context.Context, modelName, sessionKey string) ([][]byte, bool, error) {
	key := antigravityReasoningReplayCacheKey(modelName, sessionKey)
	if key == "" {
		return nil, false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		return nil, false, nil
	}
	val := elem.Value.(*antigravityReplayCacheValue)
	now := c.now()
	if now.Sub(val.timestamp) > c.ttl {
		c.order.Remove(elem)
		delete(c.entries, key)
		return nil, false, nil
	}
	val.timestamp = now
	c.order.MoveToFront(elem)
	return cloneAntigravityReasoningReplayItems(val.items), true, nil
}

// Delete removes one cached batch.
func (c *AntigravityReasoningReplayCache) Delete(modelName, sessionKey string) {
	key := antigravityReasoningReplayCacheKey(modelName, sessionKey)
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		c.order.Remove(elem)
		delete(c.entries, key)
	}
}

// Clear removes everything.
func (c *AntigravityReasoningReplayCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element)
	c.order = list.New()
}

// Len reports the current number of cached entries; useful in tests.
func (c *AntigravityReasoningReplayCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// PurgeExpired drops every entry whose timestamp is older than the TTL.
func (c *AntigravityReasoningReplayCache) PurgeExpired(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for elem := c.order.Back(); elem != nil; {
		val := elem.Value.(*antigravityReplayCacheValue)
		prev := elem.Prev()
		if now.Sub(val.timestamp) > c.ttl {
			c.order.Remove(elem)
			delete(c.entries, val.key)
		}
		elem = prev
	}
}

func (c *AntigravityReasoningReplayCache) evictBatchLocked(count int) {
	if count <= 0 {
		return
	}
	for i := 0; i < count; i++ {
		oldest := c.order.Back()
		if oldest == nil {
			return
		}
		val := oldest.Value.(*antigravityReplayCacheValue)
		c.order.Remove(oldest)
		delete(c.entries, val.key)
	}
}

func antigravityReasoningReplayCacheKey(modelName, sessionKey string) string {
	modelName = strings.TrimSpace(modelName)
	sessionKey = strings.TrimSpace(sessionKey)
	if modelName == "" || sessionKey == "" {
		return ""
	}
	// Session key is the continuity boundary. Independent from the selected
	// upstream credential so auth failover preserves replay. Matches
	// CLIProxyAPI verbatim.
	return strings.Join([]string{"antigravity-reasoning-replay", modelName, sessionKey}, "\x00")
}

func normalizeAntigravityReasoningReplayItems(items [][]byte) ([][]byte, bool) {
	normalized := make([][]byte, 0, len(items))
	for _, item := range items {
		normalizedItem, ok := normalizeAntigravityReasoningReplayItem(item)
		if ok {
			normalized = append(normalized, normalizedItem)
		}
	}
	return normalized, len(normalized) > 0
}

func normalizeAntigravityReasoningReplayItem(item []byte) ([]byte, bool) {
	var obj map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(item), &obj); err != nil {
		return nil, false
	}
	typeValue, _ := obj["type"].(string)
	switch strings.TrimSpace(typeValue) {
	case "thought_signature":
		return normalizeAntigravityThoughtSignatureReplayItem(obj)
	case "function_call_part":
		return normalizeAntigravityFunctionCallPartReplayItem(obj)
	default:
		return nil, false
	}
}

func normalizeAntigravityThoughtSignatureReplayItem(obj map[string]any) ([]byte, bool) {
	sig := strings.TrimSpace(stringValueOrEmpty(obj["thoughtSignature"]))
	if sig == "" {
		sig = strings.TrimSpace(stringValueOrEmpty(obj["thought_signature"]))
	}
	if sig == "" || len(sig) < minAntigravityThoughtSignatureReplayLen {
		return nil, false
	}
	out := map[string]any{
		"type":             "thought_signature",
		"thoughtSignature": sig,
	}
	if ci, ok := antigravityReadInt(obj["contentIndex"]); ok {
		out["contentIndex"] = ci
	}
	if pi, ok := antigravityReadInt(obj["partIndex"]); ok {
		out["partIndex"] = pi
	}
	encoded, err := encodePresentKeysInOrder(out, []string{"type", "thoughtSignature", "contentIndex", "partIndex"})
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func normalizeAntigravityFunctionCallPartReplayItem(obj map[string]any) ([]byte, bool) {
	callID := strings.TrimSpace(stringValueOrEmpty(obj["call_id"]))
	if callID == "" {
		callID = strings.TrimSpace(stringValueOrEmpty(obj["id"]))
	}
	name := strings.TrimSpace(stringValueOrEmpty(obj["name"]))
	args, hasArgs := obj["args"]
	if name == "" || !hasArgs {
		if fc, ok := obj["functionCall"].(map[string]any); ok {
			if callID == "" {
				callID = strings.TrimSpace(stringValueOrEmpty(fc["id"]))
			}
			if name == "" {
				name = strings.TrimSpace(stringValueOrEmpty(fc["name"]))
			}
			if !hasArgs {
				args, hasArgs = fc["args"]
			}
		}
	}
	if name == "" || !hasArgs {
		return nil, false
	}
	out := map[string]any{
		"type": "function_call_part",
		"name": name,
		"args": args,
	}
	if callID != "" {
		out["call_id"] = callID
	}
	if sig := strings.TrimSpace(stringValueOrEmpty(obj["thoughtSignature"])); sig != "" {
		out["thoughtSignature"] = sig
	}
	if ci, ok := antigravityReadInt(obj["contentIndex"]); ok {
		out["contentIndex"] = ci
	}
	if pi, ok := antigravityReadInt(obj["partIndex"]); ok {
		out["partIndex"] = pi
	}
	encoded, err := encodePresentKeysInOrder(out, []string{"type", "call_id", "name", "args", "thoughtSignature", "contentIndex", "partIndex"})
	if err != nil {
		return nil, false
	}
	return encoded, true
}

// encodePresentKeysInOrder emits a JSON object containing only the keys
// present in `values`, written in the order given. Missing keys are
// skipped so an omitted `contentIndex` does not encode as `null`.
func encodePresentKeysInOrder(values map[string]any, order []string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	for _, key := range order {
		value, exists := values[key]
		if !exists {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		keyBytes, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		valueBytes, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		buf.Write(valueBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func cloneAntigravityReasoningReplayItems(items [][]byte) [][]byte {
	cloned := make([][]byte, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, append([]byte(nil), item...))
	}
	return cloned
}

func antigravityReadInt(v any) (int, bool) {
	switch typed := v.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		if i, err := typed.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}
