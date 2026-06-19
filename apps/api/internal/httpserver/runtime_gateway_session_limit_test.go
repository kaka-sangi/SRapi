package httpserver

import (
	"context"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	modelservice "github.com/srapi/srapi/apps/api/internal/modules/models/service"
	modelmemory "github.com/srapi/srapi/apps/api/internal/modules/models/store/memory"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	providermemory "github.com/srapi/srapi/apps/api/internal/modules/providers/store/memory"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	schedulerservice "github.com/srapi/srapi/apps/api/internal/modules/scheduler/service"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
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
	got := rt.filterCandidatesBySessionLimit(ctx, candidates, newSession, nil)
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
	got2 := rt.filterCandidatesBySessionLimit(ctx, candidates, existing, nil)
	keep2 := false
	for _, c := range got2 {
		if c.Account.ID == 2 {
			keep2 = true
		}
	}
	if !keep2 {
		t.Fatalf("an existing conversation must keep its own account even at the cap")
	}

	// A previous_response_id binding is not tracked as an account session, but
	// the persisted owner still represents this conversation and must survive
	// max_sessions filtering.
	previousResponseSession := gatewayPreviousResponseSessionKey("resp_previous")
	got3 := rt.filterCandidatesBySessionLimit(ctx, candidates, previousResponseSession, nil)
	for _, c := range got3 {
		if c.Account.ID == 2 {
			t.Fatalf("unbound previous_response_id must not bypass max_sessions")
		}
	}
	boundAccountID := 2
	got4 := rt.filterCandidatesBySessionLimit(ctx, candidates, previousResponseSession, &boundAccountID)
	keepBound := false
	for _, c := range got4 {
		if c.Account.ID == 2 {
			keepBound = true
		}
	}
	if !keepBound {
		t.Fatalf("a persisted previous_response_id binding must keep its account even at the cap")
	}
}

func TestScheduleGatewayRequestKeepsPreviousResponseBoundAccountAtSessionCap(t *testing.T) {
	ctx := context.Background()
	rt, modelID, apiKey := newSessionLimitScheduleRuntime(t)
	sessionKey := gatewayPreviousResponseSessionKey("resp_previous")
	if err := rt.sessionAffinity.Bind(ctx, gatewaySessionScope(apiKey.ID), sessionKey, 2, time.Hour); err != nil {
		t.Fatalf("bind previous response session: %v", err)
	}
	// Account 2 is at max_sessions=1 with a different counted conversation.
	// The previous_response_id binding is not itself counted, but it still must
	// keep routing to account 2 because upstream owns that response id.
	if err := rt.sessionAffinity.AddAccountSession(ctx, 2, gatewayConversationSessionID("sid:other"), time.Hour); err != nil {
		t.Fatalf("seed account session: %v", err)
	}

	result, err := rt.scheduleGatewayRequest(ctx, schedulercontract.ScheduleRequest{
		RequestID:          "previous-response-session-cap",
		UserID:             1,
		APIKeyID:           apiKey.ID,
		SourceEndpoint:     "/v1/responses",
		Model:              "gpt-session-limit",
		SessionAffinityKey: sessionKey,
		StickyStrength:     schedulercontract.StickyStrengthSoft,
		Strategy:           schedulercontract.StrategyStickyFirst,
	}, modelID, "", apiKey)
	if err != nil {
		t.Fatalf("schedule gateway request: %v", err)
	}
	if result.Candidate.Account.ID != 2 {
		t.Fatalf("expected previous_response_id bound account 2, got %d", result.Candidate.Account.ID)
	}
	if !result.Decision.StickyHit {
		t.Fatalf("expected sticky hit for previous_response_id binding, got %+v", result.Decision)
	}
}

func newSessionLimitScheduleRuntime(t *testing.T) (*runtimeState, int, apikeycontract.APIKey) {
	t.Helper()
	ctx := context.Background()

	providerStore := providermemory.New()
	provider, err := providerStore.Create(ctx, providercontract.CreateStoredProvider{
		Name:        "openai-session-limit",
		DisplayName: "OpenAI Session Limit",
		AdapterType: "openai-compatible",
		Protocol:    "openai-compatible",
		Status:      providercontract.StatusActive,
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	providersSvc, err := providerservice.New(providerStore, nil)
	if err != nil {
		t.Fatalf("create providers service: %v", err)
	}

	modelStore := modelmemory.New()
	model, err := modelStore.Create(ctx, modelcontract.CreateStoredModel{
		CanonicalName: "gpt-session-limit",
		DisplayName:   "GPT Session Limit",
		Status:        modelcontract.StatusActive,
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	if _, err := modelStore.CreateMapping(ctx, modelcontract.CreateStoredMapping{
		ModelID:           model.ID,
		ProviderID:        provider.ID,
		UpstreamModelName: "gpt-session-limit",
		Status:            modelcontract.StatusActive,
	}); err != nil {
		t.Fatalf("create model mapping: %v", err)
	}
	modelsSvc, err := modelservice.New(modelStore, nil)
	if err != nil {
		t.Fatalf("create models service: %v", err)
	}

	accountStore := accountmemory.New()
	for _, input := range []accountcontract.CreateStoredAccount{
		{
			ProviderID:           provider.ID,
			Name:                 "account-1",
			RuntimeClass:         accountcontract.RuntimeClassAPIKey,
			CredentialCiphertext: "ciphertext-1",
			Status:               accountcontract.StatusActive,
			Priority:             10,
			Weight:               1,
			CredentialVersion:    "test",
		},
		{
			ProviderID:           provider.ID,
			Name:                 "account-2",
			RuntimeClass:         accountcontract.RuntimeClassAPIKey,
			CredentialCiphertext: "ciphertext-2",
			Status:               accountcontract.StatusActive,
			Priority:             10,
			Weight:               1,
			CredentialVersion:    "test",
			Metadata:             map[string]any{"max_sessions": 1},
		},
	} {
		if _, err := accountStore.Create(ctx, input); err != nil {
			t.Fatalf("create account %q: %v", input.Name, err)
		}
	}
	accountsSvc, err := accountservice.New(accountStore, "session-limit-test-master-key-0001", nil)
	if err != nil {
		t.Fatalf("create accounts service: %v", err)
	}

	schedulerStore := schedulermemory.New()
	schedulerSvc, err := schedulerservice.New(schedulerStore, nil)
	if err != nil {
		t.Fatalf("create scheduler service: %v", err)
	}

	return &runtimeState{
		providers:       providersSvc,
		models:          modelsSvc,
		accounts:        accountsSvc,
		scheduler:       schedulerSvc,
		sessionAffinity: sessionaffinitymemory.New(),
		schedulerStore:  schedulerStore,
		providerStore:   providerStore,
		modelStore:      modelStore,
		accountStore:    accountStore,
	}, model.ID, apikeycontract.APIKey{ID: 42}
}
