// Ported from CLIProxyAPI internal/cache/signature_cache.go with one
// Go-idiomatic deviation called out below. Reference behaviour:
//   - Cache key: model-group bucket + sha256(text) truncated to 16 hex chars.
//   - TTL: 3h, sliding (touch refreshes Timestamp).
//   - Minimum signature length: 50.
//   - Group bucket "gemini" returns "skip_thought_signature_validator" when no
//     entry exists, because that is the documented bypass sentinel.
//
// OPTIMIZATION (intentional deviation, noted per the task prompt):
// CLIProxyAPI stores groups in a sync.Map. With high-cardinality model names
// the per-group bucket can grow unbounded and the goroutine that purges it
// only fires every 10 minutes. We replace the inner map with a bounded LRU
// (container/list) so a single group cannot leak memory under load. The
// outer sync.Map is preserved because group keys are low cardinality
// ("gpt", "claude", "gemini", model name).
package signature

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// ThinkingCacheTTL mirrors CLIProxyAPI SignatureCacheTTL.
	ThinkingCacheTTL = 3 * time.Hour
	// ThinkingTextHashLen mirrors CLIProxyAPI SignatureTextHashLen.
	ThinkingTextHashLen = 16
	// MinValidThinkingSignatureLen mirrors CLIProxyAPI MinValidSignatureLen.
	MinValidThinkingSignatureLen = 50
	// ThinkingCacheCleanupInterval mirrors CLIProxyAPI CacheCleanupInterval.
	ThinkingCacheCleanupInterval = 10 * time.Minute
	// ThinkingCacheMaxEntriesPerGroup bounds a single group bucket. New entry
	// here vs CLIProxyAPI; see file header for rationale.
	ThinkingCacheMaxEntriesPerGroup = 8192
	// GeminiBypassSentinel mirrors the documented Gemini thought-signature
	// validator skip sentinel used by CLIProxyAPI when there is no cached
	// signature.
	GeminiBypassSentinel = "skip_thought_signature_validator"
)

type thinkingEntry struct {
	signature string
	timestamp time.Time
	hashKey   string
}

type thinkingGroupCache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	max     int
}

func newThinkingGroupCache(max int) *thinkingGroupCache {
	if max <= 0 {
		max = ThinkingCacheMaxEntriesPerGroup
	}
	return &thinkingGroupCache{
		entries: make(map[string]*list.Element),
		order:   list.New(),
		max:     max,
	}
}

// ThinkingCache is a bounded LRU+TTL cache for thinking-block signatures.
// The package-level instance backs the global helpers but the type is
// exported so tests and alternative wirings can construct isolated caches.
type ThinkingCache struct {
	groups               sync.Map // map[string]*thinkingGroupCache
	cleanupOnce          sync.Once
	cleanupStop          chan struct{}
	maxEntriesPerGroup   int
	cleanupInterval      time.Duration
	ttl                  time.Duration
	now                  func() time.Time
	signatureBypassMode  atomic.Bool
	signatureCacheActive atomic.Bool
}

// NewThinkingCache builds a fresh cache. nil now uses time.Now.
func NewThinkingCache(ttl time.Duration, maxEntriesPerGroup int, cleanupInterval time.Duration, now func() time.Time) *ThinkingCache {
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = ThinkingCacheTTL
	}
	if cleanupInterval <= 0 {
		cleanupInterval = ThinkingCacheCleanupInterval
	}
	c := &ThinkingCache{
		maxEntriesPerGroup: maxEntriesPerGroup,
		cleanupInterval:    cleanupInterval,
		ttl:                ttl,
		now:                now,
		cleanupStop:        make(chan struct{}),
	}
	c.signatureCacheActive.Store(true)
	return c
}

// HashThinkingText is the stable, Unicode-safe key used for the inner LRU.
// Matches CLIProxyAPI hashText.
func HashThinkingText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])[:ThinkingTextHashLen]
}

// ThinkingModelGroup classifies a model name into the bucket used by the
// cache. Matches CLIProxyAPI GetModelGroup.
func ThinkingModelGroup(modelName string) string {
	if strings.Contains(modelName, "gpt") {
		return "gpt"
	}
	if strings.Contains(modelName, "claude") {
		return "claude"
	}
	if strings.Contains(modelName, "gemini") {
		return "gemini"
	}
	return modelName
}

func (c *ThinkingCache) getOrCreateGroup(group string) *thinkingGroupCache {
	c.cleanupOnce.Do(c.startCleanup)
	if val, ok := c.groups.Load(group); ok {
		return val.(*thinkingGroupCache)
	}
	bucket := newThinkingGroupCache(c.maxEntriesPerGroup)
	actual, _ := c.groups.LoadOrStore(group, bucket)
	return actual.(*thinkingGroupCache)
}

func (c *ThinkingCache) startCleanup() {
	go func() {
		ticker := time.NewTicker(c.cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-c.cleanupStop:
				return
			case <-ticker.C:
				c.purgeExpired()
			}
		}
	}()
}

// Stop terminates the background cleanup goroutine. Safe to call multiple
// times.
func (c *ThinkingCache) Stop() {
	select {
	case <-c.cleanupStop:
		return
	default:
	}
	close(c.cleanupStop)
}

func (c *ThinkingCache) purgeExpired() {
	now := c.now()
	c.groups.Range(func(key, value any) bool {
		bucket := value.(*thinkingGroupCache)
		bucket.mu.Lock()
		for hashKey, elem := range bucket.entries {
			entry := elem.Value.(*thinkingEntry)
			if now.Sub(entry.timestamp) > c.ttl {
				bucket.order.Remove(elem)
				delete(bucket.entries, hashKey)
			}
		}
		empty := len(bucket.entries) == 0
		bucket.mu.Unlock()
		if empty {
			c.groups.Delete(key)
		}
		return true
	})
}

// Put records a signature for (modelName, text). Returns false when the
// inputs are invalid (empty text or signature, signature shorter than the
// minimum).
func (c *ThinkingCache) Put(modelName, text, signature string) bool {
	if text == "" || signature == "" {
		return false
	}
	if len(signature) < MinValidThinkingSignatureLen {
		return false
	}
	group := ThinkingModelGroup(modelName)
	bucket := c.getOrCreateGroup(group)
	hashKey := HashThinkingText(text)
	now := c.now()
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	if elem, ok := bucket.entries[hashKey]; ok {
		entry := elem.Value.(*thinkingEntry)
		entry.signature = signature
		entry.timestamp = now
		bucket.order.MoveToFront(elem)
		return true
	}
	entry := &thinkingEntry{signature: signature, timestamp: now, hashKey: hashKey}
	elem := bucket.order.PushFront(entry)
	bucket.entries[hashKey] = elem
	if bucket.order.Len() > bucket.max {
		oldest := bucket.order.Back()
		if oldest != nil {
			bucket.order.Remove(oldest)
			delete(bucket.entries, oldest.Value.(*thinkingEntry).hashKey)
		}
	}
	return true
}

// Get returns the cached signature, the documented Gemini bypass sentinel,
// or the empty string. Mirrors CLIProxyAPI GetCachedSignature.
func (c *ThinkingCache) Get(modelName, text string) string {
	group := ThinkingModelGroup(modelName)
	if text == "" {
		if group == "gemini" {
			return GeminiBypassSentinel
		}
		return ""
	}
	val, ok := c.groups.Load(group)
	if !ok {
		if group == "gemini" {
			return GeminiBypassSentinel
		}
		return ""
	}
	bucket := val.(*thinkingGroupCache)
	hashKey := HashThinkingText(text)
	now := c.now()
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	elem, exists := bucket.entries[hashKey]
	if !exists {
		if group == "gemini" {
			return GeminiBypassSentinel
		}
		return ""
	}
	entry := elem.Value.(*thinkingEntry)
	if now.Sub(entry.timestamp) > c.ttl {
		bucket.order.Remove(elem)
		delete(bucket.entries, hashKey)
		if group == "gemini" {
			return GeminiBypassSentinel
		}
		return ""
	}
	entry.timestamp = now
	bucket.order.MoveToFront(elem)
	return entry.signature
}

// Delete removes a single cached signature.
func (c *ThinkingCache) Delete(modelName, text string) {
	if text == "" {
		return
	}
	group := ThinkingModelGroup(modelName)
	val, ok := c.groups.Load(group)
	if !ok {
		return
	}
	bucket := val.(*thinkingGroupCache)
	hashKey := HashThinkingText(text)
	bucket.mu.Lock()
	if elem, exists := bucket.entries[hashKey]; exists {
		bucket.order.Remove(elem)
		delete(bucket.entries, hashKey)
	}
	empty := len(bucket.entries) == 0
	bucket.mu.Unlock()
	if empty {
		c.groups.Delete(group)
	}
}

// Clear drops cache for one model group, or all when modelName is empty.
func (c *ThinkingCache) Clear(modelName string) {
	if modelName == "" {
		c.groups.Range(func(key, _ any) bool {
			c.groups.Delete(key)
			return true
		})
		return
	}
	c.groups.Delete(ThinkingModelGroup(modelName))
}

// HasValidSignature mirrors CLIProxyAPI HasValidSignature.
func HasValidSignature(modelName, signature string) bool {
	if signature == GeminiBypassSentinel && ThinkingModelGroup(modelName) == "gemini" {
		return true
	}
	return signature != "" && len(signature) >= MinValidThinkingSignatureLen
}

// SetEnabled toggles signature cache use; mirrors SetSignatureCacheEnabled.
func (c *ThinkingCache) SetEnabled(enabled bool) {
	c.signatureCacheActive.Store(enabled)
}

// Enabled mirrors SignatureCacheEnabled.
func (c *ThinkingCache) Enabled() bool {
	return c.signatureCacheActive.Load()
}

// SetBypassStrictMode mirrors SetSignatureBypassStrictMode.
func (c *ThinkingCache) SetBypassStrictMode(strict bool) {
	c.signatureBypassMode.Store(strict)
}

// BypassStrictMode mirrors SignatureBypassStrictMode.
func (c *ThinkingCache) BypassStrictMode() bool {
	return c.signatureBypassMode.Load()
}

// DefaultThinkingCache is the package-global instance for callers that do not
// need a dedicated cache. Mirrors the package-level CLIProxyAPI behaviour.
var DefaultThinkingCache = NewThinkingCache(ThinkingCacheTTL, ThinkingCacheMaxEntriesPerGroup, ThinkingCacheCleanupInterval, nil)
