"use client";

import { useMemo } from "react";
import type { CSSProperties } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { LineChart, Server, Radio } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageQueryState } from "@/components/layout/page-query-state";
import { StatCard, StatCardSkeleton } from "@/components/ui/stat-card";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { SectionHero } from "@/components/visual/section-hero";
import { SpotlightCard } from "@/components/visual/spotlight-card";
import { BarSeries } from "@/components/charts/bar-series";
import { TrendChart } from "@/components/charts/trend-chart";
import { TokenBreakdown } from "@/components/charts/token-breakdown";
import { ChartEmpty } from "@/components/charts/chart-empty";
import { TrendChartSkeleton, BarChartSkeleton } from "@/components/charts/chart-skeleton";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { Badge } from "@/components/ui/badge";
import { useAdminDashboard, useAccountsHealthSummary } from "@/hooks/admin-queries";
import {
  useListOpsRealtimeSlots,
  type RealtimeActiveSlotCounters,
} from "@/hooks/admin-queries/realtime-slots";
import { useLanguage } from "@/context/LanguageContext";
import { ADMIN_ROUTES } from "@/lib/routes";
import { AdminTourTrigger } from "@/components/onboarding/admin-tour";
import { cn } from "@/lib/cn";
import {
  formatInteger,
  formatCompactNumber,
  formatMoney,
  formatPercent,
  formatDateTime,
} from "@/lib/admin-format";
import type { AdminDashboardSnapshot } from "@/lib/sdk-types";

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
      <AdminTourTrigger />
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
      <SectionHero
        eyebrow={t("nav.sectionAdmin")}
        title={t("dashboard.title")}
        description="Gateway 全局态势 · 流量、Token、收入、上游健康一屏可览"
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex items-center gap-0.5 rounded-xl border border-srapi-border bg-srapi-card/85 p-1 backdrop-blur-sm">
              {RANGE_PRESETS.map((p) => (
                <button
                  key={p.key}
                  type="button"
                  onClick={() => setRange(p.key)}
                  className={cn(
                    "rounded-lg px-3 py-1.5 text-xs font-medium transition-colors",
                    range === p.key
                      ? "bg-srapi-card-muted text-srapi-text-secondary shadow-[0_1px_2px_rgba(26,24,20,0.04)]"
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
            <div className="grid grid-cols-2 gap-4 lg:grid-cols-4 xl:grid-cols-4 2xl:grid-cols-8">
              {Array.from({ length: 8 }).map((_, i) => (
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
  const errorRate =
    traffic.total_requests > 0 ? traffic.error_requests / traffic.total_requests : null;

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
      <SnapshotSummary snapshot={snapshot} errorRate={errorRate} />

      {/* KPI grid — surfaces traffic, tokens, cost tiers, throughput, latency, users.
          Six tiles fit a single row on xl+ displays so the operator can read the whole
          KPI band without scrolling. Stays 2-up on mobile and 3-up on lg for
          comfortable card sizing. */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4 xl:grid-cols-4 2xl:grid-cols-8">
        <div className="anim-rise-sm" style={rise(0)}>
          <StatCard
            className="h-full"
            label={t("dashboard.traffic")}
            value={traffic.total_requests}
            format={formatCompactNumber}
            unit={t("dashboard.requests")}
            spark={requestSpark}
            hint={`${t("dashboard.today")} ${formatCompactNumber(traffic.today_requests)} · ${t(
              "dashboard.successRate",
            )} ${successRate != null ? formatPercent(successRate) : "-"} · ${t(
              "dashboard.errors",
            )} ${formatCompactNumber(traffic.error_requests)}`}
          />
        </div>
        <div className="anim-rise-sm" style={rise(1)}>
          <StatCard
            className="h-full"
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
            className="h-full"
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
            className="h-full"
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
            className="h-full"
            label={t("dashboard.latency")}
            value={performance.average_latency_ms}
            format={formatInteger}
            unit="ms"
            hint={`p95 ${formatInteger(performance.p95_latency_ms)} ms`}
          />
        </div>
        <div className="anim-rise-sm" style={rise(5)}>
          <StatCard
            className="h-full"
            label={t("dashboard.users")}
            value={users.active_users}
            format={formatInteger}
            unit={t("dashboard.activeUnit")}
            hint={`${t("dashboard.today")} +${formatInteger(users.today_new_users)} · ${t(
              "dashboard.total",
            )} ${formatInteger(users.total_users)}`}
          />
        </div>
        <div style={rise(6)}>
          <StatCard
            className="h-full"
            label={t("dashboard.errors")}
            value={errorRate != null ? errorRate * 100 : 0}
            format={(n) => `${n.toFixed(1)}%`}
            hint={`${formatCompactNumber(traffic.error_requests)} ${t("dashboard.errors")} / ${formatCompactNumber(traffic.total_requests)} ${t("dashboard.requests")}`}
          />
        </div>
        <RealtimeSlotsCard staggerIndex={7} />
      </div>

      {/* Account health overview */}
      <AccountHealthOverview staggerIndex={7} />

      {/* Token composition */}
      <Card className="anim-rise-sm" style={rise(8)}>
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
      <Card className="anim-rise-sm" style={rise(9)}>
        <CardContent>
          <div className="flex items-center gap-2 text-sm font-semibold text-srapi-text-primary">
            <span className="grid size-7 place-items-center rounded-lg bg-srapi-card-muted text-srapi-text-secondary">
              <LineChart className="size-3.5" />
            </span>
            {t("dashboard.tokenTrend")}
          </div>
          <div className="mt-4">
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
      <div className="grid gap-4 md:grid-cols-2" style={rise(10)}>
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

function SnapshotSummary({
  snapshot,
  errorRate,
}: {
  snapshot: AdminDashboardSnapshot;
  errorRate: number | null;
}) {
  const { t } = useLanguage();
  const { inventory, traffic, window, generated_at } = snapshot;
  const activeKeyRatio =
    inventory.total_api_keys > 0 ? inventory.active_api_keys / inventory.total_api_keys : null;
  const windowText = `${formatDateTime(window.start)} - ${formatDateTime(window.end)}`;

  return (
    <SpotlightCard className="relative overflow-hidden" style={rise(0)}>
      {/* A whisper-thin terracotta seam runs along the top to anchor the
          snapshot band visually without adding chrome. */}
      <div
        className="pointer-events-none absolute inset-x-6 top-0 h-px bg-gradient-to-r from-transparent via-srapi-primary/35 to-transparent"
        aria-hidden
      />
      <CardContent className="py-5">
        <div className="grid gap-x-6 gap-y-4 md:grid-cols-5">
          <SummaryMetric
            label={t("dashboard.window")}
            value={windowText}
            mono
            className="md:col-span-2"
          />
          <SummaryMetric
            label={t("dashboard.apiKeys")}
            value={`${formatInteger(inventory.active_api_keys)} / ${formatInteger(
              inventory.total_api_keys,
            )}`}
            hint={activeKeyRatio != null ? formatPercent(activeKeyRatio) : "-"}
          />
          <SummaryMetric
            label={t("dashboard.accounts")}
            value={`${formatInteger(inventory.healthy_accounts)} / ${formatInteger(
              inventory.total_accounts,
            )}`}
            hint={`${formatInteger(inventory.abnormal_accounts)} ${t("dashboard.abnormal")}`}
          />
          <SummaryMetric
            label={t("dashboard.errors")}
            value={formatInteger(traffic.error_requests)}
            hint={errorRate != null ? formatPercent(errorRate) : "-"}
            tone={traffic.error_requests > 0 ? "danger" : "success"}
          />
        </div>
        <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-srapi-border/70 pt-3">
          <span className="text-xs font-medium uppercase tracking-[0.14em] text-srapi-text-tertiary">
            {t("dashboard.generatedAt")}
          </span>
          <span className="text-xs tabular text-srapi-text-secondary">
            {formatDateTime(generated_at)}
          </span>
        </div>
      </CardContent>
    </SpotlightCard>
  );
}

function SummaryMetric({
  label,
  value,
  hint,
  tone = "neutral",
  mono,
  className,
}: {
  label: string;
  value: string;
  hint?: string;
  tone?: "neutral" | "success" | "danger";
  mono?: boolean;
  className?: string;
}) {
  return (
    <div className={cn("min-w-0", className)}>
      <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
        {label}
      </div>
      <div
        className={cn(
          "mt-1.5 truncate text-lg font-semibold leading-tight tracking-tight text-srapi-text-primary",
          mono && "text-xs font-medium tabular",
        )}
        title={value}
      >
        {value}
      </div>
      {hint ? (
        <Badge
          variant={tone === "danger" ? "danger" : tone === "success" ? "success" : "neutral"}
          className="mt-2"
        >
          {hint}
        </Badge>
      ) : null}
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
            <span className="grid size-8 place-items-center rounded-xl bg-srapi-card-muted text-srapi-text-secondary">
              <Server className="size-4" />
            </span>
            <span className="text-sm font-semibold text-srapi-text-primary">
              {t("dashboard.accountHealth")}
            </span>
          </div>
          <a
            href={ADMIN_ROUTES.accounts + "?view=health"}
            className="rounded-full bg-srapi-card-muted px-3 py-1 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:bg-srapi-accent-soft hover:text-srapi-primary"
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

// Human-readable short labels for the realtime slot breakdown. Endpoints and
// kinds arrive as raw routing identifiers; map the known ones to compact names
// and fall back to a tidied raw key so new variants still render.
const REALTIME_ENDPOINT_LABELS: Record<string, string> = {
  "/v1/realtime": "Realtime",
  "/v1/responses/ws": "Responses WS",
};
const REALTIME_KIND_LABELS: Record<string, string> = {
  realtime_websocket: "Realtime",
  responses_websocket: "Responses",
};

function prettyRealtimeKey(key: string, labels: Record<string, string>): string {
  return labels[key] ?? key.replace(/^\/v1\//, "").replace(/[_/]/g, " ").trim();
}

// breakdownHint renders a compact "Label N · Label N" string from the first
// non-empty of the per-endpoint / per-kind counter maps (endpoint preferred,
// since its labels read better). Returns null when nothing is broken out.
function breakdownHint(counters: RealtimeActiveSlotCounters): string | null {
  const byEndpoint = Object.entries(counters.active_by_endpoint ?? {});
  const byKind = Object.entries(counters.active_by_kind ?? {});
  const source = byEndpoint.length > 0 ? byEndpoint : byKind;
  const labels = byEndpoint.length > 0 ? REALTIME_ENDPOINT_LABELS : REALTIME_KIND_LABELS;
  const parts = source
    .filter(([, count]) => count > 0)
    .sort((a, b) => b[1] - a[1])
    .map(([key, count]) => `${prettyRealtimeKey(key, labels)} ${formatInteger(count)}`);
  return parts.length > 0 ? parts.join(" · ") : null;
}

// RealtimeSlotsCard surfaces the live count of active realtime/websocket slots
// (RealtimeActiveSlotCounters.active_slots) as a dashboard KPI, with an optional
// per-endpoint / per-kind breakdown in the hint. Polls every ~30s via its hook.
function RealtimeSlotsCard({ staggerIndex }: { staggerIndex: number }) {
  const { t } = useLanguage();
  const query = useListOpsRealtimeSlots();
  const counters = query.data?.counters;

  if (query.isLoading && !counters) {
    return (
      <div className="anim-rise-sm" style={rise(staggerIndex)}>
        <StatCardSkeleton className="h-full" />
      </div>
    );
  }
  if (query.isError || !counters) return null;

  const breakdown = breakdownHint(counters);
  const hint =
    breakdown ??
    `${t("dashboard.today")} ${formatInteger(counters.acquired_total)} ${t("dashboard.requests")}`;

  return (
    <div className="anim-rise-sm" style={rise(staggerIndex)}>
      <StatCard
        className="h-full"
        label="Realtime slots"
        value={counters.active_slots}
        format={formatInteger}
        unit="active"
        hint={
          <span className="flex items-center gap-1.5">
            <Radio className="size-3 text-srapi-text-tertiary" />
            {hint}
          </span>
        }
      />
    </div>
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
    <div className="flex items-center gap-2.5 rounded-xl border border-srapi-border bg-srapi-card/60 px-3.5 py-2.5">
      <span className={cn("size-2.5 rounded-full", dotColor, count === 0 && "opacity-30")} />
      <div>
        <div className={cn("text-base font-semibold tabular text-srapi-text-primary", count === 0 && "text-srapi-text-tertiary")}>
          {count}
        </div>
        <div className="text-[11px] font-medium leading-tight text-srapi-text-tertiary">{label}</div>
      </div>
    </div>
  );
}
