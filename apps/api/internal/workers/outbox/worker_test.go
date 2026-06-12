package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	admincontrolmemory "github.com/srapi/srapi/apps/api/internal/modules/admin_control/store/memory"
	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliateservice "github.com/srapi/srapi/apps/api/internal/modules/affiliate/service"
	affiliatememory "github.com/srapi/srapi/apps/api/internal/modules/affiliate/store/memory"
	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
	"github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	"github.com/srapi/srapi/apps/api/internal/workers/outbox"
)

func TestWorkerRunOnceRecordsInboxAndPublishesOutbox(t *testing.T) {
	store := eventsmemory.New()
	clock := &fixedClock{now: time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)}
	events, err := eventsservice.New(store, clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	enqueued, err := events.Enqueue(t.Context(), contract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		ProducerModule: "gateway",
		IdempotencyKey: "req_worker_publish",
	})
	if err != nil {
		t.Fatalf("enqueue outbox event: %v", err)
	}

	worker, err := outbox.New(store, discardLogger(), outbox.Config{
		ConsumerName:  "test-consumer",
		DispatchClock: clock,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Published != 1 || result.Failed != 0 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}

	outboxRows, err := events.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outboxRows) != 1 || outboxRows[0].Status != contract.OutboxStatusPublished || outboxRows[0].PublishedAt == nil {
		t.Fatalf("expected published outbox event, got %+v", outboxRows)
	}
	inboxRows, err := events.ListInbox(t.Context())
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inboxRows) != 1 || inboxRows[0].EventID != enqueued.EventID || inboxRows[0].ConsumerName != "test-consumer" || inboxRows[0].Status != contract.InboxStatusProcessed || inboxRows[0].ProcessedAt == nil {
		t.Fatalf("expected processed inbox record for dispatched event, got %+v", inboxRows)
	}
}

func TestWorkerRunOnceRefreshesGatewayAccountSnapshot(t *testing.T) {
	ctx := t.Context()
	eventStore := eventsmemory.New()
	clock := &fixedClock{now: time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)}
	events, err := eventsservice.New(eventStore, clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	accountStore := accountmemory.New()
	accounts, err := accountservice.New(accountStore, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new accounts service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   12,
		Name:         "outbox-snapshot-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
		Metadata: map[string]any{
			"runtime_quota_window_seconds": 60,
			"cost_window_seconds":          60,
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	usageStore := usagememory.New()
	accountID := account.ID
	providerID := account.ProviderID
	if _, err := usageStore.Create(ctx, usagecontract.UsageLog{
		RequestID:      "req_outbox_snapshot_usage",
		UserID:         1,
		APIKeyID:       2,
		AccountID:      &accountID,
		ProviderID:     &providerID,
		SourceEndpoint: "/v1/responses",
		Model:          "codex-model",
		Success:        true,
		TotalTokens:    42,
		BillableCost:   "0.01000000",
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed usage: %v", err)
	}
	resetAt := time.Date(2026, 5, 21, 11, 0, 0, 0, time.UTC)
	if _, err := events.Enqueue(ctx, contract.EnqueueRequest{
		EventType:      "GatewayAccountSnapshotRefreshRequested",
		ProducerModule: "gateway",
		AggregateType:  "provider_account",
		AggregateID:    "1",
		IdempotencyKey: "gateway.account_snapshot:test",
		Payload: map[string]any{
			"request_id":  "req_outbox_snapshot",
			"attempt_no":  1,
			"account_id":  account.ID,
			"provider_id": account.ProviderID,
			"quota_signals": []any{map[string]any{
				"quota_type":      "codex_5h_percent",
				"remaining":       "0",
				"used":            "100",
				"quota_limit":     "100",
				"remaining_ratio": 0,
				"reset_at":        resetAt.Format(time.RFC3339Nano),
				"snapshot_at":     resetAt.Add(-time.Hour).Format(time.RFC3339Nano),
				"metadata": map[string]any{
					"codex_primary_over_secondary_percent": 117.5,
					"codex_usage_updated_at":               resetAt.Format(time.RFC3339),
				},
			}},
		},
	}); err != nil {
		t.Fatalf("enqueue refresh event: %v", err)
	}
	worker, err := outbox.New(eventStore, discardLogger(), outbox.Config{
		ConsumerName:  "test-consumer",
		DispatchClock: clock,
		AccountStore:  accountStore,
		MasterKey:     "0123456789abcdef0123456789abcdef",
		UsageStore:    usageStore,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Published != 1 || result.Failed != 0 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	health, err := accounts.LatestHealthSnapshotByAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("latest health snapshot: %v", err)
	}
	if health.SuccessRate != 1 || health.LatencyP95MS != 0 {
		t.Fatalf("unexpected health snapshot: %+v", health)
	}
	updated, err := accounts.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find updated account: %v", err)
	}
	if testMetadataInt(updated.Metadata, "tpm_used") != 42 || testMetadataInt(updated.Metadata, "rpm_used") != 1 {
		t.Fatalf("expected bounded runtime quota metadata, got %+v", updated.Metadata)
	}
	if updated.Metadata["quota_exhausted"] != true ||
		updated.Metadata["quota_remaining_ratio"] != float64(0) ||
		updated.Metadata["quota_type"] != "codex_5h_percent" ||
		updated.Metadata["quota_reset_at"] != resetAt.Format(time.RFC3339) {
		t.Fatalf("expected provider quota metadata, got %+v", updated.Metadata)
	}
	if updated.Metadata["codex_primary_over_secondary_percent"] != 117.5 ||
		updated.Metadata["codex_usage_updated_at"] != resetAt.Format(time.RFC3339) {
		t.Fatalf("expected Codex quota metadata, got %+v", updated.Metadata)
	}
	quotas, err := accounts.ListQuotaSnapshotsByAccount(ctx, account.ID, 10)
	if err != nil {
		t.Fatalf("list quota snapshots: %v", err)
	}
	foundSignal := false
	foundSynthetic := false
	for _, quota := range quotas {
		switch quota.QuotaType {
		case "codex_5h_percent":
			foundSignal = quota.ResetAt != nil && quota.ResetAt.Equal(resetAt) && quota.Used == "100"
		case accountcontract.QuotaTypeSyntheticMonthlyTokens:
			foundSynthetic = quota.Used == "42"
		}
	}
	if !foundSignal || !foundSynthetic {
		t.Fatalf("expected provider and synthetic quota snapshots, got %+v", quotas)
	}
}

func TestWorkerRunOnceMarksFailureForRetry(t *testing.T) {
	store := eventsmemory.New()
	clock := &fixedClock{now: time.Date(2026, 5, 21, 11, 0, 0, 0, time.UTC)}
	events, err := eventsservice.New(store, clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	if _, err := events.Enqueue(t.Context(), contract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		ProducerModule: "gateway",
		IdempotencyKey: "req_worker_retry",
	}); err != nil {
		t.Fatalf("enqueue outbox event: %v", err)
	}

	worker, err := outbox.New(store, discardLogger(), outbox.Config{
		RetryBackoff:  time.Minute,
		DispatchClock: clock,
		EventHandler: eventsservice.OutboxHandlerFunc(func(context.Context, contract.OutboxEvent) error {
			return errors.New("temporary handler failure")
		}),
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Published != 0 || result.Failed != 1 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	outboxRows, err := events.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outboxRows) != 1 || outboxRows[0].Status != contract.OutboxStatusFailed || outboxRows[0].AttemptCount != 1 || outboxRows[0].NextRetryAt == nil {
		t.Fatalf("expected failed outbox retry state, got %+v", outboxRows)
	}
	if !outboxRows[0].NextRetryAt.Equal(clock.now.Add(time.Minute)) {
		t.Fatalf("next_retry_at = %s, want %s", outboxRows[0].NextRetryAt, clock.now.Add(time.Minute))
	}
	inboxRows, err := events.ListInbox(t.Context())
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inboxRows) != 1 || inboxRows[0].Status != contract.InboxStatusFailed || inboxRows[0].AttemptCount != 1 || inboxRows[0].LastError == nil {
		t.Fatalf("expected failed inbox retry state, got %+v", inboxRows)
	}
	if *inboxRows[0].LastError != "temporary handler failure" {
		t.Fatalf("last_error = %q, want temporary handler failure", *inboxRows[0].LastError)
	}
}

func TestWorkerRetriesAuthEmailWhenEmailDeliveryNotConfigured(t *testing.T) {
	store := eventsmemory.New()
	clock := &fixedClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	events, err := eventsservice.New(store, clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	if _, err := events.Enqueue(t.Context(), contract.EnqueueRequest{
		EventType:      notificationscontract.EventAuthPasswordResetRequested,
		ProducerModule: "auth",
		IdempotencyKey: "auth.password_reset:test",
		Payload: map[string]any{
			"recipient_user_id": 1,
		},
	}); err != nil {
		t.Fatalf("enqueue auth email event: %v", err)
	}

	worker, err := outbox.New(store, discardLogger(), outbox.Config{
		RetryBackoff:  time.Minute,
		DispatchClock: clock,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Published != 0 || result.Failed != 1 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	outboxRows, err := events.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outboxRows) != 1 || outboxRows[0].Status != contract.OutboxStatusFailed || outboxRows[0].LastError == nil {
		t.Fatalf("expected failed auth email outbox, got %+v", outboxRows)
	}
}

func TestWorkerSkipsAlreadyProcessedInbox(t *testing.T) {
	store := eventsmemory.New()
	clock := &fixedClock{now: time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)}
	events, err := eventsservice.New(store, clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	enqueued, err := events.Enqueue(t.Context(), contract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		ProducerModule: "gateway",
		IdempotencyKey: "req_worker_skip",
	})
	if err != nil {
		t.Fatalf("enqueue outbox event: %v", err)
	}
	inbox, created, err := events.RecordInbox(t.Context(), contract.RecordInboxRequest{
		EventID:      enqueued.EventID,
		ConsumerName: "test-consumer",
		EventType:    enqueued.EventType,
	})
	if err != nil || !created {
		t.Fatalf("record inbox: inbox=%+v created=%v err=%v", inbox, created, err)
	}
	if _, err := events.MarkInboxProcessed(t.Context(), inbox.ID); err != nil {
		t.Fatalf("mark inbox processed: %v", err)
	}

	worker, err := outbox.New(store, discardLogger(), outbox.Config{
		ConsumerName:  "test-consumer",
		DispatchClock: clock,
		EventHandler: eventsservice.OutboxHandlerFunc(func(context.Context, contract.OutboxEvent) error {
			t.Fatal("processed inbox should skip handler execution")
			return nil
		}),
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Published != 1 || result.Failed != 0 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	inboxRows, err := events.ListInbox(t.Context())
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inboxRows) != 1 || inboxRows[0].Status != contract.InboxStatusProcessed || inboxRows[0].AttemptCount != 0 {
		t.Fatalf("expected processed inbox to remain unchanged, got %+v", inboxRows)
	}
}

func TestWorkerSkipsClaimedPendingInboxWithoutPublishing(t *testing.T) {
	store := eventsmemory.New()
	clock := &fixedClock{now: time.Date(2026, 5, 21, 12, 30, 0, 0, time.UTC)}
	events, err := eventsservice.New(store, clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	enqueued, err := events.Enqueue(t.Context(), contract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		ProducerModule: "gateway",
		IdempotencyKey: "req_worker_claimed",
	})
	if err != nil {
		t.Fatalf("enqueue outbox event: %v", err)
	}
	if _, created, err := events.RecordInbox(t.Context(), contract.RecordInboxRequest{
		EventID:      enqueued.EventID,
		ConsumerName: "test-consumer",
		EventType:    enqueued.EventType,
	}); err != nil || !created {
		t.Fatalf("claim inbox: created=%v err=%v", created, err)
	}

	calls := 0
	worker, err := outbox.New(store, discardLogger(), outbox.Config{
		ConsumerName:  "test-consumer",
		DispatchClock: clock,
		EventHandler: eventsservice.OutboxHandlerFunc(func(context.Context, contract.OutboxEvent) error {
			calls++
			return nil
		}),
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected claimed pending inbox to skip handler, got %d calls", calls)
	}
	if result.Selected != 1 || result.Published != 0 || result.Failed != 0 {
		t.Fatalf("unexpected dispatch result for claimed inbox: %+v", result)
	}
	outboxRows, err := events.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outboxRows) != 1 || outboxRows[0].Status != contract.OutboxStatusPending {
		t.Fatalf("expected outbox to remain pending, got %+v", outboxRows)
	}
}

func TestWorkerDispatchesPaymentEventsToAffiliate(t *testing.T) {
	store := eventsmemory.New()
	clock := &fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	events, err := eventsservice.New(store, clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	affiliateStore := affiliatememory.New()
	affiliateSvc, err := affiliateservice.New(affiliateStore, affiliateservice.Dependencies{}, clock)
	if err != nil {
		t.Fatalf("new affiliate service: %v", err)
	}
	if _, err := affiliateSvc.CreateInviteCode(t.Context(), affiliatecontract.CreateInviteCodeRequest{UserID: 10, Code: "INVITE10"}); err != nil {
		t.Fatalf("create invite code: %v", err)
	}
	if _, err := affiliateSvc.BindInvite(t.Context(), affiliatecontract.BindInviteRequest{InviteeUserID: 20, Code: "INVITE10"}); err != nil {
		t.Fatalf("bind invite: %v", err)
	}
	if _, err := affiliateSvc.CreateRule(t.Context(), affiliatecontract.CreateRuleRequest{
		Name:        "ten-percent",
		TriggerType: affiliatecontract.TriggerTypePaymentPaid,
		Rate:        "0.10",
		Currency:    "USD",
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if _, err := events.Enqueue(t.Context(), contract.EnqueueRequest{
		EventType:      "PaymentOrderPaid",
		ProducerModule: "payments",
		AggregateType:  "payment_order",
		AggregateID:    "pay_1",
		IdempotencyKey: "payment_paid:pay_1",
		Payload: map[string]any{
			"order_id":                1,
			"order_no":                "pay_1",
			"user_id":                 20,
			"amount":                  "100.00000000",
			"currency":                "USD",
			"paid_at":                 clock.now.Format(time.RFC3339Nano),
			"provider_transaction_id": "txn_1",
		},
	}); err != nil {
		t.Fatalf("enqueue payment event: %v", err)
	}

	worker, err := outbox.New(store, discardLogger(), outbox.Config{
		ConsumerName:   "affiliate-consumer",
		DispatchClock:  clock,
		AffiliateStore: affiliateStore,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Published != 1 || result.Failed != 0 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	ledgers, err := affiliateSvc.ListLedgers(t.Context())
	if err != nil {
		t.Fatalf("list affiliate ledgers: %v", err)
	}
	if len(ledgers) != 1 || ledgers[0].Amount != "10.00000000" || ledgers[0].ReferenceID != "payment_paid:pay_1" {
		t.Fatalf("expected one affiliate accrual ledger, got %+v", ledgers)
	}
	outboxRows, err := events.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outboxRows) != 2 || outboxRows[0].EventType != "PaymentOrderPaid" || outboxRows[0].Status != contract.OutboxStatusPublished || outboxRows[1].EventType != "AffiliateRebateAccrued" {
		t.Fatalf("expected payment published and affiliate accrued pending, got %+v", outboxRows)
	}
}

func TestWorkerSkipsInvitationRebateWhenDisabledAndAuditsReason(t *testing.T) {
	store := eventsmemory.New()
	clock := &fixedClock{now: time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)}
	events, err := eventsservice.New(store, clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	affiliateStore := affiliatememory.New()
	affiliateSvc, err := affiliateservice.New(affiliateStore, affiliateservice.Dependencies{Events: events}, clock)
	if err != nil {
		t.Fatalf("new affiliate service: %v", err)
	}
	if _, err := affiliateSvc.CreateInviteCode(t.Context(), affiliatecontract.CreateInviteCodeRequest{UserID: 10, Code: "INVITE10"}); err != nil {
		t.Fatalf("create invite code: %v", err)
	}
	if _, err := affiliateSvc.BindInvite(t.Context(), affiliatecontract.BindInviteRequest{InviteeUserID: 20, Code: "INVITE10"}); err != nil {
		t.Fatalf("bind invite: %v", err)
	}
	if _, err := affiliateSvc.CreateRule(t.Context(), affiliatecontract.CreateRuleRequest{
		Name:        "ten-percent",
		TriggerType: affiliatecontract.TriggerTypePaymentPaid,
		Rate:        "0.10",
		Currency:    "USD",
	}); err != nil {
		t.Fatalf("create rule: %v", err)
	}
	adminStore := admincontrolmemory.New()
	adminSvc, err := admincontrolservice.New(adminStore, nil)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}
	settings, err := adminSvc.GetAdminSettings(t.Context())
	if err != nil {
		t.Fatalf("get admin settings: %v", err)
	}
	settings.Features.InvitationRebateEnabled = false
	if _, err := adminSvc.UpdateAdminSettings(t.Context(), settings, 1); err != nil {
		t.Fatalf("update admin settings: %v", err)
	}
	if _, err := events.Enqueue(t.Context(), contract.EnqueueRequest{
		EventType:      "PaymentOrderPaid",
		ProducerModule: "payments",
		AggregateType:  "payment_order",
		AggregateID:    "pay_disabled",
		IdempotencyKey: "payment_paid:pay_disabled",
		Payload: map[string]any{
			"order_id":                2,
			"order_no":                "pay_disabled",
			"user_id":                 20,
			"amount":                  "100.00000000",
			"currency":                "USD",
			"paid_at":                 clock.now.Format(time.RFC3339Nano),
			"provider_transaction_id": "txn_disabled",
		},
	}); err != nil {
		t.Fatalf("enqueue payment event: %v", err)
	}
	auditStore := auditmemory.New()
	worker, err := outbox.New(store, discardLogger(), outbox.Config{
		ConsumerName:      "affiliate-disabled-consumer",
		DispatchClock:     clock,
		AffiliateStore:    affiliateStore,
		AdminControlStore: adminStore,
		AuditStore:        auditStore,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Published != 1 || result.Failed != 0 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	ledgers, err := affiliateSvc.ListLedgers(t.Context())
	if err != nil {
		t.Fatalf("list affiliate ledgers: %v", err)
	}
	if len(ledgers) != 0 {
		t.Fatalf("disabled invitation rebate should not accrue ledger, got %+v", ledgers)
	}
	auditLogs, err := auditStore.List(t.Context())
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(auditLogs) != 1 || auditLogs[0].Action != "affiliate.rebate.skipped" || auditLogs[0].After["reason"] != "invitation_rebate_disabled" {
		t.Fatalf("expected disabled rebate audit reason, got %+v", auditLogs)
	}
}

func TestWorkerDispatchesPaymentPaidToSubscription(t *testing.T) {
	store := eventsmemory.New()
	clock := &fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	events, err := eventsservice.New(store, clock)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	subscriptionStore := subscriptionmemory.New()
	subscriptions, err := subscriptionservice.New(subscriptionStore, clock)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	plan, err := subscriptions.CreatePlan(t.Context(), subscriptioncontract.CreatePlanRequest{
		Name:         "commercial-pro",
		Price:        "19.99",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{"allowed_models": []any{"commercial-model"}},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := events.Enqueue(t.Context(), contract.EnqueueRequest{
		EventType:      "PaymentOrderPaid",
		ProducerModule: "payments",
		AggregateType:  "payment_order",
		AggregateID:    "pay_sub_1",
		IdempotencyKey: "payment_paid:pay_sub_1",
		Payload: map[string]any{
			"order_id":     1,
			"order_no":     "pay_sub_1",
			"user_id":      20,
			"amount":       "19.99000000",
			"currency":     "USD",
			"product_type": "subscription_plan",
			"product_id":   plan.ID,
			"paid_at":      clock.now.Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("enqueue payment event: %v", err)
	}

	worker, err := outbox.New(store, discardLogger(), outbox.Config{
		ConsumerName:      "subscription-consumer",
		DispatchClock:     clock,
		SubscriptionStore: subscriptionStore,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Published != 1 || result.Failed != 0 {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}
	userSubscriptions, err := subscriptions.ListUserSubscriptionsByUser(t.Context(), 20)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(userSubscriptions) != 1 || userSubscriptions[0].PlanID != plan.ID || userSubscriptions[0].SourceID != "pay_sub_1" {
		t.Fatalf("expected subscription activated from payment event, got %+v", userSubscriptions)
	}

	duplicate, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once again: %v", err)
	}
	if duplicate.Selected != 0 {
		t.Fatalf("published outbox event should not be redispatched, got %+v", duplicate)
	}
	userSubscriptions, err = subscriptions.ListUserSubscriptionsByUser(t.Context(), 20)
	if err != nil {
		t.Fatalf("list subscriptions after duplicate run: %v", err)
	}
	if len(userSubscriptions) != 1 {
		t.Fatalf("expected subscription activation to stay idempotent, got %+v", userSubscriptions)
	}
}

func TestWorkerStartAndShutdownAreIdempotent(t *testing.T) {
	store := eventsmemory.New()
	worker, err := outbox.New(store, discardLogger(), outbox.Config{
		Interval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	worker.Start(t.Context())
	worker.Start(t.Context())
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := worker.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown worker: %v", err)
	}
	if err := worker.Shutdown(ctx); err != nil {
		t.Fatalf("second worker shutdown: %v", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	return c.now
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testMetadataInt(metadata map[string]any, key string) int {
	switch value := metadata[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}
