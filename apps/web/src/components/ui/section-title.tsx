import * as React from "react";
import { cn } from "@/lib/cn";

export interface SectionTitleProps {
  icon?: React.ReactNode;
  label: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}

export const SectionTitle = React.forwardRef<HTMLDivElement, SectionTitleProps>(
  ({ icon, label, action, className }, ref) => (
    <div
      ref={ref}
      className={cn("flex items-center justify-between gap-3", className)}
    >
      <div className="flex items-center gap-2.5">
        {icon ? (
          <span className="grid size-8 place-items-center rounded-xl bg-srapi-card-muted text-srapi-text-secondary [&>svg]:size-4">
            {icon}
          </span>
        ) : null}
        <span className="text-sm font-semibold tracking-tight text-srapi-text-primary">
          {label}
        </span>
      </div>
      {action ? <div className="flex items-center gap-2">{action}</div> : null}
    </div>
  ),
);
SectionTitle.displayName = "SectionTitle";
