import * as React from "react";
import { cn } from "@/lib/cn";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      ref={ref}
      type={type}
      className={cn(
        // 1px inner letterpress highlight gives the input the same "stamped into
        // paper" feel as cards. Focus deepens the border without an outer ring
        // so it stays editorial-quiet.
        "h-10 w-full rounded-lg border border-srapi-border-strong bg-srapi-card px-3.5 text-sm text-srapi-text-primary shadow-[inset_0_1px_0_0_rgba(255,255,255,0.65)] transition-[border-color,background-color,box-shadow] duration-150 ease-[var(--ease-out-quint)]",
        "placeholder:text-srapi-text-tertiary",
        "outline-none hover:border-srapi-text-tertiary hover:bg-srapi-card focus:border-srapi-text-secondary focus:bg-srapi-card",
        "aria-[invalid=true]:border-srapi-error aria-[invalid=true]:hover:border-srapi-error aria-[invalid=true]:focus:border-srapi-error",
        "disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  ),
);
Input.displayName = "Input";
