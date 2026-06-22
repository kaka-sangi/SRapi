interface SchedulerRejectReasonCount {
  reason: string;
  count: number;
}

export interface SchedulerDiagnostic {
  decisionId?: number;
  candidateCount?: number;
  rejectedCount?: number;
  primaryReason?: string;
  primaryCount?: number;
  operatorAction?: string;
  responseStatus?: number;
  selectionRationale?: string;
  reasonCounts: SchedulerRejectReasonCount[];
}

export interface CompactSchedulerDiagnostic {
  reason: string;
  count?: number;
  action?: string;
}

/** Parse scheduler no-account evidence stored in ops_error_logs.error_body_excerpt. */
export function parseSchedulerDiagnostic(value?: string | null): SchedulerDiagnostic | null {
  if (!value) return null;
  try {
    const parsed = JSON.parse(value) as Record<string, unknown>;
    const reasonCounts = schedulerReasonCounts(parsed.scheduler_reject_reason_counts);
    const diagnostic: SchedulerDiagnostic = {
      decisionId: numericField(parsed.scheduler_decision_id),
      candidateCount: numericField(parsed.scheduler_candidate_count),
      rejectedCount: numericField(parsed.scheduler_rejected_count),
      primaryReason: stringField(parsed.scheduler_primary_reject_reason),
      primaryCount: numericField(parsed.scheduler_primary_reject_count),
      operatorAction: stringField(parsed.scheduler_operator_action),
      responseStatus: numericField(parsed.response_status),
      selectionRationale: stringField(parsed.scheduler_selection_rationale),
      reasonCounts,
    };
    if (
      diagnostic.decisionId == null &&
      diagnostic.primaryReason == null &&
      diagnostic.operatorAction == null &&
      diagnostic.reasonCounts.length === 0
    ) {
      return null;
    }
    return diagnostic;
  } catch {
    return null;
  }
}

/** Return the minimum scheduler evidence needed for dense table cells. */
export function compactSchedulerDiagnostic(
  value?: string | null,
): CompactSchedulerDiagnostic | null {
  const diagnostic = parseSchedulerDiagnostic(value);
  if (!diagnostic) return null;
  const reason = diagnostic.primaryReason || diagnostic.reasonCounts[0]?.reason;
  if (!reason && !diagnostic.operatorAction) return null;
  return {
    reason: reason || "scheduler_rejected",
    count: diagnostic.primaryCount,
    action: diagnostic.operatorAction,
  };
}

function schedulerReasonCounts(value: unknown): SchedulerRejectReasonCount[] {
  if (!value || typeof value !== "object" || Array.isArray(value)) return [];
  return Object.entries(value as Record<string, unknown>)
    .map(([reason, rawCount]) => ({
      reason,
      count: numericField(rawCount) ?? 0,
    }))
    .filter((item) => item.reason.trim() !== "" && item.count > 0)
    .sort((a, b) => b.count - a.count || a.reason.localeCompare(b.reason));
}

function numericField(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return undefined;
}

function stringField(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value.trim() : undefined;
}
