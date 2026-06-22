"use client";

import type { CSSProperties } from "react";
import Link from "next/link";
import {
  KeyRound,
  Activity,
  ArrowUpRight,
  Gauge,
  Wallet,
  LineChart,
  BarChart3,
  Zap,
  Hash,
  Database,
  CheckCircle2,
  Coins,
} from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { useBalance, usePlatformQuotas, useUsageLogs } from "@/hooks/queries";
import {
  useUserUsageThroughput,
  useUserUsageTrend,
  useUserUsageModels,
  useUserUsageCacheMetrics,
} from "@/hooks/use-user-dashboard";
import { useUsageTotals } from "@/hooks/use-usage-totals";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { StatCard, StatCardSkeleton } from "@/components/ui/stat-card";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { TrendChart } from "@/components/charts/trend-chart";
import { ChartEmpty } from "@/components/charts/chart-empty";
import { DialogListSkeleton, TrendChartSkeleton, BarChartSkeleton } from "@/components/charts/chart-skeleton";
import { EmptyState } from "@/components/ui/empty-state";
import { formatMoney, formatCompactNumber } from "@/lib/admin-format";
import type { UserPlatformQuota, UsageModelShare, UsageTrendPoint } from "@/lib/sdk-types";
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

function quotaLimit(quota: UserPlatformQuota): string | null {
  return quota.daily_limit ?? quota.weekly_limit ?? quota.monthly_limit ?? null;
}

/** "daily" / "weekly" / "monthly" — derived from whichever cap is set. */
function quotaPeriod(quota: UserPlatformQuota): "daily" | "weekly" | "monthly" | null {
  if (quota.daily_limit) return "daily";
  if (quota.weekly_limit) return "weekly";
  if (quota.monthly_limit) return "monthly";
  return null;
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

/** Window (days) for the trend chart and model-distribution rollups. */
const TREND_DAYS = 7;

/**
 * User workspace dashboard. Shows ONLY the signed-in user's own data: the live
 * usage-dashboard aggregates (`/user/usage/dashboard/*` — throughput, trend,
 * model share, cache metrics) plus the user's own `/me/usage` request log.
 * Gateway internals — upstream account health, scheduler decisions, fleet-wide
 * overview — are admin-only capabilities (no `/me` endpoint exists for them) and
 * live on the admin dashboard, so they are deliberately absent here rather than
 * 403-ing against admin endpoints.
 */
export function GatewayOverview() {
  const { t } = useLanguage();
  const balance = useBalance();
  const platformQuotas = usePlatformQuotas();
  const usage = useUsageLogs();

  // Live usage-dashboard aggregates (server-side rollups, not derived client-side).
  const throughput = useUserUsageThroughput();
  const cacheMetrics = useUserUsageCacheMetrics();
  const trend = useUserUsageTrend(TREND_DAYS, "day");
  const models = useUserUsageModels(TREND_DAYS);

  // Derived, honest metrics from the user's own usage logs.
  const logs = usage.data ?? [];
  const totals = useUsageTotals(logs);
  const reqSpark = bucketRequests(logs);
  const quotaRows = platformQuotas.data?.data ?? [];
  const enabledQuotas = quotaRows.filter((q) => q.enabled);

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

      <div className="grid gap-4 lg:grid-cols-2 lg:gap-5">
        {/* Balance — entire card is the billing link (tactile hover via card-interactive). */}
        <Link
          href="/billing"
          className="anim-rise-sm group block focus-visible:outline-none"
          style={rise(1)}
        >
          <Card className="card-interactive h-full">
            <CardContent className="flex h-full items-center justify-between gap-4 p-6">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.14em] text-srapi-text-tertiary">
                  <span className="grid size-8 place-items-center rounded-xl bg-srapi-accent-soft text-srapi-primary">
                    <Wallet className="size-4" />
                  </span>
                  {t("dashboard.balance")}
                </div>
                <PageQueryState
                  query={balance}
                  skeleton={<Skeleton className="mt-3 h-9 w-36" />}
                >
                  {(data) => (
                    <div className="mt-3 truncate text-3xl font-semibold tracking-tight text-srapi-text-primary tabular sm:text-[2rem]">
                      {formatMoney(data.balance, data.currency)}
                    </div>
                  )}
                </PageQueryState>
              </div>
              <div className="flex shrink-0 items-center gap-1.5 rounded-full bg-srapi-card-muted px-3 py-1 text-[11px] font-medium text-srapi-text-secondary transition-colors group-hover:bg-srapi-accent-soft group-hover:text-srapi-primary">
                {t("nav.billing")}
                <ArrowUpRight className="size-3.5 transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
              </div>
            </CardContent>
          </Card>
        </Link>

        <Card className="anim-rise-sm h-full" style={rise(2)}>
          <CardContent className="flex h-full min-w-0 flex-col gap-3.5 p-6">
            <div className="flex items-start justify-between gap-3">
              <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.14em] text-srapi-text-tertiary">
                <span className="grid size-8 place-items-center rounded-xl bg-srapi-accent-soft text-srapi-primary">
                  <Gauge className="size-4" />
                </span>
                {t("dashboard.platformQuotas")}
              </div>
              <PageQueryState
                query={platformQuotas}
                skeleton={<Skeleton className="h-6 w-16" />}
              >
                {() => (
                  <div className="text-2xl font-semibold leading-none text-srapi-text-primary tabular sm:text-[1.75rem]">
                    {enabledQuotas.length}
                    <span className="ml-1.5 align-baseline text-sm font-medium text-srapi-text-tertiary">
                      {t("dashboard.quotaPlatforms")}
                    </span>
                  </div>
                )}
              </PageQueryState>
            </div>
            <PageQueryState
              query={platformQuotas}
              skeleton={
                <div className="space-y-2 pt-1">
                  <Skeleton className="h-4 w-full" />
                  <Skeleton className="h-4 w-2/3" />
                </div>
              }
            >
              {() =>
                enabledQuotas.length === 0 ? (
                  <p className="text-sm text-srapi-text-tertiary">
                    {t("dashboard.noPlatformQuotas")}
                  </p>
                ) : (
                  <ul className="min-w-0 divide-y divide-srapi-border/70 border-t border-srapi-border/70">
                    {enabledQuotas.slice(0, 3).map((quota) => {
                      const limit = quotaLimit(quota);
                      const period = quotaPeriod(quota);
                      return (
                        <li
                          key={quota.platform}
                          className="flex min-w-0 items-baseline justify-between gap-3 py-2.5"
                        >
                          <span className="min-w-0 truncate text-sm font-medium text-srapi-text-primary">
                            {quota.platform}
                          </span>
                          <span className="shrink-0 text-xs text-srapi-text-secondary tabular">
                            {period ? (
                              <span className="mr-1.5 rounded-full bg-srapi-card-muted px-1.5 py-0.5 text-[10px] uppercase text-srapi-text-tertiary">
                                {t(`dashboard.quotaPeriod.${period}`)}
                              </span>
                            ) : null}
                            {limit ? formatMoney(limit, quota.currency) : "—"}
                          </span>
                        </li>
                      );
                    })}
                    {enabledQuotas.length > 3 ? (
                      <li className="pt-2.5 text-xs font-medium text-srapi-text-tertiary">
                        + {enabledQuotas.length - 3} {t("dashboard.quotaMore")}
                      </li>
                    ) : null}
                  </ul>
                )
              }
            </PageQueryState>
          </CardContent>
        </Card>
      </div>

      {/* Header KPIs — live throughput (RPM/TPM) + prompt-cache hit rate. */}
      <ThroughputKpis throughput={throughput} cacheMetrics={cacheMetrics} />

      {/* Usage trend over the last window + model distribution by tokens. */}
      <div className="grid gap-4 lg:grid-cols-5 lg:gap-5">
        <div className="anim-rise-sm lg:col-span-3" style={rise(1)}>
          <UsageTrendCard query={trend} />
        </div>
        <div className="anim-rise-sm lg:col-span-2" style={rise(2)}>
          <ModelDistributionCard query={models} />
        </div>
      </div>

      {/* KPI row — the user's own request/token/cost footprint. */}
      <PageQueryState
        query={usage}
        skeleton={
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4 lg:gap-5">
            {Array.from({ length: 4 }).map((_, i) => (
              <StatCardSkeleton key={i} />
            ))}
          </div>
        }
      >
        {() => (
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4 lg:gap-5">
            <div className="anim-rise-sm" style={rise(1)}>
              <StatCard
                className="h-full"
                label={t("dashboard.requests")}
                value={totals.requests}
                format={compact}
                spark={reqSpark}
                icon={<Hash />}
              />
            </div>
            <div className="anim-rise-sm" style={rise(2)}>
              <StatCard
                className="h-full"
                label={t("dashboard.successRate")}
                value={totals.requests > 0 ? totals.successRate : "—"}
                format={(n) => `${Math.round(n)}%`}
                icon={<CheckCircle2 />}
              />
            </div>
            <div className="anim-rise-sm" style={rise(3)}>
              <StatCard
                className="h-full"
                label={t("dashboard.totalTokens")}
                value={totals.totalTokens}
                format={compact}
                icon={<Activity />}
              />
            </div>
            <div className="anim-rise-sm" style={rise(4)}>
              <StatCard
                className="h-full"
                label={t("dashboard.cost")}
                value={totals.totalCost}
                format={(n) => fmtCost(n, totals.currency)}
                icon={<Coins />}
              />
            </div>
          </div>
        )}
      </PageQueryState>

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
              <div className="divide-y divide-srapi-border/70">
                {rows.slice(0, 8).map((log, idx) => (
                  <div
                    key={log.request_id}
                    className="anim-rise-sm flex items-center gap-3 px-6 py-3 transition-colors hover:bg-srapi-card-muted/50"
                    style={{ "--stagger-index": 6 + idx } as CSSProperties}
                  >
                    <div className="w-24 shrink-0 text-[12px] tabular text-srapi-text-tertiary">
                      {fmtTime(log.created_at)}
                    </div>
                    <div className="min-w-0 flex-1 truncate text-sm font-medium text-srapi-text-primary">
                      {log.model}
                    </div>
                    <QuietBadge
                      status={log.success ? "active" : "error"}
                      label={t(log.success ? "common.success" : "common.failed")}
                    />
                    <div className="hidden w-20 text-right text-[12px] tabular text-srapi-text-secondary sm:block">
                      {compact(log.total_tokens)}
                    </div>
                    <div className="w-20 text-right text-[12px] font-medium tabular text-srapi-text-secondary">
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

    </>
  );
}

/**
 * Header KPI strip backed by the live usage-dashboard aggregates: requests- and
 * tokens-per-minute (with their window peaks as the card hint) and the
 * prompt-cache hit rate (with cost saved). Each card loads independently so a
 * slow rollup never blocks the others.
 */
function ThroughputKpis({
  throughput,
  cacheMetrics,
}: {
  throughput: ReturnType<typeof useUserUsageThroughput>;
  cacheMetrics: ReturnType<typeof useUserUsageCacheMetrics>;
}) {
  const { t } = useLanguage();
  const tp = throughput.data;
  const cache = cacheMetrics.data;
  const tpLoading = throughput.isLoading;
  const cacheLoading = cacheMetrics.isLoading;

  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-3 lg:gap-5">
      <div className="anim-rise-sm" style={rise(1)}>
        {tpLoading ? (
          <StatCardSkeleton className="h-full" />
        ) : (
          <StatCard
            className="h-full"
            label={t("dashboard.rpm")}
            value={tp ? tp.rpm : "—"}
            format={formatCompactNumber}
            icon={<Zap />}
            hint={
              tp
                ? t("dashboard.peakRpm", { value: formatCompactNumber(tp.peak_rpm) })
                : undefined
            }
          />
        )}
      </div>
      <div className="anim-rise-sm" style={rise(2)}>
        {tpLoading ? (
          <StatCardSkeleton className="h-full" />
        ) : (
          <StatCard
            className="h-full"
            label={t("dashboard.tpm")}
            value={tp ? tp.tpm : "—"}
            format={formatCompactNumber}
            icon={<Activity />}
            hint={
              tp
                ? t("dashboard.peakTpm", { value: formatCompactNumber(tp.peak_tpm) })
                : undefined
            }
          />
        )}
      </div>
      <div className="anim-rise-sm col-span-2 lg:col-span-1" style={rise(3)}>
        {cacheLoading ? (
          <StatCardSkeleton className="h-full" />
        ) : (
          <StatCard
            className="h-full"
            label={t("dashboard.cacheHitRate")}
            value={cache ? cache.cache_hit_rate * 100 : "—"}
            format={(n) => `${n.toFixed(1)}%`}
            icon={<Database />}
            hint={
              cache && Number(cache.cache_cost_saved) > 0
                ? t("dashboard.cacheSaved", {
                    value: formatMoney(cache.cache_cost_saved, cache.currency),
                  })
                : cache
                  ? t("dashboard.cachedInput", {
                      cached: formatCompactNumber(cache.cache_read_tokens),
                      total: formatCompactNumber(cache.total_input_tokens),
                    })
                  : undefined
            }
          />
        )}
      </div>
    </div>
  );
}

/**
 * Usage trend over the window as a two-series pure-SVG line chart (requests +
 * tokens, each min-max normalised by the shared TrendChart so both shapes read
 * even at different magnitudes). Buckets arrive pre-aggregated from the server.
 */
function UsageTrendCard({ query }: { query: ReturnType<typeof useUserUsageTrend> }) {
  const { t } = useLanguage();
  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>{t("dashboard.usageTrend")}</CardTitle>
      </CardHeader>
      <CardContent>
        <PageQueryState query={query} skeleton={<TrendChartSkeleton height={160} />}>
          {(points) =>
            points.length === 0 ? (
              <ChartEmpty icon={LineChart} label={t("dashboard.noData")} />
            ) : (
              <TrendChart
                ariaLabel={t("dashboard.usageTrend")}
                height={160}
                series={[
                  {
                    key: "requests",
                    label: t("dashboard.trendRequests"),
                    values: points.map((p: UsageTrendPoint) => p.requests),
                    tone: "primary",
                  },
                  {
                    key: "tokens",
                    label: t("dashboard.trendTokens"),
                    values: points.map(
                      (p: UsageTrendPoint) => p.input_tokens + p.output_tokens,
                    ),
                    tone: "success",
                  },
                ]}
              />
            )
          }
        </PageQueryState>
      </CardContent>
    </Card>
  );
}

/**
 * Top models by token volume as a proportional list/donut hybrid: each row is a
 * model with a share bar scaled to the leader, mirroring the usage-breakdown
 * primitive but ranked by tokens (not cost) to match the sub2api distribution
 * view. The server returns these already aggregated per model.
 */
function ModelDistributionCard({ query }: { query: ReturnType<typeof useUserUsageModels> }) {
  const { t } = useLanguage();
  return (
    <Card className="h-full">
      <CardHeader>
        <CardTitle>{t("dashboard.topModels")}</CardTitle>
        <span className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-tertiary">
          {t("dashboard.byTokens")}
        </span>
      </CardHeader>
      <CardContent>
        <PageQueryState query={query} skeleton={<BarChartSkeleton rows={5} />}>
          {(rows) =>
            rows.length === 0 ? (
              <ChartEmpty icon={BarChart3} label={t("dashboard.noModelUsage")} />
            ) : (
              <ModelDistributionList rows={rows} />
            )
          }
        </PageQueryState>
      </CardContent>
    </Card>
  );
}

const MODEL_ROWS = 6;

function ModelDistributionList({ rows }: { rows: UsageModelShare[] }) {
  const { t } = useLanguage();
  const ranked = [...rows].sort((a, b) => b.total_tokens - a.total_tokens).slice(0, MODEL_ROWS);
  const totalTokens = rows.reduce((sum, r) => sum + r.total_tokens, 0);
  const maxTokens = ranked.reduce((max, r) => Math.max(max, r.total_tokens), 0);

  return (
    <div className="space-y-3">
      {ranked.map((row) => {
        const width = maxTokens > 0 ? Math.min(100, (row.total_tokens / maxTokens) * 100) : 0;
        const share = totalTokens > 0 ? Math.round((row.total_tokens / totalTokens) * 100) : 0;
        return (
          <div key={row.model}>
            <div className="flex items-baseline justify-between gap-3">
              <span
                className="min-w-0 truncate text-[13px] font-medium text-srapi-text-primary"
                title={row.model}
              >
                {row.model}
              </span>
              <span className="shrink-0 text-xs font-medium text-srapi-text-secondary tabular">
                {compact(row.total_tokens)}
              </span>
            </div>
            <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-srapi-card-muted">
              <div
                className="h-full rounded-full bg-gradient-to-r from-srapi-primary to-srapi-primary-hover transition-[width] duration-500"
                style={{ width: `${width}%` }}
              />
            </div>
            <div className="mt-1 flex items-center justify-between text-[11px] text-srapi-text-tertiary tabular">
              <span>
                {compact(row.requests)} req · {fmtCost(Number(row.cost), row.currency)}
              </span>
              <span>{t("dashboard.modelShare", { percent: share })}</span>
            </div>
          </div>
        );
      })}
    </div>
  );
}
