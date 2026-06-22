"use client";

import { useMemo, useState } from "react";
import { Wallet, TrendingUp, TrendingDown, StickyNote } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FilterSelect } from "@/components/admin/list-toolbar";
import { Pagination } from "@/components/ui/pagination";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { DataPill } from "@/components/ui/data-pill";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { IconBubble } from "@/components/ui/icon-bubble";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import {
  useUserBalanceHistory,
  sumRecharged,
  BALANCE_HISTORY_TYPES,
} from "@/hooks/use-user-balance-history";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import type { BillingLedgerEntry } from "@/lib/sdk-types";

const PAGE_SIZE = 15;

type DirectionFilter = "all" | "credit" | "debit";

export interface UserBalanceHistoryDialogProps {
  userId: string | number;
  email: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  /** Current balance string ("12.3400"), when the caller has the loaded user. */
  balance?: string;
  /** Currency code for the balance/amount formatting. Defaults to USD. */
  currency?: string;
  /** Free-form admin notes about the user, when available. */
  notes?: string | null;
}

/**
 * Admin user balance-history (billing-ledger) dialog.
 *
 * Shared between the admin usage page (clickable email cell) and the users
 * page. Layout follows sub2api's `UserBalanceHistoryModal`: a pinned user
 * header (email, current balance, total recharged, notes) over a paginated,
 * type-filterable list of ledger movements.
 *
 * The `total_recharged` value is derived client-side from credit-style entries
 * on the loaded page — the SDK endpoint returns only ledger rows, not a
 * recharge aggregate — so it reflects the currently visible movements.
 */
export function UserBalanceHistoryDialog({
  userId,
  email,
  open,
  onOpenChange,
  balance,
  currency = "USD",
  notes,
}: UserBalanceHistoryDialogProps) {
  const { t } = useLanguage();
  const [page, setPage] = useState(1);
  const [typeFilter, setTypeFilter] = useState<string | undefined>(undefined);
  const [direction, setDirection] = useState<DirectionFilter>("all");

  const query = useUserBalanceHistory(String(userId), page, PAGE_SIZE, open);

  const typeOptions = useMemo(
    () =>
      BALANCE_HISTORY_TYPES.map((type) => ({
        value: type,
        label: t(`adminBillingLedger.types.${type}`),
      })),
    [t],
  );

  const entries = useMemo(() => query.data?.entries ?? [], [query.data]);
  const visibleEntries = useMemo(() => {
    let rows = entries;
    if (typeFilter) rows = rows.filter((e) => e.type === typeFilter);
    if (direction !== "all") {
      rows = rows.filter((e) => {
        const positive = Number(e.amount) >= 0;
        return direction === "credit" ? positive : !positive;
      });
    }
    return rows;
  }, [entries, typeFilter, direction]);
  const totalRecharged = useMemo(() => sumRecharged(entries), [entries]);
  // Sum the visible debits so the operator sees outflow alongside the
  // recharge total. Useful when reconciling a disputed balance.
  const totalDebits = useMemo(
    () =>
      entries.reduce((acc, e) => {
        const amount = Number(e.amount);
        return Number.isFinite(amount) && amount < 0 ? acc + Math.abs(amount) : acc;
      }, 0),
    [entries],
  );
  const total = query.data?.pagination?.total ?? entries.length;

  const balanceNum = balance != null ? Number(balance) : NaN;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2.5 text-lg font-semibold tracking-tight">
            <IconBubble tone="accent" size="md">
              <Wallet />
            </IconBubble>
            <span>{t("adminUsers.balanceHistory")}</span>
            <span className="text-sm font-normal text-srapi-text-tertiary">· {email}</span>
          </DialogTitle>
        </DialogHeader>

        {/* Pinned user header — SectionHero-style summary inside the dialog */}
        <div className="relative overflow-hidden rounded-xl border border-srapi-border bg-srapi-card p-5">
          <div className="dot-grid-overlay pointer-events-none absolute right-0 top-0 h-24 w-32 opacity-50" aria-hidden />
          <div className="relative flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-srapi-text-primary">{email}</p>
              <p className="metric-tertiary mt-0.5 text-[12px]">#{userId}</p>
            </div>
            <div className="flex-shrink-0 text-right">
              <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("billing.currentBalance")}
              </p>
              <DataTooltip
                title={t("billing.currentBalance")}
                primary={
                  <span className="tabular">
                    {balance != null ? formatMoney(balance, currency) : "—"}
                  </span>
                }
                rows={[
                  {
                    label: "credits",
                    value: formatMoney(totalRecharged, currency),
                    tone: "success",
                  },
                  {
                    label: "debits",
                    value: formatMoney(totalDebits, currency),
                    tone: totalDebits > 0 ? "error" : "muted",
                  },
                  {
                    label: t("adminCommon.currency"),
                    value: currency,
                    tone: "muted",
                  },
                ]}
              >
                <p
                  className={
                    "metric-primary mt-1 cursor-help text-3xl leading-none tracking-tight " +
                    (Number.isFinite(balanceNum) && balanceNum < 0
                      ? "metric-strong-bad"
                      : "")
                  }
                >
                  {balance != null ? formatMoney(balance, currency) : "—"}
                </p>
              </DataTooltip>
            </div>
          </div>
          <div className="relative mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-srapi-border/60 pt-3">
            <p
              className="metric-tertiary inline-flex min-w-0 flex-1 items-center gap-1.5 truncate text-[12px]"
              title={notes ?? ""}
            >
              {notes ? (
                <>
                  <StickyNote className="size-3 shrink-0" aria-hidden />
                  <span className="truncate">
                    {t("adminUsers.note")}: {notes}
                  </span>
                </>
              ) : (
                <span>&nbsp;</span>
              )}
            </p>
            <div className="flex shrink-0 items-center gap-2">
              <DataTooltip
                title="Total recharged"
                primary={
                  <span className="tabular">
                    {formatMoney(totalRecharged, currency)}
                  </span>
                }
                rows={[
                  {
                    label: "visible page",
                    value: String(entries.length),
                    tone: "muted",
                  },
                ]}
                footer={
                  entries.length < total
                    ? `${entries.length} of ${total} entries on this page`
                    : undefined
                }
              >
                <DataPill tone="success" size="sm" className="cursor-help">
                  <TrendingUp className="size-3" />
                  {formatMoney(totalRecharged, currency)}
                </DataPill>
              </DataTooltip>
              {totalDebits > 0 ? (
                <DataTooltip
                  title="Total debits"
                  primary={
                    <span className="tabular">
                      {formatMoney(totalDebits, currency)}
                    </span>
                  }
                >
                  <DataPill tone="error" size="sm" className="cursor-help">
                    <TrendingDown className="size-3" />
                    {formatMoney(totalDebits, currency)}
                  </DataPill>
                </DataTooltip>
              ) : null}
            </div>
          </div>
        </div>

        {/* Filters: direction segmented control + type dropdown */}
        <div className="flex flex-wrap items-center gap-3">
          <SegmentedControl<DirectionFilter>
            value={direction}
            onChange={setDirection}
            ariaLabel={t("adminBillingLedger.allTypes")}
            options={[
              { value: "all", label: t("adminBillingLedger.allTypes") },
              {
                value: "credit",
                label: "+",
                icon: <TrendingUp />,
              },
              {
                value: "debit",
                label: "−",
                icon: <TrendingDown />,
              },
            ]}
          />
          <FilterSelect
            value={typeFilter}
            onChange={setTypeFilter}
            options={typeOptions}
            allLabel={t("adminBillingLedger.allTypes")}
          />
        </div>

        {/* History list */}
        <div className="max-h-[28rem] overflow-y-auto">
          <PageQueryState
            query={query}
            skeleton={<DialogListSkeleton rows={5} />}
            isEmpty={() => visibleEntries.length === 0}
          >
            {() =>
              visibleEntries.length === 0 ? (
                <IllustratedEmptyState
                  illust="inbox"
                  title={t("adminBillingLedger.emptyTitle")}
                />
              ) : (
                <div className="space-y-2">
                  {visibleEntries.map((entry) => (
                    <BalanceHistoryRow key={entry.id} entry={entry} />
                  ))}
                </div>
              )
            }
          </PageQueryState>
        </div>

        {total > PAGE_SIZE ? (
          <Pagination
            page={page}
            pageSize={PAGE_SIZE}
            total={total}
            onPageChange={setPage}
            labelFor={(from, to, count) =>
              t("common.pageLabel", { from, to, total: count })
            }
            labelPrev={t("common.previousPage")}
            labelNext={t("common.nextPage")}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

/** Single ledger movement: type + timestamp on the left, signed amount + running balance on the right. */
function BalanceHistoryRow({ entry }: { entry: BillingLedgerEntry }) {
  const { t } = useLanguage();
  const amount = Number(entry.amount);
  const positive = Number.isFinite(amount) && amount >= 0;
  const severityStripe = positive ? "success" : "error";

  return (
    <div
      className="log-row flex items-start justify-between gap-3 rounded-xl border border-srapi-border/70 bg-srapi-card p-3 transition-colors hover:bg-srapi-card-muted/50"
      data-sev={severityStripe}
    >
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <QuietBadge
            status={positive ? "active" : "error"}
            label={t(`adminBillingLedger.types.${entry.type}`)}
          />
        </div>
        {entry.reference_type ? (
          <p className="metric-tertiary mt-1 truncate text-[12px]">
            {entry.reference_type}
            {entry.reference_id ? ` · ${entry.reference_id}` : ""}
          </p>
        ) : null}
        <p className="metric-tertiary mt-0.5 text-[12px]">
          {formatDateTime(entry.created_at)}
        </p>
      </div>
      <div className="flex-shrink-0 text-right">
        <DataTooltip
          title={t(`adminBillingLedger.types.${entry.type}`)}
          primary={
            <span className="tabular">
              {positive ? "+" : ""}
              {formatMoney(entry.amount, entry.currency)}
            </span>
          }
          rows={[
            {
              label: "before",
              value: formatMoney(entry.balance_before, entry.currency),
              tone: "muted",
            },
            {
              label: "after",
              value: formatMoney(entry.balance_after, entry.currency),
            },
            {
              label: "delta",
              value: `${positive ? "+" : ""}${formatMoney(entry.amount, entry.currency)}`,
              tone: positive ? "success" : "error",
            },
          ]}
          footer={entry.reference_type ?? undefined}
        >
          <p
            className={
              "cursor-help text-sm font-semibold tabular " +
              (positive ? "metric-strong-good" : "metric-strong-bad")
            }
          >
            {positive ? "+" : ""}
            {formatMoney(entry.amount, entry.currency)}
          </p>
        </DataTooltip>
        <p className="metric-tertiary mt-0.5 text-[12px]">
          {formatMoney(entry.balance_before, entry.currency)}
          {" → "}
          {formatMoney(entry.balance_after, entry.currency)}
        </p>
      </div>
    </div>
  );
}
