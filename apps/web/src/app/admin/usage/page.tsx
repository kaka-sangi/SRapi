"use client";

import { useState } from "react";
import { BarChart3, Download, Trash2 } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { Button } from "@/components/ui/button";
import { UsageCleanupDialog } from "@/components/admin/usage-cleanup-dialog";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import {
  useAdminUsageLogs,
  useAdminUsageDaily,
  useAdminUsageAggregates,
  useAdminModels,
  useAdminUsers,
} from "@/hooks/admin-queries";
import {
  useAdminUsageTrends,
  useAdminUsageErrorDistribution,
  useAdminUsageDistribution,
} from "@/hooks/admin-queries/usage-charts";
import { UsageTrendChart, type UsageTrendMetric } from "@/components/admin/usage-trend-chart";
import { UsageErrorDistributionChart } from "@/components/admin/usage-error-distribution-chart";
import {
  UsageDistributionChart,
  type UsageDistributionMetric,
} from "@/components/admin/usage-distribution-chart";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { useAdminUsageExport } from "@/hooks/use-admin-usage-export";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { TrendChart } from "@/components/charts/trend-chart";
import { BarSeries, type BarDatum } from "@/components/charts/bar-series";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { BarChartSkeleton } from "@/components/charts/chart-skeleton";
import { EmptyState } from "@/components/ui/empty-state";
import { formatMoney, formatDateTime, formatInteger } from "@/lib/admin-format";
import { UserBalanceHistoryDialog } from "@/components/admin/user-balance-history-dialog";
import type { UsageLog } from "@/lib/sdk-types";
import {
  LOG_WINDOW_PRESETS,
  LOG_WINDOW_ALL_LABEL_KEY,
  logWindowSince,
} from "@/lib/log-window-filter";

// Structurally matches `UsageAggregateDimension` ('day' | 'model' | 'user' |
// 'account'). The order here drives the segmented-control order.
const AGGREGATE_DIMENSIONS = ["model", "user", "account", "day"] as const;
type AggregateDimension = (typeof AGGREGATE_DIMENSIONS)[number];

const DIMENSION_LABEL_KEY: Record<AggregateDimension, string> = {
  model: "adminUsage.byModel",
  user: "adminUsage.byUser",
  account: "adminUsage.byAccount",
  day: "adminUsage.byDay",
};

const TOP_N = 12;

// Trend-chart toggles. Dimensions match `GetAdminUsageTrendsData["query"].dimension`
// ('model' | 'account' | 'source_endpoint'); buckets match `.bucket`
// ('day' | 'hour'). The order here drives the segmented-control order.
const TREND_DIMENSIONS = ["model", "account", "source_endpoint"] as const;
type TrendDimension = (typeof TREND_DIMENSIONS)[number];

const TREND_DIMENSION_LABEL_KEY: Record<TrendDimension, string> = {
  model: "adminUsage.byModel",
  account: "adminUsage.byAccount",
  source_endpoint: "usage.endpoint",
};

const TREND_BUCKETS = ["day", "hour"] as const;
type TrendBucket = (typeof TREND_BUCKETS)[number];

// One line per series — keep the chart readable; the backend caps to this top-N.
const TREND_SERIES_LIMIT = 6;

// Share-by-dimension distribution. Dimensions match
// `GetAdminUsageDistributionData["query"].dimension`; the order drives the
// dropdown order. Each has an i18n key with a readable English fallback.
const DISTRIBUTION_DIMENSIONS = [
  "model",
  "requested_model",
  "upstream_model",
  "account",
  "provider",
  "api_key",
  "source_endpoint",
  "billing_mode",
  "user",
] as const;
type DistributionDimension = (typeof DISTRIBUTION_DIMENSIONS)[number];

const DISTRIBUTION_DIMENSION_FALLBACK: Record<DistributionDimension, string> = {
  model: "Model",
  requested_model: "Requested model",
  upstream_model: "Upstream model",
  account: "Account",
  provider: "Provider",
  api_key: "API key",
  source_endpoint: "Endpoint",
  billing_mode: "Billing mode",
  user: "User",
};

const DISTRIBUTION_METRICS = ["requests", "tokens", "cost"] as const;

// Top-N buckets to request/render for the distribution chart.
const DISTRIBUTION_LIMIT = 8;

// Verbose / low-frequency UsageLog fields start hidden; the always-useful
// columns (time, user, model, output, cost, status) stay visible by default.
const DEFAULT_HIDDEN_COLUMNS = [
  "input",
  "latency",
  "api_key_id",
  "account_id",
  "provider_id",
  "error_class",
  "cached_tokens",
  "cache_creation_tokens",
  "input_cost",
  "output_cost",
  "cache_read_cost",
  "cache_write_cost",
  "rate_multiplier",
  "protocol",
  "attempt_no",
  "usage_estimated",
];

export default function AdminUsagePage() {
  return (
    <AdminShell>
      <UsageContent />
    </AdminShell>
  );
}

function formatLatency(ms: number): string {
  if (!ms) return "—";
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)}s` : `${Math.round(ms)}ms`;
}

function UsageContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-usage", DEFAULT_HIDDEN_COLUMNS);
  const [cleanupOpen, setCleanupOpen] = useState(false);
  const { toast } = useToast();
  const usageExport = useAdminUsageExport();

  async function runExport() {
    const rows = await usageExport.start();
    if (usageExport.state.phase === "error") {
      toast({ title: t("feedback.failed"), description: usageExport.state.error, tone: "error" });
      return;
    }
    if (rows > 0) {
      toast({ title: t("adminUsageExport.doneTitle", { count: rows }), tone: "success" });
    } else {
      toast({ title: t("adminUsageExport.emptyTitle"), tone: "default" });
    }
  }
  // Clicking a usage row's user opens the shared balance-history dialog.
  const [balanceUser, setBalanceUser] = useState<{ id: string; email: string } | null>(null);
  const modelFilter = list.filters.model || undefined;
  const userFilter = list.filters.user || undefined;
  const windowFilter = list.filters.window;
  // Resolve the preset to an ISO timestamp the backend's start filter
  // honours via the shared filterUsageLogs helper. Null when the preset is
  // unset (default "All time").
  const sinceFilter = logWindowSince(windowFilter)?.toISOString();
  // Server-side: page/filters drive the query (the log can grow unbounded).
  const usage = useAdminUsageLogs({
    page: list.page,
    page_size: list.pageSize,
    model: modelFilter,
    user_id: userFilter,
    start: sinceFilter,
  });
  const daily = useAdminUsageDaily();
  const dailyData = daily.data?.data ?? [];

  // Filter option sources: catalog models + users (by email).
  const models = useAdminModels({ page: 1, page_size: 100 });
  const usersList = useAdminUsers({ page: 1, page_size: 100 });
  const modelOptions = (models.data?.data ?? []).map((m) => ({
    value: m.canonical_name,
    label: m.canonical_name,
  }));
  const userOptions = (usersList.data?.data ?? []).map((u) => ({
    value: String(u.id),
    label: u.email,
  }));
  const userEmailById = new Map(
    (usersList.data?.data ?? []).map((u) => [String(u.id), u.email] as const),
  );
  const userById = new Map(
    (usersList.data?.data ?? []).map((u) => [String(u.id), u] as const),
  );
  const balanceUserRecord = balanceUser ? userById.get(balanceUser.id) : undefined;
  const isFiltered = Boolean(modelFilter || userFilter || windowFilter);
  const total = usage.data?.pagination?.total ?? usage.data?.data.length ?? 0;

  const columns: Column<UsageLog>[] = [
    {
      key: "time",
      header: t("adminUsage.time"),
      pinned: true,
      render: (u) => (
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(u.created_at)}
        </span>
      ),
    },
    {
      key: "user",
      header: t("adminUsage.user"),
      hideOnMobile: true,
      render: (u) => {
        const email = userEmailById.get(String(u.user_id)) || String(u.user_id);
        return (
          <button
            type="button"
            onClick={() => setBalanceUser({ id: String(u.user_id), email })}
            className="truncate text-left text-srapi-text-secondary underline-offset-2 transition-colors hover:text-srapi-text-primary hover:underline"
            title={t("adminUsers.balanceHistory")}
          >
            {email}
          </button>
        );
      },
    },
    {
      key: "model",
      header: t("adminUsage.model"),
      render: (u) => <span className="text-srapi-text-primary">{u.model}</span>,
    },
    {
      key: "input",
      header: t("dashboard.inputTokens"),
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatInteger(u.input_tokens)}
        </span>
      ),
    },
    {
      key: "output",
      header: t("dashboard.outputTokens"),
      align: "right",
      render: (u) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatInteger(u.output_tokens)}
          {u.cached_tokens > 0 ? (
            <span className="ml-1 text-2xs text-srapi-success">+{formatInteger(u.cached_tokens)}</span>
          ) : null}
        </span>
      ),
    },
    {
      key: "input_cost",
      header: `${t("adminUsage.cost")} · ${t("usage.costIn")}`,
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatMoney(u.input_cost ?? "0", u.currency)}
        </span>
      ),
    },
    {
      key: "output_cost",
      header: `${t("adminUsage.cost")} · ${t("usage.costOut")}`,
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatMoney(u.output_cost ?? "0", u.currency)}
        </span>
      ),
    },
    {
      key: "cache_read_cost",
      header: `${t("adminUsage.cost")} · ${t("usage.costCacheRead")}`,
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatMoney(u.cache_read_cost ?? "0", u.currency)}
        </span>
      ),
    },
    {
      key: "cache_write_cost",
      header: `${t("adminUsage.cost")} · ${t("usage.costCacheWrite")}`,
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatMoney(u.cache_write_cost ?? "0", u.currency)}
        </span>
      ),
    },
    {
      key: "cached_tokens",
      header: `${t("dashboard.inputTokens")} · ${t("usage.costCacheRead")}`,
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatInteger(u.cached_tokens)}
        </span>
      ),
    },
    {
      key: "cache_creation_tokens",
      header: `${t("dashboard.inputTokens")} · ${t("usage.costCacheWrite")}`,
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatInteger(u.cache_creation_tokens ?? 0)}
        </span>
      ),
    },
    {
      key: "rate_multiplier",
      header: t("adminUsage.cost"),
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {u.rate_multiplier ? `×${u.rate_multiplier}` : "—"}
        </span>
      ),
    },
    {
      key: "latency",
      header: t("adminUsage.latency"),
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatLatency(u.latency_ms)}
        </span>
      ),
    },
    {
      key: "cost",
      header: t("adminUsage.cost"),
      align: "right",
      render: (u) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(u.cost, u.currency)}
        </span>
      ),
    },
    {
      key: "api_key_id",
      header: t("adminApiKeys.title"),
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{u.api_key_id || "—"}</span>
      ),
    },
    {
      key: "account_id",
      header: t("adminUsage.byAccount"),
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{u.account_id || "—"}</span>
      ),
    },
    {
      key: "provider_id",
      header: t("adminAccounts.title"),
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{u.provider_id || "—"}</span>
      ),
    },
    {
      key: "protocol",
      header: t("usage.endpoint"),
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">
          {u.source_protocol}
          {u.target_protocol ? ` → ${u.target_protocol}` : ""}
        </span>
      ),
    },
    {
      key: "attempt_no",
      header: t("adminUsage.requests"),
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatInteger(u.attempt_no)}
        </span>
      ),
    },
    {
      key: "error_class",
      header: t("usage.status"),
      hideOnMobile: true,
      render: (u) => (
        <span className="text-2xs text-srapi-text-tertiary">{u.error_class || "—"}</span>
      ),
    },
    {
      key: "usage_estimated",
      header: t("usage.tokens"),
      hideOnMobile: true,
      render: (u) => (
        <QuietBadge
          status={u.usage_estimated ? "limited" : "active"}
          label={u.usage_estimated ? "Estimated" : "Exact"}
        />
      ),
    },
    {
      key: "status",
      header: t("common.active"),
      render: (u) => (
        <QuietBadge
          status={u.success ? "active" : "error"}
          label={u.success ? t("usage.successful") : u.error_class || t("usage.failed")}
        />
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminUsage.title")}
        description={t("adminUsage.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {usage.data ? <ListCount total={total} /> : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button
              variant="outline"
              size="sm"
              loading={usageExport.isExporting}
              onClick={() => void runExport()}
            >
              <Download />
              {t("adminUsageExport.action")}
            </Button>
            <Button variant="outline" size="sm" onClick={() => setCleanupOpen(true)}>
              <Trash2 />
              {t("adminUsageCleanup.action")}
            </Button>
            <AutoRefreshControl
              onRefresh={() => void usage.refetch()}
              isRefreshing={usage.isFetching}
              storageKey="srapi.autorefresh.admin-usage"
            />
          </div>
        }
      />
      <UsageCleanupDialog
        open={cleanupOpen}
        onOpenChange={setCleanupOpen}
        modelOptions={modelOptions}
      />
      {balanceUser ? (
        <UserBalanceHistoryDialog
          userId={balanceUser.id}
          email={balanceUser.email}
          open={balanceUser !== null}
          onOpenChange={(v) => {
            if (!v) setBalanceUser(null);
          }}
          balance={balanceUserRecord?.balance}
          currency={balanceUserRecord?.currency}
        />
      ) : null}
      {dailyData.length > 0 ? (
        <Card>
          <CardContent>
            <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
              {t("dashboard.tokenTrend")}
            </span>
            <div className="mt-3">
              <TrendChart
                series={[
                  { key: "input", label: t("dashboard.inputTokens"), values: dailyData.map((d) => d.input_tokens), tone: "secondary" },
                  { key: "output", label: t("dashboard.outputTokens"), values: dailyData.map((d) => d.output_tokens), tone: "primary" },
                ]}
                ariaLabel={t("dashboard.tokenTrend")}
                height={140}
              />
            </div>
          </CardContent>
        </Card>
      ) : null}
      <UsageCharts />
      <UsageBreakdown />
      <AdminListView
        query={usage}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(u) => u.id ?? u.request_id ?? ""}
        emptyIcon={BarChart3}
        emptyTitle={t("adminUsage.emptyTitle")}
        emptyBody={t("adminUsage.emptyBody")}
        minWidth={820}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <FilterSelect
              value={list.filters.model}
              onChange={(v) => list.setFilter("model", v)}
              options={modelOptions}
              allLabel={t("adminUsage.allModels")}
            />
            <FilterSelect
              value={list.filters.user}
              onChange={(v) => list.setFilter("user", v)}
              options={userOptions}
              allLabel={t("adminUsage.allUsers")}
            />
            <FilterSelect
              value={list.filters.window}
              onChange={(v) => list.setFilter("window", v)}
              options={LOG_WINDOW_PRESETS.map((p) => ({ value: p.value, label: t(p.labelKey) }))}
              allLabel={t(LOG_WINDOW_ALL_LABEL_KEY)}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
      />
    </>
  );
}

function UsageCharts() {
  const { t } = useLanguage();
  // The shared message catalog has no keys for the new usage charts yet; fall
  // back to a readable English string so they never render as a raw dotted key.
  const tWithFallback = (key: string, fallback: string) => {
    const value = t(key);
    return value === key ? fallback : value;
  };

  const [dimension, setDimension] = useState<TrendDimension>("model");
  const [bucket, setBucket] = useState<TrendBucket>("day");
  const [metric, setMetric] = useState<UsageTrendMetric>("tokens");

  const [distDimension, setDistDimension] = useState<DistributionDimension>("model");
  const [distMetric, setDistMetric] = useState<UsageDistributionMetric>("requests");

  const trends = useAdminUsageTrends({ dimension, bucket, limit: TREND_SERIES_LIMIT });
  const errorDistribution = useAdminUsageErrorDistribution();
  const distribution = useAdminUsageDistribution({
    dimension: distDimension,
    metric: distMetric,
    limit: DISTRIBUTION_LIMIT,
  });

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <UsageTrendChart
        series={trends.data?.series ?? []}
        loading={trends.isLoading}
        metric={metric}
        onMetricChange={setMetric}
        title={tWithFallback("adminUsage.trendTitle", "Usage trend")}
        metricTokensLabel={t("usage.tokens")}
        metricCostLabel={t("adminUsage.cost")}
        emptyLabel={tWithFallback("adminUsage.trendEmpty", "No usage in window")}
        controls={
          <>
            <Tabs value={dimension} onValueChange={(v) => setDimension(v as TrendDimension)}>
              <TabsList>
                {TREND_DIMENSIONS.map((dim) => (
                  <TabsTrigger key={dim} value={dim} className="text-xs">
                    {t(TREND_DIMENSION_LABEL_KEY[dim])}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
            <Tabs value={bucket} onValueChange={(v) => setBucket(v as TrendBucket)}>
              <TabsList>
                {TREND_BUCKETS.map((b) => (
                  <TabsTrigger key={b} value={b} className="text-xs">
                    {tWithFallback(`adminUsage.bucket.${b}`, b === "day" ? "Day" : "Hour")}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </>
        }
      />
      <UsageErrorDistributionChart
        items={errorDistribution.data ?? []}
        loading={errorDistribution.isLoading}
        title={tWithFallback("adminUsage.errorDistributionTitle", "Error distribution")}
        emptyLabel={tWithFallback("adminUsage.errorDistributionEmpty", "No errors in window")}
        totalLabel={tWithFallback("adminUsage.errorsTotal", "errors")}
        otherLabel={tWithFallback("adminUsage.errorsOther", "Other")}
      />
      <UsageDistributionChart
        buckets={distribution.data?.buckets ?? []}
        metric={distMetric}
        loading={distribution.isLoading}
        title={tWithFallback("adminUsage.distributionTitle", "Usage distribution")}
        emptyLabel={tWithFallback("adminUsage.distributionEmpty", "No usage in window")}
        controls={
          <>
            <Select
              value={distDimension}
              onValueChange={(v) => setDistDimension(v as DistributionDimension)}
            >
              <SelectTrigger className="h-8 w-auto min-w-[8.5rem] gap-2 rounded-lg text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DISTRIBUTION_DIMENSIONS.map((dim) => (
                  <SelectItem key={dim} value={dim} className="text-xs">
                    {tWithFallback(`adminUsage.dimension.${dim}`, DISTRIBUTION_DIMENSION_FALLBACK[dim])}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Tabs value={distMetric} onValueChange={(v) => setDistMetric(v as UsageDistributionMetric)}>
              <TabsList>
                {DISTRIBUTION_METRICS.map((m) => (
                  <TabsTrigger key={m} value={m} className="text-xs">
                    {m === "requests"
                      ? tWithFallback("adminUsage.metric.requests", "Requests")
                      : m === "tokens"
                        ? t("usage.tokens")
                        : t("adminUsage.cost")}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </>
        }
      />
    </div>
  );
}

function UsageBreakdown() {
  const { t } = useLanguage();
  const [dimension, setDimension] = useState<AggregateDimension>("model");
  const aggregates = useAdminUsageAggregates(dimension);

  return (
    <Card>
      <CardHeader className="flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between">
        <CardTitle className="not-italic font-sans text-base text-srapi-text-primary">
          {t("adminUsage.aggregatesTitle")}
        </CardTitle>
        <Tabs value={dimension} onValueChange={(v) => setDimension(v as AggregateDimension)}>
          <TabsList>
            {AGGREGATE_DIMENSIONS.map((dim) => (
              <TabsTrigger key={dim} value={dim} className="text-xs">
                {t(DIMENSION_LABEL_KEY[dim])}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </CardHeader>
      <CardContent>
        <PageQueryState
          query={aggregates}
          skeleton={
            <BarChartSkeleton rows={6} />
          }
        >
          {(result) => {
            const top = [...result.data]
              .sort((a, b) => b.request_count - a.request_count)
              .slice(0, TOP_N);
            if (top.length === 0) {
              return (
                <EmptyState
                  icon={BarChart3}
                  title={t("adminUsage.emptyTitle")}
                  description={t("adminUsage.emptyBody")}
                />
              );
            }
            const series: BarDatum[] = top.map((row) => ({
              label: row.aggregate_id,
              value: row.request_count,
            }));
            return (
              <div className="space-y-4">
                <BarSeries
                  data={series}
                  ariaLabel={t("adminUsage.requests")}
                  formatValue={(v) => formatInteger(v)}
                />
                <div className="grid gap-2 md:grid-cols-2">
                  {top.slice(0, 4).map((row) => (
                    <div key={row.aggregate_id} className="rounded-md border border-srapi-border/70 p-3 text-2xs">
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate font-mono text-srapi-text-primary">{row.aggregate_id}</span>
                        <span className="font-mono text-srapi-text-secondary">{formatMoney(row.total_cost, row.currency)}</span>
                      </div>
                      <div className="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-srapi-text-tertiary">
                        <span>{t("usage.costIn")} {formatMoney(row.input_cost ?? "0", row.currency)}</span>
                        <span>{t("usage.costOut")} {formatMoney(row.output_cost ?? "0", row.currency)}</span>
                        <span>{t("usage.costCacheRead")} {formatMoney(row.cache_read_cost ?? "0", row.currency)}</span>
                        <span>{t("usage.costCacheWrite")} {formatMoney(row.cache_write_cost ?? "0", row.currency)}</span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            );
          }}
        </PageQueryState>
      </CardContent>
    </Card>
  );
}
