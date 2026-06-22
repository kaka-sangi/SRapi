import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/cn";
import { Card, CardContent } from "@/components/ui/card";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";

/**
 * Compact, intentional placeholder for a chart/section that has no data yet.
 * Upgraded to wrap an `IllustratedEmptyState` (illust="chart") inside a Card so
 * sparse instances feel designed instead of broken. The legacy `{ label, icon }`
 * surface is preserved so existing consumers (admin dashboard, gateway
 * overview, usage analytics) compile unchanged.
 *
 * The `icon` prop is retained for API compatibility but the new chart
 * illustration carries the visual; we no longer render the lucide icon.
 */
export function ChartEmpty({
  label,
  description,
  className,
}: {
  label: string;
  /** Optional supporting copy under the title. */
  description?: string;
  /** Retained for API compatibility — the chart illustration is used instead. */
  icon?: LucideIcon;
  className?: string;
}) {
  return (
    <Card className={cn("border-dashed bg-srapi-card-muted/30", className)}>
      <CardContent className="py-6">
        <IllustratedEmptyState
          illust="chart"
          title={label}
          description={description}
          className="border-none bg-transparent p-0"
        />
      </CardContent>
    </Card>
  );
}
