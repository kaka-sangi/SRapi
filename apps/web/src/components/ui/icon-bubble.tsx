import * as React from "react";
import { cn } from "@/lib/cn";

type IconBubbleSize = "sm" | "md" | "lg";
type IconBubbleTone = "accent" | "neutral" | "success" | "warning" | "error";

export interface IconBubbleProps {
  children: React.ReactNode;
  size?: IconBubbleSize;
  tone?: IconBubbleTone;
  className?: string;
}

const sizeClasses: Record<IconBubbleSize, string> = {
  sm: "size-7 [&>svg]:size-3.5",
  md: "size-9 [&>svg]:size-4",
  lg: "size-11 [&>svg]:size-5",
};

const toneClasses: Record<IconBubbleTone, string> = {
  accent: "bg-srapi-card-muted text-srapi-text-secondary",
  neutral: "bg-srapi-card-muted text-srapi-text-secondary",
  success: "bg-srapi-success/12 text-srapi-success",
  warning: "bg-srapi-warning/12 text-srapi-warning",
  error: "bg-srapi-error/12 text-srapi-error",
};

export const IconBubble = React.forwardRef<HTMLSpanElement, IconBubbleProps>(
  ({ children, size = "md", tone = "accent", className }, ref) => (
    <span
      ref={ref}
      className={cn(
        "grid place-items-center rounded-xl",
        sizeClasses[size],
        toneClasses[tone],
        className,
      )}
    >
      {children}
    </span>
  ),
);
IconBubble.displayName = "IconBubble";
