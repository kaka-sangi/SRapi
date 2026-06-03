import { cn } from "@/lib/cn";

/**
 * §4.3 物理仪表型 Quota 轨道：1px 背景线 + 垂直 notch 指示针。
 * <30% 转黄，<10% 转红。
 */
export function QuotaNotchRail({
  value,
  className,
}: {
  /** 0–100 百分比；null 视为无数据 */
  value: number | null;
  className?: string;
}) {
  const pct = value == null ? 0 : Math.max(0, Math.min(100, value));
  const level = value == null ? "none" : pct < 10 ? "crit" : pct < 30 ? "warn" : "ok";
  return (
    <div
      className={cn("quota-rail", className)}
      role="meter"
      aria-valuenow={value ?? undefined}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      {value != null && <span className="quota-notch" data-level={level} style={{ left: `${pct}%` }} />}
    </div>
  );
}
