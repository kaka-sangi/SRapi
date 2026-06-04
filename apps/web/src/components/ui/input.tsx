import * as React from "react";
import { cn } from "@/lib/cn";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      ref={ref}
      type={type}
      className={cn(
        "h-10 w-full rounded-lg border border-srapi-border-strong bg-srapi-card px-3.5 text-sm text-srapi-text-primary transition-colors",
        "placeholder:text-srapi-text-tertiary",
        "outline-none hover:border-srapi-text-tertiary focus:border-srapi-text-secondary",
        "aria-[invalid=true]:border-srapi-error aria-[invalid=true]:hover:border-srapi-error aria-[invalid=true]:focus:border-srapi-error",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  ),
);
Input.displayName = "Input";
