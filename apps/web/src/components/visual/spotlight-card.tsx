"use client";

import * as React from "react";
import { cn } from "@/lib/cn";
import { Card } from "@/components/ui/card";

/**
 * SpotlightCard — 一个会跟着鼠标光斑的卡片。
 *
 * 当指针进入卡片时，卡片表面会浮现一束 420px 半径的暖橙径向高光，
 * 跟随光标移动；离开时柔和淡出。CSS 的渐变中心通过 CSS 自定义属性
 * --spot-x / --spot-y 注入，无 React 重渲染开销。
 *
 * 适合用于：用户仪表盘的 hero 卡（余额、配额）、admin 选项卡片栅格、
 * pricing 推荐方案、登录卡。不要全页堆，会显得轻浮。
 */
export const SpotlightCard = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, children, onMouseMove, ...props }, ref) => {
  const internalRef = React.useRef<HTMLDivElement>(null);
  React.useImperativeHandle(ref, () => internalRef.current as HTMLDivElement);

  function handleMove(e: React.MouseEvent<HTMLDivElement>) {
    const el = internalRef.current;
    if (el) {
      const rect = el.getBoundingClientRect();
      el.style.setProperty("--spot-x", `${e.clientX - rect.left}px`);
      el.style.setProperty("--spot-y", `${e.clientY - rect.top}px`);
    }
    onMouseMove?.(e);
  }

  return (
    <Card
      ref={internalRef}
      onMouseMove={handleMove}
      className={cn("card-spotlight overflow-hidden", className)}
      {...props}
    >
      {children}
    </Card>
  );
});
SpotlightCard.displayName = "SpotlightCard";
