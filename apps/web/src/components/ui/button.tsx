"use client";

import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { Loader2 } from "lucide-react";
import { cn } from "@/lib/cn";

const buttonVariants = cva(
  // shared rhythm: medium weight, tight optical tracking, calm transitions
  "inline-flex select-none items-center justify-center gap-2 whitespace-nowrap font-medium transition-[background-color,color,box-shadow,transform,opacity] duration-150 ease-[var(--ease-out-quint)] disabled:pointer-events-none disabled:opacity-40 [&_svg]:size-4 [&_svg]:shrink-0 active:scale-[0.985]",
  {
    variants: {
      variant: {
        // §8 主操作：亮=炭黑底 / 暗=羊皮白底
        primary: "btn-raise bg-srapi-invert text-srapi-invert-fg hover:opacity-90",
        accent: "btn-raise bg-srapi-primary text-white hover:bg-srapi-primary-hover",
        outline:
          "border border-srapi-border-strong bg-srapi-card text-srapi-text-primary hover:border-srapi-text-tertiary hover:bg-srapi-card-muted",
        ghost:
          "text-srapi-text-secondary hover:bg-srapi-card-muted hover:text-srapi-text-primary",
        danger: "btn-raise bg-srapi-error text-white hover:opacity-90",
        link: "text-srapi-primary underline-offset-4 hover:underline",
      },
      size: {
        sm: "h-8 rounded-lg px-3 text-xs",
        md: "h-10 rounded-xl px-4 text-sm",
        lg: "h-11 rounded-xl px-6 text-sm",
        icon: "size-10 rounded-xl",
      },
    },
    defaultVariants: { variant: "primary", size: "md" },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  /** Show an inline spinner and auto-disable while a mutation is in flight. */
  loading?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, loading = false, disabled, children, ...props }, ref) => {
    // Slot requires a single child, so the loading affordance only applies to a
    // real <button>. The spinner inherits the button's text color (text-current).
    if (asChild) {
      return (
        <Slot ref={ref} className={cn(buttonVariants({ variant, size }), className)} {...props}>
          {children}
        </Slot>
      );
    }
    return (
      <button
        ref={ref}
        className={cn(buttonVariants({ variant, size }), className)}
        disabled={disabled || loading}
        aria-busy={loading || undefined}
        {...props}
      >
        {loading ? <Loader2 className="size-4 shrink-0 animate-spin" aria-hidden /> : null}
        {children}
      </button>
    );
  },
);
Button.displayName = "Button";

export { buttonVariants };
