import type { LucideIcon } from "lucide-react";
import { BarChart3 } from "lucide-react";
import { cn } from "@/lib/cn";

/**
 * Compact, intentional placeholder for a chart/section that has no data yet.
 * A faint icon + caption reads as "nothing here yet" rather than a cavernous
 * void of empty card. Shared by the admin dashboard and usage analytics so
 * sparse instances feel designed instead of broken.
 */
export function ChartEmpty({
  label,
  icon: Icon = BarChart3,
  className,
}: {
  label: string;
  icon?: LucideIcon;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex h-28 flex-col items-center justify-center gap-2 text-srapi-text-tertiary",
        className,
      )}
    >
      <Icon className="size-5 opacity-50" strokeWidth={1.5} aria-hidden />
      <span className="font-mono text-2xs">{label}</span>
    </div>
  );
}
