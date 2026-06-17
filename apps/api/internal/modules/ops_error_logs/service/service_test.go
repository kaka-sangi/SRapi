package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/store/memory"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	svc, err := New(memory.New(), func() time.Time { return clock })
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

func TestRecordError_RedactsSensitiveJSON(t *testing.T) {
	svc := newTestService(t)
	status := 502
	body := `{"error":"upstream","authorization":"Bearer sk-abc","payload":{"api_key":"xyz","nested":{"token":"t"}}}`
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID:        "req-1",
		StatusCode:       &status,
		ErrorClass:       "server_bad",
		ErrorBodyExcerpt: body,
		ErrorMessage:     "upstream\x00 5xx",
	}); err != nil {
		t.Fatalf("RecordError: %v", err)
	}
	list, err := svc.List(context.Background(), contract.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list.Items))
	}
	got := list.Items[0]
	for _, key := range []string{"authorization", "api_key", "token"} {
		if strings.Contains(got.ErrorBodyExcerpt, "\""+key+"\":\"[REDACTED]\"") {
			continue
		}
		t.Fatalf("expected %q to be redacted in %q", key, got.ErrorBodyExcerpt)
	}
	if strings.Contains(got.ErrorBodyExcerpt, "sk-abc") || strings.Contains(got.ErrorBodyExcerpt, "xyz") {
		t.Fatalf("leaked secret in excerpt: %q", got.ErrorBodyExcerpt)
	}
	if strings.Contains(got.ErrorMessage, "\x00") {
		t.Fatalf("control char survived sanitization: %q", got.ErrorMessage)
	}
}

func TestUpdateResolution_TogglesResolvedAt(t *testing.T) {
	svc := newTestService(t)
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID: "req-2", ErrorMessage: "boom",
	}); err != nil {
		t.Fatalf("RecordError: %v", err)
	}
	list, _ := svc.List(context.Background(), contract.ListFilter{})
	id := list.Items[0].ID
	resolverID := 99
	updated, err := svc.UpdateResolution(context.Background(), contract.UpdateResolutionRequest{
		ID:           id,
		Resolution:   contract.ResolutionResolved,
		Note:         "rotated key",
		ResolvedByID: &resolverID,
	})
	if err != nil {
		t.Fatalf("UpdateResolution: %v", err)
	}
	if updated.Resolution != contract.ResolutionResolved {
		t.Fatalf("resolution: got %q want resolved", updated.Resolution)
	}
	if updated.ResolvedAt == nil {
		t.Fatalf("expected ResolvedAt to be set on resolution")
	}
	// Re-open should clear the timestamp.
	updated, err = svc.UpdateResolution(context.Background(), contract.UpdateResolutionRequest{
		ID:         id,
		Resolution: contract.ResolutionInvestigating,
	})
	if err != nil {
		t.Fatalf("UpdateResolution(investigating): %v", err)
	}
	if updated.ResolvedAt != nil {
		t.Fatalf("expected ResolvedAt to be cleared on re-open")
	}
}

func TestRecordError_DropsEmpty(t *testing.T) {
	svc := newTestService(t)
	if err := svc.RecordError(context.Background(), contract.RecordRequest{}); err != nil {
		t.Fatalf("RecordError: %v", err)
	}
	list, _ := svc.List(context.Background(), contract.ListFilter{})
	if len(list.Items) != 0 {
		t.Fatalf("expected empty input to be dropped, got %d items", len(list.Items))
	}
}

func TestSweepOlderThan(t *testing.T) {
	store := memory.New()
	svc, err := New(store, time.Now)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-time.Hour)
	_, _ = store.Insert(context.Background(), contract.Entry{OccurredAt: old, ErrorMessage: "old"})
	_, _ = store.Insert(context.Background(), contract.Entry{OccurredAt: recent, ErrorMessage: "recent"})
	removed, err := svc.SweepOlderThan(context.Background(), time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("SweepOlderThan: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed: got %d want 1", removed)
	}
}
