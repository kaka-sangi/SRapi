import { describe, expect, it } from "vitest";
import { buildAccountHealthOpsSummary } from "@/lib/admin-account-health-ops";
import type { AccountHealthSnapshot } from "@/lib/sdk-types";

function health(
  account_id: string,
  patch: Partial<AccountHealthSnapshot> = {},
): AccountHealthSnapshot {
  return {
    account_id,
    provider_id: patch.provider_id ?? "3",
    runtime_class: "api_key",
    status: "healthy",
    success_rate: 0.99,
    error_rate: 0,
    latency_p50_ms: 120,
    latency_p95_ms: 240,
    quota_remaining_ratio: 0.8,
    quota_exhausted: false,
    rate_limit_count: 0,
    timeout_count: 0,
    circuit_state: "closed",
    snapshot_at: "2026-06-18T10:00:00Z",
    ...patch,
  };
}

describe("buildAccountHealthOpsSummary", () => {
  it("groups unhealthy account snapshots by operator action bucket", () => {
    const summary = buildAccountHealthOpsSummary([
      health("healthy"),
      health("open-circuit", { circuit_state: "open", error_class: "timeout" }),
      health("quota", { quota_exhausted: true, error_class: "quota_exceeded" }),
      health("rate", { rate_limit_count: 2, error_class: "rate_limited" }),
      health("timeout", { timeout_count: 1, error_class: "timeout" }),
      health("degraded", { success_rate: 0.72, error_rate: 0.28, error_class: "upstream_error" }),
    ]);

    expect(summary.total).toBe(6);
    expect(summary.healthy).toBe(1);
    expect(summary.attention).toBe(5);
    expect(summary.groups.map((g) => [g.key, g.accountIds])).toEqual([
      ["circuit", ["open-circuit"]],
      ["quota", ["quota"]],
      ["rate_limit", ["rate"]],
      ["timeout", ["timeout"]],
      ["degraded", ["degraded"]],
    ]);
  });

  it("links grouped dominant error classes to exact error log filters", () => {
    const summary = buildAccountHealthOpsSummary([
      health("a", { rate_limit_count: 1, error_class: "rate_limited" }),
      health("b", { rate_limit_count: 1, error_class: "rate_limited" }),
    ]);

    expect(summary.groups).toHaveLength(1);
    expect(summary.groups[0]?.investigationHref).toBe(
      "/admin/logs?tab=error&f_error_class=rate_limited",
    );
  });
});
