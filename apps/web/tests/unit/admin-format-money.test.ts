import { describe, expect, it } from "vitest";
import { formatMoney, formatPercent, formatLatency } from "@/lib/admin-format";

// These three formatters surface on every dashboard, ledger, and usage
// table — formatMoney especially is what an operator sees for every
// charge / refund / payout. Pinning the null-handling and the non-
// finite fallback so a regression doesn't ship "NaN $USD" or similar.
describe("formatMoney", () => {
  it("renders an em-dash for null / undefined / empty string", () => {
    expect(formatMoney(null)).toBe("-");
    expect(formatMoney(undefined)).toBe("-");
    expect(formatMoney("")).toBe("-");
  });

  it("formats a numeric value as currency with up to 4 fraction digits", () => {
    // Intl output varies by locale — sniff for the substring rather than
    // an exact match so the test runs in any CI runner.
    const out = formatMoney(12.345);
    expect(out).toMatch(/\$|USD/);
    expect(out).toContain("12.345");
  });

  it("respects an explicit currency code", () => {
    const out = formatMoney(99, "EUR");
    expect(out).toMatch(/€|EUR/);
  });

  it("falls back to the raw value when the input is non-finite", () => {
    // "abc" parses to NaN — must not crash and must surface SOMETHING
    // with the currency tag so operators can spot bad upstream data.
    expect(formatMoney("abc")).toBe("abc USD");
    expect(formatMoney(Number.POSITIVE_INFINITY)).toBe("Infinity USD");
  });

  it("accepts a string that parses to a finite number", () => {
    // The admin API returns money as decimal strings; the formatter must
    // accept them transparently.
    expect(formatMoney("42.5")).toContain("42.5");
  });
});

describe("formatPercent", () => {
  it("renders an em-dash for null / undefined / non-finite", () => {
    expect(formatPercent(null)).toBe("-");
    expect(formatPercent(undefined)).toBe("-");
    expect(formatPercent(Number.NaN)).toBe("-");
  });

  it("formats a 0-1 ratio as a percent string", () => {
    // Used by uptime / success_rate columns — values come in as 0-1.
    const out = formatPercent(0.875);
    expect(out).toMatch(/87/);
    expect(out).toContain("%");
  });
});

describe("formatLatency", () => {
  it("renders the literal em-dash for null / undefined / non-finite", () => {
    // Note: formatLatency returns the em-dash "—" (U+2014), while
    // formatMoney/formatPercent return the ASCII "-". Asymmetric on
    // purpose? Possibly a stylistic legacy, but locking it in so a
    // future "normalise dash" sweep doesn't quietly change the column.
    expect(formatLatency(null)).toBe("—");
    expect(formatLatency(undefined)).toBe("—");
    expect(formatLatency(0)).toBe("—"); // !ms truthiness check treats 0 as empty
    expect(formatLatency(Number.NaN)).toBe("—");
  });

  it("renders integer ms under 1s", () => {
    expect(formatLatency(120)).toBe("120ms");
    expect(formatLatency(999.4)).toBe("999ms"); // rounds
  });

  it("renders seconds with one fraction digit at >= 1000ms", () => {
    expect(formatLatency(1000)).toBe("1.0s");
    expect(formatLatency(1234)).toBe("1.2s");
  });
});
