import type { SchedulerDecisionSummary } from "./srapi-types";

/**
 * Render a scheduler decision into the readable, lowercase-tagged trace format
 * defined in docs/requirements/PRODUCT_TONE.md §6.7 (no [INFO]/[SCHEDULER] caps tags).
 */
export function decisionToLines(decision: SchedulerDecisionSummary): string[] {
  const lines: string[] = [];
  lines.push(`[1/5] request received  id=${decision.request_id} model=${decision.model}`);
  lines.push(`[2/5] scheduler         capability ok, candidates=${decision.candidate_count}`);
  lines.push(`[3/5] candidates        scoring ${decision.candidate_count} accounts`);

  const firstRejection = decision.rejected_reasons[0];
  if (firstRejection) {
    lines.push(`[4/5] excluded          ${firstRejection.account} ${firstRejection.reason}`);
  } else {
    lines.push(`[4/5] excluded          none`);
  }

  const topScore = decision.scores[0];
  const scoreText = topScore ? `score=${topScore.score.toFixed(2)}` : "";
  lines.push(`[5/5] selected          ${decision.selected_account_name}  ${scoreText}`.trimEnd());

  for (const s of decision.scores.slice(0, 4)) {
    lines.push(`                        - ${s.account} score=${s.score.toFixed(2)}`);
  }
  return lines;
}
