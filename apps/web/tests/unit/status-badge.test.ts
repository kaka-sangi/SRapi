import { describe, expect, it } from "vitest";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";

// quietStatusFor maps every backend status string surfaced anywhere in
// the admin to one of four QuietBadge tones (active/limited/error/
// disabled). The class lists are load-bearing — every operator depends
// on "paid" rendering green, "refund_failed" rendering red, etc. A
// regression where one of these tokens silently drops off the list
// would make a real outage look identical to a healthy row.
describe("quietStatusFor", () => {
  it("maps healthy / open / running statuses to 'active'", () => {
    for (const s of [
      "active",
      "published",
      "paid",
      "fulfilled",
      "resolved",
      "ok",
      "healthy",
      "enabled",
      "enforce",
    ]) {
      expect(quietStatusFor(s)).toBe("active");
    }
  });

  it("maps warning / in-progress statuses to 'limited'", () => {
    for (const s of [
      "limited",
      "pending",
      "draft",
      "suspended",
      "firing",
      "warning",
      "warn",
      "burning",
      "monitor",
      "refunding",
    ]) {
      expect(quietStatusFor(s)).toBe("limited");
    }
  });

  it("maps failure / terminal-negative statuses to 'error'", () => {
    for (const s of [
      "failed",
      "refund_failed",
      "error",
      "breached",
      "critical",
      "canceled",
      "cancelled", // both spellings — pin so a "tidy up" PR doesn't drop one
      "refunded",
      "dead",
      "block",
    ]) {
      expect(quietStatusFor(s)).toBe("error");
    }
  });

  it("is case-insensitive on the input", () => {
    expect(quietStatusFor("ACTIVE")).toBe("active");
    expect(quietStatusFor("Failed")).toBe("error");
    expect(quietStatusFor("Pending")).toBe("limited");
  });

  it("falls back to 'disabled' for unknown / null / empty inputs", () => {
    expect(quietStatusFor(null)).toBe("disabled");
    expect(quietStatusFor(undefined)).toBe("disabled");
    expect(quietStatusFor("")).toBe("disabled");
    expect(quietStatusFor("totally-made-up")).toBe("disabled");
  });
});

// statusLabel converts a backend enum into a localised display label.
// The miss-fallback (humanize underscores → spaces + Title Case) is
// what stops "needs_reauth" leaking into the UI as a raw token; the
// blank fallback returns the em-dash "—".
describe("statusLabel", () => {
  // Faux translate that mirrors how the real i18n returns the dotted key
  // when there's no message — that's the "miss" sentinel statusLabel keys off.
  const tMiss = (key: string) => key;
  const tHit = (key: string) =>
    key === "status.active" ? "Active (translated)" : key;

  it("returns em-dash '—' for empty / null / whitespace-only input", () => {
    expect(statusLabel(tMiss, null)).toBe("—");
    expect(statusLabel(tMiss, undefined)).toBe("—");
    expect(statusLabel(tMiss, "")).toBe("—");
    expect(statusLabel(tMiss, "   ")).toBe("—");
  });

  it("returns the translated label when i18n has the key", () => {
    expect(statusLabel(tHit, "active")).toBe("Active (translated)");
  });

  it("humanises underscores and title-cases when i18n misses", () => {
    // "needs_reauth" → "Needs Reauth", NOT a leaked raw enum. This is
    // the load-bearing fallback that keeps internal tokens out of the UI.
    expect(statusLabel(tMiss, "needs_reauth")).toBe("Needs Reauth");
    expect(statusLabel(tMiss, "refund_failed")).toBe("Refund Failed");
  });

  it("title-cases a single word too", () => {
    expect(statusLabel(tMiss, "burning")).toBe("Burning");
  });
});
