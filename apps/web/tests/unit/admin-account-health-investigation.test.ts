import { describe, expect, it } from "vitest";
import {
  accountHealthNeedsInvestigation,
  adminAccountHealthInvestigationHref,
  adminAccountHealthInvestigationLinks,
} from "@/lib/admin-account-health-investigation";
import type { AccountHealthSnapshot } from "@/lib/sdk-types";

const healthy: AccountHealthSnapshot = {
  account_id: "12",
  provider_id: "3",
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
};

describe("account health investigation links", () => {
  it("does not link healthy snapshots", () => {
    expect(accountHealthNeedsInvestigation(healthy)).toBe(false);
    expect(adminAccountHealthInvestigationHref(healthy)).toBeNull();
    expect(adminAccountHealthInvestigationLinks(healthy)).toBeNull();
  });

  it("links abnormal snapshots to exact cross-plane evidence filters", () => {
    const degraded: AccountHealthSnapshot = {
      ...healthy,
      success_rate: 0.72,
      error_rate: 0.28,
      error_class: "rate_limited",
    };

    expect(accountHealthNeedsInvestigation(degraded)).toBe(true);
    expect(adminAccountHealthInvestigationHref(degraded)).toBe(
      "/admin/logs?tab=error&f_account=12&f_provider=3&f_error_class=rate_limited",
    );
    expect(adminAccountHealthInvestigationLinks(degraded)).toEqual({
      errorLogs: "/admin/logs?tab=error&f_account=12&f_provider=3&f_error_class=rate_limited",
      requestEvidence:
        "/admin/logs?tab=request-evidence&f_account_id=12&f_provider_id=3&f_error_class=rate_limited",
      schedulerDecisions: "/admin/ops?tab=scheduler-decisions&f_account_id=12",
    });
  });
});
