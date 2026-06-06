package httpserver

import (
	"context"
	"testing"

	sessionaffinitymemory "github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/store/memory"
)

// TestGatewaySessionAffinityBindLookupRoundTrip verifies the runtime read/
// write-back primitives that make a conversation sticky across turns: the first
// turn binds the session→account, the next turn resolves it.
func TestGatewaySessionAffinityBindLookupRoundTrip(t *testing.T) {
	rt := &runtimeState{sessionAffinity: sessionaffinitymemory.New()}
	ctx := context.Background()
	const apiKeyID, accountID = 7, 42
	key := "sid:pck:conv-1"

	if id, ok := rt.lookupGatewaySessionAffinity(ctx, apiKeyID, key); ok {
		t.Fatalf("expected no binding before write-back, got %d", id)
	}
	rt.bindGatewaySessionAffinity(ctx, apiKeyID, key, accountID)
	id, ok := rt.lookupGatewaySessionAffinity(ctx, apiKeyID, key)
	if !ok || id != accountID {
		t.Fatalf("expected bound account %d, got %d ok=%v", accountID, id, ok)
	}
	// Different API key must not see another key's session binding.
	if id, ok := rt.lookupGatewaySessionAffinity(ctx, apiKeyID+1, key); ok {
		t.Fatalf("expected scope isolation across api keys, got %d", id)
	}
}

// TestGatewaySessionAffinityChainLongestPrefix verifies a later turn's extended
// digest chain resolves back to the account bound on an earlier turn.
func TestGatewaySessionAffinityChainLongestPrefix(t *testing.T) {
	rt := &runtimeState{sessionAffinity: sessionaffinitymemory.New()}
	ctx := context.Background()
	rt.bindGatewaySessionAffinity(ctx, 1, "dc:s-u1", 99)
	id, ok := rt.lookupGatewaySessionAffinity(ctx, 1, "dc:s-u1-a1-u2")
	if !ok || id != 99 {
		t.Fatalf("expected longest-prefix bound account 99, got %d ok=%v", id, ok)
	}
}

// TestGatewaySessionAffinityNilStoreIsNoOp ensures stickiness degrades safely
// when no binding store is configured (bind is a no-op, lookup reports nothing).
func TestGatewaySessionAffinityNilStoreIsNoOp(t *testing.T) {
	rt := &runtimeState{}
	ctx := context.Background()
	rt.bindGatewaySessionAffinity(ctx, 1, "sid:x", 2) // must not panic
	if id, ok := rt.lookupGatewaySessionAffinity(ctx, 1, "sid:x"); ok {
		t.Fatalf("nil store must report no binding, got %d", id)
	}
}

// TestGatewaySessionAffinityIgnoresInvalidArgs guards the cheap rejects.
func TestGatewaySessionAffinityIgnoresInvalidArgs(t *testing.T) {
	rt := &runtimeState{sessionAffinity: sessionaffinitymemory.New()}
	ctx := context.Background()
	rt.bindGatewaySessionAffinity(ctx, 0, "sid:x", 2) // non-positive api key
	rt.bindGatewaySessionAffinity(ctx, 1, "", 2)      // empty key
	rt.bindGatewaySessionAffinity(ctx, 1, "sid:x", 0) // non-positive account
	if id, ok := rt.lookupGatewaySessionAffinity(ctx, 1, "sid:x"); ok {
		t.Fatalf("expected nothing bound from invalid args, got %d", id)
	}
}
