"use client";

import { Timer } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatInteger } from "@/lib/admin-format";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { ChartEmpty } from "@/components/charts/chart-empty";
import type { OpsLatencyBucket } from "@/hooks/admin-queries/ops-charts";

/**
 * Latency percentile histogram — vertical bars over the OpsLatencyHistogram
 * buckets (each bucket is a latency band like `100–250ms`). Pure SVG, token
 * colors, no chart library; mirrors the hand-rolled house style (sparkline /
 * trend-chart / bar-series). Bars are heat-tinted so slow tail buckets read as
 * warmer, matching the sub2api ops latency card layout.
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

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between">
        <CardTitle className="not-italic font-sans text-base text-srapi-text-primary">
          {title}
        </CardTitle>
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatInteger(total)} {requestsLabel}
        </span>
      </CardHeader>
      <CardContent>
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
          <svg
            role="img"
            aria-label={title}
            viewBox={`0 0 ${W} ${H}`}
            preserveAspectRatio="none"
            className="w-full"
            style={{ height: H }}
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
              return (
                <g key={b.label}>
                  <rect
                    x={x}
                    y={y}
                    width={barW}
                    height={h}
                    rx={2}
                    className={cn(barClass(b.lower_ms), "transition-colors")}
                  >
                    <title>{`${b.label}: ${formatInteger(b.count)} (${(b.share * 100).toFixed(1)}%)`}</title>
                  </rect>
                  <text
                    x={x + barW / 2}
                    y={H - 7}
                    textAnchor="middle"
                    className="fill-srapi-text-tertiary font-mono"
                    style={{ fontSize: 8 }}
                  >
                    {b.label}
                  </text>
                </g>
              );
            })}
          </svg>
        )}
      </CardContent>
    </Card>
  );
}
