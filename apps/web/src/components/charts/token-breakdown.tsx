"use client";

import * as React from "react";
import { cn } from "@/lib/cn";
import { formatInteger } from "@/lib/admin-format";
import { DataTooltip } from "@/components/ui/data-tooltip";

/**
 * Token composition as a single stacked bar + legend. Splits a token total into
 * input / output / cached (and optional cache-write) segments so the mix — not
 * just the grand total — is visible at a glance. Hand-rolled, token-colored.
 *
 * Polish — segments grow from 0 → their target width on mount (one shared
 * ease-out-quint transition) so the bar feels alive when the dashboard streams
 * in. Each segment is wrapped in a DataTooltip exposing the exact count, share
 * %, and (when provided) cost.
 */
export type TokenBreakdownLabels = {
  input: string;
  output: string;
  cached: string;
  cacheCreation?: string;
};

type Seg = {
  key: keyof TokenBreakdownLabels | "cacheCreation";
  label: string;
  value: number;
  bar: string;
  dot: string;
};

export function TokenBreakdown({
  input,
  output,
  cached,
  cacheCreation = 0,
  labels,
  costs,
  formatCost = (n) => `$${n.toFixed(4)}`,
  className,
}: {
  input: number;
  output: number;
  cached: number;
  cacheCreation?: number;
  labels: TokenBreakdownLabels;
  /** Optional per-segment cost — if omitted, the tooltip just shows count + %. */
  costs?: Partial<Record<"input" | "output" | "cached" | "cacheCreation", number>>;
  formatCost?: (value: number) => string;
  className?: string;
}) {
  const segs: Seg[] = [
    { key: "input", label: labels.input, value: Math.max(0, input), bar: "bg-srapi-text-secondary", dot: "bg-srapi-text-secondary" },
    { key: "output", label: labels.output, value: Math.max(0, output), bar: "bg-srapi-primary", dot: "bg-srapi-primary" },
    { key: "cached", label: labels.cached, value: Math.max(0, cached), bar: "bg-srapi-success", dot: "bg-srapi-success" },
  ];
  if (labels.cacheCreation && cacheCreation > 0) {
    segs.push({
      key: "cacheCreation",
      label: labels.cacheCreation,
      value: cacheCreation,
      bar: "bg-srapi-warning",
      dot: "bg-srapi-warning",
    });
  }
  const total = segs.reduce((sum, s) => sum + s.value, 0);

  // Animate segments in: render at 0% on first paint, then to target width on
  // the next frame. requestAnimationFrame ensures the transition triggers.
  const [mounted, setMounted] = React.useState(false);
  React.useEffect(() => {
    const id = requestAnimationFrame(() => setMounted(true));
    return () => cancelAnimationFrame(id);
  }, []);

  return (
    <div className={className}>
      <div className="relative flex h-3 w-full overflow-hidden rounded-full bg-srapi-card-muted shadow-[inset_0_1px_0_0_rgba(28,26,23,0.05)]">
        {total > 0
          ? segs.map((s, idx) => {
              if (s.value <= 0) return null;
              const pct = (s.value / total) * 100;
              const targetWidth = mounted ? `${pct}%` : "0%";
              const cost = costs?.[s.key as "input" | "output" | "cached" | "cacheCreation"];
              return (
                <DataTooltip
                  key={s.key}
                  title={s.label}
                  primary={formatInteger(s.value)}
                  rows={[
                    { label: "Share", value: `${pct.toFixed(1)}%`, tone: "muted" },
                    ...(typeof cost === "number"
                      ? [{ label: "Cost", value: formatCost(cost), tone: "default" as const }]
                      : []),
                  ]}
                  side="top"
                >
                  <span
                    className={cn(
                      s.bar,
                      "h-3 cursor-pointer transition-[width,filter] duration-500 ease-[var(--ease-out-quint)] hover:brightness-110",
                    )}
                    style={{
                      width: targetWidth,
                      // Subtle inner highlight so the printed-on-paper feel
                      // carries through the bar segments.
                      boxShadow: "inset 0 1px 0 0 rgba(255,255,255,0.18)",
                      // Hairline divider between adjacent segments for a more
                      // "engraved" sense of measurement.
                      borderRight:
                        idx < segs.length - 1 ? "1px solid rgba(28,26,23,0.18)" : undefined,
                    }}
                    aria-label={`${s.label}: ${formatInteger(s.value)} (${pct.toFixed(1)}%)`}
                  />
                </DataTooltip>
              );
            })
          : null}
      </div>
      <div className="mt-4 grid gap-3 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4">
        {segs.map((s) => {
          const pct = total > 0 ? Math.round((s.value / total) * 100) : 0;
          const cost = costs?.[s.key as "input" | "output" | "cached" | "cacheCreation"];
          return (
            <DataTooltip
              key={s.key}
              title={s.label}
              primary={formatInteger(s.value)}
              rows={[
                { label: "Share", value: total > 0 ? `${pct}%` : "—", tone: "muted" },
                ...(typeof cost === "number"
                  ? [{ label: "Cost", value: formatCost(cost), tone: "default" as const }]
                  : []),
              ]}
              side="top"
            >
              <div className="flex w-full items-center gap-2.5 rounded-md border border-srapi-border bg-srapi-card-muted/40 px-2.5 py-1.5">
                <span className={cn("inline-block h-2.5 w-2.5 shrink-0 rounded-full", s.dot)} />
                <div className="min-w-0 flex-1 leading-tight">
                  <div className="truncate text-2xs uppercase tracking-wider text-srapi-text-tertiary">
                    {s.label}
                  </div>
                  <div className="flex items-baseline gap-1.5 font-mono tabular">
                    <span className="text-sm text-srapi-text-primary">{formatInteger(s.value)}</span>
                    {total > 0 ? (
                      <span className="text-2xs text-srapi-text-tertiary">{pct}%</span>
                    ) : null}
                  </div>
                </div>
              </div>
            </DataTooltip>
          );
        })}
      </div>
    </div>
  );
}
