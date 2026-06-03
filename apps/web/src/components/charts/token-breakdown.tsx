import { cn } from "@/lib/cn";
import { formatInteger } from "@/lib/admin-format";

/**
 * Token composition as a single stacked bar + legend. Splits a token total into
 * input / output / cached (and optional cache-write) segments so the mix — not
 * just the grand total — is visible at a glance. Hand-rolled, token-colored.
 */
export type TokenBreakdownLabels = {
  input: string;
  output: string;
  cached: string;
  cacheCreation?: string;
};

type Seg = { key: string; label: string; value: number; bar: string; dot: string };

export function TokenBreakdown({
  input,
  output,
  cached,
  cacheCreation = 0,
  labels,
  className,
}: {
  input: number;
  output: number;
  cached: number;
  cacheCreation?: number;
  labels: TokenBreakdownLabels;
  className?: string;
}) {
  const segs: Seg[] = [
    { key: "input", label: labels.input, value: Math.max(0, input), bar: "bg-srapi-text-secondary", dot: "bg-srapi-text-secondary" },
    { key: "output", label: labels.output, value: Math.max(0, output), bar: "bg-srapi-primary", dot: "bg-srapi-primary" },
    { key: "cached", label: labels.cached, value: Math.max(0, cached), bar: "bg-srapi-success", dot: "bg-srapi-success" },
  ];
  if (labels.cacheCreation && cacheCreation > 0) {
    segs.push({ key: "cacheCreation", label: labels.cacheCreation, value: cacheCreation, bar: "bg-srapi-warning", dot: "bg-srapi-warning" });
  }
  const total = segs.reduce((sum, s) => sum + s.value, 0);

  return (
    <div className={className}>
      <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-srapi-card-muted">
        {total > 0
          ? segs.map((s) =>
              s.value > 0 ? (
                <div key={s.key} className={s.bar} style={{ width: `${(s.value / total) * 100}%` }} />
              ) : null,
            )
          : null}
      </div>
      <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1.5">
        {segs.map((s) => (
          <span key={s.key} className="inline-flex items-center gap-1.5 text-xs">
            <span className={cn("inline-block h-2 w-2 rounded-full", s.dot)} />
            <span className="text-srapi-text-tertiary">{s.label}</span>
            <span className="font-mono tabular text-srapi-text-secondary">{formatInteger(s.value)}</span>
            {total > 0 ? (
              <span className="font-mono tabular text-srapi-text-tertiary">
                {Math.round((s.value / total) * 100)}%
              </span>
            ) : null}
          </span>
        ))}
      </div>
    </div>
  );
}
