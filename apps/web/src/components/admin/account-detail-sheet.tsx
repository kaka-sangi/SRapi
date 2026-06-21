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
  ACCOUNT_USAGE_DAILY_MAX_DAYS,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  adminAccountHealthInvestigationLinks,
  type AccountHealthInvestigationLinks,
} from "@/lib/admin-account-health-investigation";
import {
  formatCompactNumber,
  formatDate,
  formatDateTime,
  formatMoney,
  formatPercent,
} from "@/lib/admin-format";
import { runtimeClassLabel } from "@/lib/admin-account-form";
import { cn } from "@/lib/cn";
import { StatCard } from "@/components/ui/stat-card";
import { Sparkline } from "@/components/charts/sparkline";
import {
  accountIdentitySummary,
  accountCapacityFacts,
  accountEndpointCapabilityFacts,
  accountModelPolicyLabel,
  accountProfileFacts,
} from "@/app/admin/accounts/account-types";
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
      <span className="text-2xs text-srapi-text-tertiary tracking-wide uppercase">{label}</span>
      <span className="text-srapi-text-primary tabular font-mono text-xs">{value}</span>
    </div>
  );
}

function DetailMetric({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="border-srapi-border bg-srapi-bg-muted min-w-0 rounded-md border px-3 py-2">
      <div className="text-srapi-text-tertiary font-mono text-[10px] tracking-wide uppercase">
        {label}
      </div>
      <div
        className="text-2xs text-srapi-text-secondary mt-1 truncate font-mono"
        title={typeof value === "string" ? value : undefined}
      >
        {value}
      </div>
    </div>
  );
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

function activeDailyUsagePoints(points: AccountUsageDailyPoint[]): AccountUsageDailyPoint[] {
  return points.filter(hasDailyTraffic);
}

function activeUsageDateSummary(
  points: AccountUsageDailyPoint[],
  t: (key: string, vars?: Record<string, string | number>) => string,
): string {
  const activePoints = activeDailyUsagePoints(points);
  if (activePoints.length === 0) return t("adminAccounts.neverUsed");
  const first = activePoints[0]?.date ?? "";
  const last = activePoints[activePoints.length - 1]?.date ?? "";
  if (!first || !last || first === last) return formatDate(last || first);
  return `${formatDate(first)} - ${formatDate(last)}`;
}

function usageWindowActiveRangeLabel(
  result: AccountUsageWindowsResult | undefined,
  t: (key: string, vars?: Record<string, string | number>) => string,
): string {
  if (!result) return t("adminAccounts.neverUsed");
  const dates = result.windows
    .filter((window) => window.requests > 0)
    .flatMap((window) =>
      [window.first_request_at, window.last_request_at].filter((value): value is string =>
        Boolean(value),
      ),
    );
  if (dates.length === 0) return t("adminAccounts.neverUsed");
  dates.sort((a, b) => Date.parse(a) - Date.parse(b));
  const first = dates[0];
  const last = dates[dates.length - 1];
  if (!first || !last || first === last) return formatDateTime(last || first);
  return `${formatDateTime(first)} - ${formatDateTime(last)}`;
}

function latestUsageWindowRequestAt(result: AccountUsageWindowsResult | undefined): string {
  if (!result) return "";
  return (
    result.windows
      .map((window) => window.last_request_at)
      .filter((value): value is string => Boolean(value))
      .sort((a, b) => Date.parse(b) - Date.parse(a))[0] ?? ""
  );
}

function activeUsageDayCountLabel(
  points: AccountUsageDailyPoint[],
  t: (key: string, vars?: Record<string, string | number>) => string,
): string {
  const count = activeDailyUsagePoints(points).length;
  if (count === 0) return t("adminAccounts.neverUsed");
  return t("adminAccounts.activeUsageDays", { count });
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
        <span className="text-2xs text-srapi-text-secondary font-mono tracking-wide uppercase">
          {quotaWindowDisplayLabel(window, t)}
        </span>
        <span
          className={cn(
            "tabular font-mono text-xs",
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
      <div className="bg-srapi-border relative h-1.5 overflow-hidden rounded-full">
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
      <div className="text-2xs text-srapi-text-tertiary flex items-center justify-between gap-3 font-mono">
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
        <span className="text-2xs text-srapi-text-secondary font-mono tracking-wide uppercase">
          {usageWindowLabel(window.window, t)}
        </span>
        <span className="text-srapi-text-primary tabular font-mono text-xs">
          {formatCompactNumber(window.requests)} {t("adminAccounts.usageRequests").toLowerCase()}
        </span>
      </div>
      <div className="bg-srapi-border relative h-1.5 overflow-hidden rounded-full">
        <div
          className="bg-srapi-success h-full rounded-full transition-all"
          style={{ width: `${Math.max(ratio * 100, window.requests > 0 ? 2 : 0)}%` }}
        />
      </div>
      <div className="text-2xs text-srapi-text-tertiary flex items-center justify-between gap-3 font-mono">
        <span>
          {t("adminAccounts.usageTokens")} {formatCompactNumber(window.total_tokens)}
        </span>
        <span>{formatMoney(window.cost, window.currency)}</span>
        <span className={cn(hasErrors && "text-srapi-error")}>
          {t("adminAccounts.usageErrors")} {window.error_count}
        </span>
      </div>
      {window.last_request_at ? (
        <div className="text-2xs text-srapi-text-tertiary truncate font-mono">
          {t("adminAccounts.lastUsedAt")} {formatDateTime(window.last_request_at)}
        </div>
      ) : null}
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
function UsageDailyBody({ points }: { points: AccountUsageDailyPoint[] }) {
  const { t } = useLanguage();
  const activePoints = activeDailyUsagePoints(points);
  if (activePoints.length === 0) {
    return <p className="text-2xs text-srapi-text-tertiary">{t("adminAccounts.detailNoData")}</p>;
  }
  // The series arrives oldest→newest from the read model; render most-recent
  // first in the table but keep chronological order for the sparkline.
  const spark = activePoints.map((p) => p.requests);
  const rows = [...activePoints].reverse().slice(0, DAILY_USAGE_VISIBLE_ROWS);
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
                <TableCell className="text-srapi-text-secondary py-1.5 font-mono">
                  {formatDate(p.date)}
                </TableCell>
                <TableCell className="tabular text-srapi-text-primary py-1.5 text-right font-mono">
                  {formatCompactNumber(p.requests)}
                </TableCell>
                <TableCell className="tabular text-srapi-text-secondary py-1.5 text-right font-mono">
                  {formatCompactNumber(p.input_tokens + p.output_tokens)}
                </TableCell>
                <TableCell className="tabular text-srapi-text-secondary py-1.5 text-right font-mono">
                  {formatMoney(p.cost, p.currency)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableScroll>
      {activePoints.length > rows.length ? (
        <p className="text-2xs text-srapi-text-tertiary font-mono">
          {t("adminAccounts.usageDailyRowsShown", {
            shown: rows.length,
            total: activePoints.length,
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
    <div className="border-srapi-border border-t pt-4 first:border-t-0 first:pt-0">
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="text-2xs text-srapi-text-secondary font-mono tracking-widest uppercase">
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
  const usageDaily = useAccountUsageDaily(id, ACCOUNT_USAGE_DAILY_MAX_DAYS);
  const fetchQuota = useFetchAccountQuota();
  const identity = account ? accountIdentitySummary(t, account) : null;
  const endpointFacts = account ? accountEndpointCapabilityFacts(t, account) : [];
  const activeUsagePoints = activeDailyUsagePoints(usageDaily.data ?? []);
  const latestRequestAt = latestUsageWindowRequestAt(usageWindows.data);
  const lastUsageDate = latestRequestAt || [...activeUsagePoints].reverse()[0]?.date || "";

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
        {account ? (
          <SheetDescription
            className="text-srapi-text-secondary block max-w-full truncate text-sm"
            title={account.name}
          >
            {account.name}
          </SheetDescription>
        ) : null}

        {account ? (
          <div className="mt-3 flex flex-wrap gap-1.5">
            <span className="bg-srapi-bg-muted text-2xs text-srapi-text-secondary rounded-md px-2 py-0.5 font-mono">
              {account.status}
            </span>
            <span className="bg-srapi-bg-muted text-2xs text-srapi-text-secondary rounded-md px-2 py-0.5">
              {runtimeClassLabel(t, account.runtime_class)}
            </span>
            {account.priority != null && account.priority !== 0 ? (
              <span className="bg-srapi-bg-muted text-2xs text-srapi-text-tertiary rounded-md px-2 py-0.5 font-mono">
                P{account.priority}
              </span>
            ) : null}
            {account.weight != null && account.weight !== 1 ? (
              <span className="bg-srapi-bg-muted text-2xs text-srapi-text-tertiary rounded-md px-2 py-0.5 font-mono">
                W{account.weight}
              </span>
            ) : null}
          </div>
        ) : null}

        {account ? (
          <div className="mt-3 grid grid-cols-2 gap-2">
            {identity ? (
              <DetailMetric
                label={t("adminAccounts.identity")}
                value={
                  <span className="inline-block max-w-full truncate" title={identity.primary}>
                    {identity.primary}
                  </span>
                }
              />
            ) : null}
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
              value={
                account.proxy_id ? t("adminAccounts.proxyConfigured") : t("adminAccounts.noProxy")
              }
            />
            <DetailMetric
              label={t("adminAccounts.routing")}
              value={`P${account.priority ?? 0} / W${account.weight ?? 1}`}
            />
            <DetailMetric
              label={t("adminAccounts.lastUsedAt")}
              value={
                usageWindows.isLoading || usageDaily.isLoading
                  ? t("adminAccounts.detailLoading")
                  : latestRequestAt
                    ? formatDateTime(latestRequestAt)
                    : lastUsageDate
                      ? formatDate(lastUsageDate)
                      : t("adminAccounts.neverUsed")
              }
            />
            <DetailMetric
              label={t("adminAccounts.activeUsageRange")}
              value={
                usageWindows.isLoading || usageDaily.isLoading
                  ? t("adminAccounts.detailLoading")
                  : activeUsagePoints.length > 0
                    ? activeUsageDateSummary(usageDaily.data ?? [], t)
                    : usageWindowActiveRangeLabel(usageWindows.data, t)
              }
            />
            <DetailMetric
              label={t("adminAccounts.activeUsageDaysLabel")}
              value={
                usageDaily.isLoading
                  ? t("adminAccounts.detailLoading")
                  : activeUsageDayCountLabel(usageDaily.data ?? [], t)
              }
            />
            <DetailMetric
              label={t("adminAccounts.createdAt")}
              value={formatDateTime(account.created_at)}
            />
            <DetailMetric
              label={t("adminAccounts.endpointOverrides")}
              value={
                endpointFacts.length > 0
                  ? endpointFacts.map((fact) => `${fact.label}: ${fact.value}`).join(" ")
                  : t("adminAccounts.inheritProvider")
              }
            />
            {account.last_refreshed_at ? (
              <DetailMetric
                label={t("adminAccounts.lastRefreshedAt")}
                value={formatDateTime(account.last_refreshed_at)}
              />
            ) : null}
            {account.token_expires_at ? (
              <DetailMetric
                label={t("adminAccounts.tokenExpiresAt")}
                value={formatDateTime(account.token_expires_at)}
              />
            ) : null}
            {accountCapacityFacts(t, account).map((fact) => (
              <DetailMetric key={fact.key} label={fact.label} value={fact.value} />
            ))}
            {accountProfileFacts(t, account)
              .filter((fact) => !["max-concurrency", "max-sessions", "rpm"].includes(fact.key))
              .slice(0, 4)
              .map((fact) => (
                <DetailMetric key={fact.key} label={fact.label} value={fact.value} />
              ))}
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
                  <Row
                    label={`${t("adminAccounts.latency")} p50 / p95`}
                    value={`${Math.round(h.latency_p50_ms)} / ${Math.round(h.latency_p95_ms)}ms`}
                  />
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
                  {h.cooldown_until ? (
                    <Row
                      label={t("adminAccounts.cooldown")}
                      value={h.cooldown_reason ?? h.cooldown_until}
                    />
                  ) : null}
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
            {(points) => <UsageDailyBody points={points} />}
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
                  className="text-2xs text-srapi-text-tertiary hover:text-srapi-text-secondary flex items-center gap-1 font-mono tracking-wide uppercase transition-colors disabled:opacity-50"
                >
                  <RefreshCw className={cn("size-3", fetchQuota.isPending && "animate-spin")} />
                  {t("adminAccounts.quotaRefresh")}
                </button>
              ) : null
            }
          >
            {(q) =>
              q.data.length === 0 ? (
                <p className="text-2xs text-srapi-text-tertiary">
                  {t("adminAccounts.detailNoData")}
                </p>
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
                <Row
                  label={t("adminAccounts.proxy")}
                  value={p.proxy_id ?? t("adminAccounts.noProxy")}
                />
                <Row label={t("adminAccounts.successRate")} value={pct(p.success_rate)} />
                <Row
                  label={`${t("adminAccounts.latency")} p95`}
                  value={`${Math.round(p.latency_p95_ms)}ms`}
                />
                <Row label="samples" value={p.sample_count} />
              </div>
            )}
          </Section>
        </div>
      </SheetContent>
    </Sheet>
  );
}
