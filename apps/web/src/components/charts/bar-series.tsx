import { cn } from "@/lib/cn";

export interface BarDatum {
  label: string;
  value: number;
}

/**
 * Pure-SVG horizontal bar series for distributions / histograms. Token colors
 * only; each row is label · bar · value. No chart library.
 */
export function BarSeries({
  data,
  ariaLabel,
  className,
  formatValue = (v) => String(v),
}: {
  data: BarDatum[];
  ariaLabel: string;
  className?: string;
  formatValue?: (value: number) => string;
}) {
  const max = Math.max(...data.map((d) => d.value), 1);

  return (
    <div role="img" aria-label={ariaLabel} className={cn("group/bars space-y-2", className)}>
      {data.map((d) => (
        <div
          key={d.label}
          className="group/row flex items-center gap-3 transition-opacity duration-200 group-hover/bars:opacity-40 hover:!opacity-100"
        >
          <span className="w-28 shrink-0 truncate font-mono text-2xs text-srapi-text-secondary">
            {d.label}
          </span>
          <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-srapi-card-muted">
            <div
              className="h-full rounded-full bg-srapi-primary/70 transition-colors group-hover/row:bg-srapi-primary"
              style={{ width: `${Math.max(2, (d.value / max) * 100)}%` }}
            />
          </div>
          <span className="w-16 shrink-0 text-right font-mono text-2xs text-srapi-text-tertiary tabular">
            {formatValue(d.value)}
          </span>
        </div>
      ))}
    </div>
  );
}
