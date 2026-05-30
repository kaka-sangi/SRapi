"use client";

import * as React from "react";
import { cn } from "@/lib/cn";

export type SkeletonProps = React.HTMLAttributes<HTMLDivElement>;

/**
 * Loading placeholder using the shared `.shimmer` sweep. Compose several to
 * model a page's real layout while data is in flight.
 */
export function Skeleton({ className, ...props }: SkeletonProps) {
  return (
    <div
      aria-hidden="true"
      className={cn("shimmer h-4 w-full rounded-md", className)}
      {...props}
    />
  );
}
