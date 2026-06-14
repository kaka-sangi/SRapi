"use client";

import { useMemo } from "react";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { formatMoney } from "@/lib/admin-format";
import { useLanguage } from "@/context/LanguageContext";
import type { UsageLogSummary } from "@/lib/srapi-types";

interface BreakdownRow {
  key: string;
  requests: number;
  tokens: number;
  cost: number;
  currency: string;
}

const MAX_ROWS = 5;

function aggregate(
  logs: UsageLogSummary[],
  pick: (log: UsageLogSummary) => string,
): BreakdownRow[] {
  const map = new Map<string, BreakdownRow>();
  for (const log of logs) {
    const key = pick(log) || "-";
    const existing = map.get(key);
    if (existing) {
      existing.requests += 1;
      existing.tokens += log.total_tokens;
      existing.cost += log.cost;
    } else {
      map.set(key, {
        key,
        requests: 1,
        tokens: log.total_tokens,
        cost: log.cost,
        currency: log.currency || "USD",
      });
    }
  }
  return Array.from(map.values())
    .sort((a, b) => b.cost - a.cost)
    .slice(0, MAX_ROWS);
}

/**
 * Client-side usage breakdown derived from the already-loaded logs: top models
 * and top source endpoints ranked by cost. Each row carries request count,
 * tokens and cost plus a thin proportional bar (relative to the top row's cost)
 * so the spend distribution is visible without a charting library.
 */
export function UsageBreakdown({ logs }: { logs: UsageLogSummary[] }) {
  const { t } = useLanguage();
  const byModel = useMemo(() => aggregate(logs, (l) => l.model), [logs]);
  const byEndpoint = useMemo(() => aggregate(logs, (l) => l.source_endpoint), [logs]);

  if (logs.length === 0) {
    return null;
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <BreakdownCard title={t("usage.topModelsByCost")} rows={byModel} emptyLabel={t("usage.noModelUsage")} />
      <BreakdownCard title={t("usage.topEndpointsByCost")} rows={byEndpoint} emptyLabel={t("usage.noEndpointUsage")} />
    </div>
  );
}

function BreakdownCard({
  title,
  rows,
  emptyLabel,
}: {
  title: string;
  rows: BreakdownRow[];
  emptyLabel: string;
}) {
  const maxCost = rows.reduce((max, r) => Math.max(max, r.cost), 0);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {rows.length === 0 ? (
          <p className="text-2xs text-srapi-text-tertiary">{emptyLabel}</p>
        ) : (
          rows.map((row) => {
            const percent = maxCost > 0 ? Math.min(100, (row.cost / maxCost) * 100) : 0;
            return (
              <div key={row.key}>
                <div className="flex items-baseline justify-between gap-3">
                  <span
                    className="min-w-0 truncate font-mono text-2xs text-srapi-text-primary"
                    title={row.key}
                  >
                    {row.key}
                  </span>
                  <span className="shrink-0 font-mono text-2xs text-srapi-text-secondary tabular">
                    {formatMoney(row.cost, row.currency)}
                  </span>
                </div>
                <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-srapi-card-muted">
                  <div
                    className="h-full rounded-full bg-srapi-primary"
                    style={{ width: `${percent}%` }}
                  />
                </div>
                <div className="mt-1 font-mono text-2xs text-srapi-text-tertiary tabular">
                  {row.requests.toLocaleString()} req / {row.tokens.toLocaleString()} tok
                </div>
              </div>
            );
          })
        )}
      </CardContent>
    </Card>
  );
}
