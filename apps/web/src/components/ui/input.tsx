"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export type InputProps = React.InputHTMLAttributes<HTMLInputElement>;

export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type = "text", ...props }, ref) => (
    <input
      ref={ref}
      type={type}
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
Input.displayName = "Input";
