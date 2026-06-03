import { cn } from "@/lib/cn";

/**
 * Multi-series line + area trend chart. Pure SVG (no chart library), token
 * colors, responsive via viewBox. Each series is drawn as a line over a faint
 * baseline grid with a translucent area fill; an optional legend names them.
 * A single data point (or a flat series) renders as a flat line across the
 * full width so a fresh/low-traffic window is never blank.
 */
export type TrendTone = "primary" | "secondary" | "success";
export type TrendSeries = { key: string; label: string; values: number[]; tone: TrendTone };

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

export function TrendChart({
  series,
  ariaLabel,
  height = 132,
  showLegend = true,
  className,
}: {
  series: TrendSeries[];
  ariaLabel: string;
  height?: number;
  showLegend?: boolean;
  className?: string;
}) {
  const W = 480;
  const H = height;
  const padX = 6;
  const padTop = 10;
  const padBot = 10;
  const len = Math.max(0, ...series.map((s) => s.values.length));

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
  const xAt = (n: number, i: number) => (n <= 1 ? [padX, W - padX][i] : padX + (i * (W - 2 * padX)) / (n - 1));
  const gridYs = [0, 0.25, 0.5, 0.75, 1].map((f) => padTop + f * plotH);

  return (
    <div className={className}>
      {showLegend ? (
        <div className="mb-2 flex flex-wrap gap-x-4 gap-y-1">
          {series.map((s) => (
            <span key={s.key} className="inline-flex items-center gap-1.5 font-mono text-2xs text-srapi-text-tertiary">
              <span className={cn("inline-block h-2 w-2 rounded-full", DOT[s.tone])} />
              {s.label}
            </span>
          ))}
        </div>
      ) : null}
      <svg
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
          const line = pts.map(([x, yy], i) => `${i === 0 ? "M" : "L"}${x.toFixed(1)} ${yy.toFixed(1)}`).join(" ");
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
      </svg>
    </div>
  );
}
