"use client";

import { useMemo } from "react";
import type { CSSProperties } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { LineChart, Server } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { StatCard, StatCardSkeleton } from "@/components/ui/stat-card";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { BarSeries } from "@/components/charts/bar-series";
import { TrendChart } from "@/components/charts/trend-chart";
import { TokenBreakdown } from "@/components/charts/token-breakdown";
import { ChartEmpty } from "@/components/charts/chart-empty";
import { TrendChartSkeleton, BarChartSkeleton } from "@/components/charts/chart-skeleton";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { useAdminDashboard, useAccountsHealthSummary } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { ADMIN_ROUTES } from "@/lib/routes";
import { cn } from "@/lib/cn";
import {
  formatInteger,
  formatCompactNumber,
  formatMoney,
  formatPercent,
} from "@/lib/admin-format";
import type { AdminDashboardSnapshot } from "../../../../../../packages/sdk/typescript/src/types.gen";

const RANGE_PRESETS = [
  { key: "1d", days: 1 },
  { key: "7d", days: 7 },
  { key: "30d", days: 30 },
] as const;
type RangeKey = (typeof RANGE_PRESETS)[number]["key"];

// rangeWindow derives an RFC3339 [start, end] for the snapshot endpoint from a
// preset. Runs in the browser (not a workflow), so new Date() is fine.
function rangeWindow(key: RangeKey): { start: string; end: string } {
  const days = RANGE_PRESETS.find((p) => p.key === key)?.days ?? 7;
  const end = new Date();
  const start = new Date(end.getTime() - (days - 1) * 86_400_000);
  start.setHours(0, 0, 0, 0);
  return { start: start.toISOString(), end: end.toISOString() };
}

const rise = (i: number) => ({ "--stagger-index": i }) as CSSProperties;

export default function AdminDashboardPage() {
  return (
    <AppShell allowedRole="admin">
      <DashboardContent />
    </AppShell>
  );
}

function DashboardContent() {
  const { t } = useLanguage();
  const searchParams = useSearchParams();
  const router = useRouter();
  const rangeParam = searchParams.get("range");
  const range: RangeKey = RANGE_PRESETS.some((p) => p.key === rangeParam)
    ? (rangeParam as RangeKey)
    : "7d";
  function setRange(key: RangeKey) {
    const params = new URLSearchParams(searchParams.toString());
    if (key === "7d") params.delete("range");
    else params.set("range", key);
    const qs = params.toString();
    router.replace(`/admin/dashboard${qs ? `?${qs}` : ""}`, { scroll: false });
  }
  const { start, end } = useMemo(() => rangeWindow(range), [range]);
  const dashboard = useAdminDashboard({ start, end });

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("dashboard.title")}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex items-center gap-1 rounded-lg border border-srapi-border p-0.5">
              {RANGE_PRESETS.map((p) => (
                <button
                  key={p.key}
                  type="button"
                  onClick={() => setRange(p.key)}
                  className={cn(
                    "rounded-md px-2.5 py-1 font-mono text-2xs transition-colors",
                    range === p.key
                      ? "bg-srapi-primary/10 text-srapi-primary"
                      : "text-srapi-text-tertiary hover:text-srapi-text-secondary",
                  )}
                >
                  {t(`dashboard.range_${p.key}`)}
                </button>
              ))}
            </div>
            <AutoRefreshControl
              onRefresh={() => void dashboard.refetch()}
              isRefreshing={dashboard.isFetching}
              storageKey="srapi.autorefresh.admin-dashboard"
              defaultSec={30}
            />
          </div>
        }
      />

      <PageQueryState
        query={dashboard}
        skeleton={
          <div className="space-y-5">
            <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
              {Array.from({ length: 6 }).map((_, i) => (
                <StatCardSkeleton key={i} />
              ))}
            </div>
            <Card className="overflow-hidden p-5">
              <Skeleton className="h-3 w-28" />
              <div className="mt-3">
                <TrendChartSkeleton height={132} />
              </div>
            </Card>
            <div className="grid gap-4 md:grid-cols-2">
              <Card className="p-5">
                <Skeleton className="h-4 w-32" />
                <div className="mt-4">
                  <BarChartSkeleton rows={5} />
                </div>
              </Card>
              <Card className="p-5">
                <Skeleton className="h-4 w-28" />
                <div className="mt-4">
                  <BarChartSkeleton rows={5} />
                </div>
              </Card>
            </div>
          </div>
        }
      >
        {(snapshot) => <DashboardBody snapshot={snapshot} />}
      </PageQueryState>
    </>
  );
}

function DashboardBody({ snapshot }: { snapshot: AdminDashboardSnapshot }) {
  const { t } = useLanguage();
  const { traffic, tokens, performance, users } = snapshot;
  const c = tokens.costs;
  const successRate =
    traffic.total_requests > 0 ? traffic.success_requests / traffic.total_requests : null;

  const modelData = snapshot.model_distribution.map((m) => ({
    label: m.model,
    value: m.request_count,
  }));
  const tokenTrend = snapshot.token_trend.map((p) => p.token_count);
  const requestSpark = snapshot.token_trend.map((p) => p.request_count);
  const tokenSpark = snapshot.token_trend.map((p) => p.token_count);
  const costSpark = snapshot.token_trend.map((p) => Number(p.cost));
  const topUsers = [...snapshot.user_usage_trend]
    .sort((a, b) => b.request_count - a.request_count)
    .slice(0, 8)
    .map((u) => ({ label: u.email || u.user_id, value: u.request_count }));

  return (
    <div className="space-y-5">
      {/* KPI grid — surfaces traffic, tokens, cost tiers, throughput, latency, users */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        <div className="anim-rise-sm" style={rise(0)}>
          <StatCard
            className="card-interactive h-full"
            label={t("dashboard.traffic")}
            value={traffic.total_requests}
            format={formatCompactNumber}
            unit={t("dashboard.requests")}
            spark={requestSpark}
            hint={`${t("dashboard.today")} ${formatCompactNumber(traffic.today_requests)} · ${t(
              "dashboard.successRate",
            )} ${successRate != null ? formatPercent(successRate) : "—"}`}
          />
        </div>
        <div className="anim-rise-sm" style={rise(1)}>
          <StatCard
            className="card-interactive h-full"
            label={t("dashboard.totalTokens")}
            value={tokens.total_tokens}
            format={formatCompactNumber}
            spark={tokenSpark}
            hint={`${t("dashboard.today")} ${formatCompactNumber(tokens.today_tokens)} · ${t(
              "dashboard.inputTokens",
            )} ${formatCompactNumber(tokens.input_tokens)} / ${t(
              "dashboard.outputTokens",
            )} ${formatCompactNumber(tokens.output_tokens)}`}
          />
        </div>
        <div className="anim-rise-sm" style={rise(2)}>
          <StatCard
            className="card-interactive h-full"
            label={t("dashboard.cost")}
            value={Number(c.actual_cost)}
            format={(n) => formatMoney(n, c.currency)}
            spark={costSpark}
            hint={`${t("dashboard.standardCost")} ${formatMoney(c.standard_cost, c.currency)} · ${t(
              "dashboard.accountCost",
            )} ${formatMoney(c.account_cost, c.currency)}`}
          />
        </div>
        <div className="anim-rise-sm" style={rise(3)}>
          <StatCard
            className="card-interactive h-full"
            label={t("dashboard.throughput")}
            value={performance.current_rpm}
            format={formatInteger}
            unit="RPM"
            hint={`${formatInteger(performance.current_tpm)} TPM · ${t("dashboard.peak")} ${formatInteger(
              performance.peak_rpm,
            )}/${formatInteger(performance.peak_tpm)}`}
          />
        </div>
        <div className="anim-rise-sm" style={rise(4)}>
          <StatCard
            className="card-interactive h-full"
            label={t("dashboard.latency")}
            value={performance.average_latency_ms}
            format={formatInteger}
            unit="ms"
            hint={`p95 ${formatInteger(performance.p95_latency_ms)} ms`}
          />
        </div>
        <div className="anim-rise-sm" style={rise(5)}>
          <StatCard
            className="card-interactive h-full"
            label={t("dashboard.users")}
            value={users.active_users}
            format={formatInteger}
            unit={t("dashboard.activeUnit")}
            hint={`${t("dashboard.today")} +${formatInteger(users.today_new_users)} · ${t(
              "dashboard.total",
            )} ${formatInteger(users.total_users)}`}
          />
        </div>
      </div>

      {/* Account health overview */}
      <AccountHealthOverview staggerIndex={6} />

      {/* Token composition */}
      <Card className="anim-rise-sm" style={rise(7)}>
        <CardHeader>
          <CardTitle>
            {t("dashboard.tokenBreakdown")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <TokenBreakdown
            input={tokens.input_tokens}
            output={tokens.output_tokens}
            cached={tokens.cached_tokens}
            labels={{
              input: t("dashboard.inputTokens"),
              output: t("dashboard.outputTokens"),
              cached: t("dashboard.cachedTokens"),
            }}
          />
        </CardContent>
      </Card>

      {/* Token trend over the window */}
      <Card className="anim-rise-sm" style={rise(8)}>
        <CardContent>
          <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("dashboard.tokenTrend")}
          </span>
          <div className="mt-3">
            {tokenTrend.length > 0 ? (
              <TrendChart
                series={[
                  { key: "tokens", label: t("dashboard.tokenTrend"), values: tokenTrend, tone: "primary" },
                ]}
                ariaLabel={t("dashboard.tokenTrend")}
                showLegend={false}
                height={132}
              />
            ) : (
              <ChartEmpty icon={LineChart} label={t("dashboard.noData")} />
            )}
          </div>
        </CardContent>
      </Card>

      {/* Distributions: by model + by user */}
      <div className="anim-rise-sm grid gap-4 md:grid-cols-2" style={rise(9)}>
        <Card>
          <CardHeader>
            <CardTitle>
              {t("dashboard.modelDistribution")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {modelData.length > 0 ? (
              <BarSeries
                data={modelData}
                ariaLabel={t("dashboard.modelDistribution")}
                formatValue={(v) => formatCompactNumber(v)}
              />
            ) : (
              <ChartEmpty label={t("dashboard.noActivityTitle")} />
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>
              {t("dashboard.topUsers")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            {topUsers.length > 0 ? (
              <BarSeries
                data={topUsers}
                ariaLabel={t("dashboard.topUsers")}
                formatValue={(v) => formatCompactNumber(v)}
              />
            ) : (
              <ChartEmpty label={t("dashboard.noActivityTitle")} />
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function AccountHealthOverview({ staggerIndex }: { staggerIndex: number }) {
  const { t } = useLanguage();
  const healthSummary = useAccountsHealthSummary();
  const data = healthSummary.data ?? [];
  if (data.length === 0 && !healthSummary.isLoading) return null;

  const healthy = data.filter((h) => h.circuit_state === "closed" && h.success_rate >= 0.9).length;
  const degraded = data.filter((h) => h.circuit_state === "closed" && h.success_rate < 0.9).length;
  const tripped = data.filter((h) => h.circuit_state !== "closed").length;
  const quotaLow = data.filter((h) => h.quota_remaining_ratio < 0.2 && h.quota_remaining_ratio >= 0).length;

  return (
    <Card className="anim-rise-sm" style={rise(staggerIndex)}>
      <CardContent>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Server className="size-4 text-srapi-text-tertiary" />
            <span className="font-mono text-2xs uppercase tracking-wide text-srapi-text-tertiary">
              {t("dashboard.accountHealth")}
            </span>
          </div>
          <a
            href={ADMIN_ROUTES.accounts + "?view=health"}
            className="text-2xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
          >
            {t("dashboard.viewAll")} &rarr;
          </a>
        </div>
        {healthSummary.isLoading ? (
          <div className="mt-3 flex gap-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-10 flex-1" />
            ))}
          </div>
        ) : (
          <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <HealthMiniStat label={t("dashboard.healthyAccounts")} count={healthy} tone="success" />
            <HealthMiniStat label={t("dashboard.degradedAccounts")} count={degraded} tone="warning" />
            <HealthMiniStat label={t("dashboard.trippedAccounts")} count={tripped} tone="error" />
            <HealthMiniStat label={t("dashboard.quotaLow")} count={quotaLow} tone="warning" />
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function HealthMiniStat({
  label,
  count,
  tone,
}: {
  label: string;
  count: number;
  tone: "success" | "warning" | "error";
}) {
  const dotColor =
    tone === "success"
      ? "bg-srapi-success"
      : tone === "warning"
        ? "bg-srapi-warning"
        : "bg-srapi-error";
  return (
    <div className="flex items-center gap-2 rounded-lg border border-srapi-border px-3 py-2">
      <span className={cn("size-2 rounded-full", dotColor, count === 0 && "opacity-30")} />
      <div>
        <div className={cn("font-mono text-sm font-semibold tabular", count === 0 && "text-srapi-text-tertiary")}>
          {count}
        </div>
        <div className="text-[10px] leading-tight text-srapi-text-tertiary">{label}</div>
      </div>
    </div>
  );
}
