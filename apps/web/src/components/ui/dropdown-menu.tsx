"use client";

import * as React from "react";
import * as DropdownMenuPrimitive from "@radix-ui/react-dropdown-menu";
import { cn } from "@/lib/cn";

export const DropdownMenu = DropdownMenuPrimitive.Root;
export const DropdownMenuTrigger = DropdownMenuPrimitive.Trigger;
export const DropdownMenuGroup = DropdownMenuPrimitive.Group;
export const DropdownMenuSeparatorRoot = DropdownMenuPrimitive.Separator;

export const DropdownMenuContent = React.forwardRef<
  React.ElementRef<typeof DropdownMenuPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof DropdownMenuPrimitive.Content>
>(({ className, sideOffset = 6, ...props }, ref) => (
  <DropdownMenuPrimitive.Portal>
    <DropdownMenuPrimitive.Content
      ref={ref}
      sideOffset={sideOffset}
      className={cn(
        "srapi-anim-pop tactile-card z-50 min-w-[10rem] overflow-hidden rounded-xl border border-srapi-border bg-srapi-card p-1",
        className,
      )}
      {...props}
    />
  </DropdownMenuPrimitive.Portal>
));
DropdownMenuContent.displayName = "DropdownMenuContent";

export const DropdownMenuItem = React.forwardRef<
  React.ElementRef<typeof DropdownMenuPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof DropdownMenuPrimitive.Item> & { destructive?: boolean }
>(({ className, destructive, ...props }, ref) => (
  <DropdownMenuPrimitive.Item
    ref={ref}
    className={cn(
      "group relative flex cursor-pointer select-none items-center gap-2 rounded-lg px-2.5 py-2 text-sm outline-none transition-[background-color,color,padding-left] duration-150",
      // Highlight reveals a left terracotta indicator instead of a generic
      // colored background — same vocabulary as the sidebar active state.
      "data-[highlighted]:bg-srapi-card-muted data-[highlighted]:pl-3 data-[highlighted]:shadow-[inset_2px_0_0_var(--color-srapi-primary)]",
      destructive
        ? "text-srapi-error data-[highlighted]:shadow-[inset_2px_0_0_var(--color-srapi-error)]"
        : "text-srapi-text-primary",
      className,
    )}
    {...props}
  />
));
DropdownMenuItem.displayName = "DropdownMenuItem";

export function DropdownMenuSeparator({ className }: { className?: string }) {
  return <DropdownMenuSeparatorRoot className={cn("my-1 h-px bg-srapi-border", className)} />;
}

export function DropdownMenuLabel({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("px-2.5 py-1.5 font-mono text-2xs uppercase tracking-wider text-srapi-text-secondary", className)}
      {...props}
    />
  );
}
