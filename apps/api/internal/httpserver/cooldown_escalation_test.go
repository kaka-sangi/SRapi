package httpserver

import (
	"testing"
	"time"
)

func TestEscalateCooldownWindow(t *testing.T) {
	base := rateLimitCooldownWindow
	now := time.Unix(1_000_000, 0).UTC()
	recent := now.Add(-time.Minute)

	// First failure: base window, strike count advances to 1.
	if w, next := escalateCooldownWindow(base, 0, nil, now); w != base || next != 1 {
		t.Fatalf("first failure: got window=%s strikes=%d want %s/1", w, next, base)
	}
	// Consecutive failures double the window.
	if w, next := escalateCooldownWindow(base, 1, &recent, now); w != base*2 || next != 2 {
		t.Fatalf("second failure: got window=%s strikes=%d want %s/2", w, next, base*2)
	}
	if w, _ := escalateCooldownWindow(base, 3, &recent, now); w != base*8 {
		t.Fatalf("fourth failure: got window=%s want %s", w, base*8)
	}
	// The shift exponent is capped, so the 30s base maxes out at base<<6 (32m).
	if w, _ := escalateCooldownWindow(base, 100, &recent, now); w != base<<maxCooldownStrikeShift {
		t.Fatalf("shift cap: got window=%s want %s", w, base<<maxCooldownStrikeShift)
	}
	// A larger base hits the absolute 2h ceiling.
	if w, _ := escalateCooldownWindow(overloadCooldownWindow, 100, &recent, now); w != maxGatewayConfiguredCooldownWindow {
		t.Fatalf("absolute cap: got window=%s want %s", w, maxGatewayConfiguredCooldownWindow)
	}
	// A long gap since the last cooldown resets strikes back to the base window.
	stale := now.Add(-cooldownStrikeResetAfter - time.Minute)
	if w, next := escalateCooldownWindow(base, 5, &stale, now); w != base || next != 1 {
		t.Fatalf("reset after gap: got window=%s strikes=%d want %s/1", w, next, base)
	}
}
