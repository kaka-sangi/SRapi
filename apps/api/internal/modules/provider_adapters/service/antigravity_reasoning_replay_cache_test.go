package service

import (
	"strings"
	"testing"
	"time"
)

func TestAntigravityReasoningReplayCachePutGet(t *testing.T) {
	c := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	item := []byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789","contentIndex":2,"partIndex":1}`)
	if !c.PutItems("gemini-3-pro", "session:abc", [][]byte{item}) {
		t.Fatalf("PutItems returned false")
	}
	got, ok := c.GetItems("gemini-3-pro", "session:abc")
	if !ok || len(got) != 1 {
		t.Fatalf("GetItems missed cache: ok=%v len=%d", ok, len(got))
	}
	if !strings.Contains(string(got[0]), `"thoughtSignature":"sig-abcdef0123456789"`) {
		t.Fatalf("expected signature preserved, got %s", got[0])
	}
	if !strings.Contains(string(got[0]), `"contentIndex":2`) || !strings.Contains(string(got[0]), `"partIndex":1`) {
		t.Fatalf("expected indices preserved, got %s", got[0])
	}
	if c.Len() != 1 {
		t.Fatalf("expected len 1, got %d", c.Len())
	}
}

func TestAntigravityReasoningReplayCacheRejectsShortSignature(t *testing.T) {
	c := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	if c.PutItems("gemini-3-pro", "session:abc", [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"too-short"}`)}) {
		t.Fatalf("expected PutItems to refuse signature below min length")
	}
	if c.PutItems("gemini-3-pro", "session:abc", [][]byte{[]byte(`{"type":"thought_signature"}`)}) {
		t.Fatalf("expected PutItems to refuse missing signature")
	}
	if c.Len() != 0 {
		t.Fatalf("expected empty cache, got %d", c.Len())
	}
}

func TestAntigravityReasoningReplayCacheUnknownTypeRejected(t *testing.T) {
	c := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	if c.PutItems("gemini-3-pro", "session:abc", [][]byte{[]byte(`{"type":"text","text":"hi"}`)}) {
		t.Fatalf("expected PutItems to reject unknown type")
	}
}

func TestAntigravityReasoningReplayCacheFunctionCallPartNormalization(t *testing.T) {
	c := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	// Reference shape: `functionCall` envelope with name/args/id nested.
	item := []byte(`{"type":"function_call_part","functionCall":{"id":"call-42","name":"do_thing","args":{"x":1}},"thoughtSignature":"sig-abcdef0123456789","contentIndex":3,"partIndex":0}`)
	if !c.PutItems("gemini-3-pro", "session:abc", [][]byte{item}) {
		t.Fatalf("PutItems returned false")
	}
	got, ok := c.GetItems("gemini-3-pro", "session:abc")
	if !ok || len(got) != 1 {
		t.Fatalf("expected 1 cached item, got ok=%v len=%d", ok, len(got))
	}
	out := string(got[0])
	for _, want := range []string{`"call_id":"call-42"`, `"name":"do_thing"`, `"args":{"x":1}`, `"thoughtSignature":"sig-abcdef0123456789"`, `"contentIndex":3`, `"partIndex":0`} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %s in %s", want, out)
		}
	}
}

func TestAntigravityReasoningReplayCacheSlidingTTL(t *testing.T) {
	clock := time.Now()
	now := func() time.Time { return clock }
	c := NewAntigravityReasoningReplayCache(0, 0, 10*time.Minute, now)
	item := []byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789","contentIndex":0,"partIndex":0}`)
	c.PutItems("gemini-3-pro", "session:abc", [][]byte{item})

	// Within TTL — sliding refresh.
	clock = clock.Add(9 * time.Minute)
	if _, ok := c.GetItems("gemini-3-pro", "session:abc"); !ok {
		t.Fatalf("expected hit at t+9m")
	}

	// 9 minutes later — would expire if the previous Get didn't refresh.
	clock = clock.Add(9 * time.Minute)
	if _, ok := c.GetItems("gemini-3-pro", "session:abc"); !ok {
		t.Fatalf("expected sliding TTL to keep entry alive at t+18m")
	}

	// Now sit past the TTL — should expire.
	clock = clock.Add(11 * time.Minute)
	if _, ok := c.GetItems("gemini-3-pro", "session:abc"); ok {
		t.Fatalf("expected expiry past TTL")
	}
	if c.Len() != 0 {
		t.Fatalf("expected expiry to evict, got %d", c.Len())
	}
}

func TestAntigravityReasoningReplayCacheEvictsOldest(t *testing.T) {
	c := NewAntigravityReasoningReplayCache(4, 2, 0, nil)
	mk := func(idx int) []byte {
		// 16-byte minimum signature satisfied with a fixed prefix + idx.
		return []byte(`{"type":"thought_signature","thoughtSignature":"abcdef0123456789-` + string(rune('a'+idx)) + `"}`)
	}
	for i := 0; i < 4; i++ {
		c.PutItems("gemini-3-pro", "session:"+string(rune('a'+i)), [][]byte{mk(i)})
	}
	if c.Len() != 4 {
		t.Fatalf("expected cache full at 4, got %d", c.Len())
	}
	// Insert one more → triggers evictBatch=2 of oldest.
	c.PutItems("gemini-3-pro", "session:e", [][]byte{mk(4)})
	if c.Len() != 3 {
		t.Fatalf("expected len 3 after eviction, got %d", c.Len())
	}
	if _, ok := c.GetItems("gemini-3-pro", "session:a"); ok {
		t.Fatalf("expected oldest session:a to be evicted")
	}
	if _, ok := c.GetItems("gemini-3-pro", "session:b"); ok {
		t.Fatalf("expected second-oldest session:b to be evicted")
	}
}

func TestAntigravityReasoningReplayCacheDelete(t *testing.T) {
	c := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	c.PutItems("gemini-3-pro", "session:abc", [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789"}`)})
	c.Delete("gemini-3-pro", "session:abc")
	if c.Len() != 0 {
		t.Fatalf("expected delete to clear entry, got %d", c.Len())
	}
}

func TestAntigravityReasoningReplayCacheClearAndPurge(t *testing.T) {
	clock := time.Now()
	c := NewAntigravityReasoningReplayCache(0, 0, time.Minute, func() time.Time { return clock })
	c.PutItems("gemini-3-pro", "session:a", [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789"}`)})
	c.PutItems("gemini-3-pro", "session:b", [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789"}`)})
	if c.Len() != 2 {
		t.Fatalf("expected 2, got %d", c.Len())
	}
	clock = clock.Add(2 * time.Minute)
	c.PurgeExpired(clock)
	if c.Len() != 0 {
		t.Fatalf("expected purge to drop expired, got %d", c.Len())
	}
	c.PutItems("gemini-3-pro", "session:c", [][]byte{[]byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789"}`)})
	c.Clear()
	if c.Len() != 0 {
		t.Fatalf("expected Clear to empty, got %d", c.Len())
	}
}

func TestAntigravityReasoningReplayCacheKeyNamespacing(t *testing.T) {
	c := NewAntigravityReasoningReplayCache(0, 0, 0, nil)
	item := []byte(`{"type":"thought_signature","thoughtSignature":"sig-abcdef0123456789"}`)
	c.PutItems("gemini-3-pro", "session:abc", [][]byte{item})
	if _, ok := c.GetItems("gemini-3-flash", "session:abc"); ok {
		t.Fatalf("expected model to namespace cache entries")
	}
	if _, ok := c.GetItems("gemini-3-pro", "session:xyz"); ok {
		t.Fatalf("expected session to namespace cache entries")
	}
	if _, ok := c.GetItems("", "session:abc"); ok {
		t.Fatalf("expected empty model to return no entry")
	}
	if c.PutItems("", "session:abc", [][]byte{item}) {
		t.Fatalf("expected empty model to refuse put")
	}
	if c.PutItems("gemini-3-pro", "", [][]byte{item}) {
		t.Fatalf("expected empty session to refuse put")
	}
}
