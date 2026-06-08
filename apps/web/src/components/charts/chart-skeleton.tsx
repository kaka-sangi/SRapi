"use client";

import { cn } from "@/lib/cn";
import { Skeleton } from "@/components/ui/skeleton";

export function TrendChartSkeleton({
  height = 132,
  className,
}: {
  height?: number;
  className?: string;
}) {
  return (
    <div className={cn("relative overflow-hidden rounded-lg", className)} style={{ height }}>
      <div className="absolute inset-x-0 inset-y-2 flex flex-col justify-between">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-px bg-srapi-border/30" />
        ))}
      </div>
      <div
        className="absolute inset-0 skeleton-shimmer bg-srapi-card-muted"
        style={{
          clipPath:
            "polygon(0% 65%, 8% 55%, 17% 40%, 25% 50%, 33% 30%, 42% 35%, 50% 55%, 58% 45%, 67% 25%, 75% 30%, 83% 50%, 92% 40%, 100% 45%, 100% 100%, 0% 100%)",
        }}
      />
    </div>
  );
}

export function BarChartSkeleton({
  rows = 5,
  className,
}: {
  rows?: number;
  className?: string;
}) {
  const widths = ["75%", "60%", "45%", "85%", "35%", "55%", "70%", "40%"];
  return (
    <div className={cn("space-y-3", className)}>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="h-3 w-20 shrink-0" />
          <div className="h-4 flex-1 rounded">
            <div
              className="skeleton-shimmer h-full rounded bg-srapi-card-muted"
              style={{ width: widths[i % widths.length] }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

export function FormSkeleton({
  rows = 4,
  className,
}: {
  rows?: number;
  className?: string;
}) {
  return (
    <div className={cn("space-y-5", className)}>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i}>
          <Skeleton className="h-3 w-24" />
          <div className="mt-2 skeleton-shimmer h-9 w-full rounded-md bg-srapi-card-muted" />
        </div>
      ))}
    </div>
  );
}

export function SloCardSkeleton({ className }: { className?: string }) {
  return (
    <div className={cn("rounded-xl border border-srapi-border bg-srapi-card p-5 space-y-4", className)}>
      <div className="flex items-center justify-between">
        <Skeleton className="h-4 w-28" />
        <Skeleton className="h-5 w-14 rounded-full" />
      </div>
      <div className="flex items-baseline justify-between">
        <Skeleton className="h-3 w-16" />
        <Skeleton className="h-7 w-20" />
      </div>
      <div className="skeleton-shimmer h-2 w-full rounded-full bg-srapi-card-muted" />
      <div className="flex items-center justify-between">
        <Skeleton className="h-2.5 w-24" />
        <Skeleton className="h-2.5 w-16" />
      </div>
    </div>
  );
}

export function ChatSkeleton({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "flex h-[70vh] flex-col rounded-2xl border border-srapi-border bg-srapi-card",
        className,
      )}
    >
      <div className="flex-1 space-y-5 p-6">
        <div className="flex justify-end">
          <Skeleton className="h-10 w-48 rounded-2xl rounded-tr-sm" />
        </div>
        <div className="space-y-2">
          <Skeleton className="h-4 w-64" />
          <Skeleton className="h-4 w-56" />
          <Skeleton className="h-4 w-40" />
        </div>
        <div className="flex justify-end">
          <Skeleton className="h-10 w-36 rounded-2xl rounded-tr-sm" />
        </div>
        <div className="space-y-2">
          <Skeleton className="h-4 w-72" />
          <Skeleton className="h-4 w-48" />
        </div>
      </div>
      <div className="border-t border-srapi-border p-4">
        <Skeleton className="h-10 w-full rounded-xl" />
      </div>
    </div>
  );
}

export function DialogListSkeleton({
  rows = 4,
  className,
}: {
  rows?: number;
  className?: string;
}) {
  return (
    <div className={cn("space-y-3", className)}>
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-3 w-20" />
          <Skeleton className="ml-auto h-3 w-12" />
        </div>
      ))}
    </div>
  );
}
