"use client";

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";

const badgeVariants = cva(
  "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 font-mono text-2xs font-bold uppercase tracking-wider",
  {
    variants: {
      variant: {
        neutral: "border-srapi-border bg-srapi-card-muted text-srapi-text-secondary",
        success: "border-srapi-success/30 bg-srapi-success/5 text-srapi-success",
        warning: "border-srapi-warning/30 bg-srapi-warning/5 text-srapi-warning",
        danger: "border-srapi-error/30 bg-srapi-error/5 text-srapi-error",
        info: "border-srapi-info/30 bg-srapi-info/5 text-srapi-info",
        accent: "border-srapi-primary/30 bg-srapi-primary/5 text-srapi-primary",
      },
      size: {
        sm: "px-2 py-0.5 text-2xs",
        md: "px-2.5 py-0.5 text-2xs",
      },
    },
    defaultVariants: {
      variant: "neutral",
      size: "md",
    },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, size, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant, size }), className)} {...props} />;
}

export { badgeVariants };
