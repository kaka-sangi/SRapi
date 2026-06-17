package signature

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func longSig() string {
	return strings.Repeat("x", MinValidThinkingSignatureLen)
}

func TestThinkingCachePutGet(t *testing.T) {
	cache := NewThinkingCache(time.Hour, 4, time.Minute, nil)
	defer cache.Stop()
	if !cache.Put("gpt-5.4", "hello", longSig()) {
		t.Fatal("Put should succeed")
	}
	if got := cache.Get("gpt-5.4", "hello"); got != longSig() {
		t.Fatalf("Get = %q, want longSig", got)
	}
}

func TestThinkingCacheRejectsShortSignature(t *testing.T) {
	cache := NewThinkingCache(time.Hour, 4, time.Minute, nil)
	defer cache.Stop()
	if cache.Put("gpt-5.4", "text", "short") {
		t.Fatal("Put with short signature must fail")
	}
}

func TestThinkingCacheGeminiBypassSentinel(t *testing.T) {
	cache := NewThinkingCache(time.Hour, 4, time.Minute, nil)
	defer cache.Stop()
	if got := cache.Get("gemini-2.5-pro", "missing"); got != GeminiBypassSentinel {
		t.Fatalf("Get(gemini, missing) = %q, want %q", got, GeminiBypassSentinel)
	}
	if got := cache.Get("gpt-5.4", "missing"); got != "" {
		t.Fatalf("Get(gpt, missing) = %q, want empty", got)
	}
}

func TestThinkingCacheTTLExpire(t *testing.T) {
	now := time.Now()
	clock := now
	cache := NewThinkingCache(time.Hour, 4, time.Minute, func() time.Time { return clock })
	defer cache.Stop()
	cache.Put("claude-sonnet", "abc", longSig())
	clock = now.Add(2 * time.Hour)
	if got := cache.Get("claude-sonnet", "abc"); got != "" {
		t.Fatalf("Get after TTL = %q, want empty", got)
	}
}

func TestThinkingCacheLRUEviction(t *testing.T) {
	cache := NewThinkingCache(time.Hour, 2, time.Minute, nil)
	defer cache.Stop()
	for i := 0; i < 5; i++ {
		cache.Put("gpt-5.4", fmt.Sprintf("text-%d", i), longSig())
	}
	// Only the last 2 should remain.
	hit := 0
	for i := 0; i < 5; i++ {
		if cache.Get("gpt-5.4", fmt.Sprintf("text-%d", i)) != "" {
			hit++
		}
	}
	if hit != 2 {
		t.Fatalf("hit = %d, want 2 (LRU bound)", hit)
	}
}

func TestHasValidSignature(t *testing.T) {
	if !HasValidSignature("gpt-5.4", longSig()) {
		t.Fatal("long signature should validate")
	}
	if HasValidSignature("gpt-5.4", "short") {
		t.Fatal("short signature should not validate")
	}
	if !HasValidSignature("gemini-2.5", GeminiBypassSentinel) {
		t.Fatal("gemini bypass sentinel should validate for gemini model")
	}
	if HasValidSignature("gpt-5.4", GeminiBypassSentinel) {
		t.Fatal("gemini bypass sentinel should NOT validate for non-gemini model")
	}
}

func TestThinkingModelGroup(t *testing.T) {
	cases := map[string]string{
		"gpt-5.4":        "gpt",
		"claude-sonnet":  "claude",
		"gemini-2.5-pro": "gemini",
		"mistral-large":  "mistral-large",
	}
	for model, want := range cases {
		if got := ThinkingModelGroup(model); got != want {
			t.Fatalf("ThinkingModelGroup(%q) = %q, want %q", model, got, want)
		}
	}
}
