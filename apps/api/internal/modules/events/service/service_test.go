package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
)

func TestOutboxAndInboxAreIdempotent(t *testing.T) {
	svc, err := service.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	ctx := context.Background()

	first, err := svc.Enqueue(ctx, contract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		EventVersion:   "v1",
		ProducerModule: "gateway",
		AggregateType:  "usage_log",
		AggregateID:    "1",
		CorrelationID:  "req_1",
		CausationID:    "req_1",
		IdempotencyKey: "req_1",
		Payload:        map[string]any{"usage_log_id": 1},
	})
	if err != nil {
		t.Fatalf("enqueue first event: %v", err)
	}
	second, err := svc.Enqueue(ctx, contract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		EventVersion:   "v1",
		ProducerModule: "gateway",
		AggregateType:  "usage_log",
		AggregateID:    "1",
		CorrelationID:  "req_1",
		CausationID:    "req_1",
		IdempotencyKey: "req_1",
		Payload:        map[string]any{"usage_log_id": 1},
	})
	if err != nil {
		t.Fatalf("enqueue duplicate event: %v", err)
	}
	if first.ID != second.ID || first.EventID != second.EventID {
		t.Fatalf("expected duplicate outbox enqueue to return existing event, got %+v and %+v", first, second)
	}
	outbox, err := svc.ListOutbox(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 {
		t.Fatalf("expected one outbox row after duplicate enqueue, got %d", len(outbox))
	}

	inboxFirst, created, err := svc.RecordInbox(ctx, contract.RecordInboxRequest{
		EventID:      first.EventID,
		ConsumerName: "billing",
		EventType:    first.EventType,
	})
	if err != nil {
		t.Fatalf("record first inbox: %v", err)
	}
	if !created {
		t.Fatal("expected first inbox record to be created")
	}
	inboxSecond, created, err := svc.RecordInbox(ctx, contract.RecordInboxRequest{
		EventID:      first.EventID,
		ConsumerName: "billing",
		EventType:    first.EventType,
	})
	if err != nil {
		t.Fatalf("record duplicate inbox: %v", err)
	}
	if created {
		t.Fatal("expected duplicate inbox record to be reused")
	}
	if inboxFirst.ID != inboxSecond.ID {
		t.Fatalf("expected duplicate inbox record id %d, got %d", inboxFirst.ID, inboxSecond.ID)
	}
	inbox, err := svc.ListInbox(ctx)
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected one inbox row after duplicate record, got %d", len(inbox))
	}
}

func TestInboxStatusTransitions(t *testing.T) {
	clock := &fixedClock{now: time.Date(2026, 5, 21, 7, 0, 0, 0, time.UTC)}
	svc, err := service.New(eventsmemory.New(), clock)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	ctx := context.Background()
	inbox, created, err := svc.RecordInbox(ctx, contract.RecordInboxRequest{
		EventID:      "evt_inbox_status",
		ConsumerName: "billing",
		EventType:    "GatewayRequestCompleted",
	})
	if err != nil || !created {
		t.Fatalf("record inbox: inbox=%+v created=%v err=%v", inbox, created, err)
	}
	failed, err := svc.MarkInboxFailed(ctx, inbox, errors.New("consumer unavailable"))
	if err != nil {
		t.Fatalf("mark inbox failed: %v", err)
	}
	if failed.Status != contract.InboxStatusFailed || failed.AttemptCount != 1 || failed.LastError == nil || *failed.LastError != "consumer unavailable" || failed.ProcessedAt != nil {
		t.Fatalf("unexpected failed inbox state: %+v", failed)
	}
	clock.now = clock.now.Add(time.Minute)
	processed, err := svc.MarkInboxProcessed(ctx, failed.ID)
	if err != nil {
		t.Fatalf("mark inbox processed: %v", err)
	}
	if processed.Status != contract.InboxStatusProcessed || processed.LastError != nil || processed.ProcessedAt == nil || !processed.ProcessedAt.Equal(clock.now) {
		t.Fatalf("unexpected processed inbox state: %+v", processed)
	}
}

func TestDispatchPendingPublishesSuccessfulEvents(t *testing.T) {
	clock := &fixedClock{now: time.Date(2026, 5, 21, 8, 0, 0, 0, time.UTC)}
	svc, err := service.New(eventsmemory.New(), clock)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	ctx := context.Background()
	enqueued, err := svc.Enqueue(ctx, contract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		ProducerModule: "gateway",
		IdempotencyKey: "req_publish",
		Payload:        map[string]any{"request_id": "req_publish"},
	})
	if err != nil {
		t.Fatalf("enqueue event: %v", err)
	}

	handled := 0
	result, err := svc.DispatchPending(ctx, service.OutboxHandlerFunc(func(_ context.Context, event contract.OutboxEvent) error {
		handled++
		if event.EventID != enqueued.EventID {
			t.Fatalf("handler got event id %q, want %q", event.EventID, enqueued.EventID)
		}
		return nil
	}), service.DispatchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("dispatch pending: %v", err)
	}
	if result.Selected != 1 || result.Published != 1 || result.Failed != 0 || handled != 1 {
		t.Fatalf("unexpected dispatch result: result=%+v handled=%d", result, handled)
	}
	outbox, err := svc.ListOutbox(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].Status != contract.OutboxStatusPublished || outbox[0].PublishedAt == nil {
		t.Fatalf("expected published outbox row, got %+v", outbox)
	}
	if !outbox[0].PublishedAt.Equal(clock.now) {
		t.Fatalf("published_at = %s, want %s", outbox[0].PublishedAt, clock.now)
	}

	result, err = svc.DispatchPending(ctx, service.OutboxHandlerFunc(func(context.Context, contract.OutboxEvent) error {
		t.Fatal("published event should not be dispatched again")
		return nil
	}), service.DispatchOptions{})
	if err != nil {
		t.Fatalf("dispatch already published event: %v", err)
	}
	if result.Selected != 0 {
		t.Fatalf("expected no dispatchable events after publish, got %+v", result)
	}
}

func TestDispatchPendingMarksFailureAndRetriesWhenDue(t *testing.T) {
	clock := &fixedClock{now: time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)}
	svc, err := service.New(eventsmemory.New(), clock)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	ctx := context.Background()
	if _, err := svc.Enqueue(ctx, contract.EnqueueRequest{
		EventType:      "GatewayRequestCompleted",
		ProducerModule: "gateway",
		IdempotencyKey: "req_retry",
	}); err != nil {
		t.Fatalf("enqueue event: %v", err)
	}

	result, err := svc.DispatchPending(ctx, service.OutboxHandlerFunc(func(context.Context, contract.OutboxEvent) error {
		return errors.New("downstream unavailable")
	}), service.DispatchOptions{RetryBackoff: time.Minute})
	if err != nil {
		t.Fatalf("dispatch failing event: %v", err)
	}
	if result.Selected != 1 || result.Published != 0 || result.Failed != 1 {
		t.Fatalf("unexpected failure dispatch result: %+v", result)
	}
	outbox, err := svc.ListOutbox(ctx)
	if err != nil {
		t.Fatalf("list failed outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].Status != contract.OutboxStatusFailed || outbox[0].AttemptCount != 1 || outbox[0].NextRetryAt == nil || outbox[0].LastError == nil {
		t.Fatalf("expected failed retryable outbox row, got %+v", outbox)
	}
	if !outbox[0].NextRetryAt.Equal(clock.now.Add(time.Minute)) || *outbox[0].LastError != "downstream unavailable" {
		t.Fatalf("unexpected retry fields: %+v", outbox[0])
	}

	result, err = svc.DispatchPending(ctx, service.OutboxHandlerFunc(func(context.Context, contract.OutboxEvent) error {
		t.Fatal("event should not retry before next_retry_at")
		return nil
	}), service.DispatchOptions{RetryBackoff: time.Minute})
	if err != nil {
		t.Fatalf("dispatch before retry due: %v", err)
	}
	if result.Selected != 0 {
		t.Fatalf("expected no selected events before retry due, got %+v", result)
	}

	clock.now = *outbox[0].NextRetryAt
	result, err = svc.DispatchPending(ctx, service.OutboxHandlerFunc(func(context.Context, contract.OutboxEvent) error {
		return nil
	}), service.DispatchOptions{RetryBackoff: time.Minute})
	if err != nil {
		t.Fatalf("dispatch due retry: %v", err)
	}
	if result.Selected != 1 || result.Published != 1 || result.Failed != 0 {
		t.Fatalf("unexpected retry dispatch result: %+v", result)
	}
	outbox, err = svc.ListOutbox(ctx)
	if err != nil {
		t.Fatalf("list retried outbox: %v", err)
	}
	if outbox[0].Status != contract.OutboxStatusPublished || outbox[0].LastError != nil || outbox[0].NextRetryAt != nil {
		t.Fatalf("expected retry to publish and clear error fields, got %+v", outbox[0])
	}
}

type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	return c.now
}
