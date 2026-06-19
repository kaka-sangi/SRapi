import { describe, expect, it } from "vitest";
import { mapSchedulerDecision } from "@/lib/api/usage";

describe("mapSchedulerDecision", () => {
  it("normalizes OpenAPI scheduler evidence into operator-facing summaries", () => {
    const decision = mapSchedulerDecision({
      id: "77",
      created_at: "2026-06-18T10:00:00Z",
      request_id: "req-map",
      attempt_no: 2,
      model: "gpt-4o-mini",
      source_protocol: "openai-compatible",
      source_endpoint: "/v1/chat/completions",
      target_protocol: "anthropic-compatible",
      strategy: "cost_saver",
      strategy_version: "v4",
      fallback_from_decision_id: "76",
      candidate_count: 3,
      selected_provider_id: "3",
      selected_account_id: "12",
      rejected_count: 2,
      reject_reasons: {
        account_13: "cooldown_active",
        account_14: "capability_mismatch:responses",
      },
      scores: {
        account_12: {
          account_id: 12,
          final_score: 0.91,
          health_score: 0.8,
          quota_score: 0.7,
          latency_score: 0.6,
          cost_score: 0.9,
          quality_score: 0.5,
          sticky_score: 0,
          cache_score: 0.2,
          fairness_score: 1,
          risk_penalty: 0.1,
          saturation_penalty: 0.05,
        },
        account_15: {
          account_id: 15,
          final_score: 0.73,
          health_score: 0.9,
        },
        pareto: {
          frontier_account_ids: [12],
        },
      },
      selection_rationale: "Selected account 12 because cost was best.",
      sticky_hit: false,
      cache_affinity_hit: true,
      estimated_cost: "0.00012000",
      currency: "USD",
      compatibility_warnings: ["strategy_rollout_shadow_selected"],
    });

    expect(decision.id).toBe("77");
    expect(decision.attempt_no).toBe(2);
    expect(decision.fallback_from_decision_id).toBe("76");
    expect(decision.selected_account_name).toBe("#12");
    expect(decision.warnings).toEqual(["strategy_rollout_shadow_selected"]);
    expect(decision.rejected_reasons).toEqual([
      { account_id: "13", account: "#13", reason: "cooldown_active" },
      { account_id: "14", account: "#14", reason: "capability_mismatch:responses" },
    ]);
    expect(decision.scores[0]).toMatchObject({
      account_id: "12",
      account: "#12",
      score: 0.91,
      cost: 0.9,
      pareto_frontier: true,
    });
    expect(decision.scores[1]).toMatchObject({
      account_id: "15",
      account: "#15",
      score: 0.73,
      pareto_frontier: false,
    });
  });

  it("keeps legacy array evidence readable while preferring real OpenAPI field names", () => {
    const decision = mapSchedulerDecision({
      created_at: "2026-06-18T10:00:00Z",
      request_id: "req-legacy",
      model: "gpt-4o-mini",
      source_endpoint: "/v1/messages",
      selected_account_id: null,
      rejected_reasons: [{ account: "legacy-account", reason: "quota_exhausted" }],
      scores: [{ account: "legacy-account", score: 0.2, cost: 0.1, quota: 0 }],
      warnings: ["legacy-warning"],
    });

    expect(decision.id).toBe("req-legacy:1");
    expect(decision.selected_account_name).toBe("None");
    expect(decision.rejected_reasons).toEqual([
      { account_id: undefined, account: "legacy-account", reason: "quota_exhausted" },
    ]);
    expect(decision.scores[0]).toMatchObject({
      account: "legacy-account",
      score: 0.2,
      cost: 0.1,
      quota: 0,
    });
    expect(decision.warnings).toEqual(["legacy-warning"]);
  });
});
