package service

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"
)

func validCodexReasoningReplayEncryptedContentForTest(seed byte) string {
	payload := make([]byte, 1+8+16+16+32)
	payload[0] = 0x80
	for i := 9; i < len(payload); i++ {
		payload[i] = seed + byte(i)
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func validCodexReasoningReplayItemForTest(seed byte) []byte {
	return []byte(`{"type":"reasoning","summary":[],"content":null,"encrypted_content":"` + validCodexReasoningReplayEncryptedContentForTest(seed) + `"}`)
}

func TestCodexReasoningReplayCacheRejectsInvalidItems(t *testing.T) {
	cache := NewCodexReasoningReplayCache(0, 0, 0, nil)
	if cache.PutItem("gpt-5.4", "session", []byte(`{"type":"reasoning","encrypted_content":"bad","summary":[]}`)) {
		t.Fatal("invalid encrypted_content should not be cached")
	}
	if _, ok := cache.GetItem("gpt-5.4", "session"); ok {
		t.Fatal("invalid item was cached")
	}
}

func TestCodexReasoningReplayCacheScopesByModelAndSession(t *testing.T) {
	cache := NewCodexReasoningReplayCache(0, 0, 0, nil)
	encryptedContent := validCodexReasoningReplayEncryptedContentForTest(7)
	if !cache.PutItem("gpt-5.4", "session-a", []byte(`{"type":"reasoning","summary":[],"content":null,"encrypted_content":"`+encryptedContent+`"}`)) {
		t.Fatal("valid item was not cached")
	}
	if _, ok := cache.GetItem("gpt-5.5", "session-a"); ok {
		t.Fatal("cache should not hit across models")
	}
	if _, ok := cache.GetItem("gpt-5.4", "session-b"); ok {
		t.Fatal("cache should not hit across sessions")
	}
	item, ok := cache.GetItem("gpt-5.4", "session-a")
	if !ok {
		t.Fatal("cache miss for original model and session")
	}
	want := `{"type":"reasoning","summary":[],"content":null,"encrypted_content":"` + encryptedContent + `"}`
	if string(item) != want {
		t.Fatalf("normalized item = %s\nwant: %s", string(item), want)
	}
}

func TestCodexReasoningReplayCacheBatchEvictsWhenFull(t *testing.T) {
	cache := NewCodexReasoningReplayCache(16, 4, time.Hour, nil)
	item := validCodexReasoningReplayItemForTest(9)
	for i := 0; i < 32; i++ {
		if !cache.PutItem("gpt-5.4", fmt.Sprintf("session-%d", i), item) {
			t.Fatalf("cache insert %d failed", i)
		}
	}
	if cache.Len() > 16 {
		t.Fatalf("cache entries = %d, want <= 16", cache.Len())
	}
}

func TestCodexReasoningReplayCacheSlidingExpire(t *testing.T) {
	start := time.Now()
	clock := start
	cache := NewCodexReasoningReplayCache(16, 4, time.Hour, func() time.Time { return clock })
	cache.PutItem("gpt-5.4", "sess", validCodexReasoningReplayItemForTest(1))
	clock = start.Add(30 * time.Minute)
	if _, ok := cache.GetItem("gpt-5.4", "sess"); !ok {
		t.Fatal("entry should still be present at 30 min")
	}
	clock = start.Add(90 * time.Minute) // 30 + 60 = below ttl since last touch
	if _, ok := cache.GetItem("gpt-5.4", "sess"); !ok {
		t.Fatal("sliding TTL should have kept entry")
	}
	clock = start.Add(180 * time.Minute) // jump > 1h past last touch
	if _, ok := cache.GetItem("gpt-5.4", "sess"); ok {
		t.Fatal("entry should have expired")
	}
}

func TestCodexReasoningReplayCacheEmptyScopes(t *testing.T) {
	cache := NewCodexReasoningReplayCache(0, 0, 0, nil)
	if cache.PutItem("", "sess", validCodexReasoningReplayItemForTest(2)) {
		t.Fatal("empty model should not be cached")
	}
	if cache.PutItem("gpt-5.4", "", validCodexReasoningReplayItemForTest(2)) {
		t.Fatal("empty session should not be cached")
	}
	if _, ok := cache.GetItem("", "sess"); ok {
		t.Fatal("empty model lookup should miss")
	}
}

func TestCodexReasoningReplayCacheDeleteAndClear(t *testing.T) {
	cache := NewCodexReasoningReplayCache(0, 0, 0, nil)
	cache.PutItem("gpt-5.4", "sess", validCodexReasoningReplayItemForTest(3))
	cache.Delete("gpt-5.4", "sess")
	if _, ok := cache.GetItem("gpt-5.4", "sess"); ok {
		t.Fatal("Delete did not remove entry")
	}
	cache.PutItem("gpt-5.4", "sess-a", validCodexReasoningReplayItemForTest(4))
	cache.PutItem("gpt-5.4", "sess-b", validCodexReasoningReplayItemForTest(5))
	cache.Clear()
	if cache.Len() != 0 {
		t.Fatalf("Clear left %d entries", cache.Len())
	}
}

func TestCodexReasoningReplayCacheFunctionCallNormalization(t *testing.T) {
	cache := NewCodexReasoningReplayCache(0, 0, 0, nil)
	raw := []byte(`{"type":"function_call","call_id":"call_1","name":"do_thing","arguments":"{\"x\":1}","other":"drop"}`)
	if !cache.PutItem("gpt-5.4", "sess", raw) {
		t.Fatal("valid function_call rejected")
	}
	item, ok := cache.GetItem("gpt-5.4", "sess")
	if !ok {
		t.Fatal("function_call cache miss")
	}
	want := `{"type":"function_call","call_id":"call_1","name":"do_thing","arguments":"{\"x\":1}"}`
	if string(item) != want {
		t.Fatalf("normalized function_call = %s\nwant: %s", string(item), want)
	}
}

func TestCodexReasoningReplayCachePurgeExpired(t *testing.T) {
	start := time.Now()
	clock := start
	cache := NewCodexReasoningReplayCache(0, 0, time.Hour, func() time.Time { return clock })
	cache.PutItem("gpt-5.4", "sess-a", validCodexReasoningReplayItemForTest(6))
	clock = start.Add(2 * time.Hour)
	cache.PurgeExpired(clock)
	if cache.Len() != 0 {
		t.Fatalf("PurgeExpired left %d entries", cache.Len())
	}
}
