"use client";

import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";

const buttonVariants = cva(
  cn(
    "inline-flex items-center justify-center gap-2",
    "rounded-full font-mono text-xs font-bold uppercase tracking-widest",
    "transition-all active:scale-[0.96]",
    "border focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary focus-visible:ring-offset-2 focus-visible:ring-offset-srapi-bg",
    "disabled:pointer-events-none disabled:opacity-40",
  ),
  {
    variants: {
      variant: {
        primary: cn(
          "border-srapi-text-primary bg-srapi-text-primary text-srapi-bg",
          "hover:bg-transparent hover:text-srapi-text-primary",
        ),
        secondary: cn(
          "border-srapi-border bg-srapi-card-muted text-srapi-text-primary",
          "hover:bg-srapi-card-muted/70",
        ),
        outline: cn(
          "border-srapi-border bg-transparent text-srapi-text-primary",
          "hover:bg-srapi-card-muted",
        ),
        ghost: cn(
          "border-transparent bg-transparent text-srapi-text-secondary",
          "hover:text-srapi-text-primary hover:bg-srapi-card-muted/50",
        ),
        danger: cn(
          "border-srapi-error/30 bg-srapi-error/5 text-srapi-error",
          "hover:bg-srapi-error/10",
        ),
        accent: cn(
          "border-srapi-primary bg-srapi-primary text-white",
          "hover:bg-srapi-primary-hover",
        ),
      },
      size: {
        sm: "px-3 py-2 text-2xs",
        md: "px-5 py-3.5 text-xs",
        lg: "px-6 py-4 text-sm",
        xl: "px-7 py-4 text-sm",
        icon: "h-9 w-9 p-0",
      },
    },
    defaultVariants: {
      variant: "primary",
      size: "md",
    },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return (
      <Comp ref={ref} className={cn(buttonVariants({ variant, size }), className)} {...props} />
    );
  },
);
Button.displayName = "Button";

export { buttonVariants };
