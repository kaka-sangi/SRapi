import { describe, expect, it } from "vitest";
import { decisionToLines } from "@/lib/format-decision";
import type { SchedulerDecisionSummary } from "@/lib/srapi-types";

// decisionToLines renders the trace that operators stare at when a request
// gets routed the "wrong" way. The format is fixed by docs/requirements/
// PRODUCT_TONE.md §6.7 — lowercase tags, padded columns, no [INFO]/[SCHEDULER]
// caps. Three branches in particular are worth pinning:
//   - empty rejected_reasons → literal "[4/5] excluded          none"
//   - empty scores → the trailing "score=..." token is stripped (trimEnd)
//   - scores >4 → only the first 4 appear in the indented detail list
// A regression in any of these flips the trace shape an operator visually
// scans, so a "neater format" PR can't quietly change it without breaking
// these tests.

function baseDecision(
  overrides?: Partial<SchedulerDecisionSummary>,
): SchedulerDecisionSummary {
  return {
    id: "dec-1",
    created_at: "2026-01-01T00:00:00Z",
    request_id: "req-1",
    attempt_no: 1,
    model: "gpt-5",
    source_protocol: "openai-compatible",
    source_endpoint: "/v1/chat/completions",
    target_protocol: "openai-compatible",
    strategy: "balanced",
    strategy_version: "v1",
    fallback_from_decision_id: null,
    candidate_count: 3,
    selected_provider_id: "provider-a",
    selected_account_id: "acct-id-a",
    selected_account_name: "acct-a",
    rejected_count: 0,
    rejected_reasons: [],
    scores: [],
    selection_rationale: "",
    sticky_hit: false,
    cache_affinity_hit: false,
    estimated_cost: "0.00000000",
    currency: "USD",
    warnings: [],
    logs: [],
    ...overrides,
  };
}

const SCORE_DEFAULTS = {
  health: 0,
  latency: 0,
  cost: 0,
  quota: 0,
  quality: 0,
  sticky: 0,
  cache: 0,
  fairness: 0,
  risk_penalty: 0,
  saturation_penalty: 0,
  pareto_frontier: false,
};

describe("decisionToLines", () => {
  it("emits exactly 5 numbered lines for a minimal decision (no rejections, no scores)", () => {
    const lines = decisionToLines(
      baseDecision({ selected_account_name: "acct-a" }),
    );
    expect(lines).toHaveLength(5);
    expect(lines[0]).toBe("[1/5] request received  id=req-1 model=gpt-5");
    expect(lines[1]).toBe("[2/5] scheduler         strategy=balanced candidates=3 rejected=0");
    expect(lines[2]).toBe("[3/5] candidates        scoring 3 accounts");
    // empty rejected_reasons → "none" sentinel.
    expect(lines[3]).toBe("[4/5] excluded          none");
    // empty scores → trailing space stripped by trimEnd().
    expect(lines[4]).toBe("[5/5] selected          acct-a");
  });

  it("renders the first rejection inline as 'account reason'", () => {
    const lines = decisionToLines(
      baseDecision({
        rejected_reasons: [
          { account: "acct-b", reason: "rate_limited" },
          { account: "acct-c", reason: "no_capability" },
        ],
      }),
    );
    // Only the FIRST rejection is rendered inline — the rest are dropped
    // here on purpose. Pin that so a "show them all" sweep doesn't quietly
    // explode the trace column width.
    expect(lines[3]).toBe("[4/5] excluded          acct-b rate_limited");
  });

  it("appends top score to the selected line with two fraction digits", () => {
    const lines = decisionToLines(
      baseDecision({
        selected_account_name: "acct-a",
        scores: [
          { account: "acct-a", account_id: "acct-id-a", score: 0.875, ...SCORE_DEFAULTS },
        ],
      }),
    );
    expect(lines[4]).toBe("[5/5] selected          acct-a  score=0.88");
  });

  it("appends up to 4 score detail rows below the selected line", () => {
    const scores: SchedulerDecisionSummary["scores"] = [
      { account: "a", score: 0.9, ...SCORE_DEFAULTS },
      { account: "b", score: 0.8, ...SCORE_DEFAULTS },
      { account: "c", score: 0.7, ...SCORE_DEFAULTS },
      { account: "d", score: 0.6, ...SCORE_DEFAULTS },
      { account: "e", score: 0.5, ...SCORE_DEFAULTS },
      { account: "f", score: 0.4, ...SCORE_DEFAULTS },
    ];
    const lines = decisionToLines(baseDecision({ scores }));
    expect(lines).toHaveLength(5 + 4);
    // Only the first 4 (slice(0, 4)) appear, in the right indent.
    expect(lines.slice(5)).toEqual([
      "                        - a score=0.90 health=0.00 quota=0.00 latency=0.00 cost=0.00",
      "                        - b score=0.80 health=0.00 quota=0.00 latency=0.00 cost=0.00",
      "                        - c score=0.70 health=0.00 quota=0.00 latency=0.00 cost=0.00",
      "                        - d score=0.60 health=0.00 quota=0.00 latency=0.00 cost=0.00",
    ]);
    // The 5th and 6th score must NOT leak through — pin the truncation.
    expect(lines.some((l) => l.includes("- e score"))).toBe(false);
    expect(lines.some((l) => l.includes("- f score"))).toBe(false);
  });
});
