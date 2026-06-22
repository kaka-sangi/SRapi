"use client";

import * as React from "react";
import { Timer } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatInteger } from "@/lib/admin-format";
import { Card, CardContent } from "@/components/ui/card";
import { SectionTitle } from "@/components/ui/section-title";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { ChartEmpty } from "@/components/charts/chart-empty";
import { useHoverSync } from "@/components/charts/hover-sync-provider";
import type { OpsLatencyBucket } from "@/hooks/admin-queries/ops-charts";

/**
 * Latency percentile histogram — vertical bars over the OpsLatencyHistogram
 * buckets (each bucket is a latency band like `100–250ms`). Pure SVG, token
 * colors, no chart library; mirrors the hand-rolled house style (sparkline /
 * trend-chart / bar-series). Bars are heat-tinted so slow tail buckets read as
 * warmer, matching the sub2api ops latency card layout.
 *
 * Hover sync — hovering a bar pushes its bucket index into the shared
 * `HoverSyncProvider` so sibling ops charts (error distribution, sparklines)
 * focus the same bucket. A transparent HTML hit-grid sits on top of the SVG
 * so each slot can be wrapped in a `DataTooltip` (DataTooltip needs a DOM
 * trigger; SVG `<g>` isn't a valid Radix trigger). The tooltip exposes the
 * bucket range, count, share, and cumulative percentile.
 */

// Tail latency buckets shade warmer so a heavy right tail stands out at a glance.
function barClass(lowerMs: number): string {
  if (lowerMs >= 2000) return "fill-srapi-error/80";
  if (lowerMs >= 1000) return "fill-srapi-warning/80";
  return "fill-srapi-primary/70";
}

export function OpsLatencyHistogramChart({
  buckets,
  title,
  emptyLabel,
  requestsLabel,
  loading,
}: {
  buckets: OpsLatencyBucket[];
  title: string;
  emptyLabel: string;
  requestsLabel: string;
  loading?: boolean;
}) {
  const total = buckets.reduce((sum, b) => sum + b.count, 0);
  const max = Math.max(1, ...buckets.map((b) => b.count));

  const W = 480;
  const H = 150;
  const padX = 6;
  const padTop = 8;
  const padBot = 22;
  const plotH = H - padTop - padBot;
  const slot = buckets.length > 0 ? (W - padX * 2) / buckets.length : 0;
  const barW = slot * 0.62;

  const hoverSync = useHoverSync();
  const activeIdx =
    hoverSync.index != null && hoverSync.index >= 0 && hoverSync.index < buckets.length
      ? hoverSync.index
      : null;

  // Pre-compute cumulative share (running %) so the tooltip can show the
  // approximate percentile that each bucket spans (= cumulative ≤ upper).
  const cumulative = React.useMemo(
    () =>
      buckets.reduce<number[]>((acc, b) => {
        const prev = acc.length > 0 ? acc[acc.length - 1] : 0;
        acc.push(prev + b.share);
        return acc;
      }, []),
    [buckets],
  );

  return (
    <Card>
      <CardContent className="space-y-4">
        <SectionTitle
          icon={<Timer aria-hidden />}
          label={title}
          action={
            <span className="text-[11px] tabular text-srapi-text-tertiary">
              {formatInteger(total)} {requestsLabel}
            </span>
          }
        />
        {loading && buckets.length === 0 ? (
          <div
            role="img"
            aria-label={title}
            className="w-full animate-pulse rounded-md bg-srapi-card-muted/40"
            style={{ height: H }}
          />
        ) : total === 0 ? (
          <ChartEmpty label={emptyLabel} icon={Timer} />
        ) : (
          <div
            className="relative"
            style={{ height: H }}
            onMouseLeave={() => hoverSync.setIndex(null)}
          >
            <svg
              role="img"
              aria-label={title}
              viewBox={`0 0 ${W} ${H}`}
              preserveAspectRatio="none"
              className="absolute inset-0 h-full w-full"
            >
              {[0.25, 0.5, 0.75, 1].map((f) => {
                const gy = padTop + (1 - f) * plotH;
                return (
                  <line
                    key={f}
                    x1={padX}
                    y1={gy}
                    x2={W - padX}
                    y2={gy}
                    className="stroke-srapi-border"
                    strokeWidth={0.5}
                    vectorEffect="non-scaling-stroke"
                  />
                );
              })}
              {buckets.map((b, i) => {
                const h = Math.max(1, (b.count / max) * plotH);
                const x = padX + i * slot + (slot - barW) / 2;
                const y = padTop + (plotH - h);
                const isActive = activeIdx === i;
                return (
                  <g key={`bar-${b.label}`}>
                    <rect
                      x={x}
                      y={y}
                      width={barW}
                      height={h}
                      rx={2}
                      className={cn(
                        barClass(b.lower_ms),
                        "transition-[opacity,filter]",
                        activeIdx != null && !isActive && "opacity-50",
                        isActive && "[filter:brightness(1.15)]",
                      )}
                    />
                    <text
                      x={x + barW / 2}
                      y={H - 7}
                      textAnchor="middle"
                      className={cn(
                        "fill-srapi-text-tertiary transition-colors",
                        isActive && "fill-srapi-text-primary",
                      )}
                      style={{ fontSize: 8 }}
                    >
                      {b.label}
                    </text>
                  </g>
                );
              })}
            </svg>
            {/* HTML hit-grid mirrors the SVG slots so each bar gets a real DOM
                trigger that DataTooltip + Radix can attach to. */}
            <div
              className="absolute inset-0 flex"
              style={{
                paddingLeft: `${(padX / W) * 100}%`,
                paddingRight: `${(padX / W) * 100}%`,
                paddingBottom: `${(padBot / H) * 100}%`,
                paddingTop: `${(padTop / H) * 100}%`,
              }}
            >
              {buckets.map((b, i) => {
                const cum = cumulative[i] ?? 0;
                return (
                  <DataTooltip
                    key={`hit-${b.label}`}
                    title={`${b.label} latency`}
                    primary={formatInteger(b.count)}
                    rows={[
                      { label: "Bucket", value: b.label, tone: "muted" },
                      { label: "Share", value: `${(b.share * 100).toFixed(2)}%` },
                      { label: "Cumulative ≤", value: `${(cum * 100).toFixed(2)}%`, tone: "muted" },
                      { label: requestsLabel, value: formatInteger(b.count), tone: "muted" },
                    ]}
                    side="top"
                  >
                    <span
                      onMouseEnter={() => hoverSync.setIndex(i)}
                      onFocus={() => hoverSync.setIndex(i)}
                      className="block h-full flex-1 cursor-pointer"
                      aria-label={`${b.label}: ${formatInteger(b.count)} (${(b.share * 100).toFixed(1)}%)`}
                    />
                  </DataTooltip>
                );
              })}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
