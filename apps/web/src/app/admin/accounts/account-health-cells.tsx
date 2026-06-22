import Link from "next/link";
import { CheckSquare, ExternalLink, FileSearch, RefreshCw, RotateCcw, ShieldX } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import type { AccountHealthSnapshot } from "@/lib/sdk-types";
import { cn } from "@/lib/cn";
import { DataPill } from "@/components/ui/data-pill";
import { accountHealthNeedsInvestigation } from "@/lib/admin-account-health-investigation";
import {
  accountHealthGroupMaintenanceActions,
  buildAccountHealthOpsSummary,
  type AccountHealthMaintenanceAction,
  type AccountHealthOpsGroup,
} from "@/lib/admin-account-health-ops";
import {
  latestQuotaWindows,
  quotaWindowDisplayLabel,
  quotaWindowTiming,
} from "@/lib/quota-display";

export function HealthSummaryStrip({
  healthById,
  onSelectAccounts,
  onRunGroupAction,
  actionPending = false,
}: {
  healthById: Map<string, AccountHealthSnapshot>;
  onSelectAccounts?: (ids: string[]) => void;
  onRunGroupAction?: (group: AccountHealthOpsGroup, action: AccountHealthMaintenanceAction) => void;
  actionPending?: boolean;
}) {
  const { t } = useLanguage();
  const entries = [...healthById.values()];
  if (entries.length === 0) return null;
  const summary = buildAccountHealthOpsSummary(entries);
  return (
    <div className="mb-4 space-y-2">
      <div className="flex items-center gap-4 text-xs text-srapi-text-tertiary">
        <span className="tabular">
          <span className="font-medium text-srapi-text-secondary">{summary.healthy}</span>{" "}
          {t("dashboard.healthyAccounts")}
        </span>
        {summary.attention > 0 ? (
          <span className="font-medium text-srapi-warning tabular">
            {summary.attention} {t("adminAccounts.healthNeedsAttention")}
          </span>
        ) : null}
        <span className="ml-auto tabular">
          {t("dashboard.total")} {summary.total}
        </span>
      </div>
      {summary.groups.length > 0 ? (
        <div className="grid gap-2 lg:grid-cols-2">
          {summary.groups.map((group) => (
            <HealthOpsGroupCard
              key={group.key}
              group={group}
              onSelectAccounts={onSelectAccounts}
              onRunGroupAction={onRunGroupAction}
              actionPending={actionPending}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}

function HealthOpsGroupCard({
  group,
  onSelectAccounts,
  onRunGroupAction,
  actionPending,
}: {
  group: AccountHealthOpsGroup;
  onSelectAccounts?: (ids: string[]) => void;
  onRunGroupAction?: (group: AccountHealthOpsGroup, action: AccountHealthMaintenanceAction) => void;
  actionPending: boolean;
}) {
  const { t } = useLanguage();
  const actions = onRunGroupAction ? accountHealthGroupMaintenanceActions(group) : [];
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2 rounded-xl border border-srapi-border bg-srapi-card-muted px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-sm font-semibold tracking-tight text-srapi-text-primary">
            {t(`adminAccounts.healthIssue.${group.key}`)}
          </span>
          <DataPill tone="neutral" size="sm">
            {t("adminAccounts.healthGroupCount", { count: group.count })}
          </DataPill>
        </div>
        {group.errorClass ? (
          <div
            className="mt-1 truncate text-[11px] text-srapi-error"
            title={group.errorClass}
          >
            {group.errorClass}
          </div>
        ) : null}
      </div>

      <div className="flex flex-wrap items-center gap-1.5">
        {group.investigationHref ? (
          <Link
            href={group.investigationHref}
            className="inline-flex h-7 items-center gap-1 rounded-full border border-srapi-border bg-srapi-card px-2.5 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary"
          >
            <ExternalLink className="size-3" aria-hidden />
            {t("adminAccounts.investigateErrors")}
          </Link>
        ) : null}
        {group.requestEvidenceHref ? (
          <Link
            href={group.requestEvidenceHref}
            className="inline-flex h-7 items-center gap-1 rounded-full border border-srapi-border bg-srapi-card px-2.5 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary"
          >
            <FileSearch className="size-3" aria-hidden />
            {t("adminAccounts.viewEvidence")}
          </Link>
        ) : null}
        {onSelectAccounts ? (
          <button
            type="button"
            onClick={() => onSelectAccounts(group.accountIds)}
            className="inline-flex h-7 items-center gap-1 rounded-full border border-srapi-border bg-srapi-card px-2.5 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary"
          >
            <CheckSquare className="size-3" aria-hidden />
            {t("adminAccounts.selectHealthGroup", { count: group.count })}
          </button>
        ) : null}
        {actions.map((action) => {
          const Icon = healthActionIcon(action);
          return (
            <button
              key={action}
              type="button"
              onClick={() => onRunGroupAction?.(group, action)}
              disabled={actionPending}
              className="inline-flex h-7 items-center gap-1 rounded-full border border-srapi-border bg-srapi-card px-2.5 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary disabled:cursor-not-allowed disabled:opacity-50"
            >
              <Icon className="size-3" aria-hidden />
              {t(`adminAccounts.healthAction.${action}`)}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function healthActionIcon(action: AccountHealthMaintenanceAction) {
  switch (action) {
    case "recover":
      return RotateCcw;
    case "clear_error":
      return ShieldX;
    case "refresh_quota":
      return RefreshCw;
  }
}

export function AccountHealthCell({
  health,
  investigationHref,
}: {
  health?: AccountHealthSnapshot;
  investigationHref?: string | null;
}) {
  const { t } = useLanguage();
  if (!health) return <span className="text-xs text-srapi-text-tertiary">—</span>;
  const rate = health.success_rate;
  const circuit = health.circuit_state;
  const isOpen = circuit === "open";
  const isHalfOpen = circuit === "half-open";
  const p50 = Math.round(health.latency_p50_ms);
  // Explain the routing state in plain language: an "open" circuit means the
  // account is benched — a common reason requests get 'no available account'.
  const circuitTitle = isOpen
    ? t("adminAccounts.circuitOpen")
    : isHalfOpen
      ? t("adminAccounts.circuitHalfOpen")
      : t("adminAccounts.circuitClosed");
  const content = (
    <>
      <span
        title={circuitTitle}
        className={cn(
          "inline-block size-1.5 shrink-0 rounded-full",
          isOpen
            ? "bg-srapi-error"
            : isHalfOpen
              ? "bg-srapi-warning"
              : rate >= 0.95
                ? "bg-srapi-success"
                : rate >= 0.8
                  ? "bg-srapi-warning"
                  : "bg-srapi-error",
        )}
      />
      <span className="font-medium text-srapi-text-secondary">{Math.round(rate * 100)}%</span>
      {p50 > 0 ? <span className="text-srapi-text-tertiary">{p50}ms</span> : null}
      {health.error_class ? (
        <span
          className="max-w-[5rem] truncate text-srapi-text-tertiary"
          title={health.error_class}
        >
          {health.error_class}
        </span>
      ) : null}
    </>
  );
  const className = "flex min-w-0 items-center gap-1.5 text-xs tabular";
  if (investigationHref && accountHealthNeedsInvestigation(health)) {
    return (
      <Link
        href={investigationHref}
        className={`${className} hover:text-srapi-text-primary rounded-sm underline-offset-2 hover:underline`}
        aria-label={t("adminAccounts.investigateErrors")}
      >
        {content}
      </Link>
    );
  }
  return <div className={className}>{content}</div>;
}

export function AccountQuotaCell({ health }: { health?: AccountHealthSnapshot }) {
  const { t } = useLanguage();
  if (!health) return <span className="text-xs text-srapi-text-tertiary">—</span>;
  const windows = latestQuotaWindows(health.quota_windows ?? []);
  if (windows.length > 0) {
    const title = windows
      .map(
        (window) =>
          `${quotaWindowDisplayLabel(window, t)} ${Math.round(window.remainingPercent)}% · ${quotaWindowTiming(window, t)}`,
      )
      .join("\n");
    return (
      <span className="flex min-w-0 flex-col gap-1" title={title}>
        {windows.map((window) => {
          const ratio = window.remainingPercent / 100;
          const exhausted = window.remainingPercent <= 0;
          const pct = Math.round(window.remainingPercent);
          return (
            <span
              key={window.snapshot.quota_type}
              className="grid grid-cols-[2.5rem_minmax(2rem,1fr)_2.5rem] items-center gap-1.5"
            >
              <span className="truncate text-[10px] font-semibold uppercase tracking-[0.08em] leading-none text-srapi-text-tertiary">
                {quotaWindowDisplayLabel(window, t)}
              </span>
              <span className="relative h-1.5 overflow-hidden rounded-full bg-srapi-border">
                <span
                  className={cn(
                    "absolute inset-y-0 left-0 rounded-full transition-all",
                    exhausted
                      ? "bg-srapi-error"
                      : ratio <= 0.2
                        ? "bg-srapi-warning"
                        : "bg-srapi-success",
                  )}
                  style={{ width: `${Math.max(pct, 2)}%` }}
                />
              </span>
              <span
                className={cn(
                  "text-right text-[11px] tabular leading-none text-srapi-text-tertiary",
                  exhausted
                    ? "text-srapi-error"
                    : window.remainingPercent <= 20
                      ? "text-srapi-warning"
                      : undefined,
                )}
              >
                {pct}%
              </span>
            </span>
          );
        })}
      </span>
    );
  }
  const ratio = health.quota_remaining_ratio;
  const exhausted = health.quota_exhausted;
  const pct = Math.round(ratio * 100);
  return (
    <span className="flex items-center gap-1.5">
      <span className="bg-srapi-border relative h-1.5 w-12 overflow-hidden rounded-full">
        <span
          className={cn(
            "absolute inset-y-0 left-0 rounded-full transition-all",
            exhausted ? "bg-srapi-error" : ratio <= 0.2 ? "bg-srapi-warning" : "bg-srapi-success",
          )}
          style={{ width: `${Math.max(pct, 2)}%` }}
        />
      </span>
      <span
        className={cn(
          "text-[11px] font-medium tabular",
          exhausted
            ? "text-srapi-error"
            : ratio <= 0.2
              ? "text-srapi-warning"
              : "text-srapi-text-secondary",
        )}
      >
        {pct}%
      </span>
    </span>
  );
}
