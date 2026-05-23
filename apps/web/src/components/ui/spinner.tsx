"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export interface SpinnerProps extends React.HTMLAttributes<HTMLDivElement> {
  size?: number;
  label?: string;
}

export const Spinner = React.forwardRef<HTMLDivElement, SpinnerProps>(
  ({ className, size = 24, label, ...props }, ref) => (
    <div
      ref={ref}
      role="status"
      aria-live="polite"
      className={cn("inline-flex flex-col items-center gap-2", className)}
      {...props}
    >
      <span
        aria-hidden="true"
        style={{ width: size, height: size }}
        className="animate-spin rounded-full border-2 border-srapi-border border-t-srapi-primary"
      />
      {label ? <span className="text-xs text-srapi-text-secondary">{label}</span> : null}
      <span className="sr-only">{label ?? "Loading"}</span>
    </div>
  ),
);
Spinner.displayName = "Spinner";
