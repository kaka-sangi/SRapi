import { describe, expect, it } from "vitest";
import {
  clampPercent,
  formatInteger,
  formatMoney,
  formatPercent,
  safeJson,
  statusBadgeVariant,
} from "@/lib/admin-format";

describe("admin-format", () => {
  it("formats missing numeric values without inventing data", () => {
    expect(formatInteger(undefined)).toBe("-");
    expect(formatMoney(null)).toBe("-");
  });

  it("formats decimal string money values", () => {
    expect(formatMoney("12.34567", "USD")).toBe("$12.3457");
  });

  it("formats ratio percentages consistently", () => {
    expect(formatPercent(0.0244)).toBe("2.44%");
    expect(formatPercent(0.5)).toBe("50.0%");
  });

  it("maps production statuses to badge tones", () => {
    expect(statusBadgeVariant("active")).toBe("success");
    expect(statusBadgeVariant("needs_reauth")).toBe("warning");
    expect(statusBadgeVariant("dead")).toBe("danger");
    expect(statusBadgeVariant("configured")).toBe("neutral");
  });

  it("keeps chart widths bounded", () => {
    expect(clampPercent(-20)).toBe(0);
    expect(clampPercent(42)).toBe(42);
    expect(clampPercent(150)).toBe(100);
  });

  it("serializes JSON-like metadata for admin tables", () => {
    expect(safeJson({ model: "gpt-4o" })).toContain('"model": "gpt-4o"');
    expect(safeJson(undefined)).toBe("{}");
  });
});
