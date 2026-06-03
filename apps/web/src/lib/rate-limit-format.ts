/** Compact one-line summary of a rate limit for list rows, e.g. "R 60 · T 120k · C 5".
 *  Zero dimensions are unlimited and omitted; an all-zero limit shows "∞". */
export function rateLimitSummary(limit: {
  rpm_limit: number;
  tpm_limit: number;
  max_concurrency: number;
}): string {
  const parts: string[] = [];
  if (limit.rpm_limit > 0) parts.push(`R ${formatCount(limit.rpm_limit)}`);
  if (limit.tpm_limit > 0) parts.push(`T ${formatCount(limit.tpm_limit)}`);
  if (limit.max_concurrency > 0) parts.push(`C ${limit.max_concurrency}`);
  return parts.length > 0 ? parts.join(" · ") : "∞";
}

function formatCount(value: number): string {
  if (value >= 1_000_000) return `${trim(value / 1_000_000)}M`;
  if (value >= 1_000) return `${trim(value / 1_000)}k`;
  return String(value);
}

function trim(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}
