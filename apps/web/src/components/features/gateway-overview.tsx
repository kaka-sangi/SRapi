"use client";

import type { CSSProperties } from "react";
import Link from "next/link";
import { KeyRound, Activity, ArrowUpRight } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { useUsageLogs } from "@/hooks/queries";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { StatCard, StatCardSkeleton } from "@/components/ui/stat-card";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { EmptyState } from "@/components/ui/empty-state";
import type { UsageLogSummary } from "@/lib/srapi-types";

const rise = (i: number) => ({ "--stagger-index": i }) as CSSProperties;

function compact(n: number): string {
  return new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(n);
}

function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return new Intl.DateTimeFormat(undefined, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(d);
}

function fmtCost(n: number, currency?: string): string {
  const cur = (currency || "USD").toUpperCase();
  const sym = cur === "USD" ? "$" : cur === "CNY" ? "¥" : "";
  const v = n < 1 ? n.toFixed(4) : n.toFixed(2);
  return sym ? `${sym}${v}` : `${v} ${cur}`;
}

/** Time-bucket request counts for a sparkline. Empty (no shape) when too sparse to be honest. */
function bucketRequests(logs: UsageLogSummary[], buckets = 14): number[] {
  const times = logs.map((l) => new Date(l.created_at).getTime()).filter((t) => !Number.isNaN(t));
  if (times.length < 2) return [];
  const min = Math.min(...times);
  const max = Math.max(...times);
  if (max === min) return [];
  const span = max - min;
  const out = new Array(buckets).fill(0);
  for (const t of times) {
    let idx = Math.floor(((t - min) / span) * buckets);
    if (idx >= buckets) idx = buckets - 1;
    out[idx] += 1;
  }
  return out;
}

/**
 * User workspace dashboard. Shows ONLY the signed-in user's own data, derived
 * entirely from `/me/usage`. Gateway internals — upstream account health,
 * scheduler decisions, fleet-wide overview — are admin-only capabilities (no
 * `/me` endpoint exists for them) and live on the admin dashboard, so they are
 * deliberately absent here rather than 403-ing against admin endpoints.
 */
export function GatewayOverview() {
  const { t } = useLanguage();
  const usage = useUsageLogs();

  // Derived, honest metrics from the user's own usage logs.
  const logs = usage.data ?? [];
  const requests = logs.length;
  const successRate = logs.length
    ? Math.round((logs.filter((l) => l.success).length / logs.length) * 100)
    : null;
  const totalTokens = logs.reduce((s, l) => s + l.total_tokens, 0);
  const totalCost = logs.reduce((s, l) => s + l.cost, 0);
  const currency = logs.find((l) => l.currency)?.currency;
  const reqSpark = bucketRequests(logs);

  return (
    <>
      <div className="anim-rise-sm" style={rise(0)}>
        <PageHeader
          eyebrow={t("dashboard.eyebrow")}
          title={t("dashboard.title")}
          actions={
            <Button asChild variant="primary">
              <Link href="/api-keys">＋ {t("apiKeys.create")}</Link>
            </Button>
          }
        />
      </div>

      {/* KPI row — the user's own request/token/cost footprint. */}
      {usage.isLoading ? (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <StatCardSkeleton key={i} />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <div className="anim-rise-sm" style={rise(1)}>
            <StatCard
              className="card-interactive h-full"
              label={t("dashboard.requests")}
              value={requests}
              format={compact}
              spark={reqSpark}
            />
          </div>
          <div className="anim-rise-sm" style={rise(2)}>
            <StatCard
              className="card-interactive h-full"
              label={t("dashboard.successRate")}
              value={successRate != null ? successRate : "—"}
              format={(n) => `${Math.round(n)}%`}
            />
          </div>
          <div className="anim-rise-sm" style={rise(3)}>
            <StatCard
              className="card-interactive h-full"
              label={t("dashboard.totalTokens")}
              value={totalTokens}
              format={compact}
            />
          </div>
          <div className="anim-rise-sm" style={rise(4)}>
            <StatCard
              className="card-interactive h-full"
              label={t("dashboard.cost")}
              value={totalCost}
              format={(n) => fmtCost(n, currency)}
            />
          </div>
        </div>
      )}

      {/* Recent request activity — full width; onboarding CTA when empty */}
      <div className="anim-rise-sm" style={rise(5)}>
      <Card>
        <CardHeader>
          <CardTitle>{t("dashboard.recentActivity")}</CardTitle>
          <Button asChild variant="ghost" size="sm">
            <Link href="/usage">
              {t("dashboard.viewAll")}
              <ArrowUpRight className="size-3.5" />
            </Link>
          </Button>
        </CardHeader>
        <PageQueryState
          query={usage}
          skeleton={
            <DialogListSkeleton rows={3} className="p-5" />
          }
        >
          {(rows) =>
            rows.length === 0 ? (
              <EmptyState
                icon={Activity}
                title={t("dashboard.noActivityTitle")}
                description={t("dashboard.noActivityBody")}
                action={
                  <Button asChild variant="outline" size="sm">
                    <Link href="/api-keys">
                      <KeyRound className="size-4" /> {t("apiKeys.create")}
                    </Link>
                  </Button>
                }
              />
            ) : (
              <div className="divide-y divide-srapi-border">
                {rows.slice(0, 8).map((log) => (
                  <div
                    key={log.request_id}
                    className="flex items-center gap-3 px-5 py-3 transition-colors hover:bg-srapi-card-muted/40"
                  >
                    <div className="w-24 shrink-0 font-mono text-2xs tabular text-srapi-text-tertiary">
                      {fmtTime(log.created_at)}
                    </div>
                    <div className="min-w-0 flex-1 truncate text-sm text-srapi-text-primary">
                      {log.model}
                    </div>
                    <QuietBadge
                      status={log.success ? "active" : "error"}
                      label={t(log.success ? "common.success" : "common.failed")}
                    />
                    <div className="hidden w-20 text-right font-mono text-2xs tabular text-srapi-text-secondary sm:block">
                      {compact(log.total_tokens)}
                    </div>
                    <div className="w-20 text-right font-mono text-2xs tabular text-srapi-text-secondary">
                      {fmtCost(log.cost, log.currency)}
                    </div>
                  </div>
                ))}
              </div>
            )
          }
        </PageQueryState>
      </Card>
      </div>

      <div className="lg:hidden">
        <Button asChild variant="outline" className="w-full">
          <Link href="/api-keys">
            <KeyRound className="size-4" /> {t("apiKeys.title")}
          </Link>
        </Button>
      </div>
    </>
  );
}
