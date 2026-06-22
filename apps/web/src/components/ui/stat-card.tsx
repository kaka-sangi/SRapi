"use client";

import * as React from "react";
import { cn } from "@/lib/cn";
import { Card } from "./card";
import { Sparkline } from "@/components/charts/sparkline";
import { DataTooltip, type DataTooltipRow } from "./data-tooltip";

/**
 * Count a number up from its previous value to `target` on a rAF loop
 * (easeOutCubic). Skips straight to the value when the data is non-numeric,
 * on first SSR paint, or when the user prefers reduced motion.
 */
function useCountUp(target: number, enabled: boolean, duration = 750): number {
  const [n, setN] = React.useState(enabled ? 0 : target);
  const fromRef = React.useRef(0);
  const rafRef = React.useRef<number | null>(null);

  React.useEffect(() => {
    // This effect drives a requestAnimationFrame count-up. The two synchronous
    // setN(target) calls are the skip-animation paths (disabled / reduced
    // motion); they jump straight to the final value with no rAF loop.
    if (!enabled) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setN(target);
      return;
    }
    const reduce =
      typeof window !== "undefined" &&
      window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
    if (reduce) {
      fromRef.current = target;
      setN(target);
      return;
    }
    const from = fromRef.current;
    const start = performance.now();
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / duration);
      const eased = 1 - Math.pow(1 - t, 3); // easeOutCubic — fast then settle
      setN(from + (target - from) * eased);
      if (t < 1) {
        rafRef.current = requestAnimationFrame(tick);
      } else {
        fromRef.current = target;
      }
    };
    rafRef.current = requestAnimationFrame(tick);
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
    };
  }, [target, enabled, duration]);

  return n;
}

export function StatCardSkeleton({ className }: { className?: string }) {
  return (
    <Card className={cn("flex flex-col p-6", className)}>
      <div className="flex items-center justify-between">
        <div className="skeleton-shimmer h-3 w-20 rounded bg-srapi-card-muted" />
        <div className="skeleton-shimmer size-9 rounded-xl bg-srapi-card-muted" />
      </div>
      <div className="mt-5 skeleton-shimmer h-9 w-28 rounded bg-srapi-card-muted" />
      <div className="mt-3 skeleton-shimmer h-2.5 w-20 rounded bg-srapi-card-muted" />
    </Card>
  );
}

export interface StatCardTooltip {
  title?: React.ReactNode;
  primary?: React.ReactNode;
  rows?: DataTooltipRow[];
  footer?: React.ReactNode;
}

export function StatCard({
  label,
  value,
  unit,
  hint,
  trend,
  spark,
  icon,
  tooltip,
  className,
  style,
  format,
}: {
  label: string;
  /** A number animates (count-up); a string renders as-is (e.g. "—", "98%"). */
  value: string | number;
  unit?: string;
  hint?: React.ReactNode;
  trend?: { dir: "up" | "down"; text: string };
  spark?: number[];
  /** Optional lucide icon — renders as a soft accent bubble in the top-right. */
  icon?: React.ReactNode;
  /** When provided, hovering the icon bubble reveals a rich data popover. */
  tooltip?: StatCardTooltip;
  className?: string;
  style?: React.CSSProperties;
  /** Formats the live count-up value; defaults to a rounded integer. */
  format?: (n: number) => string;
}) {
  const isNum = typeof value === "number";
  const counted = useCountUp(isNum ? value : 0, isNum);
  const display = isNum ? (format ? format(counted) : String(Math.round(counted))) : value;

  return (
    <Card
      className={cn(
        "group relative flex flex-col overflow-hidden p-6",
        className,
      )}
      style={style}
    >
      <div className="flex items-start justify-between gap-3">
        <span className="text-xs font-medium uppercase tracking-[0.14em] text-srapi-text-tertiary">
          {label}
        </span>
        {icon && (
          tooltip ? (
            <DataTooltip
              title={tooltip.title ?? label}
              primary={tooltip.primary}
              rows={tooltip.rows}
              footer={tooltip.footer}
              side="left"
            >
              <span className="grid size-9 shrink-0 cursor-help place-items-center rounded-xl bg-srapi-accent-soft text-srapi-primary transition-transform duration-200 group-hover:scale-105 [&>svg]:size-4">
                {icon}
              </span>
            </DataTooltip>
          ) : (
            <span className="grid size-9 shrink-0 place-items-center rounded-xl bg-srapi-accent-soft text-srapi-primary transition-transform duration-200 group-hover:scale-105 [&>svg]:size-4">
              {icon}
            </span>
          )
        )}
      </div>
      <div className="mt-4 flex items-baseline gap-1.5 text-3xl font-semibold leading-none tracking-tight text-srapi-text-primary tabular sm:text-[2.25rem]">
        <span>{display}</span>
        {unit && (
          <span className="text-sm font-medium text-srapi-text-tertiary">{unit}</span>
        )}
      </div>
      <div className="mt-3 flex items-center justify-between gap-2">
        {hint ? (
          <div className="text-xs text-srapi-text-tertiary">{hint}</div>
        ) : (
          <span aria-hidden />
        )}
        {trend && (
          <span
            className={cn(
              "inline-flex items-center gap-0.5 rounded-full px-2 py-0.5 text-[11px] font-medium tabular",
              trend.dir === "up"
                ? "bg-srapi-success/12 text-srapi-success"
                : "bg-srapi-error/12 text-srapi-error",
            )}
          >
            {trend.dir === "up" ? "↑" : "↓"} {trend.text}
          </span>
        )}
      </div>
      {spark && spark.length >= 2 && (
        <div className="mt-4 border-t border-srapi-border/70 pt-3">
          <Sparkline values={spark} ariaLabel={label} className="h-8" />
        </div>
      )}
    </Card>
  );
}
