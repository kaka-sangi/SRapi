"use client";

import { useState } from "react";
import { BarChart3, Download, Trash2, LineChart, Layers } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { Button } from "@/components/ui/button";
import { UsageCleanupDialog } from "@/components/admin/usage-cleanup-dialog";
import { SectionHero } from "@/components/visual/section-hero";
import { PageQueryState } from "@/components/layout/page-query-state";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import {
  useAdminUsageLogs,
  useAdminUsageDaily,
  useAdminUsageAggregates,
  useAdminModels,
} from "@/hooks/admin-queries";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
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
import { useAccountNameLookup } from "@/hooks/use-account-name-lookup";
import { useApiKeyNameLookup } from "@/hooks/use-api-key-name-lookup";
import { useProviderNameLookup } from "@/hooks/use-provider-name-lookup";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { TrendChart } from "@/components/charts/trend-chart";
import { BarSeries, type BarDatum } from "@/components/charts/bar-series";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { SectionTitle } from "@/components/ui/section-title";
import { DataPill } from "@/components/ui/data-pill";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { InlineDetailGrid, type InlineDetailSection } from "@/components/ui/inline-detail-grid";
import { BarChartSkeleton } from "@/components/charts/chart-skeleton";
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
  const accountLookup = useAccountNameLookup();
  const apiKeyLookup = useApiKeyNameLookup();
  const providerLookup = useProviderNameLookup();

  async function runExport() {
    // Honour the window preset the operator currently has on the table —
    // exporting a 7-day filtered view and getting an all-time CSV would be
    // a silent surprise.
    const rows = await usageExport.start(undefined, {
      start: sinceFilter,
    });
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
  const accountFilter = list.filters.account || undefined;
  // "ok" → success=true, "error" → success=false, "" (All) → undefined.
  const statusFilter = list.filters.status || undefined;
  const successFilter = statusFilter === "ok" ? true : statusFilter === "error" ? false : undefined;
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
    account_id: accountFilter,
    success: successFilter,
    start: sinceFilter,
  });
  const daily = useAdminUsageDaily();
  const dailyData = daily.data?.data ?? [];

  // 顶部 hero KPI — 本月请求 sums daily request counts whose aggregate_id (date
  // string, YYYY-MM-DD) falls in the current month. dailyData arrives keyed by
  // day so the filter is essentially free.
  const monthKey = new Date().toISOString().slice(0, 7);
  const monthRequests = dailyData
    .filter((d) => typeof d.aggregate_id === "string" && d.aggregate_id.startsWith(monthKey))
    .reduce((acc, d) => acc + (d.request_count ?? 0), 0);

  // Filter option sources: catalog models + users (by email).
  const models = useAdminModels({ page: 1, page_size: 100 });
  const userLookup = useUserEmailLookup();
  const modelOptions = (models.data?.data ?? []).map((m) => ({
    value: m.canonical_name,
    label: m.canonical_name,
  }));
  const userOptions = (userLookup.query.data?.data ?? []).map((u) => ({
    value: String(u.id),
    label: u.email,
  }));
  // userById carries the full User object (for the balance-history dialog's
  // initial data); the shared hook only exposes the email map, so we still
  // build this from the underlying query.data.
  const userById = new Map(
    (userLookup.query.data?.data ?? []).map((u) => [String(u.id), u] as const),
  );
  const balanceUserRecord = balanceUser ? userById.get(balanceUser.id) : undefined;
  const isFiltered = Boolean(modelFilter || userFilter || accountFilter || statusFilter || windowFilter);
  // The shared lookup hook already fetches /admin/accounts page 1 of 200,
  // so we get the dropdown options "for free" from its query.data without an
  // extra fetch.
  const accountOptions = (accountLookup.query.data?.data ?? []).map((a) => ({
    value: String(a.id),
    label: a.name,
  }));
  const total = usage.data?.pagination?.total ?? usage.data?.data.length ?? 0;

  const columns: Column<UsageLog>[] = [
    {
      key: "time",
      header: t("adminUsage.time"),
      pinned: true,
      render: (u) => (
        <span className="whitespace-nowrap text-[12px] text-srapi-text-tertiary tabular">
          {formatDateTime(u.created_at)}
        </span>
      ),
    },
    {
      key: "user",
      header: t("adminUsage.user"),
      hideOnMobile: true,
      render: (u) => {
        const email = userLookup.get(u.user_id);
        return (
          <button
            type="button"
            onClick={() => setBalanceUser({ id: String(u.user_id), email })}
            className="truncate text-left text-sm text-srapi-text-secondary underline-offset-2 transition-colors hover:text-srapi-text-primary hover:underline"
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
      render: (u) => <span className="text-sm font-medium text-srapi-text-primary">{u.model}</span>,
    },
    {
      key: "input",
      header: t("dashboard.inputTokens"),
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <DataTooltip
          title={t("dashboard.inputTokens")}
          primary={formatInteger(u.input_tokens)}
          rows={[
            { label: t("usage.tokens"), value: formatInteger(u.input_tokens) },
            { label: t("usage.costCacheRead"), value: formatInteger(u.cached_tokens), tone: "muted" },
            ...(u.cache_creation_tokens
              ? [{
                  label: t("usage.costCacheWrite"),
                  value: formatInteger(u.cache_creation_tokens),
                  tone: "muted" as const,
                }]
              : []),
          ]}
        >
          <span className="metric-tertiary tabular">
            {formatInteger(u.input_tokens)}
          </span>
        </DataTooltip>
      ),
    },
    {
      key: "output",
      header: t("dashboard.outputTokens"),
      align: "right",
      render: (u) => (
        <DataTooltip
          title={t("dashboard.outputTokens")}
          primary={formatInteger(u.output_tokens)}
          rows={[
            { label: t("dashboard.outputTokens"), value: formatInteger(u.output_tokens) },
            { label: t("dashboard.inputTokens"), value: formatInteger(u.input_tokens), tone: "muted" },
            ...(u.cached_tokens > 0
              ? [{
                  label: t("usage.costCacheRead"),
                  value: `+${formatInteger(u.cached_tokens)}`,
                  tone: "success" as const,
                }]
              : []),
            { label: t("usage.tokens"), value: formatInteger(u.total_tokens), tone: "muted" },
          ]}
        >
          <span className="metric-secondary tabular">
            {formatInteger(u.output_tokens)}
            {u.cached_tokens > 0 ? (
              <span className="ml-1 text-[11px] font-medium text-srapi-success">+{formatInteger(u.cached_tokens)}</span>
            ) : null}
          </span>
        </DataTooltip>
      ),
    },
    {
      key: "input_cost",
      header: `${t("adminUsage.cost")} · ${t("usage.costIn")}`,
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <span className="text-[12px] text-srapi-text-tertiary tabular">
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
        <span className="text-[12px] text-srapi-text-tertiary tabular">
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
        <span className="text-[12px] text-srapi-text-tertiary tabular">
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
        <span className="text-[12px] text-srapi-text-tertiary tabular">
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
        <span className="text-[12px] text-srapi-text-tertiary tabular">
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
        <span className="text-[12px] text-srapi-text-tertiary tabular">
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
        <span className="text-[12px] text-srapi-text-tertiary tabular">
          {u.rate_multiplier ? `×${u.rate_multiplier}` : "—"}
        </span>
      ),
    },
    {
      key: "latency",
      header: t("adminUsage.latency"),
      align: "right",
      hideOnMobile: true,
      render: (u) => {
        const tone: "success" | "warning" | "error" =
          u.latency_ms <= 0
            ? "warning"
            : u.latency_ms < 2000
              ? "success"
              : u.latency_ms < 8000
                ? "warning"
                : "error";
        return (
          <DataTooltip
            title={t("adminUsage.latency")}
            primary={formatLatency(u.latency_ms)}
            rows={[
              { label: t("adminUsage.latency"), value: `${formatInteger(u.latency_ms)} ms`, tone },
              { label: t("usage.tokens"), value: formatInteger(u.total_tokens), tone: "muted" },
              ...(u.attempt_no > 1
                ? [{ label: t("adminUsage.requests"), value: `#${u.attempt_no}`, tone: "warning" as const }]
                : []),
            ]}
          >
            <span className="metric-tertiary tabular">
              {formatLatency(u.latency_ms)}
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "cost",
      header: t("adminUsage.cost"),
      align: "right",
      render: (u) => (
        <DataTooltip
          title={t("adminUsage.cost")}
          primary={formatMoney(u.cost, u.currency)}
          rows={[
            { label: t("usage.costIn"), value: formatMoney(u.input_cost ?? "0", u.currency) },
            { label: t("usage.costOut"), value: formatMoney(u.output_cost ?? "0", u.currency) },
            { label: t("usage.costCacheRead"), value: formatMoney(u.cache_read_cost ?? "0", u.currency), tone: "muted" },
            { label: t("usage.costCacheWrite"), value: formatMoney(u.cache_write_cost ?? "0", u.currency), tone: "muted" },
            ...(u.rate_multiplier && u.rate_multiplier !== "1"
              ? [{ label: "×", value: u.rate_multiplier, tone: "warning" as const }]
              : []),
          ]}
        >
          <span className="metric-secondary tabular">
            {formatMoney(u.cost, u.currency)}
          </span>
        </DataTooltip>
      ),
    },
    {
      key: "api_key_id",
      header: t("adminApiKeys.title"),
      hideOnMobile: true,
      render: (u) => (
        <span className="text-sm text-srapi-text-secondary">{apiKeyLookup.get(u.api_key_id)}</span>
      ),
    },
    {
      key: "account_id",
      header: t("adminUsage.byAccount"),
      hideOnMobile: true,
      render: (u) => (
        <span className="text-sm text-srapi-text-secondary">{accountLookup.get(u.account_id)}</span>
      ),
    },
    {
      key: "provider_id",
      header: t("adminAccounts.title"),
      hideOnMobile: true,
      render: (u) => (
        <span className="text-sm text-srapi-text-secondary">{providerLookup.get(u.provider_id)}</span>
      ),
    },
    {
      key: "protocol",
      header: t("usage.endpoint"),
      hideOnMobile: true,
      render: (u) => (
        <DataPill size="sm">
          {u.source_protocol}
          {u.target_protocol ? ` → ${u.target_protocol}` : ""}
        </DataPill>
      ),
    },
    {
      key: "attempt_no",
      header: t("adminUsage.requests"),
      align: "right",
      hideOnMobile: true,
      render: (u) => (
        <DataTooltip
          title={t("adminUsage.requests")}
          primary={`#${u.attempt_no}`}
          rows={[
            { label: "request_id", value: u.request_id, tone: "muted" },
            ...(u.attempt_no > 1
              ? [{ label: t("usage.failed"), value: `${u.attempt_no - 1}`, tone: "warning" as const }]
              : []),
          ]}
        >
          <span className={u.attempt_no > 1 ? "metric-strong-warn" : "metric-tertiary tabular"}>
            {formatInteger(u.attempt_no)}
          </span>
        </DataTooltip>
      ),
    },
    {
      key: "error_class",
      header: t("usage.status"),
      hideOnMobile: true,
      render: (u) => (
        <span className="text-[12px] text-srapi-text-tertiary">{u.error_class || "—"}</span>
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
      <SectionHero
        eyebrow={`Ops · ${t("nav.sectionAdmin")}`}
        title={t("adminUsage.title")}
        description={t("adminUsage.subtitle")}
        metrics={[
          { label: "本月请求", value: formatInteger(monthRequests) },
        ]}
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
        <Card className="anim-rise-sm">
          <CardContent>
            <SectionTitle
              icon={<LineChart />}
              label={t("dashboard.tokenTrend")}
            />
            <div className="mt-4">
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
        emptyContent={
          <IllustratedEmptyState
            illust="chart"
            title={t("adminUsage.emptyTitle")}
            description={t("adminUsage.emptyBody")}
          />
        }
        minWidth={820}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        rowSeverity={(u) =>
          !u.success
            ? "error"
            : u.usage_estimated
              ? "warning"
              : undefined
        }
        expandRow={(u) => {
          const identity: InlineDetailSection = {
            title: t("adminUsage.time"),
            rows: [
              { label: t("adminUsage.time"), value: formatDateTime(u.created_at) },
              { label: "request_id", value: u.request_id, mono: true },
              { label: t("adminUsage.model"), value: u.model },
              ...(u.requested_model && u.requested_model !== u.model
                ? [{ label: "requested", value: u.requested_model, mono: true as const, tone: "muted" as const }]
                : []),
              ...(u.upstream_model && u.upstream_model !== u.model
                ? [{ label: "upstream", value: u.upstream_model, mono: true as const, tone: "muted" as const }]
                : []),
              {
                label: t("usage.endpoint"),
                value: `${u.source_protocol}${u.target_protocol ? ` → ${u.target_protocol}` : ""}`,
              },
              {
                label: t("usage.status"),
                value: u.success ? t("usage.successful") : u.error_class || t("usage.failed"),
                tone: u.success ? "success" : "error",
              },
            ],
          };
          const tokens: InlineDetailSection = {
            title: t("usage.tokens"),
            rows: [
              { label: t("dashboard.inputTokens"), value: formatInteger(u.input_tokens) },
              { label: t("dashboard.outputTokens"), value: formatInteger(u.output_tokens) },
              ...(u.cached_tokens > 0
                ? [{
                    label: t("usage.costCacheRead"),
                    value: formatInteger(u.cached_tokens),
                    tone: "success" as const,
                  }]
                : []),
              ...(u.cache_creation_tokens
                ? [{
                    label: t("usage.costCacheWrite"),
                    value: formatInteger(u.cache_creation_tokens),
                    tone: "muted" as const,
                  }]
                : []),
              { label: t("usage.tokens"), value: formatInteger(u.total_tokens) },
              {
                label: t("adminUsage.latency"),
                value: formatLatency(u.latency_ms),
                tone:
                  u.latency_ms <= 0
                    ? "muted"
                    : u.latency_ms < 2000
                      ? "success"
                      : u.latency_ms < 8000
                        ? "warning"
                        : "error",
              },
            ],
          };
          const costs: InlineDetailSection = {
            title: t("adminUsage.cost"),
            rows: [
              { label: t("usage.costIn"), value: formatMoney(u.input_cost ?? "0", u.currency) },
              { label: t("usage.costOut"), value: formatMoney(u.output_cost ?? "0", u.currency) },
              { label: t("usage.costCacheRead"), value: formatMoney(u.cache_read_cost ?? "0", u.currency), tone: "muted" },
              { label: t("usage.costCacheWrite"), value: formatMoney(u.cache_write_cost ?? "0", u.currency), tone: "muted" },
              { label: t("adminUsage.cost"), value: formatMoney(u.cost, u.currency) },
              ...(u.rate_multiplier && u.rate_multiplier !== "1"
                ? [{ label: "×", value: u.rate_multiplier, tone: "warning" as const }]
                : []),
              ...(u.billing_mode
                ? [{ label: "mode", value: u.billing_mode, tone: "muted" as const }]
                : []),
            ],
          };
          return <InlineDetailGrid sections={[identity, tokens, costs]} />;
        }}
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
              value={list.filters.account}
              onChange={(v) => list.setFilter("account", v)}
              options={accountOptions}
              allLabel={t("adminAccounts.allAccounts")}
            />
            <FilterSelect
              value={list.filters.status}
              onChange={(v) => list.setFilter("status", v)}
              options={[
                { value: "ok", label: t("adminUsage.statusOk") },
                { value: "error", label: t("adminUsage.statusError") },
              ]}
              allLabel={t("adminCommon.allStatuses")}
            />
            <SegmentedControl
              size="sm"
              ariaLabel={t(LOG_WINDOW_ALL_LABEL_KEY)}
              value={list.filters.window ?? "__all__"}
              onChange={(v) => list.setFilter("window", v === "__all__" ? undefined : v)}
              options={[
                { value: "__all__", label: t(LOG_WINDOW_ALL_LABEL_KEY) },
                ...LOG_WINDOW_PRESETS.map((p) => ({ value: p.value, label: t(p.labelKey) })),
              ]}
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
            <SegmentedControl
              size="sm"
              value={dimension}
              onChange={(v) => setDimension(v as TrendDimension)}
              options={TREND_DIMENSIONS.map((dim) => ({
                value: dim,
                label: t(TREND_DIMENSION_LABEL_KEY[dim]),
              }))}
            />
            <SegmentedControl
              size="sm"
              value={bucket}
              onChange={(v) => setBucket(v as TrendBucket)}
              options={TREND_BUCKETS.map((b) => ({
                value: b,
                label: tWithFallback(`adminUsage.bucket.${b}`, b === "day" ? "Day" : "Hour"),
              }))}
            />
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
            <SegmentedControl
              size="sm"
              value={distMetric}
              onChange={(v) => setDistMetric(v as UsageDistributionMetric)}
              options={DISTRIBUTION_METRICS.map((m) => ({
                value: m,
                label:
                  m === "requests"
                    ? tWithFallback("adminUsage.metric.requests", "Requests")
                    : m === "tokens"
                      ? t("usage.tokens")
                      : t("adminUsage.cost"),
              }))}
            />
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
    <Card className="anim-rise-sm">
      <CardHeader className="flex-col items-start gap-3 sm:flex-row sm:items-center sm:justify-between">
        <SectionTitle
          icon={<Layers />}
          label={t("adminUsage.aggregatesTitle")}
        />
        <SegmentedControl
          size="sm"
          value={dimension}
          onChange={(v) => setDimension(v as AggregateDimension)}
          options={AGGREGATE_DIMENSIONS.map((dim) => ({
            value: dim,
            label: t(DIMENSION_LABEL_KEY[dim]),
          }))}
        />
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
                <IllustratedEmptyState
                  illust="chart"
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
                <div className="grid gap-3 md:grid-cols-2">
                  {top.slice(0, 4).map((row) => (
                    <div
                      key={row.aggregate_id}
                      className="rounded-2xl border border-srapi-border/70 bg-srapi-card-muted/40 p-4"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate text-sm font-medium text-srapi-text-primary">{row.aggregate_id}</span>
                        <span className="text-sm font-medium text-srapi-text-secondary tabular">{formatMoney(row.total_cost, row.currency)}</span>
                      </div>
                      <div className="mt-3 grid grid-cols-2 gap-x-3 gap-y-1 text-[12px] text-srapi-text-tertiary">
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
