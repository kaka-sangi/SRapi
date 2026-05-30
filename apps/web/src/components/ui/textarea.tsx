"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export type TextareaProps = React.TextareaHTMLAttributes<HTMLTextAreaElement>;

/**
 * SRapi design-system textarea. Mirrors the Input treatment. Promoted from the
 * admin pages so multi-line inputs are consistent app-wide.
 */
export const Textarea = React.forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, ...props }, ref) => (
    <textarea
      ref={ref}
      className={cn(
        "w-full rounded-xl border border-srapi-border bg-srapi-bg px-3.5 py-3 font-mono text-xs text-srapi-text-primary",
        "placeholder:text-srapi-text-secondary/50",
        "focus:border-srapi-primary focus:outline-none focus:ring-2 focus:ring-srapi-primary/20",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  ),
);
Textarea.displayName = "Textarea";
