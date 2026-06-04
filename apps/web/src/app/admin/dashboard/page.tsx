"use client";

import { useMemo, useState } from "react";
import { LineChart } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { StatCard } from "@/components/ui/stat-card";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { BarSeries } from "@/components/charts/bar-series";
import { TrendChart } from "@/components/charts/trend-chart";
import { TokenBreakdown } from "@/components/charts/token-breakdown";
import { ChartEmpty } from "@/components/charts/chart-empty";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { useAdminDashboard } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
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

export default function AdminDashboardPage() {
  return (
    <AppShell allowedRole="admin">
      <DashboardContent />
    </AppShell>
  );
}

function DashboardContent() {
  const { t } = useLanguage();
  const [range, setRange] = useState<RangeKey>("7d");
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
                <Skeleton key={i} className="h-32 w-full rounded-xl" />
              ))}
            </div>
            <Skeleton className="h-56 w-full rounded-xl" />
            <div className="grid gap-4 md:grid-cols-2">
              <Skeleton className="h-48 w-full rounded-xl" />
              <Skeleton className="h-48 w-full rounded-xl" />
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
  const topUsers = [...snapshot.user_usage_trend]
    .sort((a, b) => b.request_count - a.request_count)
    .slice(0, 8)
    .map((u) => ({ label: u.email || u.user_id, value: u.request_count }));

  return (
    <div className="space-y-5">
      {/* KPI grid — surfaces traffic, tokens, cost tiers, throughput, latency, users */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        <StatCard
          label={t("dashboard.traffic")}
          value={formatCompactNumber(traffic.total_requests)}
          unit={t("dashboard.requests")}
          hint={`${t("dashboard.today")} ${formatCompactNumber(traffic.today_requests)} · ${t(
            "dashboard.successRate",
          )} ${successRate != null ? formatPercent(successRate) : "—"}`}
        />
        <StatCard
          label={t("dashboard.totalTokens")}
          value={formatCompactNumber(tokens.total_tokens)}
          hint={`${t("dashboard.today")} ${formatCompactNumber(tokens.today_tokens)} · ${t(
            "dashboard.inputTokens",
          )} ${formatCompactNumber(tokens.input_tokens)} / ${t(
            "dashboard.outputTokens",
          )} ${formatCompactNumber(tokens.output_tokens)}`}
        />
        <StatCard
          label={t("dashboard.cost")}
          value={formatMoney(c.actual_cost, c.currency)}
          hint={`${t("dashboard.standardCost")} ${formatMoney(c.standard_cost, c.currency)} · ${t(
            "dashboard.accountCost",
          )} ${formatMoney(c.account_cost, c.currency)}`}
        />
        <StatCard
          label={t("dashboard.throughput")}
          value={formatInteger(performance.current_rpm)}
          unit="RPM"
          hint={`${formatInteger(performance.current_tpm)} TPM · ${t("dashboard.peak")} ${formatInteger(
            performance.peak_rpm,
          )}/${formatInteger(performance.peak_tpm)}`}
        />
        <StatCard
          label={t("dashboard.latency")}
          value={formatInteger(performance.average_latency_ms)}
          unit="ms"
          hint={`p95 ${formatInteger(performance.p95_latency_ms)} ms`}
        />
        <StatCard
          label={t("dashboard.users")}
          value={formatInteger(users.active_users)}
          unit={t("dashboard.activeUnit")}
          hint={`${t("dashboard.today")} +${formatInteger(users.today_new_users)} · ${t(
            "dashboard.total",
          )} ${formatInteger(users.total_users)}`}
        />
      </div>

      {/* Token composition */}
      <Card>
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
      <Card>
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
      <div className="grid gap-4 md:grid-cols-2">
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
