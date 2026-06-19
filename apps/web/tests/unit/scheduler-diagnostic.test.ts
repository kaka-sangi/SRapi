import { describe, expect, it } from "vitest";
import {
  compactSchedulerDiagnostic,
  parseSchedulerDiagnostic,
} from "@/lib/scheduler-diagnostic";

describe("scheduler diagnostics", () => {
  it("parses no-account scheduler evidence", () => {
    const diagnostic = parseSchedulerDiagnostic(
      JSON.stringify({
        response_status: "503",
        scheduler_decision_id: 77,
        scheduler_candidate_count: "3",
        scheduler_rejected_count: 3,
        scheduler_primary_reject_reason: "capability_mismatch:responses",
        scheduler_primary_reject_count: "2",
        scheduler_operator_action: "check_model_capabilities_or_mapping",
        scheduler_reject_reason_counts: {
          cooldown_active: 1,
          "capability_mismatch:responses": 2,
        },
        scheduler_selection_rationale: "no account satisfied responses capability",
      }),
    );

    expect(diagnostic).toEqual({
      responseStatus: 503,
      decisionId: 77,
      candidateCount: 3,
      rejectedCount: 3,
      primaryReason: "capability_mismatch:responses",
      primaryCount: 2,
      operatorAction: "check_model_capabilities_or_mapping",
      reasonCounts: [
        { reason: "capability_mismatch:responses", count: 2 },
        { reason: "cooldown_active", count: 1 },
      ],
      selectionRationale: "no account satisfied responses capability",
    });
  });

  it("returns compact evidence for table cells", () => {
    expect(
      compactSchedulerDiagnostic(
        JSON.stringify({
          scheduler_primary_reject_reason: "cooldown_active",
          scheduler_primary_reject_count: 4,
          scheduler_operator_action: "wait_for_cooldown_or_enable_capacity",
        }),
      ),
    ).toEqual({
      reason: "cooldown_active",
      count: 4,
      action: "wait_for_cooldown_or_enable_capacity",
    });
  });

  it("ignores unrelated or malformed excerpts", () => {
    expect(parseSchedulerDiagnostic("upstream failed")).toBeNull();
    expect(parseSchedulerDiagnostic(JSON.stringify({ response_status: 503 }))).toBeNull();
    expect(compactSchedulerDiagnostic(null)).toBeNull();
  });
});
