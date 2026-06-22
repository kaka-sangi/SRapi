import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/cn";

const badgeVariants = cva(
  // Editorial badge: hairline border, soft warm fill so semantics
  // (success / warn / danger) read at a glance without screaming color.
  "inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 font-mono text-2xs tracking-wide",
  {
    variants: {
      variant: {
        neutral:
          "border-srapi-border bg-srapi-card-muted/60 text-srapi-text-secondary",
        success:
          "border-srapi-success/25 bg-srapi-success/10 text-srapi-success",
        warning:
          "border-srapi-warning/25 bg-srapi-warning/10 text-srapi-warning",
        danger: "border-srapi-error/25 bg-srapi-error/10 text-srapi-error",
        info: "border-srapi-border bg-srapi-card text-srapi-text-primary",
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
