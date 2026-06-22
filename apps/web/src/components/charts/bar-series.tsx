"use client";

import { cn } from "@/lib/cn";
import { useHoverSync } from "./hover-sync-provider";

export interface BarDatum {
  label: string;
  value: number;
}

/**
 * Pure-SVG horizontal bar series for distributions / histograms. Token colors
 * only; each row is label · bar · value. No chart library.
 *
 * Hover behavior — moving the mouse onto a bar pushes that row's index into the
 * shared `HoverSyncProvider` so sibling charts highlight the same bucket. When
 * another chart drives the index, the matching row here lifts to full opacity
 * while siblings fade — same dim-the-rest pattern as the existing CSS-only
 * group hover, just synchronised across charts.
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
  const hoverSync = useHoverSync();
  const activeIdx =
    hoverSync.index != null && hoverSync.index >= 0 && hoverSync.index < data.length
      ? hoverSync.index
      : null;
  const hasActive = activeIdx != null;

  return (
    <div
      role="img"
      aria-label={ariaLabel}
      className={cn("group/bars space-y-2", className)}
      onMouseLeave={() => hoverSync.setIndex(null)}
    >
      {data.map((d, idx) => {
        const isActive = activeIdx === idx;
        return (
          <div
            key={d.label}
            onMouseEnter={() => hoverSync.setIndex(idx)}
            className={cn(
              "group/row flex items-center gap-3 transition-opacity duration-200",
              // Local CSS-only group hover still works standalone; the data-attr
              // path lets cross-chart sync take over when an index is active.
              "group-hover/bars:opacity-40 hover:!opacity-100",
              hasActive && !isActive && "opacity-40",
              hasActive && isActive && "opacity-100",
            )}
          >
            <span className="w-28 shrink-0 truncate font-mono text-2xs text-srapi-text-secondary">
              {d.label}
            </span>
            <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-srapi-card-muted">
              <div
                className={cn(
                  "h-full rounded-full transition-colors",
                  isActive
                    ? "bg-srapi-primary"
                    : "bg-srapi-primary/70 group-hover/row:bg-srapi-primary",
                )}
                style={{ width: `${Math.max(2, (d.value / max) * 100)}%` }}
              />
            </div>
            <span className="w-16 shrink-0 text-right font-mono text-2xs text-srapi-text-tertiary tabular">
              {formatValue(d.value)}
            </span>
          </div>
        );
      })}
    </div>
  );
}
