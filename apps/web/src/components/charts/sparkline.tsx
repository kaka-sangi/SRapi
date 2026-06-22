"use client";

import * as React from "react";
import { cn } from "@/lib/cn";
import { DataTooltip, type DataTooltipRow } from "@/components/ui/data-tooltip";

/**
 * Pure-SVG sparkline — a single value series drawn as a line, optionally filled.
 * No chart library: token colors only, viewBox-scaled so it's fully responsive,
 * and exposed to assistive tech via role="img" + aria-label. Matches the house
 * hand-rolled SVG style (quota-notch-rail, ambient-canvas).
 *
 * Hover behavior — a subtle dot tracks the cursor's nearest data point so a
 * stat card's mini trend isn't just decorative. Pass `tooltipTitle` (and
 * optional `tooltipRows` / `tooltipFooter` / `tooltipLabels`) to expose the
 * exact value at the focused index through a `DataTooltip` wrapping the host
 * card region.
 */
export function Sparkline({
  values,
  ariaLabel,
  area = true,
  className,
  height = 56,
  tooltipTitle,
  tooltipLabels,
  tooltipExtraRows,
  tooltipFooter,
  formatValue = (v) => Intl.NumberFormat().format(v),
}: {
  values: number[];
  ariaLabel: string;
  area?: boolean;
  className?: string;
  height?: number;
  /** When set, wrap the chart in a DataTooltip with the focused-point value. */
  tooltipTitle?: React.ReactNode;
  /** Optional per-index labels (e.g. dates) — surfaces as the tooltip primary. */
  tooltipLabels?: string[];
  /** Extra rows appended after the index/value row, e.g. context metrics. */
  tooltipExtraRows?: DataTooltipRow[];
  /** Optional footer text rendered inside the tooltip. */
  tooltipFooter?: React.ReactNode;
  /** Format function for the value shown in the tooltip. */
  formatValue?: (value: number) => string;
}) {
  const width = 240;
  const pad = 2;

  const [activeIdx, setActiveIdx] = React.useState<number | null>(null);
  const svgRef = React.useRef<SVGSVGElement | null>(null);

  if (values.length === 0) {
    return (
      <div
        role="img"
        aria-label={ariaLabel}
        className={cn("h-14 w-full rounded-md bg-srapi-card-muted/40", className)}
      />
    );
  }

  const max = Math.max(...values, 1);
  const min = Math.min(...values, 0);
  const span = max - min || 1;
  const flat = values.length === 1 || max === min;

  // A single bucket (or a perfectly flat series) has no slope to plot — draw a
  // flat midline across the full width so the card shows a reading, not a blank.
  const series = values.length === 1 ? [values[0], values[0]] : values;
  const step = (width - pad * 2) / (series.length - 1);

  const points = series.map((v, i) => {
    const x = pad + i * step;
    const y = flat ? height / 2 : height - pad - ((v - min) / span) * (height - pad * 2);
    return [x, y] as const;
  });

  const line = points
    .map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`)
    .join(" ");
  const fill =
    area && points.length > 1
      ? `${line} L${points[points.length - 1][0].toFixed(1)} ${height - pad} L${points[0][0].toFixed(1)} ${height - pad} Z`
      : "";

  const handleMove = (event: React.MouseEvent<SVGSVGElement>) => {
    const svg = svgRef.current;
    if (!svg) return;
    const rect = svg.getBoundingClientRect();
    if (rect.width === 0) return;
    const relX = ((event.clientX - rect.left) / rect.width) * width;
    const len = values.length;
    const dataStep = len <= 1 ? width - pad * 2 : (width - pad * 2) / (len - 1);
    const idx = dataStep > 0 ? Math.round((relX - pad) / dataStep) : 0;
    setActiveIdx(Math.max(0, Math.min(len - 1, idx)));
  };

  const dot =
    activeIdx != null
      ? (() => {
          const i = Math.min(activeIdx, points.length - 1);
          const [cx, cy] = points[i];
          return { cx, cy };
        })()
      : null;

  const chart = (
    <svg
      ref={svgRef}
      role="img"
      aria-label={ariaLabel}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className={cn("h-14 w-full", className)}
      onMouseMove={handleMove}
      onMouseLeave={() => setActiveIdx(null)}
    >
      {fill ? <path d={fill} className="fill-srapi-primary/10" /> : null}
      <path
        d={line}
        fill="none"
        className="stroke-srapi-primary"
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
      {dot ? (
        <g pointerEvents="none">
          <circle cx={dot.cx} cy={dot.cy} r={3} className="fill-srapi-card stroke-srapi-card" strokeWidth={2} />
          <circle cx={dot.cx} cy={dot.cy} r={2} className="fill-srapi-primary" />
        </g>
      ) : null}
    </svg>
  );

  if (!tooltipTitle) return chart;

  const idx = activeIdx ?? values.length - 1;
  const value = values[Math.min(idx, values.length - 1)] ?? 0;
  const rows: DataTooltipRow[] = [];
  const labelAtIdx = tooltipLabels?.[idx];
  rows.push({
    label: labelAtIdx ?? `#${idx + 1}`,
    value: formatValue(value),
  });
  if (tooltipExtraRows && tooltipExtraRows.length > 0) {
    rows.push(...tooltipExtraRows);
  }

  return (
    <DataTooltip title={tooltipTitle} rows={rows} footer={tooltipFooter} side="top">
      <span className="block w-full">{chart}</span>
    </DataTooltip>
  );
}
