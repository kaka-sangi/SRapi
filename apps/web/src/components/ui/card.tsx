import * as React from "react";
import { cn } from "@/lib/cn";

export const Card = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        // Editorial paper card: tactile letterpress depth + warm border.
        // The transition keeps interactive variants (hover lift) calm and
        // honest — no shimmer, no glow.
        "tactile-card rounded-xl border border-srapi-border bg-srapi-card transition-[box-shadow,transform,border-color] duration-200 ease-[var(--ease-out-quint)]",
        className,
      )}
      {...props}
    />
  ),
);
Card.displayName = "Card";

export const CardHeader = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        "flex items-center justify-between gap-3 border-b border-srapi-border px-6 py-4",
        className,
      )}
      {...props}
    />
  ),
);
CardHeader.displayName = "CardHeader";

export const CardTitle = React.forwardRef<
  HTMLHeadingElement,
  React.HTMLAttributes<HTMLHeadingElement>
>(({ className, ...props }, ref) => (
  <h2
    ref={ref}
    className={cn(
      "font-sans text-base font-medium tracking-tight text-srapi-text-primary",
      className,
    )}
    {...props}
  />
));
CardTitle.displayName = "CardTitle";

export const CardContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("p-6", className)} {...props} />,
);
CardContent.displayName = "CardContent";

export const CardFooter = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        "flex items-center justify-between gap-3 border-t border-srapi-border px-6 py-4",
        className,
      )}
      {...props}
    />
  ),
);
CardFooter.displayName = "CardFooter";
