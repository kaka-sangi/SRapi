import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";

const badgeVariants = cva(
  "inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 font-mono text-2xs",
  {
    variants: {
      variant: {
        neutral: "border-srapi-border text-srapi-text-secondary",
        success: "border-srapi-border text-srapi-success",
        warning: "border-srapi-border text-srapi-warning",
        danger: "border-srapi-border text-srapi-error",
        info: "border-srapi-border text-srapi-text-primary",
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
