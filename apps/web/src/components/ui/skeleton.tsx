import { cn } from "@/lib/cn";

export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      className={cn("skeleton-pulse rounded-md bg-srapi-card-muted", className)}
      aria-hidden
    />
  );
}
