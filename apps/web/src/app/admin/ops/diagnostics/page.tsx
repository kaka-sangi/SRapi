"use client";

import { Activity, RefreshCw, RotateCcw, Database, Wifi, WifiOff } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { DataTooltip } from "@/components/ui/data-tooltip";
import {
  useAdminCircuitBreakers,
  useResetCircuitBreaker,
  useAdminCacheStats,
  useClearCache,
} from "@/hooks/admin-queries";
import { useAccountNameLookup } from "@/hooks/use-account-name-lookup";
import { useAdminEventStream } from "@/hooks/use-admin-events";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import {
  cacheHealthSummary,
  circuitBreakerStateLabelKey,
  circuitBreakerSummary,
} from "@/lib/admin-diagnostics-summary";
import { quietStatusFor } from "@/lib/status-badge";
import { adminErrorMessage } from "@/lib/admin-api";
import type { CircuitBreakerEntry } from "@/lib/admin-api";

export default function AdminDiagnosticsPage() {
  return (
    <AdminShell>
      <DiagnosticsContent />
    </AdminShell>
  );
}

function breakerBadgeVariant(state: CircuitBreakerEntry["state"]) {
  switch (state) {
    case "closed":
      return quietStatusFor("active");
    case "open":
      return quietStatusFor("disabled");
    case "half-open":
      return quietStatusFor("pending");
    default:
      return quietStatusFor("active");
  }
}

function DiagnosticsContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const breakers = useAdminCircuitBreakers();
  const cacheStats = useAdminCacheStats();
  const resetMut = useResetCircuitBreaker();
  const clearCacheMut = useClearCache();
  const accountLookup = useAccountNameLookup();

  const { connected: streamConnected } = useAdminEventStream(
    (event) => {
      if (event.type === "circuit_breaker") {
        void breakers.refetch();
      }
    },
  );

  async function handleReset(accountId: number) {
    try {
      await resetMut.mutateAsync(accountId);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const refetchAll = () => {
    void breakers.refetch();
    void cacheStats.refetch();
  };

  return (
    <>
      <SectionHero
        eyebrow={t("hero.eyebrowOpsDiagnostics")}
        title={t("diagnostics.title")}
        description={t("diagnostics.subtitle")}
        metrics={[
          {
            label: t("diagnostics.lastRun"),
            value: breakers.dataUpdatedAt
              ? new Date(breakers.dataUpdatedAt).toLocaleTimeString()
              : "—",
          },
        ]}
        actions={
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1.5 text-[11px] text-srapi-text-tertiary">
              {streamConnected ? (
                <><Wifi className="size-3 text-srapi-success" /> {t("common.live")}</>
              ) : (
                <><WifiOff className="size-3" /> SSE</>
              )}
            </span>
            <AutoRefreshControl
              onRefresh={refetchAll}
              isRefreshing={breakers.isFetching || cacheStats.isFetching}
              storageKey="srapi.autorefresh.diagnostics"
              defaultSec={10}
            />
          </div>
        }
      />

      <div className="space-y-6">
        {/* Circuit Breakers */}
        <Card>
          <CardHeader className="flex-row items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <Activity className="size-4 text-srapi-text-tertiary" />
              <CardTitle>{t("diagnostics.circuitBreakers")}</CardTitle>
            </div>
            <button
              type="button"
              onClick={() => void breakers.refetch()}
              className="flex size-7 items-center justify-center rounded-md border border-srapi-border text-srapi-text-tertiary transition-colors hover:text-srapi-text-primary"
              aria-label={t("common.refreshNow")}
            >
              <RefreshCw className={`size-3 ${breakers.isFetching ? "animate-spin" : ""}`} />
            </button>
          </CardHeader>
          <CardContent>
            {breakers.isLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} className="h-12 w-full" />
                ))}
              </div>
            ) : breakers.isError ? (
              <IllustratedEmptyState
                illust="cog"
                title={t("common.error")}
                description={t("common.errorBody")}
                action={
                  <Button variant="outline" size="sm" onClick={() => void breakers.refetch()}>
                    {t("common.retry")}
                  </Button>
                }
              />
            ) : !breakers.data?.length ? (
              <IllustratedEmptyState
                illust="cog"
                title={t("diagnostics.noBreakers")}
                description={t("diagnostics.subtitle") === "diagnostics.subtitle" ? undefined : t("diagnostics.subtitle")}
              />
            ) : (
              <div className="divide-y divide-srapi-border">
                {breakers.data.map((entry) => {
                  const summary = circuitBreakerSummary(entry);
                  return (
                    <div
                      key={entry.account_id}
                      className="flex flex-wrap items-center justify-between gap-3 py-3"
                    >
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-srapi-text-primary">
                            {accountLookup.get(entry.account_id)}
                          </span>
                          <QuietBadge
                            status={breakerBadgeVariant(entry.state)}
                            label={t(circuitBreakerStateLabelKey(entry.state))}
                          />
                          <QuietBadge status={summary.tone} label={t(summary.labelKey)} />
                        </div>
                        <div className="mt-1 text-[11px] text-srapi-text-tertiary">
                          {summary.detail}
                        </div>
                      </div>
                      <div className="flex items-center gap-4">
                        <DataTooltip
                          title={t("diagnostics.successRate")}
                          primary={`${Math.round(entry.success_rate * 100)}%`}
                          rows={[
                            { label: t("diagnostics.state"), value: entry.state, tone: "muted" },
                            { label: t("diagnostics.requestCount"), value: String(entry.requests), tone: "muted" },
                            {
                              label: t("diagnostics.failures"),
                              value: String(entry.total_failures),
                              tone: entry.total_failures > 0 ? "error" : "muted",
                            },
                            {
                              label: t("diagnostics.consecutiveFailures"),
                              value: String(entry.consecutive_failures),
                              tone: entry.consecutive_failures > 0 ? "warning" : "muted",
                            },
                          ]}
                          footer={summary.detail}
                        >
                          <span
                            className={
                              "tabular " +
                              (entry.success_rate >= 0.95
                                ? "metric-strong-good"
                                : entry.success_rate >= 0.8
                                  ? "metric-strong-warn"
                                  : "metric-strong-bad")
                            }
                          >
                            {Math.round(entry.success_rate * 100)}%
                          </span>
                        </DataTooltip>
                        {entry.state !== "closed" && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleReset(entry.account_id)}
                            loading={resetMut.isPending}
                          >
                            <RotateCcw className="mr-1 size-3" />
                            {t("common.reset")}
                          </Button>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Cache Stats */}
        <Card>
          <CardHeader className="flex-row items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <Database className="size-4 text-srapi-text-tertiary" />
              <CardTitle>{t("diagnostics.cacheStats")}</CardTitle>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                void clearCacheMut.mutateAsync().then((r) =>
                  toast({ title: t("diagnostics.cacheCleared", { count: r.cleared }), tone: "success" }),
                ).catch((err: unknown) =>
                  toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" }),
                );
              }}
              disabled={clearCacheMut.isPending}
            >
              <RotateCcw className={`mr-1 size-3 ${clearCacheMut.isPending ? "animate-spin" : ""}`} />
              {t("diagnostics.clearCache")}
            </Button>
          </CardHeader>
          <CardContent>
            {cacheStats.isLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-12 w-full" />
              </div>
            ) : cacheStats.isError ? (
              <IllustratedEmptyState
                illust="cog"
                title={t("common.error")}
                description={t("common.errorBody")}
                action={
                  <Button variant="outline" size="sm" onClick={() => void cacheStats.refetch()}>
                    {t("common.retry")}
                  </Button>
                }
              />
            ) : !cacheStats.data?.length ? (
              <IllustratedEmptyState illust="cog" title={t("diagnostics.noCaches")} />
            ) : (
              <div className="divide-y divide-srapi-border">
                {cacheStats.data.map((cache) => {
                  const rateNum = parseFloat(cache.hit_rate) || 0;
                  const summary = cacheHealthSummary(cache);
                  return (
                    <div key={cache.name} className="space-y-2 py-3">
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-sm text-srapi-text-primary">{cache.name}</span>
                          <QuietBadge status={summary.tone} label={t(summary.labelKey)} />
                        </div>
                        <div className="flex gap-4 text-[11px] text-srapi-text-tertiary">
                          <span>size: {cache.size}</span>
                          <span className="text-srapi-success">hits: {cache.hits}</span>
                          <span className="text-srapi-error">misses: {cache.misses}</span>
                          <span>evictions: {cache.evictions}</span>
                        </div>
                      </div>
                      <div className="text-[11px] text-srapi-text-tertiary">
                        {summary.detail}
                      </div>
                      <div className="flex items-center gap-2">
                        <div className="relative h-1.5 flex-1 overflow-hidden rounded-full bg-srapi-border">
                          <div
                            className={
                              "h-full rounded-full transition-all " +
                              (rateNum >= 80
                                ? "bg-srapi-success"
                                : rateNum >= 50
                                  ? "bg-srapi-warning"
                                  : "bg-srapi-error")
                            }
                            style={{ width: `${Math.max(rateNum, 1)}%` }}
                          />
                        </div>
                        <DataTooltip
                          title={t("diagnostics.cacheHitRate")}
                          primary={cache.hit_rate}
                          rows={[
                            { label: "size", value: String(cache.size), tone: "muted" },
                            { label: "hits", value: String(cache.hits), tone: "success" },
                            { label: "misses", value: String(cache.misses), tone: cache.misses > 0 ? "error" : "muted" },
                            { label: "evictions", value: String(cache.evictions), tone: "muted" },
                          ]}
                          footer={summary.detail}
                        >
                          <span
                            className={
                              "tabular " +
                              (rateNum >= 80
                                ? "metric-strong-good"
                                : rateNum >= 50
                                  ? "metric-strong-warn"
                                  : "metric-strong-bad")
                            }
                          >
                            {cache.hit_rate}
                          </span>
                        </DataTooltip>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}
