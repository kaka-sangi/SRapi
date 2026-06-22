import * as React from "react";
import { cn } from "@/lib/cn";

type DataPillTone = "neutral" | "accent" | "success" | "warning" | "error";
type DataPillSize = "sm" | "md";

export interface DataPillProps {
  children: React.ReactNode;
  tone?: DataPillTone;
  size?: DataPillSize;
  className?: string;
}

const toneClasses: Record<DataPillTone, string> = {
  neutral: "bg-srapi-card-muted text-srapi-text-tertiary",
  accent: "bg-srapi-card-muted text-srapi-text-secondary",
  success: "bg-srapi-success/10 text-srapi-success",
  warning: "bg-srapi-warning/12 text-srapi-warning",
  error: "bg-srapi-error/12 text-srapi-error",
};

const sizeClasses: Record<DataPillSize, string> = {
  sm: "text-[10px] px-2 py-0.5",
  md: "text-[11px] px-2.5 py-1",
};

export const DataPill = React.forwardRef<HTMLSpanElement, DataPillProps>(
  ({ children, tone = "neutral", size = "md", className }, ref) => (
    <span
      ref={ref}
      className={cn(
        "inline-flex items-center gap-1 rounded-full font-medium leading-none whitespace-nowrap",
        toneClasses[tone],
        sizeClasses[size],
        className,
      )}
    >
      {children}
    </span>
  ),
);
DataPill.displayName = "DataPill";
