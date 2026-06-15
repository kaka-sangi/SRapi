"use client";

import { useMemo, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FilterSelect } from "@/components/admin/list-toolbar";
import { Pagination } from "@/components/ui/pagination";
import { QuietBadge } from "@/components/ui/quiet-badge";
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
  const visibleEntries = useMemo(
    () => (typeFilter ? entries.filter((e) => e.type === typeFilter) : entries),
    [entries, typeFilter],
  );
  const totalRecharged = useMemo(() => sumRecharged(entries), [entries]);
  const total = query.data?.pagination?.total ?? entries.length;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t("adminUsers.balanceHistory")}
            <span className="font-mono text-srapi-text-tertiary"> · {email}</span>
          </DialogTitle>
        </DialogHeader>

        {/* Pinned user header */}
        <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-4">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-srapi-text-primary">{email}</p>
              <p className="mt-0.5 font-mono text-2xs text-srapi-text-tertiary">#{userId}</p>
            </div>
            <div className="flex-shrink-0 text-right">
              <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("billing.currentBalance")}
              </p>
              <p className="mt-0.5 font-serif text-2xl leading-none text-srapi-text-primary tabular">
                {balance != null ? formatMoney(balance, currency) : "—"}
              </p>
            </div>
          </div>
          <div className="mt-3 flex items-center justify-between gap-4 border-t border-srapi-border/60 pt-3">
            <p
              className="min-w-0 flex-1 truncate text-2xs text-srapi-text-tertiary"
              title={notes ?? ""}
            >
              {notes ? (
                <>
                  {t("adminUsers.note")}: {notes}
                </>
              ) : (
                <>&nbsp;</>
              )}
            </p>
            <p className="ml-4 flex-shrink-0 text-2xs text-srapi-text-tertiary">
              Total recharged:{" "}
              <span className="font-mono font-semibold text-srapi-success tabular">
                {formatMoney(totalRecharged, currency)}
              </span>
            </p>
          </div>
        </div>

        {/* Type filter */}
        <div className="flex items-center gap-3">
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
            emptyTitle={t("adminBillingLedger.emptyTitle")}
          >
            {() => (
              <div className="space-y-2">
                {visibleEntries.map((entry) => (
                  <BalanceHistoryRow key={entry.id} entry={entry} />
                ))}
              </div>
            )}
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

  return (
    <div className="flex items-start justify-between gap-3 rounded-lg border border-srapi-border/70 p-3">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <QuietBadge
            status={positive ? "active" : "error"}
            label={t(`adminBillingLedger.types.${entry.type}`)}
          />
        </div>
        {entry.reference_type ? (
          <p className="mt-1 truncate font-mono text-2xs text-srapi-text-tertiary">
            {entry.reference_type}
            {entry.reference_id ? ` · ${entry.reference_id}` : ""}
          </p>
        ) : null}
        <p className="mt-0.5 font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(entry.created_at)}
        </p>
      </div>
      <div className="flex-shrink-0 text-right">
        <p
          className={
            "font-mono text-sm font-semibold tabular " +
            (positive ? "text-srapi-success" : "text-srapi-error")
          }
        >
          {positive ? "+" : ""}
          {formatMoney(entry.amount, entry.currency)}
        </p>
        <p className="mt-0.5 font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatMoney(entry.balance_before, entry.currency)}
          {" → "}
          {formatMoney(entry.balance_after, entry.currency)}
        </p>
      </div>
    </div>
  );
}
