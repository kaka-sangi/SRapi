import { cn } from "@/lib/cn";

/**
 * Pure-SVG sparkline — a single value series drawn as a line, optionally filled.
 * No chart library: token colors only, viewBox-scaled so it's fully responsive,
 * and exposed to assistive tech via role="img" + aria-label. Matches the house
 * hand-rolled SVG style (quota-notch-rail, ambient-canvas).
 */
export function Sparkline({
  values,
  ariaLabel,
  area = true,
  className,
  height = 56,
}: {
  values: number[];
  ariaLabel: string;
  area?: boolean;
  className?: string;
  height?: number;
}) {
  const width = 240;
  const pad = 2;

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

  const line = points.map(([x, y], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`).join(" ");
  const fill =
    area && points.length > 1
      ? `${line} L${points[points.length - 1][0].toFixed(1)} ${height - pad} L${points[0][0].toFixed(1)} ${height - pad} Z`
      : "";

  return (
    <svg
      role="img"
      aria-label={ariaLabel}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className={cn("h-14 w-full", className)}
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
    </svg>
  );
}
