"use client";

import { useMemo } from "react";
import { LineChart } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatInteger, formatMoney, formatDateTime } from "@/lib/admin-format";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { ChartEmpty } from "@/components/charts/chart-empty";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type { UsageTrendSeries } from "@/hooks/admin-queries/usage-charts";

/**
 * Multi-series admin usage TREND chart — one line/area per top-N series
 * (model / account / source_endpoint), with the metric (tokens vs cost) and the
 * grouping dimension + day|hour bucket toggled by the caller. Pure SVG, token
 * colors, no chart library; mirrors the hand-rolled house style of
 * `charts/trend-chart` and the ops latency/error cards.
 *
 * Series can report slightly different bucket sets (a series with no traffic in
 * a bucket simply omits the point), so we build one sorted union of buckets and
 * align every series onto that shared X axis — a gap reads as zero rather than
 * shifting the line.
 */

export type UsageTrendMetric = "tokens" | "cost";

// A small fixed palette so up to 6 series stay visually distinct; anything past
// the palette length wraps (the backend already caps to the requested top-N).
// Drawn from the theme tokens defined in globals.css — no ad-hoc colors.
const SERIES_STROKE = [
  "stroke-srapi-primary",
  "stroke-srapi-success",
  "stroke-srapi-warning",
  "stroke-srapi-error",
  "stroke-srapi-text-secondary",
  "stroke-srapi-text-primary",
] as const;
const SERIES_AREA = [
  "text-srapi-primary",
  "text-srapi-success",
  "text-srapi-warning",
  "text-srapi-error",
  "text-srapi-text-secondary",
  "text-srapi-text-primary",
] as const;
const SERIES_DOT = [
  "bg-srapi-primary",
  "bg-srapi-success",
  "bg-srapi-warning",
  "bg-srapi-error",
  "bg-srapi-text-secondary",
  "bg-srapi-text-primary",
] as const;

function pointValue(
  point: { input_tokens: number; output_tokens: number; cost: string },
  metric: UsageTrendMetric,
): number {
  if (metric === "cost") {
    const n = Number(point.cost);
    return Number.isFinite(n) ? n : 0;
  }
  return point.input_tokens + point.output_tokens;
}

export function UsageTrendChart({
  series,
  metric,
  onMetricChange,
  title,
  metricTokensLabel,
  metricCostLabel,
  emptyLabel,
  controls,
  loading,
}: {
  series: UsageTrendSeries[];
  metric: UsageTrendMetric;
  onMetricChange: (metric: UsageTrendMetric) => void;
  title: string;
  metricTokensLabel: string;
  metricCostLabel: string;
  emptyLabel: string;
  // Dimension + bucket segmented controls live on the page and are slotted in
  // here so the card header owns all of the chart's toggles in one row.
  controls?: React.ReactNode;
  loading?: boolean;
}) {
  const { buckets, lines, maxVal, currency } = useMemo(() => {
    // Union of every series' buckets, sorted ascending — the shared X axis.
    const bucketSet = new Set<string>();
    for (const s of series) {
      for (const p of s.points) bucketSet.add(p.bucket);
    }
    const orderedBuckets = [...bucketSet].sort();
    const bucketIndex = new Map(orderedBuckets.map((b, i) => [b, i] as const));

    // First non-empty currency across all points — usage is single-currency in
    // practice, so the first one we see labels the whole cost axis.
    const firstCurrency =
      series.flatMap((s) => s.points).find((p) => p.currency)?.currency ?? "USD";

    const built = series.map((s, si) => {
      // A gap (series had no point in a bucket) reads as zero.
      const values = new Array<number>(orderedBuckets.length).fill(0);
      for (const p of s.points) {
        const idx = bucketIndex.get(p.bucket);
        if (idx === undefined) continue;
        values[idx] = pointValue(p, metric);
      }
      const total = values.reduce((acc, v) => acc + v, 0);
      return { key: s.label, label: s.label, paletteIndex: si % SERIES_STROKE.length, values, total };
    });

    // Heaviest series first so the legend leads with the dominant contributor.
    built.sort((a, b) => b.total - a.total);

    const max = Math.max(0, ...built.flatMap((l) => l.values));

    return { buckets: orderedBuckets, lines: built, maxVal: max, currency: firstCurrency };
  }, [series, metric]);

  const hasData = buckets.length > 0 && lines.some((l) => l.total > 0);

  const W = 480;
  const H = 160;
  const padX = 6;
  const padTop = 10;
  const padBot = 10;
  const plotH = H - padTop - padBot;
  const max = Math.max(1, maxVal);
  const n = buckets.length;
  const xAt = (i: number) =>
    n <= 1 ? (i === 0 ? padX : W - padX) : padX + (i * (W - 2 * padX)) / (n - 1);
  const yAt = (v: number) => padTop + (1 - v / max) * plotH;
  const gridYs = [0, 0.25, 0.5, 0.75, 1].map((f) => padTop + f * plotH);

  const fmt = (v: number) => (metric === "cost" ? formatMoney(v, currency) : formatInteger(v));

  return (
    <Card>
      <CardHeader className="flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between">
        <CardTitle className="not-italic font-sans text-base text-srapi-text-primary">
          {title}
        </CardTitle>
        <div className="flex flex-wrap items-center gap-2">
          {controls}
          <Tabs value={metric} onValueChange={(v) => onMetricChange(v as UsageTrendMetric)}>
            <TabsList>
              <TabsTrigger value="tokens" className="text-xs">
                {metricTokensLabel}
              </TabsTrigger>
              <TabsTrigger value="cost" className="text-xs">
                {metricCostLabel}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>
      </CardHeader>
      <CardContent>
        {loading && series.length === 0 ? (
          <div
            role="img"
            aria-label={title}
            className="w-full animate-pulse rounded-md bg-srapi-card-muted/40"
            style={{ height: H }}
          />
        ) : !hasData ? (
          <ChartEmpty label={emptyLabel} icon={LineChart} />
        ) : (
          <div className="space-y-3">
            <div className="flex flex-wrap gap-x-4 gap-y-1">
              {lines.map((l) => (
                <span
                  key={l.key}
                  className="inline-flex max-w-[14rem] items-center gap-1.5 font-mono text-2xs text-srapi-text-tertiary"
                  title={`${l.label}: ${fmt(l.total)}`}
                >
                  <span
                    className={cn(
                      "inline-block h-2 w-2 shrink-0 rounded-full",
                      SERIES_DOT[l.paletteIndex],
                    )}
                  />
                  <span className="truncate text-srapi-text-secondary">{l.label}</span>
                  <span className="shrink-0 tabular">{fmt(l.total)}</span>
                </span>
              ))}
            </div>
            <svg
              role="img"
              aria-label={title}
              viewBox={`0 0 ${W} ${H}`}
              preserveAspectRatio="none"
              className="w-full"
              style={{ height: H }}
            >
              {gridYs.map((gy, i) => (
                <line
                  key={i}
                  x1={padX}
                  y1={gy}
                  x2={W - padX}
                  y2={gy}
                  className="stroke-srapi-border"
                  strokeWidth={0.5}
                  vectorEffect="non-scaling-stroke"
                />
              ))}
              {lines.map((l) => {
                // A single bucket draws as a flat segment across the full width
                // so a fresh/low-traffic window never collapses to a dot.
                const xs =
                  n <= 1
                    ? [padX, W - padX]
                    : l.values.map((_, i) => xAt(i));
                const ys =
                  n <= 1
                    ? [yAt(l.values[0] ?? 0), yAt(l.values[0] ?? 0)]
                    : l.values.map((v) => yAt(v));
                const pts = xs.map((x, i) => [x, ys[i]] as const);
                const line = pts
                  .map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`)
                  .join(" ");
                const area = `${line} L${pts[pts.length - 1][0].toFixed(1)} ${
                  H - padBot
                } L${pts[0][0].toFixed(1)} ${H - padBot} Z`;
                return (
                  <g key={l.key}>
                    <path
                      d={area}
                      className={cn(SERIES_AREA[l.paletteIndex], "fill-current")}
                      fillOpacity={0.08}
                    />
                    <path
                      d={line}
                      fill="none"
                      className={SERIES_STROKE[l.paletteIndex]}
                      strokeWidth={1.5}
                      strokeLinejoin="round"
                      strokeLinecap="round"
                      vectorEffect="non-scaling-stroke"
                    />
                  </g>
                );
              })}
            </svg>
            <div className="flex justify-between font-mono text-2xs text-srapi-text-tertiary tabular">
              <span>{formatDateTime(buckets[0])}</span>
              {buckets.length > 1 ? <span>{formatDateTime(buckets[buckets.length - 1])}</span> : null}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
