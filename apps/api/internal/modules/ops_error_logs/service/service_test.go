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

func TestListFingerprints_GroupsVariableMessagesAndTracksResolution(t *testing.T) {
	svc := newTestService(t)
	statusBadGateway := 502
	statusRateLimit := 429
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	records := []contract.RecordRequest{{
		OccurredAt:     base.Add(-time.Minute),
		RequestID:      "req_gateway_first",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "codex-mini",
		StatusCode:     &statusBadGateway,
		ErrorClass:     "server_bad",
		ErrorPhase:     "upstream",
		ErrorOwner:     "provider",
		ErrorSource:    "upstream_http",
		ErrorMessage:   "Provider returned 502 for request req_gateway_first after 123ms at https://upstream.example/v1/responses",
	}, {
		OccurredAt:     base,
		RequestID:      "req_gateway_second",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "codex-mini",
		StatusCode:     &statusBadGateway,
		ErrorClass:     "server_bad",
		ErrorPhase:     "upstream",
		ErrorOwner:     "provider",
		ErrorSource:    "upstream_http",
		ErrorMessage:   "Provider returned 503 for request req_gateway_second after 456ms at https://upstream.example/v1/chat",
	}, {
		OccurredAt:     base.Add(-2 * time.Minute),
		RequestID:      "req_rate_limit",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "codex-mini",
		StatusCode:     &statusRateLimit,
		ErrorClass:     "rate_limit",
		ErrorPhase:     "upstream",
		ErrorOwner:     "provider",
		ErrorSource:    "upstream_http",
		ErrorMessage:   "Provider returned 429",
	}}
	for _, rec := range records {
		if err := svc.RecordError(context.Background(), rec); err != nil {
			t.Fatalf("RecordError: %v", err)
		}
	}
	list, err := svc.List(context.Background(), contract.ListFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, entry := range list.Items {
		if entry.RequestID == "req_gateway_first" {
			if _, err := svc.UpdateResolution(context.Background(), contract.UpdateResolutionRequest{
				ID:         entry.ID,
				Resolution: contract.ResolutionInvestigating,
			}); err != nil {
				t.Fatalf("mark investigating: %v", err)
			}
		}
	}

	summary, err := svc.ListFingerprints(context.Background(), contract.FingerprintFilter{
		ListFilter: contract.ListFilter{
			From: ptrTime(base.Add(-time.Hour)),
			To:   ptrTime(base.Add(time.Hour)),
		},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListFingerprints: %v", err)
	}
	if summary.Total != 2 || len(summary.Items) != 2 || summary.Scanned != 3 || summary.Truncated {
		t.Fatalf("unexpected fingerprint summary: %+v", summary)
	}
	group := summary.Items[0]
	if group.Count != 2 || group.OpenCount != 1 || group.InvestigatingCount != 1 {
		t.Fatalf("expected grouped gateway errors with resolution counts, got %+v", group)
	}
	if group.StatusClass != "5xx" || group.StatusCode == nil || *group.StatusCode != statusBadGateway {
		t.Fatalf("unexpected status summary: %+v", group)
	}
	for _, leaked := range []string{"req_gateway_first", "req_gateway_second", "123", "456", "upstream.example"} {
		if strings.Contains(group.MessagePattern, leaked) || strings.Contains(group.Fingerprint, leaked) {
			t.Fatalf("fingerprint leaked variable value %q in %+v", leaked, group)
		}
	}
	if group.MessagePattern != "provider returned {n} for request {request} after {n}ms at {url}" {
		t.Fatalf("unexpected message pattern: %q", group.MessagePattern)
	}
	if group.ExampleRequestID != "req_gateway_second" || group.LastOccurredAt != base {
		t.Fatalf("expected latest row as example, got %+v", group)
	}
}

func TestListFingerprints_DefaultWindowLimitAndTruncation(t *testing.T) {
	store := memory.New()
	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	svc, err := New(store, func() time.Time { return clock })
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	status := 502
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		OccurredAt:     clock.Add(-2 * time.Hour),
		RequestID:      "req_recent",
		StatusCode:     &status,
		ErrorClass:     "server_bad",
		ErrorMessage:   "recent failure",
		SourceEndpoint: "/v1/responses",
	}); err != nil {
		t.Fatalf("RecordError recent: %v", err)
	}
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		OccurredAt:     clock.Add(-48 * time.Hour),
		RequestID:      "req_old",
		StatusCode:     &status,
		ErrorClass:     "server_bad",
		ErrorMessage:   "old failure",
		SourceEndpoint: "/v1/responses",
	}); err != nil {
		t.Fatalf("RecordError old: %v", err)
	}

	summary, err := svc.ListFingerprints(context.Background(), contract.FingerprintFilter{})
	if err != nil {
		t.Fatalf("ListFingerprints: %v", err)
	}
	if summary.Total != 1 || len(summary.Items) != 1 || summary.Items[0].ExampleRequestID != "req_recent" {
		t.Fatalf("expected default 24h window to exclude old row, got %+v", summary)
	}
	if summary.WindowStart == nil || summary.WindowEnd == nil || !summary.WindowStart.Equal(clock.Add(-DefaultFingerprintWindow)) || !summary.WindowEnd.Equal(clock) {
		t.Fatalf("unexpected default window: start=%v end=%v", summary.WindowStart, summary.WindowEnd)
	}

	for i := 0; i < MaxFingerprintScanRows+1; i++ {
		if _, err := store.Insert(context.Background(), contract.Entry{
			OccurredAt:     clock.Add(time.Duration(i+1) * time.Millisecond),
			RequestID:      "req_scan",
			SourceEndpoint: "/v1/chat/completions",
			ErrorClass:     "server_bad",
			ErrorMessage:   "scan capped",
			StatusCode:     &status,
			Resolution:     contract.ResolutionOpen,
		}); err != nil {
			t.Fatalf("insert scan row %d: %v", i, err)
		}
	}
	allStart := clock.Add(-time.Hour)
	allEnd := clock.Add(time.Hour)
	summary, err = svc.ListFingerprints(context.Background(), contract.FingerprintFilter{
		ListFilter: contract.ListFilter{From: &allStart, To: &allEnd},
		Limit:      MaxFingerprintLimit + 1,
	})
	if err != nil {
		t.Fatalf("ListFingerprints capped scan: %v", err)
	}
	if summary.Scanned != MaxFingerprintScanRows || !summary.Truncated {
		t.Fatalf("expected truncated capped scan, got scanned=%d truncated=%v", summary.Scanned, summary.Truncated)
	}
	if len(summary.Items) > MaxFingerprintLimit {
		t.Fatalf("limit was not capped: %d", len(summary.Items))
	}
}

func TestUpdateResolution_RedactsNoteAndValidatesResolverID(t *testing.T) {
	svc := newTestService(t)
	status := 502
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID:    "req-resolution",
		StatusCode:   &status,
		ErrorMessage: "provider returned 502",
	}); err != nil {
		t.Fatalf("RecordError: %v", err)
	}
	list, err := svc.List(context.Background(), contract.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	id := list.Items[0].ID
	resolverID := 17
	updated, err := svc.UpdateResolution(context.Background(), contract.UpdateResolutionRequest{
		ID:           id,
		Resolution:   contract.ResolutionInvestigating,
		Note:         "checked Authorization: Bearer note-token api_key=note-key",
		ResolvedByID: &resolverID,
	})
	if err != nil {
		t.Fatalf("UpdateResolution: %v", err)
	}
	if strings.Contains(updated.ResolutionNote, "note-token") || strings.Contains(updated.ResolutionNote, "note-key") {
		t.Fatalf("resolution note leaked secret: %q", updated.ResolutionNote)
	}
	if !strings.Contains(updated.ResolutionNote, "Bearer [REDACTED]") || !strings.Contains(updated.ResolutionNote, "api_key=[REDACTED]") {
		t.Fatalf("expected redaction markers in resolution note, got %q", updated.ResolutionNote)
	}

	invalidResolverID := 0
	if _, err := svc.UpdateResolution(context.Background(), contract.UpdateResolutionRequest{
		ID:           id,
		Resolution:   contract.ResolutionResolved,
		ResolvedByID: &invalidResolverID,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("UpdateResolution invalid resolver = %v, want ErrInvalidInput", err)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func TestRecordError_SanitizesIndexedFields(t *testing.T) {
	svc := newTestService(t)
	status := 502
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID:      "  req\n\tindexed  ",
		TraceID:        "  trace\r\nindexed  ",
		APIKeyPrefix:   "sk_abcdef1234567890abcdef1234567890",
		Platform:       "  openai-compatible\n\t",
		SourceEndpoint: "  /v1/responses\r\n",
		TargetProtocol: "  openai-compatible\n\t",
		Model:          "  codex-mini\n\t",
		StatusCode:     &status,
		ErrorMessage:   "provider returned 502",
	}); err != nil {
		t.Fatalf("RecordError: %v", err)
	}
	list, err := svc.List(context.Background(), contract.ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := list.Items[0]
	if got.RequestID != "req indexed" || got.TraceID != "trace indexed" {
		t.Fatalf("expected cleaned identifiers, got request_id=%q trace_id=%q", got.RequestID, got.TraceID)
	}
	if got.Platform != "openai-compatible" || got.SourceEndpoint != "/v1/responses" || got.TargetProtocol != "openai-compatible" || got.Model != "codex-mini" {
		t.Fatalf("expected cleaned indexed fields, got %+v", got)
	}
	if got.APIKeyPrefix != "sk_[REDACTED]" {
		t.Fatalf("expected api key prefix to be redacted, got %q", got.APIKeyPrefix)
	}
}

func TestRecordError_PreservesStructuredAttemptEvidence(t *testing.T) {
	svc := newTestService(t)
	status := 429
	accountID := 42
	if err := svc.RecordError(context.Background(), contract.RecordRequest{
		RequestID:             "req-attempt",
		StatusCode:            &status,
		APIKeyPrefix:          "sk_abc123",
		TargetProtocol:        "openai-compatible",
		UpstreamRequestID:     "upstream_req",
		StreamCompletionState: "interrupted",
		AttemptNo:             2,
		LatencyMS:             345,
		InputTokens:           12,
		OutputTokens:          3,
		UsageEstimated:        true,
		ErrorOwner:            "provider",
		ErrorSource:           "upstream_http",
		ErrorMessage:          "rate limited",
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
	if got.AttemptNo != 2 || got.LatencyMS != 345 || !got.UsageEstimated || got.TargetProtocol != "openai-compatible" || got.UpstreamRequestID != "upstream_req" || got.StreamCompletionState != "interrupted" || got.APIKeyPrefix != "sk_abc123" {
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
