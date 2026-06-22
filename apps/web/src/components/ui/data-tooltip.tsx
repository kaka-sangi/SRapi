"use client";

import * as React from "react";
import { Tooltip, TooltipTrigger, TooltipContent } from "./tooltip";
import { cn } from "@/lib/cn";

/**
 * DataTooltip —— 「鼠标悬停在图标上 / 数字上，弹出对应数据明细」。
 *
 * 不是 label 提示（那是 HelpTooltip），而是真正的「上下文数据气泡」：
 *   - 标题（指标名）
 *   - 主值（大数字）
 *   - breakdown 行（多条「key: value」分项）
 *   - 可选 sparkline / 趋势 chip / footnote
 *
 * 设计意图：让仪表盘上的每个 icon 不只是装饰，而是「数据入口」——
 * 把屏幕上没空间放下的细节做成 on-demand。配合 magnetic-icon 的微抬升，
 * 用户会感觉到「这玩意儿是可点的」。
 *
 * 触发既支持鼠标 hover，也支持键盘焦点（asChild trigger + tabIndex=0）。
 */

export interface DataTooltipRow {
  label: React.ReactNode;
  value: React.ReactNode;
  tone?: "default" | "success" | "warning" | "error" | "muted";
}

export function DataTooltip({
  children,
  title,
  primary,
  rows,
  footer,
  side = "top",
  align = "center",
  delay = 80,
  className,
}: {
  /** 触发器 —— 任何可 hover/focus 的子元素，建议 icon bubble、数字、状态徽章 */
  children: React.ReactNode;
  /** 顶部小标题 */
  title?: React.ReactNode;
  /** 主数据行（粗体大字，可选） */
  primary?: React.ReactNode;
  /** breakdown 多行 */
  rows?: DataTooltipRow[];
  /** 底部辅助文字（脚注 / 链接 / 时间戳） */
  footer?: React.ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
  /** ms，默认 80 ms 让 hover 响应更跟手 */
  delay?: number;
  className?: string;
}) {
  return (
    <Tooltip delayDuration={delay}>
      <TooltipTrigger asChild>
        <span tabIndex={0} className="inline-flex outline-none">
          {children}
        </span>
      </TooltipTrigger>
      <TooltipContent variant="rich" side={side} align={align} className={cn("space-y-2", className)}>
        {title ? (
          <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {title}
          </div>
        ) : null}
        {primary ? (
          <div className="text-base font-semibold tracking-tight text-srapi-text-primary tabular">
            {primary}
          </div>
        ) : null}
        {rows && rows.length > 0 ? (
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 border-t border-srapi-border/60 pt-2">
            {rows.map((row, i) => (
              <React.Fragment key={i}>
                <dt className="text-[11px] text-srapi-text-tertiary">{row.label}</dt>
                <dd
                  className={cn(
                    "text-right text-[12px] font-medium tabular",
                    row.tone === "success" && "text-srapi-success",
                    row.tone === "warning" && "text-srapi-warning",
                    row.tone === "error" && "text-srapi-error",
                    row.tone === "muted" && "text-srapi-text-tertiary",
                    (!row.tone || row.tone === "default") && "text-srapi-text-primary",
                  )}
                >
                  {row.value}
                </dd>
              </React.Fragment>
            ))}
          </dl>
        ) : null}
        {footer ? (
          <div className="border-t border-srapi-border/60 pt-2 text-[11px] text-srapi-text-tertiary">
            {footer}
          </div>
        ) : null}
      </TooltipContent>
    </Tooltip>
  );
}
