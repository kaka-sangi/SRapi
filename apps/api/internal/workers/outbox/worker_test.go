package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	affiliatecontract "github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	affiliateservice "github.com/srapi/srapi/apps/api/internal/modules/affiliate/service"
	affiliatememory "github.com/srapi/srapi/apps/api/internal/modules/affiliate/store/memory"
	"github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
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
