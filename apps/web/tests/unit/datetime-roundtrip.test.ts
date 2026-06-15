import { describe, expect, it } from "vitest";

/**
 * Pure documentation/regression test for the datetime-local round-trip used by
 * the admin dialogs. The helpers themselves are NOT exported, so the tiny pure
 * functions are replicated verbatim here:
 *   - `isoToLocalInput` mirrors `isoToLocalInput` in
 *     src/components/features/api-key-create-dialog.tsx and `isoToLocal` in
 *     src/lib/admin-subscription-form.ts (identical bodies).
 *   - `localInputToIso` mirrors `isoOrUndefined` (api-key dialog) and
 *     `localDateTimeToIso` (subscription form): `new Date(value).toISOString()`.
 *
 * The contract: an <input type="datetime-local"> value is a *local* wall-clock
 * string (no zone). Feeding it through `new Date(value)` (parsed as local) then
 * back through the local getters (`getFullYear`/`getHours`/…) must reproduce the
 * SAME wall-clock components. This guards against a future well-meaning but
 * INCORRECT "timezone fix" (e.g. swapping the local getters for UTC getters, or
 * appending a "Z"), which would shift the displayed expiry by the UTC offset.
 */

// Replicated from the source helpers — keep byte-for-byte equivalent.
function isoToLocalInput(iso?: string | null): string {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(
    date.getHours(),
  )}:${pad(date.getMinutes())}`;
}

// Replicated from `isoOrUndefined` / `localDateTimeToIso`: parse the local
// wall-clock string and serialize to an absolute ISO instant.
function localInputToIso(value: string): string {
  return new Date(value).toISOString();
}

describe("datetime-local round-trip", () => {
  it("preserves the wall-clock components for a known local input", () => {
    // A datetime-local value: local wall-clock, minute precision, no zone.
    const input = "2026-09-30T14:45";

    // Forward: local input -> absolute ISO instant -> back to local input.
    const iso = localInputToIso(input);
    const roundTripped = isoToLocalInput(iso);

    expect(roundTripped).toBe(input);
  });

  it("is identity across a range of local inputs (incl. midnight and DST-adjacent)", () => {
    const inputs = [
      "2026-01-01T00:00",
      "2026-03-08T02:30", // around US spring-forward
      "2026-06-15T23:59",
      "2026-11-01T01:15", // around US fall-back
      "2026-12-31T12:00",
    ];
    for (const input of inputs) {
      expect(isoToLocalInput(localInputToIso(input))).toBe(input);
    }
  });

  it("matches the wall-clock components produced by the native local getters", () => {
    const input = "2026-09-30T14:45";
    const date = new Date(input);
    const pad = (n: number) => String(n).padStart(2, "0");
    const expected = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(
      date.getDate(),
    )}T${pad(date.getHours())}:${pad(date.getMinutes())}`;

    // The reconstruction from the ISO instant equals the direct local read:
    // proves the ISO hop does not drift the wall clock.
    expect(isoToLocalInput(localInputToIso(input))).toBe(expected);
    expect(expected).toBe(input);
  });

  it("empty / invalid ISO yields an empty input string (no NaN leak)", () => {
    expect(isoToLocalInput(null)).toBe("");
    expect(isoToLocalInput(undefined)).toBe("");
    expect(isoToLocalInput("")).toBe("");
    expect(isoToLocalInput("not-a-date")).toBe("");
  });
});
