"use client";

import { useMemo } from "react";
import { AlertTriangle } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatInteger } from "@/lib/admin-format";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { ChartEmpty } from "@/components/charts/chart-empty";
import type { UsageErrorBucketItem } from "@/hooks/admin-queries/usage-charts";

/**
 * Usage error-distribution doughnut grouped by `error_class`. A hand-rolled SVG
 * donut — token colors, no chart library — with the grand total in the hole, a
 * legend, and the top error_class rows beneath. Mirrors the ops error
 * distribution card (`ops-error-distribution-chart`) but keyed on SRapi's usage
 * error read-model (error_class / count / percentage) rather than owner.
 */

// Each slice/row is colored by rank from the theme palette; the tail beyond the
// palette and any "other" remainder fall back to the neutral tertiary tone.
const SLICE_STROKE: readonly string[] = [
  "stroke-srapi-error",
  "stroke-srapi-warning",
  "stroke-srapi-primary",
  "stroke-srapi-success",
  "stroke-srapi-text-secondary",
];
const SLICE_DOT: readonly string[] = [
  "bg-srapi-error",
  "bg-srapi-warning",
  "bg-srapi-primary",
  "bg-srapi-success",
  "bg-srapi-text-secondary",
];
const OTHER_STROKE = "stroke-srapi-text-tertiary";
const OTHER_DOT = "bg-srapi-text-tertiary";

const MAX_SLICES = 5;

export function UsageErrorDistributionChart({
  items,
  title,
  emptyLabel,
  totalLabel,
  otherLabel,
  loading,
}: {
  items: UsageErrorBucketItem[];
  title: string;
  emptyLabel: string;
  totalLabel: string;
  otherLabel: string;
  loading?: boolean;
}) {
  const { total, segments } = useMemo(() => {
    const sorted = [...items]
      .filter((it) => it.count > 0)
      .sort((a, b) => b.count - a.count);
    const sum = sorted.reduce((acc, it) => acc + it.count, 0);

    // Keep the top N classes; fold the long tail into a single "other" slice so
    // the donut never fragments into hair-thin arcs.
    const head = sorted.slice(0, MAX_SLICES);
    const tail = sorted.slice(MAX_SLICES);
    const segs = head.map((it, i) => ({
      key: it.error_class,
      label: it.error_class,
      count: it.count,
      stroke: SLICE_STROKE[i],
      dot: SLICE_DOT[i],
    }));
    if (tail.length > 0) {
      const tailCount = tail.reduce((acc, it) => acc + it.count, 0);
      segs.push({
        key: "__other__",
        label: otherLabel,
        count: tailCount,
        stroke: OTHER_STROKE,
        dot: OTHER_DOT,
      });
    }
    return { total: sum, segments: segs };
  }, [items, otherLabel]);

  // Donut geometry — a stroke-dasharray ring so each class is one arc.
  const R = 52;
  const C = 2 * Math.PI * R;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="not-italic font-sans text-base text-srapi-text-primary">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading && items.length === 0 ? (
          <div
            role="img"
            aria-label={title}
            className="h-40 w-full animate-pulse rounded-md bg-srapi-card-muted/40"
          />
        ) : total === 0 ? (
          <ChartEmpty label={emptyLabel} icon={AlertTriangle} />
        ) : (
          <div className="flex flex-col items-center gap-5 sm:flex-row sm:items-start">
            <div className="relative shrink-0">
              <svg viewBox="0 0 140 140" role="img" aria-label={title} className="size-36">
                <circle
                  cx={70}
                  cy={70}
                  r={R}
                  fill="none"
                  className="stroke-srapi-card-muted"
                  strokeWidth={14}
                />
                {segments.map((seg, i) => {
                  const frac = seg.count / total;
                  const len = frac * C;
                  const dash = `${len} ${C - len}`;
                  const offset = segments
                    .slice(0, i)
                    .reduce((acc, prev) => acc + (prev.count / total) * C, 0);
                  return (
                    <circle
                      key={seg.key}
                      cx={70}
                      cy={70}
                      r={R}
                      fill="none"
                      className={cn(seg.stroke, "transition-[stroke-dashoffset]")}
                      strokeWidth={14}
                      strokeDasharray={dash}
                      strokeDashoffset={-offset}
                      strokeLinecap="butt"
                      transform="rotate(-90 70 70)"
                    >
                      <title>{`${seg.label}: ${formatInteger(seg.count)} (${(frac * 100).toFixed(1)}%)`}</title>
                    </circle>
                  );
                })}
              </svg>
              <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                <span className="font-serif text-xl text-srapi-text-primary tabular">
                  {formatInteger(total)}
                </span>
                <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                  {totalLabel}
                </span>
              </div>
            </div>

            <div className="min-w-0 flex-1 space-y-1.5">
              {segments.map((seg) => {
                const frac = seg.count / total;
                return (
                  <div
                    key={seg.key}
                    className="flex items-center gap-2 border-t border-srapi-border pt-1.5 first:border-t-0 first:pt-0"
                  >
                    <span className={cn("inline-block h-2 w-2 shrink-0 rounded-full", seg.dot)} />
                    <span className="min-w-0 flex-1 truncate font-mono text-2xs text-srapi-text-secondary">
                      {seg.label}
                    </span>
                    <span className="shrink-0 font-mono text-2xs text-srapi-text-tertiary tabular">
                      {formatInteger(seg.count)}
                    </span>
                    <span className="w-12 shrink-0 text-right font-mono text-2xs text-srapi-text-tertiary tabular">
                      {(frac * 100).toFixed(1)}%
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
