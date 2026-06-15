"use client";

import { useMemo } from "react";
import { AlertTriangle } from "lucide-react";
import { cn } from "@/lib/cn";
import { formatInteger } from "@/lib/admin-format";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { ChartEmpty } from "@/components/charts/chart-empty";
import type { OpsErrorDistributionItem } from "@/hooks/admin-queries/ops-charts";

/**
 * Error-distribution doughnut grouped by owner (who is accountable for the
 * failure: provider / client / platform). A hand-rolled SVG donut — token
 * colors, no chart library — with the grand total in the hole, an owner legend,
 * and the top error_class rows beneath. Mirrors the sub2api ops error
 * distribution card but keyed on SRapi's owner/error_class read-model.
 */

type Owner = "provider" | "client" | "platform" | "other";

const OWNER_STYLE: Record<Owner, { stroke: string; dot: string }> = {
  // Provider faults dominate the warning palette; client errors are neutral;
  // platform faults are the loud red; anything unattributed falls back to grey.
  provider: { stroke: "stroke-srapi-warning", dot: "bg-srapi-warning" },
  client: { stroke: "stroke-srapi-text-secondary", dot: "bg-srapi-text-secondary" },
  platform: { stroke: "stroke-srapi-error", dot: "bg-srapi-error" },
  other: { stroke: "stroke-srapi-text-tertiary", dot: "bg-srapi-text-tertiary" },
};

function normalizeOwner(owner: string): Owner {
  const o = owner.trim().toLowerCase();
  if (o === "provider" || o === "upstream") return "provider";
  if (o === "client" || o === "user") return "client";
  if (o === "platform" || o === "system" || o === "internal") return "platform";
  return "other";
}

export function OpsErrorDistributionChart({
  items,
  title,
  emptyLabel,
  totalLabel,
  ownerLabels,
  loading,
}: {
  items: OpsErrorDistributionItem[];
  title: string;
  emptyLabel: string;
  totalLabel: string;
  ownerLabels: Record<Owner, string>;
  loading?: boolean;
}) {
  const { total, segments, topClasses } = useMemo(() => {
    const byOwner = new Map<Owner, number>();
    let sum = 0;
    for (const item of items) {
      const owner = normalizeOwner(item.owner);
      const count = Math.max(0, item.count);
      byOwner.set(owner, (byOwner.get(owner) ?? 0) + count);
      sum += count;
    }

    const order: Owner[] = ["provider", "platform", "client", "other"];
    const segs = order
      .filter((owner) => (byOwner.get(owner) ?? 0) > 0)
      .map((owner) => ({ owner, count: byOwner.get(owner) ?? 0 }));

    const top = [...items]
      .filter((item) => item.count > 0)
      .sort((a, b) => b.count - a.count)
      .slice(0, 5);

    return { total: sum, segments: segs, topClasses: top };
  }, [items]);

  // Donut geometry — a stroke-dasharray ring so each owner slice is one arc.
  const R = 52;
  const C = 2 * Math.PI * R;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="not-italic font-sans text-base text-srapi-text-primary">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {loading && items.length === 0 ? (
          <div
            role="img"
            aria-label={title}
            className="h-40 w-full animate-pulse rounded-md bg-srapi-card-muted/40"
          />
        ) : total === 0 ? (
          <ChartEmpty label={emptyLabel} icon={AlertTriangle} />
        ) : (
          <div className="flex flex-col items-center gap-5 sm:flex-row sm:items-start">
            <div className="relative shrink-0">
              <svg viewBox="0 0 140 140" role="img" aria-label={title} className="size-36">
                <circle
                  cx={70}
                  cy={70}
                  r={R}
                  fill="none"
                  className="stroke-srapi-card-muted"
                  strokeWidth={14}
                />
                {segments.map((seg, i) => {
                  const frac = seg.count / total;
                  const len = frac * C;
                  const dash = `${len} ${C - len}`;
                  const offset = segments
                    .slice(0, i)
                    .reduce((acc, prev) => acc + (prev.count / total) * C, 0);
                  const dashOffset = -offset;
                  return (
                    <circle
                      key={seg.owner}
                      cx={70}
                      cy={70}
                      r={R}
                      fill="none"
                      className={cn(OWNER_STYLE[seg.owner].stroke, "transition-[stroke-dashoffset]")}
                      strokeWidth={14}
                      strokeDasharray={dash}
                      strokeDashoffset={dashOffset}
                      strokeLinecap="butt"
                      transform="rotate(-90 70 70)"
                    >
                      <title>{`${ownerLabels[seg.owner]}: ${formatInteger(seg.count)} (${(frac * 100).toFixed(1)}%)`}</title>
                    </circle>
                  );
                })}
              </svg>
              <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                <span className="font-serif text-xl text-srapi-text-primary tabular">
                  {formatInteger(total)}
                </span>
                <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                  {totalLabel}
                </span>
              </div>
            </div>

            <div className="min-w-0 flex-1 space-y-3">
              <div className="flex flex-wrap gap-x-4 gap-y-1.5">
                {segments.map((seg) => (
                  <span
                    key={seg.owner}
                    className="inline-flex items-center gap-1.5 font-mono text-2xs text-srapi-text-tertiary"
                  >
                    <span
                      className={cn("inline-block h-2 w-2 rounded-full", OWNER_STYLE[seg.owner].dot)}
                    />
                    <span className="text-srapi-text-secondary">{ownerLabels[seg.owner]}</span>
                    <span className="tabular">{Math.round((seg.count / total) * 100)}%</span>
                  </span>
                ))}
              </div>

              <div className="space-y-1.5">
                {topClasses.map((item) => {
                  const owner = normalizeOwner(item.owner);
                  return (
                    <div
                      key={`${item.owner}:${item.error_class}`}
                      className="flex items-center gap-2 border-t border-srapi-border pt-1.5 first:border-t-0 first:pt-0"
                    >
                      <span
                        className={cn("inline-block h-2 w-2 shrink-0 rounded-full", OWNER_STYLE[owner].dot)}
                      />
                      <span className="min-w-0 flex-1 truncate font-mono text-2xs text-srapi-text-secondary">
                        {item.error_class}
                      </span>
                      <span className="shrink-0 font-mono text-2xs text-srapi-text-tertiary tabular">
                        {formatInteger(item.count)}
                      </span>
                      <span className="w-10 shrink-0 text-right font-mono text-2xs text-srapi-text-tertiary tabular">
                        {(item.share * 100).toFixed(1)}%
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
