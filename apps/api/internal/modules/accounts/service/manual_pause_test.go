package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
)

// fixedClock is a tiny test Clock — the production Service interface is
// unexported but this package owns it so we can declare an inline fake.
type fixedManualPauseClock struct{ t time.Time }

func (c fixedManualPauseClock) Now() time.Time { return c.t }

func newManualPauseService(t *testing.T, now time.Time) (*Service, contract.ProviderAccount) {
	t.Helper()
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", fixedManualPauseClock{t: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	account, err := svc.Create(context.Background(), contract.CreateRequest{
		ProviderID:   1,
		Name:         "main",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, account
}

func TestApplyManualPauseRecordsWindowAndReason(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	svc, account := newManualPauseService(t, now)

	until := now.Add(30 * time.Minute)
	updated, err := svc.ApplyManualPause(context.Background(), account.ID, ManualPauseRequest{
		Until:  until,
		Reason: "  investigating slow upstream  ",
	})
	if err != nil {
		t.Fatalf("ApplyManualPause: %v", err)
	}
	gotUntil, _ := updated.Metadata["manual_pause_until"].(string)
	if gotUntil != until.Format(time.RFC3339) {
		t.Fatalf("manual_pause_until = %q, want %q", gotUntil, until.Format(time.RFC3339))
	}
	gotReason, _ := updated.Metadata["manual_pause_reason"].(string)
	if gotReason != "investigating slow upstream" {
		t.Fatalf("manual_pause_reason trimmed wrong: %q", gotReason)
	}
	if _, ok := updated.Metadata["manual_pause_applied_at"]; !ok {
		t.Fatalf("expected manual_pause_applied_at to be set")
	}
	// Sanity: an empty/whitespace-only reason clears the key rather than storing whitespace.
	updated2, err := svc.ApplyManualPause(context.Background(), account.ID, ManualPauseRequest{Until: until.Add(time.Minute), Reason: "   "})
	if err != nil {
		t.Fatalf("ApplyManualPause empty reason: %v", err)
	}
	if _, ok := updated2.Metadata["manual_pause_reason"]; ok {
		t.Fatalf("empty-reason apply must drop manual_pause_reason, got %v", updated2.Metadata["manual_pause_reason"])
	}
}

func TestApplyManualPauseRejectsPastInstant(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	svc, account := newManualPauseService(t, now)

	cases := []time.Time{
		now,                   // boundary — must be strictly after
		now.Add(-time.Minute), // past
		time.Time{},           // zero
	}
	for _, until := range cases {
		_, err := svc.ApplyManualPause(context.Background(), account.ID, ManualPauseRequest{Until: until})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("until=%v: expected ErrInvalidInput, got %v", until, err)
		}
	}
}

func TestClearManualPauseIsIdempotent(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	svc, account := newManualPauseService(t, now)

	// Clearing an unpaused account is a successful no-op.
	out, err := svc.ClearManualPause(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("ClearManualPause no-op: %v", err)
	}
	if _, ok := out.Metadata["manual_pause_until"]; ok {
		t.Fatalf("no-op clear must not introduce manual_pause_until")
	}

	// Apply then clear — the three keys must all be gone.
	if _, err := svc.ApplyManualPause(context.Background(), account.ID, ManualPauseRequest{Until: now.Add(time.Hour), Reason: "x"}); err != nil {
		t.Fatalf("ApplyManualPause: %v", err)
	}
	cleared, err := svc.ClearManualPause(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("ClearManualPause: %v", err)
	}
	for _, key := range []string{"manual_pause_until", "manual_pause_reason", "manual_pause_applied_at"} {
		if _, ok := cleared.Metadata[key]; ok {
			t.Fatalf("expected %q to be removed", key)
		}
	}
}

func TestApplyManualPauseDoesNotChangeStatus(t *testing.T) {
	// The pause is a scheduling skip, not a logical disable: status must
	// stay "active" so the admin list's logical/transient distinction
	// remains accurate.
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	svc, account := newManualPauseService(t, now)
	if account.Status != contract.StatusActive {
		t.Fatalf("precondition: account status %q, want active", account.Status)
	}
	updated, err := svc.ApplyManualPause(context.Background(), account.ID, ManualPauseRequest{Until: now.Add(time.Hour), Reason: "x"})
	if err != nil {
		t.Fatalf("ApplyManualPause: %v", err)
	}
	if updated.Status != contract.StatusActive {
		t.Fatalf("status changed to %q during manual pause; pause must not flip status", updated.Status)
	}
}
