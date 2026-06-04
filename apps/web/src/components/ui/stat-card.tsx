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
    if (!enabled) {
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
    <Card className={cn("flex flex-col p-5", className)} style={style}>
      <div className="flex items-center justify-between">
        <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">{label}</span>
        {trend && (
          <span
            className={cn(
              "font-mono text-2xs tabular",
              trend.dir === "up" ? "text-srapi-success" : "text-srapi-error",
            )}
          >
            {trend.dir === "up" ? "↑" : "↓"} {trend.text}
          </span>
        )}
      </div>
      <div className="mt-3 font-serif text-3xl leading-none text-srapi-text-primary tabular">
        {display}
        {unit && (
          <span className="ml-1.5 text-sm font-sans font-normal text-srapi-text-tertiary">
            {unit}
          </span>
        )}
      </div>
      {spark && spark.length >= 2 && (
        <div className="mt-3.5">
          <Sparkline values={spark} ariaLabel={label} className="h-8" />
        </div>
      )}
      {hint && <div className="mt-2.5 font-mono text-2xs text-srapi-text-tertiary">{hint}</div>}
    </Card>
  );
}
