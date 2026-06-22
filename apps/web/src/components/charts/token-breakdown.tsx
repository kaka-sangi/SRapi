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
      <div className="relative flex h-3 w-full overflow-hidden rounded-full bg-srapi-card-muted shadow-[inset_0_1px_0_0_rgba(28,26,23,0.05)]">
        {total > 0
          ? segs.map((s, idx) =>
              s.value > 0 ? (
                <div
                  key={s.key}
                  className={cn(s.bar, "h-full transition-[width] duration-500 ease-[var(--ease-out-quint)]")}
                  style={{
                    width: `${(s.value / total) * 100}%`,
                    // Subtle inner highlight so the printed-on-paper feel
                    // carries through the bar segments.
                    boxShadow: "inset 0 1px 0 0 rgba(255,255,255,0.18)",
                    // Hairline divider between adjacent segments for a more
                    // "engraved" sense of measurement.
                    borderRight:
                      idx < segs.length - 1 ? "1px solid rgba(28,26,23,0.18)" : undefined,
                  }}
                />
              ) : null,
            )
          : null}
      </div>
      <div className="mt-4 grid gap-3 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
        {segs.map((s) => (
          <div
            key={s.key}
            className="flex items-center gap-2.5 rounded-md border border-srapi-border bg-srapi-card-muted/40 px-2.5 py-1.5"
          >
            <span className={cn("inline-block h-2.5 w-2.5 shrink-0 rounded-full", s.dot)} />
            <div className="min-w-0 flex-1 leading-tight">
              <div className="truncate text-2xs uppercase tracking-wider text-srapi-text-tertiary">
                {s.label}
              </div>
              <div className="flex items-baseline gap-1.5 font-mono tabular">
                <span className="text-sm text-srapi-text-primary">{formatInteger(s.value)}</span>
                {total > 0 ? (
                  <span className="text-2xs text-srapi-text-tertiary">
                    {Math.round((s.value / total) * 100)}%
                  </span>
                ) : null}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
