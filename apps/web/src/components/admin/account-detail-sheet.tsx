"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { Sheet, SheetContent, SheetTitle, SheetDescription } from "@/components/ui/sheet";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import {
  useAccountHealth,
  useAccountQuota,
  useFetchAccountQuota,
  useAccountRpmStatus,
  useAccountProxyQuality,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { cn } from "@/lib/cn";
import {
  latestQuotaWindows,
  quotaWindowDisplayLabel,
  quotaWindowTiming,
  quotaWindowValue,
  type QuotaDisplayWindow,
} from "@/lib/quota-display";
import type { ProviderAccount } from "@/lib/sdk-types";

function pct(ratio: number): string {
  return `${Math.round(ratio * 100)}%`;
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5">
      <span className="text-2xs uppercase tracking-wide text-srapi-text-tertiary">{label}</span>
      <span className="font-mono text-xs text-srapi-text-primary tabular">{value}</span>
    </div>
  );
}

function QuotaWindowRow({ window }: { window: QuotaDisplayWindow }) {
  const { t } = useLanguage();
  const level =
    window.remainingPercent <= 10 ? "crit" : window.remainingPercent <= 30 ? "warn" : "ok";
  return (
    <div className="space-y-1.5 py-2">
      <div className="flex items-baseline justify-between gap-3">
        <span className="font-mono text-2xs uppercase tracking-wide text-srapi-text-secondary">
          {quotaWindowDisplayLabel(window, t)}
        </span>
        <span
          className={cn(
            "font-mono text-xs tabular",
            level === "crit"
              ? "text-srapi-error"
              : level === "warn"
                ? "text-srapi-warning"
                : "text-srapi-text-primary",
          )}
        >
          {Math.round(window.remainingPercent)}%
        </span>
      </div>
      <div className="relative h-1.5 overflow-hidden rounded-full bg-srapi-border">
        <div
          className={cn(
            "h-full rounded-full transition-all",
            level === "crit"
              ? "bg-srapi-error"
              : level === "warn"
                ? "bg-srapi-warning"
                : "bg-srapi-success",
          )}
          style={{ width: `${Math.max(window.remainingPercent, 2)}%` }}
        />
      </div>
      <div className="flex items-center justify-between gap-3 font-mono text-2xs text-srapi-text-tertiary">
        <span>{quotaWindowValue(window)}</span>
        <span>{quotaWindowTiming(window, t)}</span>
      </div>
    </div>
  );
}

/** Wrap one detail query: skeleton while loading, children when data, a quiet
 * "no data" line otherwise (the read endpoints 404 for accounts that never ran). */
function Section<T>({
  title,
  query,
  action,
  children,
}: {
  title: string;
  query: UseQueryResult<T>;
  action?: React.ReactNode;
  children: (data: T) => React.ReactNode;
}) {
  const { t } = useLanguage();
  return (
    <div className="border-t border-srapi-border pt-4 first:border-t-0 first:pt-0">
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="font-mono text-2xs uppercase tracking-widest text-srapi-text-secondary">
          {title}
        </h3>
        {action}
      </div>
      {query.isLoading ? (
        <DialogListSkeleton rows={2} />
      ) : query.data === undefined ? (
        <p className="text-2xs text-srapi-text-tertiary">{t("adminAccounts.detailNoData")}</p>
      ) : (
        children(query.data)
      )}
    </div>
  );
}

/**
 * Read-only diagnostics drawer for one provider account. Surfaces the admin
 * health / quota / RPM / proxy-quality endpoints that previously had no UI.
 */
export function AccountDetailSheet({
  account,
  onOpenChange,
}: {
  account: ProviderAccount | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const id = account?.id ?? null;
  const health = useAccountHealth(id);
  const quota = useAccountQuota(id);
  const rpm = useAccountRpmStatus(id);
  const proxy = useAccountProxyQuality(id);
  const fetchQuota = useFetchAccountQuota();

  async function refreshQuota() {
    if (!id) return;
    try {
      const report = await fetchQuota.mutateAsync(id);
      toast({
        title: report.supported
          ? t("adminAccounts.quotaRefreshed")
          : t("adminAccounts.quotaUnsupported"),
        description: report.supported
          ? t("adminAccounts.quotaCredits", {
              remaining: report.credits_remaining,
              limit: report.credits_limit,
            })
          : undefined,
        tone: report.supported ? "success" : "default",
      });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  return (
    <Sheet open={account !== null} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-[28rem] gap-0 overflow-y-auto p-6">
        <SheetTitle>{t("adminAccounts.detailTitle")}</SheetTitle>
        {account ? <SheetDescription className="text-sm text-srapi-text-secondary">{account.name}</SheetDescription> : null}

        <div className="mt-5 space-y-4">
          <Section title={t("adminAccounts.healthTitle")} query={health}>
            {(h) => (
              <div>
                <Row label={t("adminCommon.status")} value={h.status} />
                <Row label={t("adminAccounts.successRate")} value={pct(h.success_rate)} />
                <Row label={`${t("adminAccounts.latency")} p50 / p95`} value={`${Math.round(h.latency_p50_ms)} / ${Math.round(h.latency_p95_ms)}ms`} />
                <Row label={t("adminAccounts.circuitState")} value={h.circuit_state} />
                {h.error_class ? <Row label={t("adminAccounts.lastError")} value={h.error_class} /> : null}
                {h.cooldown_until ? <Row label={t("adminAccounts.cooldown")} value={h.cooldown_reason ?? h.cooldown_until} /> : null}
              </div>
            )}
          </Section>

          <Section title={t("adminAccounts.rpmTitle")} query={rpm}>
            {(r) => (
              <div>
                <Row
                  label={t("adminAccounts.rpmTitle")}
                  value={`${r.rpm_used} / ${r.rpm_limit ?? "∞"}`}
                />
                <Row label="window" value={`${r.window_seconds}s`} />
              </div>
            )}
          </Section>

          <Section
            title={t("adminAccounts.quotaTitle")}
            query={quota}
            action={
              id ? (
                <button
                  type="button"
                  onClick={() => void refreshQuota()}
                  disabled={fetchQuota.isPending}
                  className="flex items-center gap-1 font-mono text-2xs uppercase tracking-wide text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary disabled:opacity-50"
                >
                  <RefreshCw className={cn("size-3", fetchQuota.isPending && "animate-spin")} />
                  {t("adminAccounts.quotaRefresh")}
                </button>
              ) : null
            }
          >
            {(q) =>
              q.data.length === 0 ? (
                <p className="text-2xs text-srapi-text-tertiary">{t("adminAccounts.detailNoData")}</p>
              ) : (
                <div className="space-y-2">
                  {latestQuotaWindows(q.data).map((window) => (
                    <QuotaWindowRow key={window.snapshot.quota_type} window={window} />
                  ))}
                </div>
              )
            }
          </Section>

          <Section title={t("adminAccounts.proxyQualityTitle")} query={proxy}>
            {(p) => (
              <div>
                <Row label={t("adminAccounts.proxy")} value={p.proxy_id ?? t("adminAccounts.noProxy")} />
                <Row label={t("adminAccounts.successRate")} value={pct(p.success_rate)} />
                <Row label={`${t("adminAccounts.latency")} p95`} value={`${Math.round(p.latency_p95_ms)}ms`} />
                <Row label="samples" value={p.sample_count} />
              </div>
            )}
          </Section>
        </div>
      </SheetContent>
    </Sheet>
  );
}
