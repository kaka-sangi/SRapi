package entstore_test

import (
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	"github.com/srapi/srapi/apps/api/internal/persistence/entstore"

	_ "github.com/mattn/go-sqlite3"
)

func TestRuntimeStoresPersistRecords(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/runtime-stores.db?_fk=1")
	defer client.Close()

	stores, err := entstore.New(client)
	if err != nil {
		t.Fatalf("new ent stores: %v", err)
	}

	ctx := t.Context()
	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	providerID := 10
	accountID := 20
	errorClass := "rate_limit"

	usage, err := stores.Usage.Create(ctx, usagecontract.UsageLog{
		RequestID:             "req_runtime_store",
		UserID:                1,
		APIKeyID:              2,
		ProviderID:            &providerID,
		AccountID:             &accountID,
		SourceProtocol:        "openai-compatible",
		SourceEndpoint:        "/v1/chat/completions",
		TargetProtocol:        "openai-compatible",
		Model:                 "persist-model",
		InputTokens:           3,
		OutputTokens:          4,
		CachedTokens:          5,
		TotalTokens:           12,
		UsageEstimated:        true,
		LatencyMS:             123,
		Success:               false,
		ErrorClass:            &errorClass,
		Cost:                  "0.00000000",
		Currency:              "USD",
		CompatibilityWarnings: []string{"image_ignored"},
		CreatedAt:             now,
	})
	if err != nil {
		t.Fatalf("create usage: %v", err)
	}
	usageByUser, err := stores.Usage.ListByUser(ctx, 1)
	if err != nil {
		t.Fatalf("list usage by user: %v", err)
	}
	if len(usageByUser) != 1 || usageByUser[0].ID != usage.ID || usageByUser[0].ProviderID == nil || usageByUser[0].ErrorClass == nil {
		t.Fatalf("expected persisted usage with optional fields, got %+v", usageByUser)
	}

	decision, err := stores.Scheduler.CreateDecision(ctx, schedulercontract.Decision{
		RequestID:             "req_runtime_store",
		AttemptNo:             1,
		UserID:                1,
		APIKeyID:              2,
		SourceProtocol:        "openai-compatible",
		SourceEndpoint:        "/v1/chat/completions",
		TargetProtocol:        "openai-compatible",
		Model:                 "persist-model",
		Strategy:              schedulercontract.StrategyBalanced,
		StrategyVersion:       "v1",
		StrategyConfigHash:    "sha256:test",
		SelectedProviderID:    &providerID,
		SelectedAccountID:     &accountID,
		CandidateCount:        1,
		Scores:                map[string]any{"20": map[string]any{"final_score": 1}},
		RejectReasons:         map[string]any{},
		StrategyWeights:       map[string]any{"health": 0.3},
		CompatibilityWarnings: []string{"fallback"},
		EstimatedCost:         "0.00000000",
		Currency:              "USD",
		CreatedAt:             now,
	})
	if err != nil {
		t.Fatalf("create scheduler decision: %v", err)
	}
	decisions, err := stores.Scheduler.ListDecisions(ctx)
	if err != nil {
		t.Fatalf("list scheduler decisions: %v", err)
	}
	if len(decisions) != 1 || decisions[0].StrategyConfigHash != "sha256:test" || decisions[0].SelectedAccountID == nil {
		t.Fatalf("expected persisted scheduler decision, got %+v", decisions)
	}

	statusCode := 429
	if _, err := stores.Scheduler.CreateFeedback(ctx, schedulercontract.Feedback{
		RequestID:    "req_runtime_store",
		DecisionID:   decision.ID,
		AttemptNo:    1,
		AccountID:    accountID,
		ProviderID:   providerID,
		Model:        "persist-model",
		Success:      false,
		ErrorClass:   &errorClass,
		StatusCode:   &statusCode,
		LatencyMS:    123,
		InputTokens:  3,
		OutputTokens: 4,
		CachedTokens: 5,
		ActualCost:   "0.00000000",
		Currency:     "USD",
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("create scheduler feedback: %v", err)
	}
	feedbacks, err := stores.Scheduler.ListFeedbacks(ctx)
	if err != nil {
		t.Fatalf("list scheduler feedbacks: %v", err)
	}
	if len(feedbacks) != 1 || feedbacks[0].StatusCode == nil || *feedbacks[0].StatusCode != statusCode {
		t.Fatalf("expected persisted scheduler feedback, got %+v", feedbacks)
	}
	if _, created, err := stores.QualityEval.CreateSample(ctx, qualitycontract.Sample{
		FeedbackID:              feedbacks[0].ID,
		RequestID:               "req_runtime_store",
		DecisionID:              decision.ID,
		AttemptNo:               1,
		AccountID:               accountID,
		ProviderID:              providerID,
		Model:                   "persist-model",
		SourceEndpoint:          "/v1/chat/completions",
		SampleRequestHash:       "sha256:runtime",
		SamplePayloadCiphertext: "v1:nonce:ciphertext",
		PayloadVersion:          "v1",
		CapturedAt:              now,
	}); err != nil || !created {
		t.Fatalf("create quality eval sample: created=%v err=%v", created, err)
	}
	if _, created, err := stores.QualityEval.CreateEvaluation(ctx, qualitycontract.Evaluation{
		FeedbackID:        feedbacks[0].ID,
		RequestID:         "req_runtime_store",
		DecisionID:        decision.ID,
		AttemptNo:         1,
		AccountID:         accountID,
		ProviderID:        providerID,
		Model:             "persist-model",
		SourceEndpoint:    "/v1/chat/completions",
		SampleRequestHash: "sha256:runtime",
		JudgeModel:        "fake-judge",
		Score:             0.8,
		Rubric:            map[string]any{"correctness": 4},
		JudgedAt:          now,
	}); err != nil || !created {
		t.Fatalf("create quality evaluation: created=%v err=%v", created, err)
	}
	quality, err := stores.QualityEval.AggregateScore(ctx, accountID, "persist-model", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("aggregate quality score: %v", err)
	}
	if quality.SampleCount != 1 || quality.Score != 0.8 {
		t.Fatalf("expected persisted quality aggregate, got %+v", quality)
	}

	_, err = stores.Audit.Create(ctx, contract.Log{
		ActorUserID:  &providerID,
		Action:       "provider.create",
		ResourceType: "provider",
		ResourceID:   "10",
		Before:       map[string]any{},
		After:        map[string]any{"name": "persist-provider"},
		TraceID:      "req_runtime_store",
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("create audit: %v", err)
	}
	auditLogs, err := stores.Audit.List(ctx)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(auditLogs) != 1 || auditLogs[0].After["name"] != "persist-provider" {
		t.Fatalf("expected persisted audit log, got %+v", auditLogs)
	}

	_, err = stores.Billing.Create(ctx, billingcontract.LedgerEntry{
		UserID:        1,
		Type:          billingcontract.LedgerTypeUsageCharge,
		Amount:        "0.00000000",
		Currency:      "USD",
		BalanceBefore: "0.00000000",
		BalanceAfter:  "0.00000000",
		ReferenceType: "usage_log",
		ReferenceID:   "1",
		Metadata:      map[string]any{"request_id": "req_runtime_store"},
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create billing: %v", err)
	}
	ledger, err := stores.Billing.List(ctx)
	if err != nil {
		t.Fatalf("list billing: %v", err)
	}
	if len(ledger) != 1 || ledger[0].Metadata["request_id"] != "req_runtime_store" {
		t.Fatalf("expected persisted billing ledger, got %+v", ledger)
	}

	outbox, err := stores.Events.CreateOutbox(ctx, eventscontract.OutboxEvent{
		EventID:        "evt_runtime_store",
		EventType:      "GatewayRequestCompleted",
		EventVersion:   "v1",
		ProducerModule: "gateway",
		AggregateType:  "usage_log",
		AggregateID:    "1",
		CorrelationID:  "req_runtime_store",
		CausationID:    "req_runtime_store",
		IdempotencyKey: "req_runtime_store",
		Payload:        map[string]any{"usage_log_id": 1},
		Metadata:       map[string]any{"source_endpoint": "/v1/chat/completions"},
		Status:         eventscontract.OutboxStatusPending,
		CreatedAt:      now,
	})
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}
	duplicateOutbox, err := stores.Events.CreateOutbox(ctx, eventscontract.OutboxEvent{
		EventID:        "evt_runtime_store_duplicate",
		EventType:      "GatewayRequestCompleted",
		EventVersion:   "v1",
		ProducerModule: "gateway",
		IdempotencyKey: "req_runtime_store",
		Status:         eventscontract.OutboxStatusPending,
	})
	if err != nil {
		t.Fatalf("create duplicate outbox: %v", err)
	}
	if duplicateOutbox.ID != outbox.ID {
		t.Fatalf("expected outbox idempotency to return existing row, got %d want %d", duplicateOutbox.ID, outbox.ID)
	}
	dispatchableOutbox, err := stores.Events.ListDispatchableOutbox(ctx, now, 10)
	if err != nil {
		t.Fatalf("list dispatchable outbox: %v", err)
	}
	if len(dispatchableOutbox) != 1 || dispatchableOutbox[0].ID != outbox.ID {
		t.Fatalf("expected pending outbox to be dispatchable, got %+v", dispatchableOutbox)
	}
	retryAt := now.Add(time.Minute)
	failedOutbox, err := stores.Events.MarkOutboxFailed(ctx, outbox.ID, 1, &retryAt, "temporary failure")
	if err != nil {
		t.Fatalf("mark outbox failed: %v", err)
	}
	if failedOutbox.Status != eventscontract.OutboxStatusFailed || failedOutbox.AttemptCount != 1 || failedOutbox.NextRetryAt == nil || failedOutbox.LastError == nil {
		t.Fatalf("expected failed outbox transition, got %+v", failedOutbox)
	}
	dispatchableOutbox, err = stores.Events.ListDispatchableOutbox(ctx, now, 10)
	if err != nil {
		t.Fatalf("list dispatchable outbox before retry: %v", err)
	}
	if len(dispatchableOutbox) != 0 {
		t.Fatalf("expected failed outbox to wait for retry_at, got %+v", dispatchableOutbox)
	}
	dispatchableOutbox, err = stores.Events.ListDispatchableOutbox(ctx, retryAt, 10)
	if err != nil {
		t.Fatalf("list dispatchable outbox at retry: %v", err)
	}
	if len(dispatchableOutbox) != 1 || dispatchableOutbox[0].ID != outbox.ID {
		t.Fatalf("expected due failed outbox to become dispatchable, got %+v", dispatchableOutbox)
	}
	publishedOutbox, err := stores.Events.MarkOutboxPublished(ctx, outbox.ID, retryAt)
	if err != nil {
		t.Fatalf("mark outbox published: %v", err)
	}
	if publishedOutbox.Status != eventscontract.OutboxStatusPublished || publishedOutbox.PublishedAt == nil || publishedOutbox.NextRetryAt != nil || publishedOutbox.LastError != nil {
		t.Fatalf("expected published outbox transition, got %+v", publishedOutbox)
	}

	inbox, created, err := stores.Events.CreateInbox(ctx, eventscontract.InboxRecord{
		EventID:      "evt_runtime_store",
		ConsumerName: "usage-projector",
		EventType:    "GatewayRequestCompleted",
		Status:       eventscontract.InboxStatusPending,
		CreatedAt:    now,
	})
	if err != nil || !created {
		t.Fatalf("create inbox: record=%+v created=%v err=%v", inbox, created, err)
	}
	duplicateInbox, created, err := stores.Events.CreateInbox(ctx, eventscontract.InboxRecord{
		EventID:      "evt_runtime_store",
		ConsumerName: "usage-projector",
		EventType:    "GatewayRequestCompleted",
		Status:       eventscontract.InboxStatusPending,
	})
	if err != nil || created || duplicateInbox.ID != inbox.ID {
		t.Fatalf("expected inbox idempotency, record=%+v created=%v err=%v", duplicateInbox, created, err)
	}
	failedInbox, err := stores.Events.MarkInboxFailed(ctx, inbox.ID, 1, "consumer unavailable")
	if err != nil {
		t.Fatalf("mark inbox failed: %v", err)
	}
	if failedInbox.Status != eventscontract.InboxStatusFailed || failedInbox.AttemptCount != 1 || failedInbox.LastError == nil || failedInbox.ProcessedAt != nil {
		t.Fatalf("expected failed inbox transition, got %+v", failedInbox)
	}
	processedAt := now.Add(2 * time.Minute)
	processedInbox, err := stores.Events.MarkInboxProcessed(ctx, inbox.ID, processedAt)
	if err != nil {
		t.Fatalf("mark inbox processed: %v", err)
	}
	if processedInbox.Status != eventscontract.InboxStatusProcessed || processedInbox.ProcessedAt == nil || processedInbox.LastError != nil {
		t.Fatalf("expected processed inbox transition, got %+v", processedInbox)
	}
}
