import * as React from "react";
import { cn } from "@/lib/cn";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      ref={ref}
      type={type}
      className={cn(
        "h-9 w-full rounded-lg border border-srapi-border bg-transparent px-3 text-sm text-srapi-text-primary transition-colors duration-150",
        "placeholder:text-srapi-text-tertiary",
        "outline-none hover:border-srapi-border-strong focus:border-srapi-border-strong",
        "aria-[invalid=true]:border-srapi-error aria-[invalid=true]:hover:border-srapi-error aria-[invalid=true]:focus:border-srapi-error",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  ),
);
Input.displayName = "Input";
