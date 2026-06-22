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
        "relative flex h-32 flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-srapi-border bg-srapi-card-muted/30 text-srapi-text-tertiary",
        className,
      )}
    >
      <div className="grid size-9 place-items-center rounded-lg border border-srapi-border bg-srapi-card/80 text-srapi-text-tertiary">
        <Icon className="size-4" strokeWidth={1.5} aria-hidden />
      </div>
      <span className="font-mono text-2xs uppercase tracking-[0.18em]">{label}</span>
    </div>
  );
}
