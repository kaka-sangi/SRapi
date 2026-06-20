"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import Link from "next/link";
import { Sheet, SheetContent, SheetTitle, SheetDescription } from "@/components/ui/sheet";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import {
  useAccountHealth,
  useAccountQuota,
  useFetchAccountQuota,
  useAccountRpmStatus,
  useAccountProxyQuality,
  useAccountUsageWindows,
  useAccountUsageToday,
  useAccountUsageDaily,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  adminAccountHealthInvestigationLinks,
  type AccountHealthInvestigationLinks,
} from "@/lib/admin-account-health-investigation";
import { formatCompactNumber, formatDate, formatMoney, formatPercent } from "@/lib/admin-format";
import { runtimeClassLabel } from "@/lib/admin-account-form";
import { cn } from "@/lib/cn";
import { StatCard } from "@/components/ui/stat-card";
import { Sparkline } from "@/components/charts/sparkline";
import { accountModelPolicyLabel } from "@/app/admin/accounts/account-types";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TableScroll,
} from "@/components/ui/table";
import {
  latestQuotaWindows,
  quotaWindowDisplayLabel,
  quotaWindowTiming,
  quotaWindowValue,
  type QuotaDisplayWindow,
} from "@/lib/quota-display";
import type {
  ProviderAccount,
  AccountUsageDailyPoint,
  AccountUsageToday,
  AccountUsageWindow,
  AccountUsageWindowsResult,
} from "@/lib/sdk-types";

const DAILY_USAGE_VISIBLE_ROWS = 14;

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

function DetailMetric({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-srapi-border bg-srapi-bg-muted px-3 py-2">
      <div className="font-mono text-[10px] uppercase tracking-wide text-srapi-text-tertiary">
        {label}
      </div>
      <div
        className="mt-1 truncate font-mono text-2xs text-srapi-text-secondary"
        title={typeof value === "string" ? value : undefined}
      >
        {value}
      </div>
    </div>
  );
}

function dayKeyFromTimestamp(value?: string | null): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return date.toISOString().slice(0, 10);
}

function hasDailyTraffic(point: AccountUsageDailyPoint): boolean {
  const cost = Number(point.cost);
  return (
    point.requests > 0 ||
    point.input_tokens > 0 ||
    point.output_tokens > 0 ||
    (Number.isFinite(cost) && cost > 0)
  );
}

function accountGroupSummary(
  account: ProviderAccount,
  groupNameById?: Map<string, string>,
): string {
  const ids = account.group_ids ?? [];
  if (ids.length === 0) return "";
  const names = ids.slice(0, 3).map((id) => groupNameById?.get(String(id)) ?? `#${id}`);
  const extra = ids.length - names.length;
  return extra > 0 ? `${names.join(", ")} +${extra}` : names.join(", ");
}

function compactDailyUsagePoints(
  account: ProviderAccount,
  points: AccountUsageDailyPoint[],
): AccountUsageDailyPoint[] {
  if (points.length === 0) return [];
  const firstTrafficDay = points.find(hasDailyTraffic)?.date ?? null;
  const accountCreatedDay = dayKeyFromTimestamp(account.created_at);
  const startDay = firstTrafficDay ?? accountCreatedDay;
  if (!startDay) return points;
  return points.filter((point) => point.date >= startDay);
}

function AccountHealthEvidenceLinks({ links }: { links: AccountHealthInvestigationLinks }) {
  const { t } = useLanguage();
  const items = [
    links.errorLogs ? [links.errorLogs, t("adminErrorLogs.title")] : null,
    links.requestEvidence ? [links.requestEvidence, t("adminRequestEvidence.title")] : null,
    links.schedulerDecisions ? [links.schedulerDecisions, t("scheduler.title")] : null,
  ].filter((item): item is [string, string] => Boolean(item));

  return (
    <span className="flex flex-wrap justify-end gap-x-2 gap-y-1">
      {items.map(([href, label]) => (
        <Link
          key={href}
          href={href}
          className="text-srapi-accent underline-offset-2 hover:underline"
        >
          {label}
        </Link>
      ))}
    </span>
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

/** Map a usage-window key ("5h"/"7d") to a localized label, falling back to the
 * raw key for any window the SDK adds later. */
function usageWindowLabel(window: string, t: (k: string) => string): string {
  if (window === "5h") return t("adminAccounts.usageWindow5h");
  if (window === "7d") return t("adminAccounts.usageWindow7d");
  return window;
}

/** One 5h/7d window: a requests bar (scaled to the busiest window in the set)
 * plus tokens / cost / error-count stats. Reuses the QuotaWindowRow bar style. */
function UsageWindowCard({
  window,
  maxRequests,
}: {
  window: AccountUsageWindow;
  maxRequests: number;
}) {
  const { t } = useLanguage();
  const ratio = maxRequests > 0 ? window.requests / maxRequests : 0;
  const hasErrors = window.error_count > 0;
  return (
    <div className="space-y-1.5 py-2">
      <div className="flex items-baseline justify-between gap-3">
        <span className="font-mono text-2xs uppercase tracking-wide text-srapi-text-secondary">
          {usageWindowLabel(window.window, t)}
        </span>
        <span className="font-mono text-xs text-srapi-text-primary tabular">
          {formatCompactNumber(window.requests)} {t("adminAccounts.usageRequests").toLowerCase()}
        </span>
      </div>
      <div className="relative h-1.5 overflow-hidden rounded-full bg-srapi-border">
        <div
          className="h-full rounded-full bg-srapi-success transition-all"
          style={{ width: `${Math.max(ratio * 100, window.requests > 0 ? 2 : 0)}%` }}
        />
      </div>
      <div className="flex items-center justify-between gap-3 font-mono text-2xs text-srapi-text-tertiary">
        <span>
          {t("adminAccounts.usageTokens")} {formatCompactNumber(window.total_tokens)}
        </span>
        <span>{formatMoney(window.cost, window.currency)}</span>
        <span className={cn(hasErrors && "text-srapi-error")}>
          {t("adminAccounts.usageErrors")} {window.error_count}
        </span>
      </div>
    </div>
  );
}

function UsageWindowsBody({ result }: { result: AccountUsageWindowsResult }) {
  const { t } = useLanguage();
  if (result.windows.length === 0) {
    return <p className="text-2xs text-srapi-text-tertiary">{t("adminAccounts.detailNoData")}</p>;
  }
  const maxRequests = Math.max(...result.windows.map((w) => w.requests), 0);
  return (
    <div className="space-y-2">
      {result.windows.map((window) => (
        <UsageWindowCard key={window.window} window={window} maxRequests={maxRequests} />
      ))}
    </div>
  );
}

/** Compact 2x2 stat grid for the "since midnight" roll-up. */
function UsageTodayBody({ today }: { today: AccountUsageToday }) {
  const { t } = useLanguage();
  const tokens = today.total_tokens || today.input_tokens + today.output_tokens;
  return (
    <div className="grid grid-cols-2 gap-2">
      <StatCard
        className="p-3"
        label={t("adminAccounts.usageRequests")}
        value={today.requests}
        format={(n) => formatCompactNumber(n)}
      />
      <StatCard
        className="p-3"
        label={t("adminAccounts.usageTokens")}
        value={tokens}
        format={(n) => formatCompactNumber(n)}
      />
      <StatCard
        className="p-3"
        label={t("adminAccounts.usageCost")}
        value={formatMoney(today.cost, today.currency)}
      />
      <StatCard
        className="p-3"
        label={t("adminAccounts.usageSuccessRate")}
        value={formatPercent(today.success_rate)}
        hint={`${today.success_count}/${today.requests}`}
      />
    </div>
  );
}

/** Recent account usage as a requests sparkline above a dense mini-table. */
function UsageDailyBody({
  account,
  points,
}: {
  account: ProviderAccount;
  points: AccountUsageDailyPoint[];
}) {
  const { t } = useLanguage();
  const compactPoints = compactDailyUsagePoints(account, points);
  if (compactPoints.length === 0) {
    return <p className="text-2xs text-srapi-text-tertiary">{t("adminAccounts.detailNoData")}</p>;
  }
  // The series arrives oldest→newest from the read model; render most-recent
  // first in the table but keep chronological order for the sparkline.
  const spark = compactPoints.map((p) => p.requests);
  const rows = [...compactPoints].reverse().slice(0, DAILY_USAGE_VISIBLE_ROWS);
  return (
    <div className="space-y-3">
      {spark.length >= 2 ? (
        <Sparkline values={spark} ariaLabel={t("adminAccounts.usageDailyTitle")} className="h-10" />
      ) : null}
      <TableScroll minWidth={320}>
        <Table className="text-2xs">
          <TableHeader>
            <TableRow>
              <TableHead className="h-7">{t("adminAccounts.usageDailyDate")}</TableHead>
              <TableHead className="h-7 text-right">{t("adminAccounts.usageRequests")}</TableHead>
              <TableHead className="h-7 text-right">{t("adminAccounts.usageTokens")}</TableHead>
              <TableHead className="h-7 text-right">{t("adminAccounts.usageCost")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((p) => (
              <TableRow key={p.date}>
                <TableCell className="py-1.5 font-mono text-srapi-text-secondary">
                  {formatDate(p.date)}
                </TableCell>
                <TableCell className="py-1.5 text-right font-mono tabular text-srapi-text-primary">
                  {formatCompactNumber(p.requests)}
                </TableCell>
                <TableCell className="py-1.5 text-right font-mono tabular text-srapi-text-secondary">
                  {formatCompactNumber(p.input_tokens + p.output_tokens)}
                </TableCell>
                <TableCell className="py-1.5 text-right font-mono tabular text-srapi-text-secondary">
                  {formatMoney(p.cost, p.currency)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableScroll>
      {compactPoints.length > rows.length ? (
        <p className="font-mono text-2xs text-srapi-text-tertiary">
          {t("adminAccounts.usageDailyRowsShown", {
            shown: rows.length,
            total: compactPoints.length,
          })}
        </p>
      ) : null}
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
  providerName,
  groupNameById,
  onOpenChange,
}: {
  account: ProviderAccount | null;
  providerName?: string;
  groupNameById?: Map<string, string>;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const id = account?.id ?? null;
  const health = useAccountHealth(id);
  const quota = useAccountQuota(id);
  const rpm = useAccountRpmStatus(id);
  const proxy = useAccountProxyQuality(id);
  const usageWindows = useAccountUsageWindows(id);
  const usageToday = useAccountUsageToday(id);
  const usageDaily = useAccountUsageDaily(id);
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

        {account ? (
          <div className="mt-3 flex flex-wrap gap-1.5">
            <span className="rounded-md bg-srapi-bg-muted px-2 py-0.5 font-mono text-2xs text-srapi-text-secondary">
              {account.status}
            </span>
            <span className="rounded-md bg-srapi-bg-muted px-2 py-0.5 text-2xs text-srapi-text-secondary">
              {runtimeClassLabel(t, account.runtime_class)}
            </span>
            {account.priority != null && account.priority !== 0 ? (
              <span className="rounded-md bg-srapi-bg-muted px-2 py-0.5 font-mono text-2xs text-srapi-text-tertiary">
                P{account.priority}
              </span>
            ) : null}
            {account.weight != null && account.weight !== 1 ? (
              <span className="rounded-md bg-srapi-bg-muted px-2 py-0.5 font-mono text-2xs text-srapi-text-tertiary">
                W{account.weight}
              </span>
            ) : null}
          </div>
        ) : null}

        {account ? (
          <div className="mt-3 grid grid-cols-2 gap-2">
            <DetailMetric
              label={t("adminAccounts.provider")}
              value={providerName ?? account.provider_id}
            />
            <DetailMetric
              label={t("adminAccounts.models")}
              value={accountModelPolicyLabel(t, account.metadata)}
            />
            <DetailMetric
              label={t("adminAccounts.groups")}
              value={accountGroupSummary(account, groupNameById) || t("adminAccounts.ungrouped")}
            />
            <DetailMetric
              label={t("adminAccounts.proxy")}
              value={account.proxy_id ? t("adminAccounts.proxyConfigured") : t("adminAccounts.noProxy")}
            />
            <DetailMetric
              label={t("adminAccounts.routing")}
              value={`P${account.priority ?? 0} / W${account.weight ?? 1}`}
            />
            <DetailMetric
              label={t("adminAccounts.createdAt")}
              value={formatDate(account.created_at)}
            />
          </div>
        ) : null}

        <div className="mt-5 space-y-4">
          <Section title={t("adminAccounts.healthTitle")} query={health}>
            {(h) => {
              const investigationLinks = adminAccountHealthInvestigationLinks(h);
              return (
                <div>
                  <Row label={t("adminCommon.status")} value={h.status} />
                  <Row label={t("adminAccounts.successRate")} value={pct(h.success_rate)} />
                  <Row label={`${t("adminAccounts.latency")} p50 / p95`} value={`${Math.round(h.latency_p50_ms)} / ${Math.round(h.latency_p95_ms)}ms`} />
                  <Row label={t("adminAccounts.circuitState")} value={h.circuit_state} />
                  {h.error_class ? (
                    <Row
                      label={t("adminAccounts.lastError")}
                      value={
                        investigationLinks?.errorLogs ? (
                          <Link
                            href={investigationLinks.errorLogs}
                            className="text-srapi-error underline-offset-2 hover:underline"
                          >
                            {h.error_class}
                          </Link>
                        ) : (
                          h.error_class
                        )
                      }
                    />
                  ) : null}
                  {investigationLinks ? (
                    <Row
                      label={t("adminAccounts.evidence")}
                      value={<AccountHealthEvidenceLinks links={investigationLinks} />}
                    />
                  ) : null}
                  {h.cooldown_until ? <Row label={t("adminAccounts.cooldown")} value={h.cooldown_reason ?? h.cooldown_until} /> : null}
                </div>
              );
            }}
          </Section>

          <Section title={t("adminAccounts.usageTodayTitle")} query={usageToday}>
            {(today) => <UsageTodayBody today={today} />}
          </Section>

          <Section title={t("adminAccounts.usageWindowsTitle")} query={usageWindows}>
            {(result) => <UsageWindowsBody result={result} />}
          </Section>

          <Section title={t("adminAccounts.usageDailyTitle")} query={usageDaily}>
            {(points) => account ? <UsageDailyBody account={account} points={points} /> : null}
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
