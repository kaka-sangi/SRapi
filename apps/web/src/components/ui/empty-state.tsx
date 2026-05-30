"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export interface EmptyStateProps extends React.HTMLAttributes<HTMLDivElement> {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
}

/**
 * Standard empty state: a calm, centered block with optional icon, supporting
 * copy and a primary action. Replaces the ad-hoc "no rows" markup scattered
 * across pages.
 */
export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
  ...props
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-3 rounded-2xl border border-dashed border-srapi-border",
        "px-6 py-12 text-center",
        className,
      )}
      {...props}
    >
      {icon ? (
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-srapi-card-muted text-srapi-text-secondary">
          {icon}
        </div>
      ) : null}
      <div className="space-y-1">
        <p className="font-serif text-base font-medium text-srapi-text-primary">{title}</p>
        {description ? (
          <p className="mx-auto max-w-sm text-xs leading-relaxed text-srapi-text-secondary">
            {description}
          </p>
        ) : null}
      </div>
      {action ? <div className="pt-1">{action}</div> : null}
    </div>
  );
}
