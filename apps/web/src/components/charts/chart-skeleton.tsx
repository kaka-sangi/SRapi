"use client";

import { cn } from "@/lib/cn";
import { Skeleton } from "@/components/ui/skeleton";

/**
 * Chart-shaped skeletons — instead of plain rectangles, each loader paints
 * approximate axis ticks + bars/lines/donut so the placeholder sits in the same
 * visual rhythm as the real chart. The shimmer layer is the shared
 * `.skeleton-shimmer` class so all skeletons across the app pulse together.
 */
export function TrendChartSkeleton({
  height = 132,
  className,
}: {
  height?: number;
  className?: string;
}) {
  // A SVG sine-ish line + dashed grid evoke the real trend chart's shape so the
  // loading state reads as "trend chart, pending data" not "generic block".
  return (
    <div
      className={cn("relative w-full overflow-hidden rounded-md border border-srapi-border/40 bg-srapi-card-muted/20", className)}
      style={{ height }}
    >
      {/* Horizontal grid ticks */}
      <div className="absolute inset-x-3 inset-y-3 flex flex-col justify-between" aria-hidden>
        {Array.from({ length: 5 }).map((_, i) => (
          <div key={i} className="h-px bg-srapi-border/50" />
        ))}
      </div>
      {/* X-axis tick marks (vertical) */}
      <div className="absolute inset-x-3 bottom-2 flex justify-between" aria-hidden>
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="h-1.5 w-px bg-srapi-border/40" />
        ))}
      </div>
      {/* The shimmering line+area "ghost" — clipped to a line shape so the
          shimmer rides along where the real series will land. */}
      <div
        className="skeleton-shimmer absolute inset-0 bg-srapi-primary/15"
        style={{
          clipPath:
            "polygon(0% 78%, 12% 62%, 25% 70%, 38% 48%, 50% 54%, 62% 38%, 75% 46%, 87% 30%, 100% 36%, 100% 100%, 0% 100%)",
        }}
        aria-hidden
      />
      {/* Stroke ghost — a thin band on top of the area for the "line". */}
      <div
        className="skeleton-shimmer absolute inset-0 bg-srapi-primary/40"
        style={{
          clipPath:
            "polygon(0% 76%, 12% 60%, 25% 68%, 38% 46%, 50% 52%, 62% 36%, 75% 44%, 87% 28%, 100% 34%, 100% 36%, 87% 30%, 75% 46%, 62% 38%, 50% 54%, 38% 48%, 25% 70%, 12% 62%, 0% 78%)",
        }}
        aria-hidden
      />
      <span className="sr-only">Loading chart</span>
    </div>
  );
}

export function BarChartSkeleton({
  rows = 5,
  className,
}: {
  rows?: number;
  className?: string;
}) {
  // Each ghost row is label + bar + value — same proportions as BarSeries so
  // the layout doesn't shift when data resolves.
  const widths = ["75%", "60%", "45%", "85%", "35%", "55%", "70%", "40%"];
  return (
    <div className={cn("space-y-2.5", className)} aria-busy>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="h-3 w-24 shrink-0" />
          <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-srapi-card-muted/60">
            <div
              className="skeleton-shimmer h-full rounded-full bg-srapi-primary/30"
              style={{ width: widths[i % widths.length] }}
            />
          </div>
          <Skeleton className="h-3 w-10 shrink-0" />
        </div>
      ))}
      <span className="sr-only">Loading chart</span>
    </div>
  );
}

/**
 * Vertical-bar histogram skeleton — used by latency / throughput shaped
 * surfaces. Renders a row of bars at varying heights with x-axis ghost labels.
 */
export function HistogramChartSkeleton({
  height = 150,
  bars = 12,
  className,
}: {
  height?: number;
  bars?: number;
  className?: string;
}) {
  const heights = [40, 65, 80, 95, 78, 60, 48, 36, 28, 22, 18, 14, 12];
  return (
    <div
      className={cn(
        "relative w-full overflow-hidden rounded-md border border-srapi-border/40 bg-srapi-card-muted/20 p-3",
        className,
      )}
      style={{ height }}
      aria-busy
    >
      {/* Grid ticks */}
      <div className="absolute inset-x-3 inset-y-3 bottom-6 flex flex-col justify-between" aria-hidden>
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-px bg-srapi-border/50" />
        ))}
      </div>
      <div className="relative flex h-full items-end justify-between gap-1 pb-3">
        {Array.from({ length: bars }).map((_, i) => (
          <div key={i} className="flex h-full flex-1 items-end">
            <div
              className="skeleton-shimmer w-full rounded-t bg-srapi-primary/30"
              style={{ height: `${heights[i % heights.length]}%` }}
            />
          </div>
        ))}
      </div>
      <span className="sr-only">Loading histogram</span>
    </div>
  );
}

/**
 * Donut skeleton — circle + legend rows for percentile distribution / error
 * owner breakdown charts that aren't time-series.
 */
export function DonutChartSkeleton({
  className,
}: {
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-center gap-5 sm:flex-row sm:items-start", className)} aria-busy>
      <div className="relative shrink-0">
        <div className="skeleton-shimmer size-36 rounded-full bg-srapi-card-muted/60" />
        <div className="absolute inset-4 rounded-full bg-srapi-card" />
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-1">
          <Skeleton className="h-5 w-12" />
          <Skeleton className="h-2 w-10" />
        </div>
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-2.5">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="flex items-center gap-3">
            <div className="size-2 shrink-0 rounded-full bg-srapi-card-muted" />
            <Skeleton className="h-3 flex-1 max-w-[12rem]" />
            <Skeleton className="h-3 w-10 shrink-0" />
          </div>
        ))}
      </div>
      <span className="sr-only">Loading distribution</span>
    </div>
  );
}

export function FormSkeleton({
  rows = 4,
  className,
}: {
  rows?: number;
  className?: string;
}) {
  return (
    <div className={cn("space-y-5", className)}>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i}>
          <Skeleton className="h-3 w-24" />
          <div className="mt-2 skeleton-shimmer h-9 w-full rounded-md bg-srapi-card-muted" />
        </div>
      ))}
    </div>
  );
}

export function SloCardSkeleton({ className }: { className?: string }) {
  return (
    <div className={cn("rounded-xl border border-srapi-border bg-srapi-card p-5 space-y-4", className)}>
      <div className="flex items-center justify-between">
        <Skeleton className="h-4 w-28" />
        <Skeleton className="h-5 w-14 rounded-full" />
      </div>
      <div className="flex items-baseline justify-between">
        <Skeleton className="h-3 w-16" />
        <Skeleton className="h-7 w-20" />
      </div>
      <div className="skeleton-shimmer h-2 w-full rounded-full bg-srapi-card-muted" />
      <div className="flex items-center justify-between">
        <Skeleton className="h-2.5 w-24" />
        <Skeleton className="h-2.5 w-16" />
      </div>
    </div>
  );
}

export function ChatSkeleton({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "flex h-[70vh] flex-col rounded-xl border border-srapi-border bg-srapi-card",
        className,
      )}
    >
      <div className="flex-1 space-y-5 p-6">
        <div className="flex justify-end">
          <Skeleton className="h-10 w-48 rounded-xl rounded-tr-sm" />
        </div>
        <div className="space-y-2">
          <Skeleton className="h-4 w-64" />
          <Skeleton className="h-4 w-56" />
          <Skeleton className="h-4 w-40" />
        </div>
        <div className="flex justify-end">
          <Skeleton className="h-10 w-36 rounded-xl rounded-tr-sm" />
        </div>
        <div className="space-y-2">
          <Skeleton className="h-4 w-72" />
          <Skeleton className="h-4 w-48" />
        </div>
      </div>
      <div className="border-t border-srapi-border p-4">
        <Skeleton className="h-10 w-full rounded-xl" />
      </div>
    </div>
  );
}

export function DialogListSkeleton({
  rows = 4,
  className,
}: {
  rows?: number;
  className?: string;
}) {
  return (
    <div className={cn("space-y-3", className)}>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-3 w-20" />
          <Skeleton className="ml-auto h-3 w-12" />
        </div>
      ))}
    </div>
  );
}
