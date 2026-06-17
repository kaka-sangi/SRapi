import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  LOG_WINDOW_ALL_LABEL_KEY,
  LOG_WINDOW_PRESETS,
  logWindowSince,
} from "@/lib/log-window-filter";

// The iter-33 helper is fed user input from the FilterSelect and must:
// 1. Return null for "unset / All time" (so callers fall through).
// 2. Return null for unknown values (defensive — URL params can be junk).
// 3. Return Date(now - minutes) for each known preset.
//
// All three behaviours are wired into pages that render logs, so a quiet
// regression silently breaks "Last 24 hours" → an unbounded page load.
describe("logWindowSince", () => {
  beforeEach(() => {
    // Pin Date.now so we can assert exact cutoff math without flake.
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-06-17T12:00:00.000Z"));
  });
  afterEach(() => vi.useRealTimers());

  it("returns null when the filter is unset", () => {
    expect(logWindowSince(undefined)).toBeNull();
  });

  it("returns null for an unknown preset (defensive — operator-supplied URL)", () => {
    expect(logWindowSince("bogus")).toBeNull();
    expect(logWindowSince("")).toBeNull();
  });

  it.each([
    ["24h", 24 * 60],
    ["7d", 7 * 24 * 60],
    ["30d", 30 * 24 * 60],
  ])("subtracts %s = %d minutes from now", (value, minutes) => {
    const cutoff = logWindowSince(value);
    expect(cutoff).not.toBeNull();
    const expectedMs = Date.now() - minutes * 60 * 1000;
    expect(cutoff!.getTime()).toBe(expectedMs);
  });

  it("exposes the i18n label key constants the FilterSelect needs", () => {
    // Sanity-check the public API surface — touched by every panel that
    // renders the dropdown. The all-time key is read directly by the
    // FilterSelect's allLabel prop.
    expect(LOG_WINDOW_ALL_LABEL_KEY).toBe("adminLogs.allTime");
    expect(LOG_WINDOW_PRESETS.map((p) => p.value)).toEqual(["24h", "7d", "30d"]);
    for (const p of LOG_WINDOW_PRESETS) {
      expect(p.labelKey).toMatch(/^adminLogs\.window/);
    }
  });
});
