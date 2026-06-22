"use client";

import * as React from "react";
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import { cn } from "@/lib/cn";

export const TooltipProvider = TooltipPrimitive.Provider;
export const Tooltip = TooltipPrimitive.Root;
export const TooltipTrigger = TooltipPrimitive.Trigger;

/**
 * TooltipContent — 两套外观：
 *   variant="hint"  默认。深色反相，短文案场景（按钮/字段帮助）。
 *   variant="rich"  浅色 card，用于 <DataTooltip> 富数据气泡（含 sparkline、
 *                   breakdown 行），跟正文卡片同源 token，hover 不刺眼。
 */
export const TooltipContent = React.forwardRef<
  React.ElementRef<typeof TooltipPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Content> & {
    variant?: "hint" | "rich";
  }
>(({ className, sideOffset = 8, variant = "hint", ...props }, ref) => (
  <TooltipPrimitive.Portal>
    <TooltipPrimitive.Content
      ref={ref}
      sideOffset={sideOffset}
      className={cn(
        "srapi-anim-pop z-50",
        variant === "hint"
          ? "max-w-72 rounded-lg border border-srapi-text-primary bg-srapi-text-primary px-2.5 py-1.5 text-xs leading-relaxed text-srapi-bg shadow-md"
          : "max-w-sm rounded-lg border border-srapi-border bg-srapi-card p-3 text-xs text-srapi-text-primary shadow-md",
        className,
      )}
      {...props}
    />
  </TooltipPrimitive.Portal>
));
TooltipContent.displayName = "TooltipContent";
