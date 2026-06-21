"use client";

import * as React from "react";
import { Check, Minus } from "lucide-react";
import { cn } from "@/lib/cn";

/**
 * Lightweight checkbox built on a native input (no extra Radix dep). Supports an
 * `indeterminate` visual for "some rows on this page selected" in bulk tables.
 */
export const Checkbox = React.forwardRef<
  HTMLInputElement,
  Omit<React.InputHTMLAttributes<HTMLInputElement>, "type"> & { indeterminate?: boolean }
>(({ className, indeterminate, checked, ...props }, ref) => {
  const innerRef = React.useRef<HTMLInputElement>(null);
  React.useImperativeHandle(ref, () => innerRef.current as HTMLInputElement);
  React.useEffect(() => {
    if (innerRef.current) innerRef.current.indeterminate = Boolean(indeterminate);
  }, [indeterminate]);

  return (
    <span className="relative inline-flex size-4 shrink-0 items-center justify-center">
      <input
        ref={innerRef}
        type="checkbox"
        checked={checked}
        className={cn(
          "peer size-4 cursor-pointer appearance-none rounded border border-srapi-border-strong bg-srapi-card transition-[background-color,border-color,transform] duration-150 active:scale-90",
          "checked:border-srapi-primary checked:bg-srapi-primary",
          "indeterminate:border-srapi-primary indeterminate:bg-srapi-primary",
          "hover:border-srapi-text-tertiary",
          "disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
        {...props}
      />
      <span className="pointer-events-none absolute scale-50 text-srapi-invert-fg opacity-0 transition-[opacity,transform] duration-200 ease-[var(--ease-spring-bounce)] peer-checked:scale-100 peer-checked:opacity-100 peer-indeterminate:opacity-0">
        <Check className="size-3" strokeWidth={3} />
      </span>
      {indeterminate ? (
        <span className="pointer-events-none absolute text-srapi-invert-fg">
          <Minus className="size-3" strokeWidth={3} />
        </span>
      ) : null}
    </span>
  );
});
Checkbox.displayName = "Checkbox";
