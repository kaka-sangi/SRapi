package httpserver

import (
	"net/http"
	"testing"
	"time"
)

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
