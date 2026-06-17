import { describe, expect, it } from "vitest";
import { rateLimitSummary } from "@/lib/rate-limit-format";

// Compact one-line rate-limit summary that ends up in account/key list rows
// and admin tables. The all-zero "∞" rendering and the per-dimension drop
// when value is 0 are load-bearing — operators read the table at a glance,
// so a regression that started rendering "R 0 · T 0 · C 0" instead of "∞"
// would silently terrorise the team.
describe("rateLimitSummary", () => {
  it("renders ∞ when every dimension is zero (unlimited)", () => {
    expect(
      rateLimitSummary({ rpm_limit: 0, tpm_limit: 0, max_concurrency: 0 }),
    ).toBe("∞");
  });

  it("drops zero dimensions and keeps non-zero ones with · separator", () => {
    expect(
      rateLimitSummary({ rpm_limit: 60, tpm_limit: 0, max_concurrency: 5 }),
    ).toBe("R 60 · C 5");
  });

  it("formats sub-1k counts as integers, no suffix", () => {
    expect(
      rateLimitSummary({ rpm_limit: 999, tpm_limit: 0, max_concurrency: 0 }),
    ).toBe("R 999");
  });

  it("formats 1k-1M as 'k' with one fraction digit when non-integer", () => {
    // 1500 -> 1.5k, but 2000 -> 2k (no trailing ".0").
    expect(
      rateLimitSummary({ rpm_limit: 1500, tpm_limit: 2000, max_concurrency: 0 }),
    ).toBe("R 1.5k · T 2k");
  });

  it("formats >=1M as 'M' with one fraction digit when non-integer", () => {
    expect(
      rateLimitSummary({
        rpm_limit: 1_500_000,
        tpm_limit: 2_000_000,
        max_concurrency: 0,
      }),
    ).toBe("R 1.5M · T 2M");
  });

  it("never abbreviates max_concurrency — it's always a small integer", () => {
    // C never crosses 1000 in practice; the function leaves it as a raw
    // string to keep that field literal. Pin the contract.
    expect(
      rateLimitSummary({
        rpm_limit: 0,
        tpm_limit: 0,
        max_concurrency: 1500,
      }),
    ).toBe("C 1500");
  });

  it("renders all three dimensions in fixed R · T · C order", () => {
    // Order matters for visual scanning — pin it.
    expect(
      rateLimitSummary({
        rpm_limit: 60,
        tpm_limit: 120_000,
        max_concurrency: 5,
      }),
    ).toBe("R 60 · T 120k · C 5");
  });
});
