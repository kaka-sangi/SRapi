"use client";

import { useState } from "react";
import { BarChart3 } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
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
import { useAdminList } from "@/hooks/use-admin-list";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { TrendChart } from "@/components/charts/trend-chart";
import { BarSeries, type BarDatum } from "@/components/charts/bar-series";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "@/components/ui/empty-state";
import { formatMoney, formatDateTime, formatInteger } from "@/lib/admin-format";
import type { UsageLog } from "@/lib/sdk-types";

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
  const modelFilter = list.filters.model || undefined;
  const userFilter = list.filters.user || undefined;
  // Server-side: page/filters drive the query (the log can grow unbounded).
  const usage = useAdminUsageLogs({
    page: list.page,
    page_size: list.pageSize,
    model: modelFilter,
    user_id: userFilter,
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
  const isFiltered = Boolean(modelFilter || userFilter);
  const total = usage.data?.pagination?.total ?? usage.data?.data.length ?? 0;

  const columns: Column<UsageLog>[] = [
    {
      key: "time",
      header: t("adminUsage.time"),
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
      render: (u) => (
        <span className="text-srapi-text-secondary">
          {userEmailById.get(String(u.user_id)) || u.user_id}
        </span>
      ),
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
            <AutoRefreshControl
              onRefresh={() => void usage.refetch()}
              isRefreshing={usage.isFetching}
              storageKey="srapi.autorefresh.admin-usage"
            />
          </div>
        }
      />
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
      <UsageBreakdown />
      <AdminListView
        query={usage}
        columns={columns}
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
            <div className="space-y-2">
              {Array.from({ length: 6 }).map((_, i) => (
                <Skeleton key={i} className="h-2.5 w-full rounded-full" />
              ))}
            </div>
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
              <BarSeries
                data={series}
                ariaLabel={t("adminUsage.requests")}
                formatValue={(v) => formatInteger(v)}
              />
            );
          }}
        </PageQueryState>
      </CardContent>
    </Card>
  );
}
