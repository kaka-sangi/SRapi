import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";

const badgeVariants = cva(
  // Modern pill badge: soft tinted bg, no border, rounded-full. Reads at a
  // glance for semantic state without competing with primary content.
  "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-medium",
  {
    variants: {
      variant: {
        neutral: "bg-srapi-card-muted text-srapi-text-secondary",
        success: "bg-srapi-success/12 text-srapi-success",
        warning: "bg-srapi-warning/12 text-srapi-warning",
        danger: "bg-srapi-error/12 text-srapi-error",
        info: "bg-srapi-card-muted text-srapi-text-secondary",
      },
    },
    defaultVariants: { variant: "neutral" },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

export function Badge({ className, variant, ...props }: BadgeProps) {
  return <span className={cn(badgeVariants({ variant }), className)} {...props} />;
}

export { badgeVariants };
