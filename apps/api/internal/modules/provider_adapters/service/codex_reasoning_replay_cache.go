// Ported from CLIProxyAPI internal/cache/codex_reasoning_replay_cache.go.
// The reference implementation stores the final GPT/Codex assistant output
// items needed to replay a stateless next turn (reasoning, function_call,
// custom_tool_call) keyed by (model_name, session_key). The replay cache is
// the only way a stateless caller can carry encrypted_content forward without
// the upstream's hosted session, so behaviour parity matters here.
//
// Deviations from the reference (called out per the port directive):
//
//  1. CLIProxyAPI uses tidwall/gjson + tidwall/sjson for the per-item shape
//     normalization. srapi does not depend on tidwall; we use encoding/json
//     with map[string]any and re-marshal in the same order the reference
//     emits ("type" first, then optional fields). The on-the-wire bytes are
//     equivalent for the documented item shapes.
//
//  2. CLIProxyAPI evicts the oldest N entries via a sort.Slice over the whole
//     map every time it overflows (O(n log n) per overflow). We use a
//     container/list LRU so overflow eviction is O(1). The
//     CodexReasoningReplayCacheMaxEntries / CodexReasoningReplayCacheEvictBatchSize
//     constants are kept verbatim; we still evict a batch (not a single
//     entry) to leave headroom and avoid evicting on every insert under
//     load. Sliding TTL on Get is preserved verbatim.
//
//  3. The KV-backed mode in CLIProxyAPI is omitted from this port because
//     srapi does not expose homekv. The in-memory mode is the only path we
//     wire today; the cache exposes a typed Backend interface so a future
//     KV backend can be plugged without changing call sites.
package service

import (
	"bytes"
	"container/list"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	signaturepkg "github.com/srapi/srapi/apps/api/internal/pkg/signature"
)

const (
	// CodexReasoningReplayCacheTTL limits how long encrypted reasoning replay
	// items stay in process memory. Matches CLIProxyAPI verbatim.
	CodexReasoningReplayCacheTTL = 1 * time.Hour
	// CodexReasoningReplayCacheMaxEntries bounds process memory for replay
	// continuity. Oldest entries are evicted first. Matches CLIProxyAPI
	// verbatim.
	CodexReasoningReplayCacheMaxEntries = 10240
	// CodexReasoningReplayCacheEvictBatchSize leaves headroom after the cache
	// reaches capacity so high write volume does not rescan the map every
	// turn. Matches CLIProxyAPI verbatim.
	CodexReasoningReplayCacheEvictBatchSize = 128
)

// CodexReasoningReplayCache is the bounded LRU+TTL cache.
type CodexReasoningReplayCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	max     int
	evict   int
	ttl     time.Duration
	now     func() time.Time
}

type codexReplayCacheValue struct {
	key       string
	items     [][]byte
	timestamp time.Time
}

// NewCodexReasoningReplayCache builds a cache with the reference defaults. A
// nil clock uses time.Now.
func NewCodexReasoningReplayCache(max int, evictBatch int, ttl time.Duration, now func() time.Time) *CodexReasoningReplayCache {
	if max <= 0 {
		max = CodexReasoningReplayCacheMaxEntries
	}
	if evictBatch <= 0 {
		evictBatch = CodexReasoningReplayCacheEvictBatchSize
	}
	if ttl <= 0 {
		ttl = CodexReasoningReplayCacheTTL
	}
	if now == nil {
		now = time.Now
	}
	return &CodexReasoningReplayCache{
		entries: make(map[string]*list.Element),
		order:   list.New(),
		max:     max,
		evict:   evictBatch,
		ttl:     ttl,
		now:     now,
	}
}

// DefaultCodexReasoningReplayCache is the package-global instance used by
// request handlers. Reset/inspection is via the exported methods.
var DefaultCodexReasoningReplayCache = NewCodexReasoningReplayCache(
	CodexReasoningReplayCacheMaxEntries,
	CodexReasoningReplayCacheEvictBatchSize,
	CodexReasoningReplayCacheTTL,
	nil,
)

// PutItem stores a single normalized item. Returns true when the item passed
// shape normalization and was cached.
func (c *CodexReasoningReplayCache) PutItem(modelName, sessionKey string, item []byte) bool {
	return c.PutItems(modelName, sessionKey, [][]byte{item})
}

// PutItems stores a batch of items after normalization. Returns true when at
// least one normalized item was cached.
func (c *CodexReasoningReplayCache) PutItems(modelName, sessionKey string, items [][]byte) bool {
	return c.PutItemsCtx(context.Background(), modelName, sessionKey, items)
}

// PutItemsCtx is the request-time-aware variant. The context is currently
// unused for the in-memory backend; it exists for parity with the reference
// signature and forward-compat with a KV backend.
func (c *CodexReasoningReplayCache) PutItemsCtx(_ context.Context, modelName, sessionKey string, items [][]byte) bool {
	key := codexReasoningReplayCacheKey(modelName, sessionKey)
	if key == "" {
		return false
	}
	normalized, ok := normalizeCodexReasoningReplayItems(items)
	if !ok {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if elem, exists := c.entries[key]; exists {
		val := elem.Value.(*codexReplayCacheValue)
		val.items = normalized
		val.timestamp = now
		c.order.MoveToFront(elem)
		return true
	}
	val := &codexReplayCacheValue{key: key, items: normalized, timestamp: now}
	elem := c.order.PushFront(val)
	c.entries[key] = elem
	if c.order.Len() > c.max {
		c.evictBatchLocked(c.evict)
	}
	return true
}

// GetItem returns the first cached item. Mirrors GetCodexReasoningReplayItem.
func (c *CodexReasoningReplayCache) GetItem(modelName, sessionKey string) ([]byte, bool) {
	items, ok := c.GetItems(modelName, sessionKey)
	if !ok || len(items) == 0 {
		return nil, false
	}
	return items[0], true
}

// GetItems returns the cached batch and refreshes the timestamp (sliding
// TTL).
func (c *CodexReasoningReplayCache) GetItems(modelName, sessionKey string) ([][]byte, bool) {
	items, ok, err := c.GetItemsCtx(context.Background(), modelName, sessionKey)
	if err != nil {
		return nil, false
	}
	return items, ok
}

// GetItemsCtx is the request-time-aware variant; see PutItemsCtx.
func (c *CodexReasoningReplayCache) GetItemsCtx(_ context.Context, modelName, sessionKey string) ([][]byte, bool, error) {
	key := codexReasoningReplayCacheKey(modelName, sessionKey)
	if key == "" {
		return nil, false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[key]
	if !ok {
		return nil, false, nil
	}
	val := elem.Value.(*codexReplayCacheValue)
	now := c.now()
	if now.Sub(val.timestamp) > c.ttl {
		c.order.Remove(elem)
		delete(c.entries, key)
		return nil, false, nil
	}
	val.timestamp = now
	c.order.MoveToFront(elem)
	return cloneCodexReasoningReplayItems(val.items), true, nil
}

// Delete removes one cached batch.
func (c *CodexReasoningReplayCache) Delete(modelName, sessionKey string) {
	key := codexReasoningReplayCacheKey(modelName, sessionKey)
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
func (c *CodexReasoningReplayCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element)
	c.order = list.New()
}

// Len reports the current number of cached entries; useful in tests.
func (c *CodexReasoningReplayCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// PurgeExpired drops every entry whose timestamp is older than the TTL.
// Mirrors purgeExpiredCodexReasoningReplayCache.
func (c *CodexReasoningReplayCache) PurgeExpired(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for elem := c.order.Back(); elem != nil; {
		val := elem.Value.(*codexReplayCacheValue)
		prev := elem.Prev()
		if now.Sub(val.timestamp) > c.ttl {
			c.order.Remove(elem)
			delete(c.entries, val.key)
		}
		elem = prev
	}
}

func (c *CodexReasoningReplayCache) evictBatchLocked(count int) {
	if count <= 0 {
		return
	}
	for i := 0; i < count; i++ {
		oldest := c.order.Back()
		if oldest == nil {
			return
		}
		val := oldest.Value.(*codexReplayCacheValue)
		c.order.Remove(oldest)
		delete(c.entries, val.key)
	}
}

func codexReasoningReplayCacheKey(modelName, sessionKey string) string {
	modelName = strings.TrimSpace(modelName)
	sessionKey = strings.TrimSpace(sessionKey)
	if modelName == "" || sessionKey == "" {
		return ""
	}
	// The session key is the continuity boundary. Keep this independent from
	// the selected upstream Codex credential so auth failover can preserve
	// replay. Matches CLIProxyAPI verbatim.
	return strings.Join([]string{"codex-reasoning-replay", modelName, sessionKey}, "\x00")
}

func normalizeCodexReasoningReplayItems(items [][]byte) ([][]byte, bool) {
	normalized := make([][]byte, 0, len(items))
	for _, item := range items {
		normalizedItem, ok := normalizeCodexReasoningReplayItem(item)
		if ok {
			normalized = append(normalized, normalizedItem)
		}
	}
	return normalized, len(normalized) > 0
}

func normalizeCodexReasoningReplayItem(item []byte) ([]byte, bool) {
	var obj map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(item), &obj); err != nil {
		return nil, false
	}
	typeValue, _ := obj["type"].(string)
	switch strings.TrimSpace(typeValue) {
	case "reasoning":
		return normalizeCodexReasoningReplayReasoningItem(obj)
	case "function_call":
		return normalizeCodexReasoningReplayFunctionCallItem(obj)
	case "custom_tool_call":
		return normalizeCodexReasoningReplayCustomToolCallItem(obj)
	default:
		return nil, false
	}
}

func normalizeCodexReasoningReplayReasoningItem(obj map[string]any) ([]byte, bool) {
	encryptedValue, exists := obj["encrypted_content"]
	if !exists {
		return nil, false
	}
	encryptedContent, ok := encryptedValue.(string)
	if !ok {
		return nil, false
	}
	if encryptedContent != strings.TrimSpace(encryptedContent) {
		return nil, false
	}
	if _, err := signaturepkg.InspectGPTReasoningSignature(encryptedContent); err != nil {
		return nil, false
	}
	// Emit "type", "summary", "content", "encrypted_content" in that order to
	// match the reference output byte stream.
	out := []byte(`{"type":"reasoning","summary":[],"content":null,"encrypted_content":`)
	encoded, err := json.Marshal(encryptedContent)
	if err != nil {
		return nil, false
	}
	out = append(out, encoded...)
	out = append(out, '}')
	return out, true
}

func normalizeCodexReasoningReplayFunctionCallItem(obj map[string]any) ([]byte, bool) {
	callID := strings.TrimSpace(stringValueOrEmpty(obj["call_id"]))
	name := strings.TrimSpace(stringValueOrEmpty(obj["name"]))
	argumentsValue, exists := obj["arguments"]
	if !exists {
		return nil, false
	}
	arguments, ok := argumentsValue.(string)
	if !ok {
		return nil, false
	}
	if callID == "" || name == "" {
		return nil, false
	}
	envelope := map[string]string{
		"type":      "function_call",
		"call_id":   callID,
		"name":      name,
		"arguments": arguments,
	}
	encoded, err := encodeFixedKeyOrder(envelope, []string{"type", "call_id", "name", "arguments"})
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func normalizeCodexReasoningReplayCustomToolCallItem(obj map[string]any) ([]byte, bool) {
	callID := strings.TrimSpace(stringValueOrEmpty(obj["call_id"]))
	name := strings.TrimSpace(stringValueOrEmpty(obj["name"]))
	if callID == "" || name == "" {
		return nil, false
	}
	inputValue, exists := obj["input"]
	if !exists {
		return nil, false
	}
	status := strings.TrimSpace(stringValueOrEmpty(obj["status"]))
	if status == "" {
		status = "completed"
	}
	out := map[string]any{
		"type":    "custom_tool_call",
		"status":  status,
		"call_id": callID,
		"name":    name,
		"input":   inputValue,
	}
	// Custom tool input may be either a string OR a structured JSON object.
	// json.Marshal will preserve both. The reference emits keys in a stable
	// order; we reproduce that.
	encoded, err := encodeKeysInOrder(out, []string{"type", "status", "call_id", "name", "input"})
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func cloneCodexReasoningReplayItems(items [][]byte) [][]byte {
	cloned := make([][]byte, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, append([]byte(nil), item...))
	}
	return cloned
}

func stringValueOrEmpty(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func encodeFixedKeyOrder(values map[string]string, order []string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range order {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		valueBytes, err := json.Marshal(values[key])
		if err != nil {
			return nil, err
		}
		buf.Write(valueBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func encodeKeysInOrder(values map[string]any, order []string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range order {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		valueBytes, err := json.Marshal(values[key])
		if err != nil {
			return nil, err
		}
		buf.Write(valueBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
