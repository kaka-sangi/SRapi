"use client";

import * as React from "react";
import { cn } from "@/lib/cn";
import { useHoverSync } from "./hover-sync-provider";

/**
 * Multi-series line + area trend chart. Pure SVG (no chart library), token
 * colors, responsive via viewBox. Each series is drawn as a line over a faint
 * baseline grid with a translucent area fill; an optional legend names them.
 * A single data point (or a flat series) renders as a flat line across the
 * full width so a fresh/low-traffic window is never blank.
 *
 * Hover behavior — when the mouse moves over the chart, the nearest x-index is
 * pushed into the shared `HoverSyncProvider` so sibling charts highlight the
 * same bucket. A vertical guideline + per-series dot mark the focused index
 * locally, and a floating mini-popover near the cursor shows date + each
 * series's value at that index (rich-tooltip style, inline render).
 */
export type TrendTone = "primary" | "secondary" | "success";
export type TrendSeries = {
  key: string;
  label: string;
  values: number[];
  tone: TrendTone;
};

const STROKE: Record<TrendTone, string> = {
  primary: "stroke-srapi-primary",
  secondary: "stroke-srapi-text-secondary",
  success: "stroke-srapi-success",
};
const AREA: Record<TrendTone, string> = {
  primary: "text-srapi-primary",
  secondary: "text-srapi-text-secondary",
  success: "text-srapi-success",
};
const DOT: Record<TrendTone, string> = {
  primary: "bg-srapi-primary",
  secondary: "bg-srapi-text-secondary",
  success: "bg-srapi-success",
};
const DOT_FILL: Record<TrendTone, string> = {
  primary: "fill-srapi-primary",
  secondary: "fill-srapi-text-secondary",
  success: "fill-srapi-success",
};

export function TrendChart({
  series,
  ariaLabel,
  height = 132,
  showLegend = true,
  className,
  labels,
  formatValue = (v) => Intl.NumberFormat().format(v),
}: {
  series: TrendSeries[];
  ariaLabel: string;
  height?: number;
  showLegend?: boolean;
  className?: string;
  /** Optional x-axis labels (e.g. dates) — same length as values. */
  labels?: string[];
  /** Format function for tooltip values. */
  formatValue?: (value: number) => string;
}) {
  const W = 480;
  const H = height;
  const padX = 6;
  const padTop = 10;
  const padBot = 10;
  const len = Math.max(0, ...series.map((s) => s.values.length));

  const hoverSync = useHoverSync();
  const hostRef = React.useRef<HTMLDivElement | null>(null);
  const svgRef = React.useRef<SVGSVGElement | null>(null);
  const [cursor, setCursor] = React.useState<{ x: number; y: number } | null>(null);

  if (len === 0) {
    return (
      <div
        role="img"
        aria-label={ariaLabel}
        className={cn("w-full rounded-md bg-srapi-card-muted/40", className)}
        style={{ height }}
      />
    );
  }

  const allVals = series.flatMap((s) => s.values);
  const maxVal = Math.max(1, ...allVals);
  // A single bucket (or a perfectly flat series) has no slope — pin it to the
  // midline so it reads as a steady reading rather than a maxed-out spike.
  const flat = allVals.length <= 1 || allVals.every((v) => v === allVals[0]);
  const plotH = H - padTop - padBot;
  const y = (v: number) => (flat ? padTop + plotH / 2 : padTop + (1 - v / maxVal) * plotH);
  const xAt = (n: number, i: number) =>
    n <= 1 ? [padX, W - padX][i] : padX + (i * (W - 2 * padX)) / (n - 1);
  const gridYs = [0, 0.25, 0.5, 0.75, 1].map((f) => padTop + f * plotH);

  // Reduce cursor → nearest index (in the shared coord-space). We use the SVG
  // viewBox conversion so the math works no matter the rendered width.
  const handleMove = (event: React.MouseEvent<HTMLDivElement>) => {
    const host = hostRef.current;
    const svg = svgRef.current;
    if (!host || !svg) return;
    const rect = svg.getBoundingClientRect();
    if (rect.width === 0) return;
    const relX = ((event.clientX - rect.left) / rect.width) * W;
    const usableW = W - 2 * padX;
    const step = len <= 1 ? usableW : usableW / (len - 1);
    const idx = step > 0 ? Math.round((relX - padX) / step) : 0;
    const clamped = Math.max(0, Math.min(len - 1, idx));
    hoverSync.setIndex(clamped);
    const hostRect = host.getBoundingClientRect();
    setCursor({
      x: event.clientX - hostRect.left,
      y: event.clientY - hostRect.top,
    });
  };
  const handleLeave = () => {
    hoverSync.setIndex(null);
    setCursor(null);
  };

  const activeIdx =
    hoverSync.index != null && hoverSync.index >= 0 && hoverSync.index < len
      ? hoverSync.index
      : null;
  const guidelineX = activeIdx != null ? xAt(len, activeIdx) : null;
  const activeLabel = activeIdx != null ? labels?.[activeIdx] : undefined;

  return (
    <div
      ref={hostRef}
      className={cn("relative", className)}
      onMouseMove={handleMove}
      onMouseLeave={handleLeave}
    >
      {showLegend ? (
        <div className="mb-2 flex flex-wrap gap-x-4 gap-y-1">
          {series.map((s) => (
            <span
              key={s.key}
              className="inline-flex items-center gap-1.5 font-mono text-2xs text-srapi-text-tertiary"
            >
              <span className={cn("inline-block h-2 w-2 rounded-full", DOT[s.tone])} />
              {s.label}
            </span>
          ))}
        </div>
      ) : null}
      <svg
        ref={svgRef}
        role="img"
        aria-label={ariaLabel}
        viewBox={`0 0 ${W} ${H}`}
        preserveAspectRatio="none"
        className="w-full"
        style={{ height }}
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
        {series.map((s) => {
          const vals = s.values.length <= 1 ? [s.values[0] ?? 0, s.values[0] ?? 0] : s.values;
          const pts = vals.map((v, i) => [xAt(vals.length, i), y(v)] as const);
          const line = pts
            .map(([x, yy], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)} ${yy.toFixed(1)}`)
            .join(" ");
          const area = `${line} L${pts[pts.length - 1][0].toFixed(1)} ${H - padBot} L${pts[0][0].toFixed(1)} ${H - padBot} Z`;
          return (
            <g key={s.key}>
              <path d={area} className={cn(AREA[s.tone], "fill-current")} fillOpacity={0.1} />
              <path
                d={line}
                fill="none"
                className={STROKE[s.tone]}
                strokeWidth={1.5}
                strokeLinejoin="round"
                strokeLinecap="round"
                vectorEffect="non-scaling-stroke"
              />
            </g>
          );
        })}
        {guidelineX != null ? (
          <line
            x1={guidelineX}
            x2={guidelineX}
            y1={padTop}
            y2={H - padBot}
            className="stroke-srapi-text-secondary"
            strokeOpacity={0.45}
            strokeDasharray="3 3"
            strokeWidth={1}
            vectorEffect="non-scaling-stroke"
            pointerEvents="none"
          />
        ) : null}
        {activeIdx != null
          ? series.map((s) => {
              const v = s.values[activeIdx] ?? s.values[s.values.length - 1] ?? 0;
              const cx = xAt(Math.max(2, s.values.length), Math.min(activeIdx, s.values.length - 1));
              const cy = y(v);
              return (
                <g key={`dot-${s.key}`} pointerEvents="none">
                  <circle cx={cx} cy={cy} r={3.5} className="fill-srapi-card stroke-srapi-card" strokeWidth={2} />
                  <circle cx={cx} cy={cy} r={2.5} className={cn(DOT_FILL[s.tone])} />
                </g>
              );
            })
          : null}
      </svg>
      {activeIdx != null && cursor ? (
        <TrendHoverPopover
          x={cursor.x}
          y={cursor.y}
          label={activeLabel}
          rows={series.map((s) => ({
            key: s.key,
            label: s.label,
            tone: s.tone,
            value: formatValue(s.values[activeIdx] ?? 0),
          }))}
        />
      ) : null}
    </div>
  );
}

/**
 * Inline floating mini-popover near the cursor — built in the spirit of the
 * `variant="rich"` Tooltip content (card-style background, soft shadow, key /
 * value rows) but rendered locally so it can chase the cursor without going
 * through Radix portals.
 */
function TrendHoverPopover({
  x,
  y,
  label,
  rows,
}: {
  x: number;
  y: number;
  label?: string;
  rows: Array<{ key: string; label: string; tone: TrendTone; value: string }>;
}) {
  // Nudge so the popover never covers the cursor / clips the right edge.
  const style: React.CSSProperties = {
    left: x + 12,
    top: Math.max(0, y - 12),
    transform: "translate(0, -100%)",
  };
  return (
    <div
      role="tooltip"
      aria-hidden
      className="srapi-anim-pop pointer-events-none absolute z-30 max-w-[14rem] rounded-xl border border-srapi-border bg-srapi-card p-3 text-xs text-srapi-text-primary shadow-md"
      style={style}
    >
      {label ? (
        <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
          {label}
        </div>
      ) : null}
      <dl
        className={cn(
          "grid grid-cols-[auto_1fr] gap-x-3 gap-y-1",
          label ? "mt-2 border-t border-srapi-border/60 pt-2" : "",
        )}
      >
        {rows.map((r) => (
          <React.Fragment key={r.key}>
            <dt className="inline-flex items-center gap-1.5 text-[11px] text-srapi-text-tertiary">
              <span className={cn("inline-block h-2 w-2 rounded-full", DOT[r.tone])} />
              {r.label}
            </dt>
            <dd className="text-right text-[12px] font-medium tabular text-srapi-text-primary">
              {r.value}
            </dd>
          </React.Fragment>
        ))}
      </dl>
    </div>
  );
}
