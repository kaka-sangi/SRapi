package memory

import (
	"context"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/contract"
)

func TestBindLookupRoundTrip(t *testing.T) {
	store := New()
	ctx := context.Background()
	if err := store.Bind(ctx, "key1", "sid:pck:abc", 7, time.Hour); err != nil {
		t.Fatalf("bind: %v", err)
	}
	binding, err := store.Lookup(ctx, "key1", "sid:pck:abc", time.Hour)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !binding.Found() || binding.AccountID != 7 {
		t.Fatalf("expected account 7, got %+v", binding)
	}
}

func TestLookupIsScopedPerKey(t *testing.T) {
	store := New()
	ctx := context.Background()
	_ = store.Bind(ctx, "key1", "sid:pck:abc", 7, time.Hour)
	binding, _ := store.Lookup(ctx, "key2", "sid:pck:abc", time.Hour)
	if binding.Found() {
		t.Fatalf("expected no cross-scope binding, got %+v", binding)
	}
}

func TestLongestPrefixMatchForDigestChain(t *testing.T) {
	store := New()
	ctx := context.Background()
	// Turn 1 bound the chain s-u1. Turn 2 arrives with the extended chain.
	if err := store.Bind(ctx, "k", "dc:s-u1", 5, time.Hour); err != nil {
		t.Fatalf("bind: %v", err)
	}
	binding, err := store.Lookup(ctx, "k", "dc:s-u1-a1-u2", time.Hour)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !binding.Found() || binding.AccountID != 5 {
		t.Fatalf("expected longest-prefix hit on account 5, got %+v", binding)
	}
	if binding.MatchedKey != "dc:s-u1" {
		t.Fatalf("expected matched key dc:s-u1, got %q", binding.MatchedKey)
	}
}

func TestExactKeyDoesNotPrefixMatch(t *testing.T) {
	store := New()
	ctx := context.Background()
	// Non-chain keys (sid:) must match exactly, never by prefix, even though they
	// may contain separators.
	_ = store.Bind(ctx, "k", "sid:hdr:abc-def", 9, time.Hour)
	if binding, _ := store.Lookup(ctx, "k", "sid:hdr:abc-def-ghi", time.Hour); binding.Found() {
		t.Fatalf("expected no prefix match for sid keys, got %+v", binding)
	}
}

func TestLookupRefreshesTTLAndExpiry(t *testing.T) {
	store := New()
	now := time.Unix(1_000_000, 0).UTC()
	store.now = func() time.Time { return now }
	ctx := context.Background()
	_ = store.Bind(ctx, "k", "sid:pck:abc", 3, time.Minute)

	// Advance 30s and look up: should hit and refresh expiry to now+1m.
	now = now.Add(30 * time.Second)
	if binding, _ := store.Lookup(ctx, "k", "sid:pck:abc", time.Minute); !binding.Found() {
		t.Fatalf("expected hit before expiry")
	}
	// Advance another 45s (75s since bind, but only 45s since refresh): still alive.
	now = now.Add(45 * time.Second)
	if binding, _ := store.Lookup(ctx, "k", "sid:pck:abc", time.Minute); !binding.Found() {
		t.Fatalf("expected hit after TTL refresh kept it alive")
	}
	// Advance past the refreshed TTL: gone.
	now = now.Add(2 * time.Minute)
	if binding, _ := store.Lookup(ctx, "k", "sid:pck:abc", time.Minute); binding.Found() {
		t.Fatalf("expected expiry after TTL, got %+v", binding)
	}
}

func TestRelease(t *testing.T) {
	store := New()
	ctx := context.Background()
	_ = store.Bind(ctx, "k", "sid:pck:abc", 4, time.Hour)
	if err := store.Release(ctx, "k", "sid:pck:abc"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if binding, _ := store.Lookup(ctx, "k", "sid:pck:abc", time.Hour); binding.Found() {
		t.Fatalf("expected released binding gone, got %+v", binding)
	}
}

func TestBindRejectsInvalidInput(t *testing.T) {
	store := New()
	ctx := context.Background()
	if err := store.Bind(ctx, "k", "", 1, time.Hour); err != contract.ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for empty key, got %v", err)
	}
	if err := store.Bind(ctx, "k", "x", 0, time.Hour); err != contract.ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for non-positive account, got %v", err)
	}
}

func TestCandidateKeysOrdering(t *testing.T) {
	got := contract.CandidateKeys("dc:s-u1-a1-u2")
	want := []string{"dc:s-u1-a1-u2", "dc:s-u1-a1", "dc:s-u1", "dc:s"}
	if len(got) != len(want) {
		t.Fatalf("expected %d candidates, got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate %d: expected %q, got %q", i, want[i], got[i])
		}
	}
	if single := contract.CandidateKeys("sid:pck:abc-def"); len(single) != 1 || single[0] != "sid:pck:abc-def" {
		t.Fatalf("expected single exact candidate for sid key, got %v", single)
	}
}
