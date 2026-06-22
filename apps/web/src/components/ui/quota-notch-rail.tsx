import { cn } from "@/lib/cn";

/**
 * Quota gauge rail: a hairline track on srapi-border with a circular notch that
 * slides to the current percentage. Color shifts (ok → warn → crit) signal
 * remaining headroom so the rail reads at a glance.
 */
export function QuotaNotchRail({
  value,
  className,
}: {
  /** 0–100 percent; null = no data */
  value: number | null;
  className?: string;
}) {
  const pct = value == null ? 0 : Math.max(0, Math.min(100, value));
  const level = value == null ? "none" : pct < 10 ? "crit" : pct < 30 ? "warn" : "ok";
  const dotColor =
    level === "crit"
      ? "bg-srapi-error"
      : level === "warn"
        ? "bg-srapi-warning"
        : "bg-srapi-primary";
  return (
    <div
      className={cn("relative h-px w-full bg-srapi-border", className)}
      role="meter"
      aria-valuenow={value ?? undefined}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      {value != null && (
        <span
          className={cn(
            "absolute top-1/2 size-2 -translate-x-1/2 -translate-y-1/2 rounded-full transition-[left] duration-500 ease-out",
            dotColor,
          )}
          style={{ left: `${pct}%` }}
        />
      )}
    </div>
  );
}
