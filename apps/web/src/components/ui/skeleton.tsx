import { cn } from "@/lib/cn";

export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      className={cn("skeleton-shimmer rounded-lg bg-srapi-card-muted", className)}
      aria-hidden
    />
  );
}
