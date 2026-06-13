package localcache

import (
	"testing"
	"time"
)

func TestGetSet(t *testing.T) {
	c := New[string](Config{MaxEntries: 10, DefaultTTL: time.Minute, SweepInterval: time.Hour})
	defer c.Close()

	if _, ok := c.Get("k"); ok {
		t.Fatal("expected miss on empty cache")
	}

	c.Set("k", "v")
	got, ok := c.Get("k")
	if !ok || got != "v" {
		t.Fatalf("expected hit k=v, got ok=%v got=%q", ok, got)
	}

	stats := c.Stats()
	if stats.Hits != 1 || stats.Misses != 1 || stats.Size != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestTTLExpiry(t *testing.T) {
	c := New[int](Config{MaxEntries: 10, DefaultTTL: 5 * time.Millisecond, SweepInterval: time.Hour})
	defer c.Close()

	c.Set("x", 42)
	time.Sleep(10 * time.Millisecond)
	if _, ok := c.Get("x"); ok {
		t.Fatal("expected miss after TTL")
	}
}

func TestGetOrSet(t *testing.T) {
	c := New[string](Config{MaxEntries: 10, DefaultTTL: time.Minute, SweepInterval: time.Hour})
	defer c.Close()

	calls := 0
	fn := func() string { calls++; return "computed" }

	v1 := c.GetOrSet("key", fn)
	if v1 != "computed" || calls != 1 {
		t.Fatalf("first call: got %q calls=%d", v1, calls)
	}
	v2 := c.GetOrSet("key", fn)
	if v2 != "computed" || calls != 1 {
		t.Fatalf("second call should hit cache: got %q calls=%d", v2, calls)
	}
}

func TestClear(t *testing.T) {
	c := New[int](Config{MaxEntries: 100, DefaultTTL: time.Minute, SweepInterval: time.Hour})
	defer c.Close()

	for i := range 50 {
		c.Set(string(rune('A'+i)), i)
	}
	if c.Len() != 50 {
		t.Fatalf("expected 50 entries, got %d", c.Len())
	}

	c.Clear()
	if c.Len() != 0 {
		t.Fatalf("expected 0 entries after Clear, got %d", c.Len())
	}
	if _, ok := c.Get("A"); ok {
		t.Fatal("expected miss after Clear")
	}
}

func TestInvalidatePrefix(t *testing.T) {
	c := New[string](Config{MaxEntries: 100, DefaultTTL: time.Minute, SweepInterval: time.Hour})
	defer c.Close()

	c.Set("model:gpt-4", "a")
	c.Set("model:claude", "b")
	c.Set("user:alice", "c")
	c.Set("model:gemini", "d")

	removed := c.InvalidatePrefix("model:")
	if removed != 3 {
		t.Fatalf("expected 3 removed, got %d", removed)
	}
	if c.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", c.Len())
	}
	if _, ok := c.Get("user:alice"); !ok {
		t.Fatal("user:alice should survive prefix invalidation")
	}
}

func TestEviction(t *testing.T) {
	// MaxEntries is per-shard (16 shards), so set to 1 and insert enough
	// keys that at least one shard gets two entries and triggers eviction.
	c := New[int](Config{MaxEntries: 1, DefaultTTL: time.Minute, SweepInterval: time.Hour})
	defer c.Close()

	for i := range 100 {
		c.Set(string(rune(i+33)), i)
	}
	// With MaxEntries=1 per shard and 16 shards, max is 16 entries.
	if c.Len() > numShards {
		t.Fatalf("expected at most %d entries, got %d", numShards, c.Len())
	}
	if c.Stats().Evictions == 0 {
		t.Fatal("expected at least one eviction")
	}
}

func TestKeys(t *testing.T) {
	c := New[int](Config{MaxEntries: 10, DefaultTTL: time.Minute, SweepInterval: time.Hour})
	defer c.Close()

	c.Set("x", 1)
	c.Set("y", 2)
	c.Set("z", 3)

	keys := c.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	has := map[string]bool{}
	for _, k := range keys {
		has[k] = true
	}
	for _, want := range []string{"x", "y", "z"} {
		if !has[want] {
			t.Fatalf("missing key %q", want)
		}
	}
}

func TestForEach(t *testing.T) {
	c := New[int](Config{MaxEntries: 10, DefaultTTL: time.Minute, SweepInterval: time.Hour})
	defer c.Close()

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	sum := 0
	count := 0
	c.ForEach(func(_ string, v int) {
		sum += v
		count++
	})
	if count != 3 || sum != 6 {
		t.Fatalf("expected 3 entries summing to 6, got count=%d sum=%d", count, sum)
	}
}

func TestSetIfAbsent(t *testing.T) {
	c := New[string](Config{MaxEntries: 10, DefaultTTL: time.Minute, SweepInterval: time.Hour})
	defer c.Close()

	if !c.SetIfAbsent("k", "first") {
		t.Fatal("expected SetIfAbsent to return true on empty cache")
	}
	if c.SetIfAbsent("k", "second") {
		t.Fatal("expected SetIfAbsent to return false when key exists")
	}
	got, _ := c.Get("k")
	if got != "first" {
		t.Fatalf("expected 'first', got %q", got)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	c := New[int](Config{MaxEntries: 64, DefaultTTL: time.Minute, SweepInterval: 50 * time.Millisecond})
	defer c.Close()

	done := make(chan struct{})
	for g := range 8 {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for i := range 500 {
				key := string(rune('A'+id)) + string(rune('a'+i%26))
				c.Set(key, i)
				c.Get(key)
				if i%50 == 0 {
					c.InvalidatePrefix(string(rune('A' + id)))
				}
			}
		}(g)
	}
	for range 8 {
		<-done
	}
	// No panic or data race is the success criterion (run with -race).
	_ = c.Stats()
	_ = c.Len()
}

func TestDelete(t *testing.T) {
	c := New[string](Config{MaxEntries: 10, DefaultTTL: time.Minute, SweepInterval: time.Hour})
	defer c.Close()

	c.Set("k", "v")
	c.Delete("k")
	if _, ok := c.Get("k"); ok {
		t.Fatal("expected miss after Delete")
	}
	c.Delete("nonexistent") // should not panic
}
