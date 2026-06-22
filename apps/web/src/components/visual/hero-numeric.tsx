"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

/**
 * HeroNumeric —— hero 区超大渐变数字。
 *
 * 用 .text-aurora（炭黑 → 陶土 → 暖金 渐变 mask）替换冷淡的纯黑大数字，
 * 让仪表盘 KPI 有「主角光环」。配合 count-up 动画从 0 弹起到目标值，
 * easeOutCubic 曲线，约 850ms。
 *
 * 适合：dashboard hero 数值（余额、月收入、今日请求数）。
 * 不适合：表格里的密集数字（会喧宾夺主）。
 */
export function HeroNumeric({
  value,
  format,
  unit,
  className,
  duration = 850,
}: {
  /** 数字 → 触发 count-up；字符串 → 原样渲染（占位 "—" 等） */
  value: number | string;
  /** 数字格式化函数（不传则四舍五入） */
  format?: (n: number) => string;
  unit?: React.ReactNode;
  className?: string;
  duration?: number;
}) {
  const isNum = typeof value === "number";
  const counted = useCountUp(isNum ? value : 0, isNum, duration);
  const display = isNum ? (format ? format(counted) : String(Math.round(counted))) : value;

  return (
    <span
      className={cn(
        "text-aurora inline-flex items-baseline gap-2 text-5xl font-semibold leading-none tracking-tight tabular sm:text-6xl",
        className,
      )}
    >
      <span>{display}</span>
      {unit && (
        <span className="text-base font-medium text-srapi-text-tertiary">{unit}</span>
      )}
    </span>
  );
}

function useCountUp(target: number, enabled: boolean, duration: number) {
  const [n, setN] = React.useState(enabled ? 0 : target);
  const fromRef = React.useRef(0);
  const rafRef = React.useRef<number | null>(null);

  React.useEffect(() => {
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
      const eased = 1 - Math.pow(1 - t, 3);
      setN(from + (target - from) * eased);
      if (t < 1) rafRef.current = requestAnimationFrame(tick);
      else fromRef.current = target;
    };
    rafRef.current = requestAnimationFrame(tick);
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
    };
  }, [target, enabled, duration]);

  return n;
}
