"use client";

import * as React from "react";
import * as ToastPrimitive from "@radix-ui/react-toast";
import { cva, type VariantProps } from "class-variance-authority";
import { X } from "lucide-react";
import { cn } from "@/lib/cn";

export const ToastProvider = ToastPrimitive.Provider;

export const ToastViewport = React.forwardRef<
  React.ElementRef<typeof ToastPrimitive.Viewport>,
  React.ComponentPropsWithoutRef<typeof ToastPrimitive.Viewport>
>(({ className, ...props }, ref) => (
  <ToastPrimitive.Viewport
    ref={ref}
    className={cn(
      "fixed bottom-0 right-0 z-[60] m-0 flex w-full max-w-sm list-none flex-col gap-2 p-4 outline-none sm:bottom-4 sm:right-4",
      className,
    )}
    {...props}
  />
));
ToastViewport.displayName = "ToastViewport";

// Card-raised toast: warm surface, soft shadow, tone shows as a 2px left rule.
const toastVariants = cva(
  "srapi-anim-toast pointer-events-auto relative flex w-full items-start gap-3 rounded-xl border border-l-2 border-srapi-border bg-srapi-card p-4 pr-9 shadow-md",
  {
    variants: {
      tone: {
        default: "border-l-srapi-border-strong",
        success: "border-l-srapi-success",
        error: "border-l-srapi-error",
      },
    },
    defaultVariants: { tone: "default" },
  },
);

export const Toast = React.forwardRef<
  React.ElementRef<typeof ToastPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof ToastPrimitive.Root> & VariantProps<typeof toastVariants>
>(({ className, tone, ...props }, ref) => (
  <ToastPrimitive.Root ref={ref} className={cn(toastVariants({ tone }), className)} {...props} />
));
Toast.displayName = "Toast";

export const ToastTitle = React.forwardRef<
  React.ElementRef<typeof ToastPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof ToastPrimitive.Title>
>(({ className, ...props }, ref) => (
  <ToastPrimitive.Title
    ref={ref}
    className={cn("text-sm font-medium text-srapi-text-primary", className)}
    {...props}
  />
));
ToastTitle.displayName = "ToastTitle";

export const ToastDescription = React.forwardRef<
  React.ElementRef<typeof ToastPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof ToastPrimitive.Description>
>(({ className, ...props }, ref) => (
  <ToastPrimitive.Description
    ref={ref}
    className={cn("mt-0.5 text-sm leading-relaxed text-srapi-text-secondary", className)}
    {...props}
  />
));
ToastDescription.displayName = "ToastDescription";

export const ToastClose = React.forwardRef<
  React.ElementRef<typeof ToastPrimitive.Close>,
  React.ComponentPropsWithoutRef<typeof ToastPrimitive.Close>
>(({ className, ...props }, ref) => (
  <ToastPrimitive.Close
    ref={ref}
    className={cn(
      "absolute right-2.5 top-2.5 rounded-lg p-1 text-srapi-text-tertiary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary",
      className,
    )}
    aria-label="Close"
    {...props}
  >
    <X className="size-3.5" />
  </ToastPrimitive.Close>
));
ToastClose.displayName = "ToastClose";
