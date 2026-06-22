import * as React from "react";
import { cn } from "@/lib/cn";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      ref={ref}
      type={type}
      className={cn(
        // Modern soft input: pill-rounded, hairline border, calm focus deepens the border only.
        "h-10 w-full rounded-xl border border-srapi-border bg-srapi-card px-3.5 text-sm text-srapi-text-primary transition-[border-color,background-color] duration-150 ease-[var(--ease-out-quint)]",
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
