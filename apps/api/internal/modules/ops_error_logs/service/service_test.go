package service

import (
	"context"
	"errors"
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
	body := `{"error":"upstream","authorization":"Bearer sk-abc","provider_error":"Authorization: Bearer raw-token refresh_token=raw-refresh","payload":{"api_key":"xyz","nested":{"token":"t"}}}`
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID:        "req-1",
		StatusCode:       &status,
		ErrorClass:       "server_bad",
		ErrorBodyExcerpt: body,
		ErrorMessage:     "upstream\x00 Authorization: Bearer raw-token api_key=sk-abc123456789",
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
	if strings.Contains(got.ErrorBodyExcerpt, "raw-token") || strings.Contains(got.ErrorBodyExcerpt, "raw-refresh") {
		t.Fatalf("leaked secret in JSON string value: %q", got.ErrorBodyExcerpt)
	}
	if strings.Contains(got.ErrorMessage, "\x00") || strings.Contains(got.ErrorMessage, "raw-token") || strings.Contains(got.ErrorMessage, "sk-abc123456789") {
		t.Fatalf("error message was not sanitized: %q", got.ErrorMessage)
	}
	if !strings.Contains(got.ErrorMessage, "Bearer [REDACTED]") || !strings.Contains(got.ErrorMessage, "api_key=[REDACTED]") {
		t.Fatalf("expected redaction markers in error message, got %q", got.ErrorMessage)
	}
}

func TestRecordError_RedactsSensitivePlainTextEvidence(t *testing.T) {
	svc := newTestService(t)
	status := 502
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID:        "req-plain",
		StatusCode:       &status,
		ErrorClass:       "server_bad",
		ErrorMessage:     "Authorization: Bearer raw-token access_token=raw-access key sk-abc123456789",
		ErrorBodyExcerpt: "upstream said Authorization: Bearer body-token refresh_token=body-refresh api_key=sk_111111111111_22222222222222222222222222222222",
		UpstreamErrors: []contract.UpstreamErrorEvent{{
			AttemptNo:          1,
			UpstreamStatusCode: 502,
			UpstreamURL:        "https://upstream.example/v1?access_token=url-token&client_secret=url-secret&safe=1",
			Message:            "nested Authorization: Bearer nested-token client_secret=nested-secret",
			BodyExcerpt:        "nested api_key=sk-plain123456789 refresh_token=nested-refresh",
		}},
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
	for _, leaked := range []string{
		"raw-token", "raw-access", "sk-abc123456789", "body-token", "body-refresh",
		"22222222222222222222222222222222", "url-token", "url-secret", "nested-token",
		"nested-secret", "sk-plain123456789", "nested-refresh",
	} {
		if strings.Contains(got.ErrorMessage, leaked) ||
			strings.Contains(got.ErrorBodyExcerpt, leaked) ||
			strings.Contains(got.UpstreamErrors[0].UpstreamURL, leaked) ||
			strings.Contains(got.UpstreamErrors[0].Message, leaked) ||
			strings.Contains(got.UpstreamErrors[0].BodyExcerpt, leaked) {
			t.Fatalf("leaked %q in entry %+v", leaked, got)
		}
	}
	if !strings.Contains(got.ErrorMessage, "Bearer [REDACTED]") || !strings.Contains(got.ErrorBodyExcerpt, "api_key=[REDACTED]") {
		t.Fatalf("expected top-level redaction markers, got message=%q body=%q", got.ErrorMessage, got.ErrorBodyExcerpt)
	}
	if got.UpstreamErrors[0].UpstreamURL != "https://upstream.example/v1?access_token=[REDACTED]&client_secret=[REDACTED]&safe=1" {
		t.Fatalf("unexpected redacted upstream url: %q", got.UpstreamErrors[0].UpstreamURL)
	}
}

func TestList_NormalizesFilterAtServiceBoundary(t *testing.T) {
	svc := newTestService(t)
	status := 502
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID:    "req-filter",
		StatusCode:   &status,
		Platform:     "  openai-compatible\n\t",
		Model:        "  codex-mini\n\t",
		ErrorClass:   "  server_bad\n\t",
		ErrorMessage: "Authorization: Bearer raw-token api_key=sk-filter123456789",
	}); err != nil {
		t.Fatalf("RecordError: %v", err)
	}

	list, err := svc.List(context.Background(), contract.ListFilter{
		Platform:   "  openai-compatible\n\t",
		Model:      "  codex-mini\n\t",
		ErrorClass: "  server_bad\n\t",
		Query:      "  Authorization: Bearer raw-token\n\t",
		Page:       -1,
		PageSize:   500,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].RequestID != "req-filter" {
		t.Fatalf("expected normalized filter to match redacted evidence, got %+v", list)
	}
	if list.Page != 1 || list.PageSize != 200 {
		t.Fatalf("expected pagination normalization, got page=%d page_size=%d", list.Page, list.PageSize)
	}
}

func TestList_RejectsInvalidFiltersAtServiceBoundary(t *testing.T) {
	svc := newTestService(t)
	invalidStatus := 600
	statusMin := 500
	statusMax := 400
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	earlier := now.Add(-time.Hour)

	tests := []struct {
		name   string
		filter contract.ListFilter
	}{
		{name: "resolution", filter: contract.ListFilter{Resolution: contract.Resolution("bad")}},
		{name: "status min", filter: contract.ListFilter{StatusCodeMin: &invalidStatus}},
		{name: "status range", filter: contract.ListFilter{StatusCodeMin: &statusMin, StatusCodeMax: &statusMax}},
		{name: "time range", filter: contract.ListFilter{From: &now, To: &earlier}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.List(context.Background(), tt.filter); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("List error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestRecordError_PreservesStructuredAttemptEvidence(t *testing.T) {
	svc := newTestService(t)
	status := 429
	accountID := 42
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID:         "req-attempt",
		StatusCode:        &status,
		APIKeyPrefix:      "sk_abc123",
		TargetProtocol:    "openai-compatible",
		UpstreamRequestID: "upstream_req",
		AttemptNo:         2,
		LatencyMS:         345,
		InputTokens:       12,
		OutputTokens:      3,
		UsageEstimated:    true,
		ErrorOwner:        "provider",
		ErrorSource:       "upstream_http",
		ErrorMessage:      "rate limited",
		UpstreamErrors: []contract.UpstreamErrorEvent{{
			AtUnixMs:           1780000000000,
			AttemptNo:          1,
			AccountID:          &accountID,
			AccountName:        "primary-account",
			UpstreamStatusCode: 429,
			UpstreamRequestID:  "upstream_req_1",
			UpstreamURL:        "gpt-4o",
			Kind:               "http_error",
			Message:            "first attempt",
			BodyExcerpt:        `{"token":"secret","message":"limited"}`,
		}, {
			AtUnixMs:           1780000000001,
			AttemptNo:          2,
			UpstreamStatusCode: 999,
			Kind:               "http_error",
			Message:            "invalid upstream status",
		}},
	}); err != nil {
		t.Fatalf("RecordError: %v", err)
	}
	list, err := svc.List(context.Background(), contract.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := list.Items[0]
	if got.AttemptNo != 2 || got.LatencyMS != 345 || !got.UsageEstimated || got.TargetProtocol != "openai-compatible" || got.UpstreamRequestID != "upstream_req" || got.APIKeyPrefix != "sk_abc123" {
		t.Fatalf("attempt evidence mismatch: %+v", got)
	}
	if len(got.UpstreamErrors) != 2 || got.UpstreamErrors[0].AccountID == nil || *got.UpstreamErrors[0].AccountID != accountID {
		t.Fatalf("missing upstream history: %+v", got.UpstreamErrors)
	}
	if strings.Contains(got.UpstreamErrors[0].BodyExcerpt, "secret") {
		t.Fatalf("upstream history leaked sensitive token: %q", got.UpstreamErrors[0].BodyExcerpt)
	}
	if got.UpstreamErrors[0].UpstreamStatusCode != 429 || got.UpstreamErrors[1].UpstreamStatusCode != 0 {
		t.Fatalf("unexpected upstream status cleanup: %+v", got.UpstreamErrors)
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
	if err := svc.RecordError(context.Background(), contract.RecordRequest{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("RecordError error = %v, want ErrInvalidInput", err)
	}
	list, _ := svc.List(context.Background(), contract.ListFilter{})
	if len(list.Items) != 0 {
		t.Fatalf("expected empty input to be dropped, got %d items", len(list.Items))
	}
}

func TestRecordError_RejectsInvalidHTTPStatus(t *testing.T) {
	svc := newTestService(t)
	status := 999
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID:    "req-invalid-status",
		StatusCode:   &status,
		ErrorMessage: "impossible upstream status",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("RecordError error = %v, want ErrInvalidInput", err)
	}
	list, _ := svc.List(context.Background(), contract.ListFilter{})
	if len(list.Items) != 0 {
		t.Fatalf("expected invalid status input to be rejected, got %d items", len(list.Items))
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
