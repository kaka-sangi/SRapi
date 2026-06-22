"use client";

import * as React from "react";
import * as TabsPrimitive from "@radix-ui/react-tabs";
import { cn } from "@/lib/cn";

export const Tabs = TabsPrimitive.Root;

export const TabsList = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.List>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.List>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.List
    ref={ref}
    className={cn(
      "inline-flex items-center gap-1 rounded-xl border border-srapi-border bg-srapi-card-muted p-1",
      className,
    )}
    {...props}
  />
));
TabsList.displayName = "TabsList";

export const TabsTrigger = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.Trigger
    ref={ref}
    className={cn(
      "relative inline-flex items-center justify-center rounded-lg px-3.5 py-1.5 text-sm font-medium text-srapi-text-secondary transition-[color,background-color] duration-150 ease-[var(--ease-out-quint)]",
      "hover:text-srapi-text-primary data-[state=inactive]:hover:bg-srapi-card/60",
      "data-[state=active]:bg-srapi-card data-[state=active]:text-srapi-text-primary data-[state=active]:shadow-[0_1px_2px_rgba(28,26,23,0.06),inset_0_1px_0_rgba(255,255,255,0.65)]",
      // A 2px terracotta underline appears under the active tab — printed,
      // not glowing. Animates in via inline width transition.
      "after:pointer-events-none after:absolute after:inset-x-3 after:bottom-0.5 after:h-[2px] after:rounded-full after:bg-srapi-primary after:opacity-0 after:transition-opacity after:duration-200 data-[state=active]:after:opacity-100",
      className,
    )}
    {...props}
  />
));
TabsTrigger.displayName = "TabsTrigger";

export const TabsContent = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.Content>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.Content ref={ref} className={cn("mt-4 outline-none", className)} {...props} />
));
TabsContent.displayName = "TabsContent";
