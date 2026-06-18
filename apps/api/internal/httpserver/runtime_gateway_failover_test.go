package httpserver

import (
	"net/http"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

// newTestProviderError builds a synthetic provider error matching the contract
// used by ClassifyUpstreamError + providerErrorBodyExcerpt.
func newTestProviderError(status int, message string, headers http.Header, metadata map[string]any) error {
	return provideradaptercontract.ProviderError{
		Class:      "upstream_http",
		StatusCode: status,
		Message:    message,
		Headers:    headers,
		Metadata:   metadata,
	}
}

// newTestScheduleResultForAttempt builds a ScheduleResult populated with the
// minimum candidate / decision fields the per-attempt event builder reads.
func newTestScheduleResultForAttempt(attemptNo int, accountID int, accountName string, _ string) schedulercontract.ScheduleResult {
	return schedulercontract.ScheduleResult{
		Decision: schedulercontract.Decision{AttemptNo: attemptNo},
		Candidate: schedulercontract.Candidate{
			Account: accountcontract.ProviderAccount{ID: accountID, Name: accountName},
		},
	}
}

// TestCodexCooldownMetadataUpdates_ParsesAndNormalizesHeaders verifies the
// faithfully-ported sub2api logic: raw primary/secondary x-codex-* fields are
// preserved and the windows are normalized into the canonical 5h/7d fields by
// comparing window-minutes (smaller = 5h, larger = 7d). Reset-after values are
// also projected to absolute RFC3339 timestamps off the supplied base time.
func TestCodexCooldownMetadataUpdates_ParsesAndNormalizesHeaders(t *testing.T) {
	base := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	headers := http.Header{}
	// Primary = 5h window (300 min), secondary = 7d window (10080 min).
	headers.Set("x-codex-primary-used-percent", "42.5")
	headers.Set("x-codex-primary-reset-after-seconds", "1800")
	headers.Set("x-codex-primary-window-minutes", "300")
	headers.Set("x-codex-secondary-used-percent", "10")
	headers.Set("x-codex-secondary-reset-after-seconds", "3600")
	headers.Set("x-codex-secondary-window-minutes", "10080")
	headers.Set("x-codex-primary-over-secondary-limit-percent", "7.25")

	updates := codexCooldownMetadataUpdates(headers, base)
	if updates == nil {
		t.Fatalf("expected updates, got nil")
	}

	// Raw fields preserved verbatim.
	expectFloat(t, updates, "codex_primary_used_percent", 42.5)
	expectInt(t, updates, "codex_primary_reset_after_seconds", 1800)
	expectInt(t, updates, "codex_primary_window_minutes", 300)
	expectFloat(t, updates, "codex_secondary_used_percent", 10)
	expectInt(t, updates, "codex_secondary_reset_after_seconds", 3600)
	expectInt(t, updates, "codex_secondary_window_minutes", 10080)
	expectFloat(t, updates, "codex_primary_over_secondary_percent", 7.25)

	// Primary has the smaller window so it normalizes to the 5h slot.
	expectFloat(t, updates, "codex_5h_used_percent", 42.5)
	expectInt(t, updates, "codex_5h_reset_after_seconds", 1800)
	expectInt(t, updates, "codex_5h_window_minutes", 300)
	expectFloat(t, updates, "codex_7d_used_percent", 10)
	expectInt(t, updates, "codex_7d_reset_after_seconds", 3600)
	expectInt(t, updates, "codex_7d_window_minutes", 10080)

	// Absolute reset timestamps projected off the base time.
	expectString(t, updates, "codex_5h_reset_at", base.Add(1800*time.Second).Format(time.RFC3339))
	expectString(t, updates, "codex_7d_reset_at", base.Add(3600*time.Second).Format(time.RFC3339))
	expectString(t, updates, "codex_usage_updated_at", base.Format(time.RFC3339))
}

// TestCodexCooldownMetadataUpdates_LegacyNoWindowMinutes covers the legacy Codex
// response shape that omits window-minutes: primary is treated as 7d and the
// secondary window as the short 5h window.
func TestCodexCooldownMetadataUpdates_LegacyNoWindowMinutes(t *testing.T) {
	base := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", "80")
	headers.Set("x-codex-secondary-used-percent", "15")

	updates := codexCooldownMetadataUpdates(headers, base)
	if updates == nil {
		t.Fatalf("expected updates, got nil")
	}
	expectFloat(t, updates, "codex_7d_used_percent", 80)
	expectFloat(t, updates, "codex_5h_used_percent", 15)
	if _, ok := updates["codex_5h_reset_at"]; ok {
		t.Fatalf("did not expect codex_5h_reset_at without reset-after header")
	}
}

// TestCodexCooldownMetadataUpdates_NoHeaders returns nil so the cooldown stage
// skips the metadata write entirely.
func TestCodexCooldownMetadataUpdates_NoHeaders(t *testing.T) {
	if got := codexCooldownMetadataUpdates(http.Header{}, time.Now()); got != nil {
		t.Fatalf("expected nil for empty headers, got %v", got)
	}
	if got := codexCooldownMetadataUpdates(nil, time.Now()); got != nil {
		t.Fatalf("expected nil for nil headers, got %v", got)
	}
	// Unrelated headers must not trigger a write.
	other := http.Header{}
	other.Set("Content-Type", "application/json")
	if got := codexCooldownMetadataUpdates(other, time.Now()); got != nil {
		t.Fatalf("expected nil for unrelated headers, got %v", got)
	}
}

func expectFloat(t *testing.T, updates map[string]any, key string, want float64) {
	t.Helper()
	got, ok := updates[key].(float64)
	if !ok {
		t.Fatalf("key %q: expected float64, got %T (%v)", key, updates[key], updates[key])
	}
	if got != want {
		t.Fatalf("key %q: got %v, want %v", key, got, want)
	}
}

func expectInt(t *testing.T, updates map[string]any, key string, want int) {
	t.Helper()
	got, ok := updates[key].(int)
	if !ok {
		t.Fatalf("key %q: expected int, got %T (%v)", key, updates[key], updates[key])
	}
	if got != want {
		t.Fatalf("key %q: got %v, want %v", key, got, want)
	}
}

func expectString(t *testing.T, updates map[string]any, key string, want string) {
	t.Helper()
	got, ok := updates[key].(string)
	if !ok {
		t.Fatalf("key %q: expected string, got %T (%v)", key, updates[key], updates[key])
	}
	if got != want {
		t.Fatalf("key %q: got %q, want %q", key, got, want)
	}
}

// TestBuildGatewayUpstreamErrorEvent verifies the per-attempt event builder
// captures the upstream status, request id (extracted from x-request-id /
// openai-request-id headers) and chooses kind=http_error vs request_error.
func TestBuildGatewayUpstreamErrorEvent(t *testing.T) {
	// Build a synthetic provider error with headers carrying x-request-id +
	// metadata so providerErrorBodyExcerpt produces a non-empty excerpt.
	providerErr := newTestProviderError(503, "boom", http.Header{
		"X-Request-Id":         []string{"req-abc"},
		"Anthropic-Request-Id": []string{"a-z"},
	}, map[string]any{"type": "overloaded_error"})

	result := newTestScheduleResultForAttempt(1, 7, "acct-7", "model-x")
	ev := buildGatewayUpstreamErrorEvent(1, result, providerErr, 503)
	if ev.AttemptNo != 1 {
		t.Fatalf("attempt_no: got %d want 1", ev.AttemptNo)
	}
	if ev.UpstreamStatusCode != 503 {
		t.Fatalf("status: got %d want 503", ev.UpstreamStatusCode)
	}
	if ev.UpstreamRequestID != "req-abc" {
		t.Fatalf("upstream req id: got %q want req-abc", ev.UpstreamRequestID)
	}
	if ev.Kind != "http_error" {
		t.Fatalf("kind: got %q want http_error", ev.Kind)
	}
	if ev.AccountID == nil || *ev.AccountID != 7 {
		t.Fatalf("account id: got %v want 7", ev.AccountID)
	}
	if ev.AccountName != "acct-7" {
		t.Fatalf("account name: got %q want acct-7", ev.AccountName)
	}
	if ev.Message == "" {
		t.Fatalf("expected non-empty message")
	}
	if ev.BodyExcerpt == "" {
		t.Fatalf("expected non-empty body excerpt")
	}
	if ev.AtUnixMs <= 0 {
		t.Fatalf("expected positive at_unix_ms")
	}

	// Transport-only failure (statusCode == 0) -> kind=request_error.
	transientErr := newTestProviderError(0, "dial tcp: i/o timeout", nil, nil)
	ev2 := buildGatewayUpstreamErrorEvent(2, result, transientErr, 0)
	if ev2.Kind != "request_error" {
		t.Fatalf("transport kind: got %q want request_error", ev2.Kind)
	}
	if ev2.UpstreamStatusCode != 0 {
		t.Fatalf("transport status: got %d want 0", ev2.UpstreamStatusCode)
	}
}
