"use client";

import { Activity, RefreshCw, RotateCcw, Database, Wifi, WifiOff } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { Skeleton } from "@/components/ui/skeleton";
import {
  useAdminCircuitBreakers,
  useResetCircuitBreaker,
  useAdminCacheStats,
  useClearCache,
  useAdminAccounts,
} from "@/hooks/admin-queries";
import { useAdminEventStream } from "@/hooks/use-admin-events";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { quietStatusFor } from "@/lib/status-badge";
import type { CircuitBreakerEntry, CacheStatsEntry } from "@/lib/admin-api";

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
  const accounts = useAdminAccounts({ page: 1, page_size: 200 });

  const { connected: streamConnected } = useAdminEventStream(
    (event) => {
      if (event.type === "circuit_breaker") {
        void breakers.refetch();
      }
    },
  );
  const accountNameById = new Map(
    (accounts.data?.data ?? []).map((a) => [a.id, a.name] as const),
  );

  async function handleReset(accountId: number) {
    try {
      await resetMut.mutateAsync(accountId);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch {
      toast({ title: t("feedback.failed"), tone: "error" });
    }
  }

  const refetchAll = () => {
    void breakers.refetch();
    void cacheStats.refetch();
  };

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("diagnostics.title")}
        actions={
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1.5 font-mono text-2xs text-srapi-text-tertiary">
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
            ) : !breakers.data?.length ? (
              <p className="py-6 text-center text-sm text-srapi-text-tertiary">
                {t("diagnostics.noBreakers")}
              </p>
            ) : (
              <div className="divide-y divide-srapi-border">
                {breakers.data.map((entry) => (
                  <div
                    key={entry.account_id}
                    className="flex flex-wrap items-center justify-between gap-3 py-3"
                  >
                    <div className="flex items-center gap-3">
                      <span className="font-mono text-sm text-srapi-text-primary">
                        {accountNameById.get(String(entry.account_id)) ?? `#${entry.account_id}`}
                      </span>
                      <QuietBadge status={breakerBadgeVariant(entry.state)} label={entry.state} />
                    </div>
                    <div className="flex items-center gap-4">
                      <div className="flex gap-3 font-mono text-2xs text-srapi-text-tertiary">
                        <span className={entry.success_rate >= 0.95 ? "text-srapi-success" : entry.success_rate >= 0.8 ? "text-srapi-warning" : "text-srapi-error"}>
                          {Math.round(entry.success_rate * 100)}%
                        </span>
                        <span>req: {entry.requests}</span>
                        <span className="text-emerald-500">ok: {entry.total_successes}</span>
                        <span className="text-red-400">fail: {entry.total_failures}</span>
                        <span>streak: +{entry.consecutive_successes} / -{entry.consecutive_failures}</span>
                      </div>
                      {entry.state !== "closed" && (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleReset(entry.account_id)}
                          disabled={resetMut.isPending}
                        >
                          <RotateCcw className="mr-1 size-3" />
                          Reset
                        </Button>
                      )}
                    </div>
                  </div>
                ))}
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
                ).catch(() =>
                  toast({ title: t("feedback.failed"), tone: "error" }),
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
            ) : !cacheStats.data?.length ? (
              <p className="py-6 text-center text-sm text-srapi-text-tertiary">
                {t("diagnostics.noCaches")}
              </p>
            ) : (
              <div className="divide-y divide-srapi-border">
                {cacheStats.data.map((cache) => {
                  const rateNum = parseFloat(cache.hit_rate) || 0;
                  return (
                    <div key={cache.name} className="space-y-2 py-3">
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <span className="font-mono text-sm text-srapi-text-primary">{cache.name}</span>
                        <div className="flex gap-4 font-mono text-2xs text-srapi-text-tertiary">
                          <span>size: {cache.size}</span>
                          <span className="text-emerald-500">hits: {cache.hits}</span>
                          <span className="text-red-400">misses: {cache.misses}</span>
                          <span>evictions: {cache.evictions}</span>
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <div className="relative h-1.5 flex-1 overflow-hidden rounded-full bg-srapi-border">
                          <div
                            className="h-full rounded-full bg-srapi-success transition-all"
                            style={{ width: `${Math.max(rateNum, 1)}%` }}
                          />
                        </div>
                        <span className="font-mono text-2xs text-srapi-text-secondary tabular">{cache.hit_rate}</span>
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
