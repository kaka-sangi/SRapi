"use client";

import * as React from "react";
import * as PopoverPrimitive from "@radix-ui/react-popover";
import { cn } from "@/lib/cn";

export const Popover = PopoverPrimitive.Root;
export const PopoverTrigger = PopoverPrimitive.Trigger;
export const PopoverAnchor = PopoverPrimitive.Anchor;

export const PopoverContent = React.forwardRef<
  React.ElementRef<typeof PopoverPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof PopoverPrimitive.Content>
>(({ className, align = "start", sideOffset = 6, ...props }, ref) => (
  <PopoverPrimitive.Portal>
    <PopoverPrimitive.Content
      ref={ref}
      align={align}
      sideOffset={sideOffset}
      className={cn(
        // Modern soft popover: big radius, hairline border, layered drop shadow.
        "srapi-anim-pop z-50 max-h-[min(22rem,var(--radix-popover-content-available-height))] overflow-hidden rounded-2xl border border-srapi-border bg-srapi-card p-1.5 text-srapi-text-primary shadow-[0_12px_32px_-12px_rgba(28,24,20,0.18),0_4px_12px_-4px_rgba(28,24,20,0.08)]",
        className,
      )}
      {...props}
    />
  </PopoverPrimitive.Portal>
));
PopoverContent.displayName = "PopoverContent";
