import { describe, expect, it } from "vitest";
import {
  latestQuotaWindows,
  quotaWindowDisplayLabel,
  quotaWindowTiming,
} from "@/lib/quota-display";
import type { AccountQuotaSnapshot } from "@/lib/sdk-types";

function snapshot(input: Partial<AccountQuotaSnapshot> & { quota_type: string }): AccountQuotaSnapshot {
  return {
    account_id: "1",
    provider_id: "2",
    remaining: "100",
    used: "0",
    quota_limit: "100",
    remaining_ratio: 1,
    snapshot_at: "2026-06-11T00:00:00Z",
    ...input,
  };
}

describe("quota display windows", () => {
  it("keeps the latest snapshot per type and orders 5h, 7d, then month", () => {
    const windows = latestQuotaWindows([
      snapshot({
        quota_type: "synthetic_monthly_tokens",
        remaining: "unlimited",
        used: "100",
        quota_limit: "unlimited",
        remaining_ratio: 1,
      }),
      snapshot({
        quota_type: "provider_credits",
        remaining: "900",
        used: "100",
        quota_limit: "1000",
        remaining_ratio: 0.9,
      }),
      snapshot({
        quota_type: "codex_7d_percent",
        remaining: "75",
        used: "25",
        remaining_ratio: 0.75,
      }),
      snapshot({
        quota_type: "codex_5h_percent",
        remaining: "80",
        used: "20",
        remaining_ratio: 0.8,
        snapshot_at: "2026-06-11T00:00:00Z",
      }),
      snapshot({
        quota_type: "codex_5h_percent",
        remaining: "42",
        used: "58",
        remaining_ratio: 0.42,
        snapshot_at: "2026-06-11T01:00:00Z",
      }),
    ]);

    expect(windows.map((window) => window.kind)).toEqual(["5h", "7d", "month"]);
    expect(windows[0].remainingPercent).toBe(42);
  });

  it("uses synthetic monthly quota only when no real provider window exists", () => {
    const windows = latestQuotaWindows([
      snapshot({
        quota_type: "synthetic_monthly_tokens",
        remaining: "unlimited",
        used: "100",
        quota_limit: "unlimited",
        remaining_ratio: 1,
      }),
    ]);

    expect(windows).toHaveLength(1);
    expect(windows[0].kind).toBe("month");
  });

  it("formats translated labels and reset timing", () => {
    const [window] = latestQuotaWindows([
      snapshot({
        quota_type: "codex_5h_percent",
        reset_at: "2026-06-11T05:00:00Z",
      }),
    ]);
    const t = (key: string, vars?: Record<string, string | number>) =>
      `${key}:${vars?.time ?? ""}`;

    expect(quotaWindowDisplayLabel(window, t)).toBe("adminAccounts.quotaWindow5h:");
    expect(quotaWindowTiming(window, t)).toContain("adminAccounts.quotaResetsAt:");
  });
});
