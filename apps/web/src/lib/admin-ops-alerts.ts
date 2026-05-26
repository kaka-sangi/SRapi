import type { OpsAlertEvent } from "../../../../packages/sdk/typescript/src/types.gen";

export type OpsAlertSummary = {
  total: number;
  firing: number;
  critical: number;
  acknowledged: number;
  resolved: number;
};

export function summarizeOpsAlerts(alerts: OpsAlertEvent[] | null | undefined): OpsAlertSummary {
  const summary: OpsAlertSummary = {
    total: 0,
    firing: 0,
    critical: 0,
    acknowledged: 0,
    resolved: 0,
  };
  for (const alert of alerts ?? []) {
    summary.total += 1;
    if (alert.status === "firing") {
      summary.firing += 1;
    }
    if (alert.severity === "critical") {
      summary.critical += 1;
    }
    if (alert.status === "acknowledged") {
      summary.acknowledged += 1;
    }
    if (alert.status === "resolved") {
      summary.resolved += 1;
    }
  }
  return summary;
}

export function opsAlertSummaryTone(
  summary: OpsAlertSummary,
): "success" | "warning" | "danger" | "neutral" {
  if (summary.critical > 0) {
    return "danger";
  }
  if (summary.firing > 0) {
    return "warning";
  }
  if (summary.total > 0) {
    return "neutral";
  }
  return "success";
}
