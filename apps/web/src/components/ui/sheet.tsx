"use client";

import * as React from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/cn";

/**
 * Sheet = Radix Dialog rendered as an edge-anchored panel.
 * Used for the mobile nav drawer (side="left") and the §7.2 scheduler-decision
 * bottom sheet (side="bottom").
 */
export const Sheet = DialogPrimitive.Root;
export const SheetTrigger = DialogPrimitive.Trigger;
export const SheetClose = DialogPrimitive.Close;

type Side = "left" | "right" | "bottom";

const SIDE_CLASSES: Record<Side, string> = {
  left: "inset-y-0 left-0 h-full w-80 max-w-[85vw] overflow-y-auto border-r md:rounded-r-2xl srapi-anim-sheet-left",
  right: "inset-y-0 right-0 h-full w-80 max-w-[85vw] overflow-y-auto border-l md:rounded-l-2xl srapi-anim-sheet-right",
  bottom: "inset-x-0 bottom-0 max-h-[85dvh] w-full overflow-y-auto rounded-t-2xl border-t srapi-anim-sheet-bottom",
};

export const SheetContent = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Content> & { side?: Side }
>(({ className, children, side = "left", ...props }, ref) => (
  <DialogPrimitive.Portal>
    {/* Frosted aurora overlay — dark scrim with primary-tinted radial highlight
        so dialogs feel «lit from behind», not just dim. */}
    <DialogPrimitive.Overlay className="srapi-anim-fade fixed inset-0 z-50 bg-[radial-gradient(ellipse_at_top,_color-mix(in_oklab,_var(--color-srapi-primary)_18%,_transparent),_rgba(0,0,0,0.55)_60%)] backdrop-blur-[6px]" />
    <DialogPrimitive.Content
      ref={ref}
      className={cn(
        "glass-frosted-strong fixed z-50 flex flex-col border-srapi-border shadow-[0_24px_64px_-24px_rgba(28,24,20,0.32),0_12px_32px_-10px_rgba(28,24,20,0.18)]",
        SIDE_CLASSES[side],
        className,
      )}
      {...props}
    >
      {children}
      <DialogPrimitive.Close className="absolute right-4 top-4 rounded-lg p-1 text-srapi-text-secondary hover:bg-srapi-card-muted hover:text-srapi-text-primary">
        <X className="size-4" />
        <span className="sr-only">Close</span>
      </DialogPrimitive.Close>
    </DialogPrimitive.Content>
  </DialogPrimitive.Portal>
));
SheetContent.displayName = "SheetContent";

export const SheetTitle = React.forwardRef<
  React.ElementRef<typeof DialogPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(({ className, ...props }, ref) => (
  <DialogPrimitive.Title
    ref={ref}
    className={cn("text-lg font-semibold tracking-tight text-srapi-text-primary", className)}
    {...props}
  />
));
SheetTitle.displayName = "SheetTitle";

export const SheetDescription = DialogPrimitive.Description;
