"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

/**
 * Asymmetric bento grid. A 6-column responsive grid; children pick their own
 * column/row spans via `BentoItem` so layouts read as deliberate and editorial
 * rather than a uniform card wall.
 */
export function BentoGrid({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "grid auto-rows-[minmax(0,1fr)] grid-cols-2 gap-4 md:grid-cols-6 md:gap-5",
        className,
      )}
      {...props}
    />
  );
}

const colSpan = {
  2: "md:col-span-2",
  3: "md:col-span-3",
  4: "md:col-span-4",
  6: "md:col-span-6",
} as const;

const rowSpan = {
  1: "md:row-span-1",
  2: "md:row-span-2",
} as const;

export interface BentoItemProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Columns to span on md+ (out of 6). Mobile is always full-width pairs. */
  span?: keyof typeof colSpan;
  /** Rows to span on md+. */
  rows?: keyof typeof rowSpan;
}

export function BentoItem({ span = 3, rows = 1, className, ...props }: BentoItemProps) {
  return (
    <div
      className={cn("col-span-2", colSpan[span], rowSpan[rows], className)}
      {...props}
    />
  );
}
