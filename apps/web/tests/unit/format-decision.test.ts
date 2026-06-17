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
    request_id: "req-1",
    model: "gpt-5",
    candidate_count: 3,
    selected_account_name: "acct-a",
    rejected_reasons: [],
    scores: [],
    ...overrides,
  };
}

describe("decisionToLines", () => {
  it("emits exactly 5 numbered lines for a minimal decision (no rejections, no scores)", () => {
    const lines = decisionToLines(
      baseDecision({ selected_account_name: "acct-a" }),
    );
    expect(lines).toHaveLength(5);
    expect(lines[0]).toBe("[1/5] request received  id=req-1 model=gpt-5");
    expect(lines[1]).toBe("[2/5] scheduler         capability ok, candidates=3");
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
          { account: "acct-a", score: 0.875 },
        ],
      }),
    );
    expect(lines[4]).toBe("[5/5] selected          acct-a  score=0.88");
  });

  it("appends up to 4 score detail rows below the selected line", () => {
    const scores = [
      { account: "a", score: 0.9 },
      { account: "b", score: 0.8 },
      { account: "c", score: 0.7 },
      { account: "d", score: 0.6 },
      { account: "e", score: 0.5 },
      { account: "f", score: 0.4 },
    ];
    const lines = decisionToLines(baseDecision({ scores }));
    expect(lines).toHaveLength(5 + 4);
    // Only the first 4 (slice(0, 4)) appear, in the right indent.
    expect(lines.slice(5)).toEqual([
      "                        - a score=0.90",
      "                        - b score=0.80",
      "                        - c score=0.70",
      "                        - d score=0.60",
    ]);
    // The 5th and 6th score must NOT leak through — pin the truncation.
    expect(lines.some((l) => l.includes("- e score"))).toBe(false);
    expect(lines.some((l) => l.includes("- f score"))).toBe(false);
  });
});
