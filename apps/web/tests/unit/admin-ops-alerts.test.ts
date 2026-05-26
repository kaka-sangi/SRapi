import { describe, expect, it } from "vitest";
import { opsAlertSummaryTone, summarizeOpsAlerts } from "@/lib/admin-ops-alerts";
import type { OpsAlertEvent } from "../../../../packages/sdk/typescript/src/types.gen";

function alert(overrides: Partial<OpsAlertEvent>): OpsAlertEvent {
  return {
    id: "alert-1",
    rule_id: "slo.burn_rate.warning",
    severity: "warning",
    status: "firing",
    fingerprint: "fingerprint",
    summary: "Gateway SLO burn rate high",
    details: {},
    started_at: "2026-05-26T00:00:00Z",
    created_at: "2026-05-26T00:00:00Z",
    updated_at: "2026-05-26T00:00:00Z",
    ...overrides,
  };
}

describe("admin-ops-alerts", () => {
  it("summarizes ops alerts by current state", () => {
    expect(
      summarizeOpsAlerts([
        alert({ id: "alert-1", severity: "critical", status: "firing" }),
        alert({ id: "alert-2", severity: "warning", status: "acknowledged" }),
        alert({ id: "alert-3", severity: "ticket", status: "resolved" }),
      ]),
    ).toEqual({
      total: 3,
      firing: 1,
      critical: 1,
      acknowledged: 1,
      resolved: 1,
    });
  });

  it("selects alert posture tones without exposing high-cardinality fields", () => {
    expect(opsAlertSummaryTone(summarizeOpsAlerts([]))).toBe("success");
    expect(opsAlertSummaryTone(summarizeOpsAlerts([alert({ status: "acknowledged" })]))).toBe(
      "neutral",
    );
    expect(
      opsAlertSummaryTone(summarizeOpsAlerts([alert({ severity: "warning", status: "firing" })])),
    ).toBe("warning");
    expect(
      opsAlertSummaryTone(
        summarizeOpsAlerts([alert({ severity: "critical", status: "acknowledged" })]),
      ),
    ).toBe("danger");
  });
});
