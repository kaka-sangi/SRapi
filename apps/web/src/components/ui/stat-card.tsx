"use client";

import * as React from "react";
import { cn } from "@/lib/cn";
import { Card } from "./card";
import { Sparkline } from "@/components/charts/sparkline";

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
    <Card className={cn("flex flex-col p-5", className)}>
      <div className="skeleton-shimmer h-3 w-20 rounded bg-srapi-card-muted" />
      <div className="mt-4 skeleton-shimmer h-8 w-24 rounded bg-srapi-card-muted" />
      <div className="mt-3 skeleton-shimmer h-2.5 w-16 rounded bg-srapi-card-muted" />
    </Card>
  );
}

export function StatCard({
  label,
  value,
  unit,
  hint,
  trend,
  spark,
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
        "group relative flex flex-col overflow-hidden p-5",
        // A 1px ember accent runs along the top edge — quiet by default,
        // saturates on hover. Replaces the generic "colored card" trope.
        "before:pointer-events-none before:absolute before:inset-x-5 before:top-0 before:h-px before:bg-gradient-to-r before:from-transparent before:via-srapi-border-strong before:to-transparent before:opacity-70 before:transition-opacity before:duration-200 hover:before:via-srapi-primary/45 hover:before:opacity-100",
        className,
      )}
      style={style}
    >
      <div className="flex items-center justify-between">
        <span className="font-mono text-2xs uppercase tracking-[0.18em] text-srapi-text-tertiary">
          {label}
        </span>
        {trend && (
          <span
            className={cn(
              "inline-flex items-center gap-0.5 rounded-full border px-1.5 py-0.5 font-mono text-[10px] tabular",
              trend.dir === "up"
                ? "border-srapi-success/25 bg-srapi-success/10 text-srapi-success"
                : "border-srapi-error/25 bg-srapi-error/10 text-srapi-error",
            )}
          >
            {trend.dir === "up" ? "↑" : "↓"} {trend.text}
          </span>
        )}
      </div>
      <div className="mt-3 flex items-baseline gap-1.5 font-serif text-[2.5rem] leading-none tracking-tight text-srapi-text-primary tabular">
        <span>{display}</span>
        {unit && (
          <span className="font-sans text-sm font-normal text-srapi-text-tertiary">{unit}</span>
        )}
      </div>
      {spark && spark.length >= 2 && (
        <div className="mt-3.5">
          <Sparkline values={spark} ariaLabel={label} className="h-8" />
        </div>
      )}
      {hint && (
        <div className="mt-2.5 font-mono text-2xs text-srapi-text-tertiary">{hint}</div>
      )}
    </Card>
  );
}
