"use client";

import * as React from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/cn";

export type SelectProps = React.SelectHTMLAttributes<HTMLSelectElement>;

/**
 * SRapi design-system select. A styled native `<select>` (keeps full keyboard
 * and form semantics) with a custom chevron. Promoted from the admin pages so
 * every surface shares one token-driven control.
 */
export const Select = React.forwardRef<HTMLSelectElement, SelectProps>(
  ({ className, children, ...props }, ref) => (
    <div className="relative inline-flex w-full">
      <select
        ref={ref}
        className={cn(
          "w-full appearance-none rounded-xl border border-srapi-border bg-srapi-bg px-3.5 py-3 pr-9",
          "font-mono text-xs text-srapi-text-primary",
          "transition-colors focus:border-srapi-primary focus:outline-none focus:ring-2 focus:ring-srapi-primary/20",
          "disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
        {...props}
      >
        {children}
      </select>
      <ChevronDown
        size={14}
        aria-hidden="true"
        className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-srapi-text-secondary"
      />
    </div>
  ),
);
Select.displayName = "Select";
