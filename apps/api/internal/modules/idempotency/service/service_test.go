package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
	memory "github.com/srapi/srapi/apps/api/internal/modules/idempotency/store/memory"
)

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

func TestBeginOutcomes(t *testing.T) {
	ctx := context.Background()
	clock := &fixedClock{now: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)}
	svc, err := New(memory.New(), clock, time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	const key, method, path = "tenant-a:abc", "POST", "/v1/responses"

	first, err := svc.Begin(ctx, key, method, path, "hashA")
	if err != nil || first.Outcome != OutcomeProceed {
		t.Fatalf("expected first begin to proceed, got %v err=%v", first.Outcome, err)
	}

	inFlight, err := svc.Begin(ctx, key, method, path, "hashA")
	if err != nil || inFlight.Outcome != OutcomeInFlight {
		t.Fatalf("expected in-flight twin to conflict, got %v err=%v", inFlight.Outcome, err)
	}

	mismatch, err := svc.Begin(ctx, key, method, path, "hashB")
	if err != nil || mismatch.Outcome != OutcomeMismatch {
		t.Fatalf("expected different body to mismatch, got %v err=%v", mismatch.Outcome, err)
	}

	if err := svc.Complete(ctx, key, method, path, &contract.Snapshot{StatusCode: 200, Body: []byte("replayed-body")}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	replay, err := svc.Begin(ctx, key, method, path, "hashA")
	if err != nil || replay.Outcome != OutcomeReplay {
		t.Fatalf("expected completed request to replay, got %v err=%v", replay.Outcome, err)
	}
	if replay.Record.Snapshot == nil || string(replay.Record.Snapshot.Body) != "replayed-body" {
		t.Fatalf("expected replay snapshot body, got %+v", replay.Record.Snapshot)
	}

	// A completed request with a different body is still a mismatch, not a replay.
	if out, _ := svc.Begin(ctx, key, method, path, "hashB"); out.Outcome != OutcomeMismatch {
		t.Fatalf("expected completed+different-body to mismatch, got %v", out.Outcome)
	}
}

func TestBeginCompletedWithoutSnapshotIsNonReplayable(t *testing.T) {
	ctx := context.Background()
	clock := &fixedClock{now: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)}
	svc, _ := New(memory.New(), clock, time.Minute, time.Hour)
	const key, method, path = "tenant-b:xyz", "POST", "/v1/chat/completions"

	if _, err := svc.Begin(ctx, key, method, path, "h"); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := svc.Complete(ctx, key, method, path, nil); err != nil {
		t.Fatalf("complete without snapshot: %v", err)
	}
	out, _ := svc.Begin(ctx, key, method, path, "h")
	if out.Outcome != OutcomeInFlight {
		t.Fatalf("expected non-replayable completion to conflict, got %v", out.Outcome)
	}
}

func TestBeginReacquiresStaleLock(t *testing.T) {
	ctx := context.Background()
	clock := &fixedClock{now: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)}
	svc, _ := New(memory.New(), clock, time.Minute, time.Hour)
	const key, method, path = "tenant-c:stale", "POST", "/v1/responses"

	if out, _ := svc.Begin(ctx, key, method, path, "h"); out.Outcome != OutcomeProceed {
		t.Fatalf("expected first begin to proceed")
	}
	// Owner crashed without completing; advance past the lock TTL.
	clock.now = clock.now.Add(2 * time.Minute)
	out, err := svc.Begin(ctx, key, method, path, "h")
	if err != nil || out.Outcome != OutcomeProceed {
		t.Fatalf("expected stale lock to be re-acquired, got %v err=%v", out.Outcome, err)
	}
}

func TestBeginConcurrentStaleReacquireOnlyOneProceeds(t *testing.T) {
	ctx := context.Background()
	clock := &fixedClock{now: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)}
	svc, _ := New(memory.New(), clock, time.Minute, time.Hour)
	const key, method, path = "tenant-c:stale-concurrent", "POST", "/v1/responses"

	if out, _ := svc.Begin(ctx, key, method, path, "h"); out.Outcome != OutcomeProceed {
		t.Fatalf("expected first begin to proceed")
	}
	clock.now = clock.now.Add(2 * time.Minute)

	var wg sync.WaitGroup
	outcomes := make(chan Outcome, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := svc.Begin(ctx, key, method, path, "h")
			if err != nil {
				t.Errorf("begin stale concurrent: %v", err)
				return
			}
			outcomes <- out.Outcome
		}()
	}
	wg.Wait()
	close(outcomes)

	var proceed, inFlight int
	for outcome := range outcomes {
		switch outcome {
		case OutcomeProceed:
			proceed++
		case OutcomeInFlight:
			inFlight++
		default:
			t.Fatalf("unexpected outcome %v", outcome)
		}
	}
	if proceed != 1 || inFlight != 1 {
		t.Fatalf("expected one proceed and one in-flight, got proceed=%d inFlight=%d", proceed, inFlight)
	}
}

func TestBeginRejectsEmptyInput(t *testing.T) {
	svc, _ := New(memory.New(), &fixedClock{now: time.Now()}, time.Minute, time.Hour)
	if _, err := svc.Begin(context.Background(), "", "POST", "/v1/x", "h"); err != ErrInvalidInput {
		t.Fatalf("expected ErrInvalidInput for empty key, got %v", err)
	}
}
