package httpserver

import (
	"context"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	sessionaffinitymemory "github.com/srapi/srapi/apps/api/internal/modules/sessionaffinity/store/memory"
)

// TestGatewayConversationSessionIDStableAcrossTurns proves a growing digest
// chain still maps to one stable conversation id (so session counts are right).
func TestGatewayConversationSessionIDStableAcrossTurns(t *testing.T) {
	turn1 := gatewayConversationSessionID("dc:s:aaa-u:bbb")
	turn2 := gatewayConversationSessionID("dc:s:aaa-u:bbb-a:ccc-u:ddd")
	if turn1 == "" || turn1 != turn2 {
		t.Fatalf("expected stable conversation id across turns, got %q vs %q", turn1, turn2)
	}
	// A different conversation (different first turn) yields a different id.
	other := gatewayConversationSessionID("dc:s:aaa-u:zzz")
	if other == turn1 {
		t.Fatalf("expected distinct conversations to differ")
	}
	// Explicit session keys map directly and are stable.
	if a, b := gatewayConversationSessionID("sid:pck:x"), gatewayConversationSessionID("sid:pck:x"); a == "" || a != b {
		t.Fatalf("explicit session id should be stable, got %q vs %q", a, b)
	}
}

func TestFilterCandidatesBySessionLimit(t *testing.T) {
	store := sessionaffinitymemory.New()
	ctx := context.Background()
	// Account 2 is capped at 1 session and already serves one other conversation.
	_ = store.AddAccountSession(ctx, 2, gatewayConversationSessionID("dc:s:aaa-u:existing"), 0)
	rt := &runtimeState{sessionAffinity: store}

	candidates := []schedulercontract.Candidate{
		{Account: accountcontract.ProviderAccount{ID: 1}},                                              // no cap
		{Account: accountcontract.ProviderAccount{ID: 2, Metadata: map[string]any{"max_sessions": 1}}}, // capped, full
		{Account: accountcontract.ProviderAccount{ID: 3, Metadata: map[string]any{"max_sessions": 5}}}, // capped, room
	}
	newSession := "dc:s:aaa-u:brandnew"
	got := rt.filterCandidatesBySessionLimit(ctx, candidates, newSession)
	ids := map[int]bool{}
	for _, c := range got {
		ids[c.Account.ID] = true
	}
	if ids[2] {
		t.Fatalf("account 2 is at its session cap and should be dropped for a new conversation")
	}
	if !ids[1] || !ids[3] {
		t.Fatalf("uncapped and under-cap accounts must remain, got %v", ids)
	}

	// The conversation already on account 2 is NOT evicted from it.
	existing := "dc:s:aaa-u:existing"
	got2 := rt.filterCandidatesBySessionLimit(ctx, candidates, existing)
	keep2 := false
	for _, c := range got2 {
		if c.Account.ID == 2 {
			keep2 = true
		}
	}
	if !keep2 {
		t.Fatalf("an existing conversation must keep its own account even at the cap")
	}
}
