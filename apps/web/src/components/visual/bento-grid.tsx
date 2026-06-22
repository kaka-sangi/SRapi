import { cn } from "@/lib/cn";

/**
 * BentoGrid —— 12 列不对称栅格。
 *
 * 摆脱「3 列等宽」的平凡感。子项通过 span 选择跨度，达到 Apple Bento 风：
 *   <BentoGrid>
 *     <BentoItem span="hero">balance card big</BentoItem>
 *     <BentoItem span="tall">quotas tall</BentoItem>
 *     <BentoItem span="quarter">KPI</BentoItem>
 *     <BentoItem span="quarter">KPI</BentoItem>
 *     <BentoItem span="half">trend chart</BentoItem>
 *     <BentoItem span="half">model share</BentoItem>
 *   </BentoGrid>
 *
 * 跨度选项见 globals.css §7.7：hero=8x2 / tall=4x2 / wide=8 / half=6 /
 * third=4 / quarter=3。响应式自动塌缩。
 */
export function BentoGrid({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return <div className={cn("bento", className)}>{children}</div>;
}

export type BentoSpan = "hero" | "tall" | "wide" | "half" | "third" | "quarter";

export function BentoItem({
  span = "third",
  className,
  children,
  style,
}: {
  span?: BentoSpan;
  className?: string;
  children: React.ReactNode;
  style?: React.CSSProperties;
}) {
  return (
    <div className={cn(`bento-${span}`, "min-w-0", className)} style={style}>
      {children}
    </div>
  );
}
