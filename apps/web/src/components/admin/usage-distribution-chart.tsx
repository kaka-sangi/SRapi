"use client";

import { useMemo } from "react";
import { PieChart } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatInteger } from "@/lib/admin-format";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { ChartEmpty } from "@/components/charts/chart-empty";
import type { UsageDistributionBucketItem } from "@/hooks/admin-queries/usage-charts";

/**
 * Share-by-dimension distribution as a ranked horizontal bar list — the
 * complement to the time-series usage trend chart. Each bucket renders its
 * label, the value under the chosen metric (requests | tokens | cost), its
 * percentage share, and a proportional bar. Pure token styling, no chart lib,
 * matching the usage error-distribution card's look.
 *
 * The metric drives which numeric the bar/percentage reflect: requests and
 * tokens are formatted as integers; cost shows the per-bucket cost string with
 * its currency. Buckets arrive already sorted desc and capped to top-N by the
 * backend, so the component only renders.
 */

export type UsageDistributionMetric = "requests" | "tokens" | "cost";

// Rank-colored bar/dot from the theme palette; rows past the palette reuse the
// neutral tertiary tone so the list never runs out of colors.
const BAR_FILL: readonly string[] = [
  "bg-srapi-primary",
  "bg-srapi-success",
  "bg-srapi-warning",
  "bg-srapi-error",
  "bg-srapi-text-secondary",
];
const NEUTRAL_FILL = "bg-srapi-text-tertiary";

function metricValue(bucket: UsageDistributionBucketItem, metric: UsageDistributionMetric): number {
  switch (metric) {
    case "tokens":
      return bucket.total_tokens;
    case "cost":
      return Number.parseFloat(bucket.cost) || 0;
    default:
      return bucket.requests;
  }
}

function metricDisplay(bucket: UsageDistributionBucketItem, metric: UsageDistributionMetric): string {
  switch (metric) {
    case "tokens":
      return formatInteger(bucket.total_tokens);
    case "cost":
      return `${bucket.cost} ${bucket.currency}`;
    default:
      return formatInteger(bucket.requests);
  }
}

export function UsageDistributionChart({
  buckets,
  metric,
  title,
  emptyLabel,
  loading,
  controls,
}: {
  buckets: UsageDistributionBucketItem[];
  metric: UsageDistributionMetric;
  title: string;
  emptyLabel: string;
  loading?: boolean;
  controls?: React.ReactNode;
}) {
  // The backend's percentage is the share of the chosen metric, but recompute a
  // bar fraction relative to the largest bucket so the leading bar fills the row
  // (a 40%-share leader should still read as a full-width bar, not 40%).
  const { rows, maxValue } = useMemo(() => {
    const enriched = buckets.map((b) => ({ bucket: b, value: metricValue(b, metric) }));
    const max = enriched.reduce((acc, r) => Math.max(acc, r.value), 0);
    return { rows: enriched, maxValue: max };
  }, [buckets, metric]);

  const isEmpty = rows.length === 0 || maxValue <= 0;

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 space-y-0">
        <CardTitle className="not-italic font-sans text-base text-srapi-text-primary">
          {title}
        </CardTitle>
        {controls ? <div className="flex flex-wrap items-center gap-2">{controls}</div> : null}
      </CardHeader>
      <CardContent>
        {loading && buckets.length === 0 ? (
          <div
            role="img"
            aria-label={title}
            className="h-40 w-full animate-pulse rounded-md bg-srapi-card-muted/40"
          />
        ) : isEmpty ? (
          <ChartEmpty label={emptyLabel} icon={PieChart} />
        ) : (
          <div className="space-y-2.5">
            {rows.map(({ bucket, value }, i) => {
              const frac = maxValue > 0 ? value / maxValue : 0;
              const fill = BAR_FILL[i] ?? NEUTRAL_FILL;
              return (
                <div key={`${bucket.label}-${i}`} className="space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="min-w-0 flex-1 truncate font-mono text-2xs text-srapi-text-secondary">
                      {bucket.label}
                    </span>
                    <span className="shrink-0 font-mono text-2xs text-srapi-text-primary tabular">
                      {metricDisplay(bucket, metric)}
                    </span>
                    <span className="w-12 shrink-0 text-right font-mono text-2xs text-srapi-text-tertiary tabular">
                      {bucket.percentage.toFixed(1)}%
                    </span>
                  </div>
                  <div className="h-1.5 w-full overflow-hidden rounded-full bg-srapi-card-muted">
                    <div
                      className={cn("h-full rounded-full transition-[width]", fill)}
                      style={{ width: `${Math.max(frac * 100, 2)}%` }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
