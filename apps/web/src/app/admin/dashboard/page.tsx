"use client";

import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { StatCard } from "@/components/ui/stat-card";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { BarSeries } from "@/components/charts/bar-series";
import { Sparkline } from "@/components/charts/sparkline";
import { useAdminDashboard } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import {
  formatInteger,
  formatCompactNumber,
  formatMoney,
  formatPercent,
} from "@/lib/admin-format";
import type { AdminDashboardSnapshot } from "../../../../../../packages/sdk/typescript/src/types.gen";

export default function AdminDashboardPage() {
  return (
    <AppShell allowedRole="admin">
      <DashboardContent />
    </AppShell>
  );
}

function DashboardContent() {
  const { t } = useLanguage();
  const dashboard = useAdminDashboard();

  return (
    <>
      <PageHeader eyebrow={t("nav.sectionAdmin")} title={t("dashboard.title")} />

      <PageQueryState
        query={dashboard}
        skeleton={
          <div className="space-y-5">
            <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
              {Array.from({ length: 4 }).map((_, i) => (
                <Skeleton key={i} className="h-32 w-full rounded-xl" />
              ))}
            </div>
            <Skeleton className="h-56 w-full rounded-xl" />
            <div className="grid gap-4 md:grid-cols-2">
              <Skeleton className="h-40 w-full rounded-xl" />
              <Skeleton className="h-40 w-full rounded-xl" />
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
  const { inventory, traffic, tokens, performance } = snapshot;

  // Success rate over the window; null when there's no traffic to be honest about.
  const successRate =
    traffic.total_requests > 0 ? traffic.success_requests / traffic.total_requests : null;

  const currency = tokens.costs.currency;

  const modelData = snapshot.model_distribution.map((m) => ({
    label: m.model,
    value: m.request_count,
  }));

  const tokenTrend = snapshot.token_trend.map((p) => p.token_count);
  const userTrend = snapshot.user_usage_trend.map((u) => u.request_count);

  return (
    <div className="space-y-5">
      {/* KPI row */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label={t("dashboard.inventory")}
          value={formatInteger(inventory.total_accounts)}
          unit={t("dashboard.accounts")}
          hint={`${formatInteger(inventory.healthy_accounts)} ${t("common.active")} · ${formatInteger(
            inventory.active_api_keys,
          )} ${t("dashboard.usageLogs")}`}
        />
        <StatCard
          label={t("dashboard.traffic")}
          value={formatCompactNumber(traffic.total_requests)}
          unit={t("dashboard.requests")}
          hint={`${t("dashboard.successRate")} ${
            successRate != null ? formatPercent(successRate) : "—"
          }`}
        />
        <StatCard
          label={t("dashboard.totalTokens")}
          value={formatCompactNumber(tokens.total_tokens)}
          hint={`${t("dashboard.cost")} ${formatMoney(tokens.costs.actual_cost, currency)}`}
        />
        <StatCard
          label={t("dashboard.performance")}
          value={formatInteger(performance.average_latency_ms)}
          unit="ms"
          hint={`${t("dashboard.avgLatency")} · p95 ${formatInteger(performance.p95_latency_ms)}ms`}
        />
      </div>

      {/* Model distribution */}
      <Card>
        <CardHeader>
          <CardTitle className="not-italic font-sans text-base text-srapi-text-primary">
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
            <p className="py-6 text-center font-mono text-2xs text-srapi-text-tertiary">
              {t("dashboard.noActivityTitle")}
            </p>
          )}
        </CardContent>
      </Card>

      {/* Trend sparklines */}
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardContent>
            <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
              {t("dashboard.tokenTrend")}
            </span>
            <div className="mt-3">
              {tokenTrend.length > 0 ? (
                <Sparkline values={tokenTrend} ariaLabel={t("dashboard.tokenTrend")} />
              ) : (
                <p className="flex h-14 items-center justify-center font-mono text-2xs text-srapi-text-tertiary">
                  {t("dashboard.noData")}
                </p>
              )}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent>
            <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
              {t("dashboard.userTrend")}
            </span>
            <div className="mt-3">
              {userTrend.length > 0 ? (
                <Sparkline values={userTrend} ariaLabel={t("dashboard.userTrend")} />
              ) : (
                <p className="flex h-14 items-center justify-center font-mono text-2xs text-srapi-text-tertiary">
                  {t("dashboard.noData")}
                </p>
              )}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
