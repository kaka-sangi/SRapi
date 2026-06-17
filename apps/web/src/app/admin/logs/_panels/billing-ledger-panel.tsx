"use client";

import { useCallback, useMemo } from "react";
import { Receipt } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useClientPagedList } from "@/hooks/use-client-list";
import { useBillingLedger } from "@/hooks/admin-queries";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatMoney } from "@/lib/admin-format";
import type { BillingLedgerEntry } from "@/lib/sdk-types";
import {
  LOG_WINDOW_PRESETS,
  LOG_WINDOW_ALL_LABEL_KEY,
  logWindowSince,
} from "@/lib/log-window-filter";

type LedgerType = BillingLedgerEntry["type"];

const LEDGER_TYPES: LedgerType[] = [
  "usage_charge",
  "payment_credit",
  "refund",
  "adjustment",
  "compensation",
  "affiliate_transfer",
  "redeem_code_credit",
];

function ledgerTone(type: LedgerType): QuietStatus {
  switch (type) {
    case "payment_credit":
    case "redeem_code_credit":
    case "compensation":
      return "active";
    case "usage_charge":
      return "limited";
    default:
      return "disabled";
  }
}

const ledgerCompare = (a: BillingLedgerEntry, b: BillingLedgerEntry) =>
  (b.created_at ?? "").localeCompare(a.created_at ?? "");

function distinct(values: Array<string | null | undefined>): string[] {
  return [...new Set(values.filter((v): v is string => Boolean(v)))].sort();
}

export function BillingLedgerPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-billing-ledger", ["reference"]);
  const all = useBillingLedger();
  const userLookup = useUserEmailLookup();
  // Closure variant of ledgerMatch so the search term also hits the resolved
  // email — the same upgrade iter-78 applied to /admin/orders.
  const match = useCallback(
    (
      row: BillingLedgerEntry,
      term: string,
      filters: Record<string, string>,
    ): boolean => {
      if (filters.type && row.type !== filters.type) return false;
      if (filters.reference_type && row.reference_type !== filters.reference_type) return false;
      if (filters.window) {
        const since = logWindowSince(filters.window);
        if (since && row.created_at && new Date(row.created_at) < since) return false;
      }
      if (!term) return true;
      const email = userLookup.map.get(String(row.user_id)) ?? "";
      return [String(row.user_id), email, row.reference_id, row.reference_type, row.type]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(term);
    },
    [userLookup.map],
  );
  const { query, total } = useClientPagedList(all, list, {
    match,
    compare: ledgerCompare,
  });

  const rows = useMemo(() => all.data?.data ?? [], [all.data]);
  const referenceOptions = useMemo(() => distinct(rows.map((r) => r.reference_type)), [rows]);
  const isFiltered = Boolean(
    list.search || list.filters.type || list.filters.reference_type || list.filters.window,
  );

  const columns: Column<BillingLedgerEntry>[] = [
    {
      key: "time",
      header: t("adminBillingLedger.time"),
      pinned: true,
      render: (r) => (
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(r.created_at)}
        </span>
      ),
    },
    {
      key: "user",
      header: t("adminBillingLedger.user"),
      hideOnMobile: true,
      render: (r) => (
        <span className="text-srapi-text-secondary">{userLookup.get(r.user_id)}</span>
      ),
    },
    {
      key: "type",
      header: t("adminBillingLedger.type"),
      render: (r) => (
        <QuietBadge status={ledgerTone(r.type)} label={t(`adminBillingLedger.types.${r.type}`)} />
      ),
    },
    {
      key: "amount",
      header: t("adminBillingLedger.amount"),
      align: "right",
      render: (r) => (
        <span className="font-mono text-srapi-text-primary tabular">
          {formatMoney(r.amount, r.currency)}
        </span>
      ),
    },
    {
      key: "balance",
      header: t("adminBillingLedger.balanceAfter"),
      align: "right",
      hideOnMobile: true,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatMoney(r.balance_after, r.currency)}
        </span>
      ),
    },
    {
      key: "reference",
      header: t("adminBillingLedger.reference"),
      hideOnMobile: true,
      render: (r) =>
        r.reference_type ? (
          <span className="text-srapi-text-secondary">
            {r.reference_type}
            {r.reference_id ? (
              <span className="ml-1 font-mono text-2xs text-srapi-text-tertiary">#{r.reference_id}</span>
            ) : null}
          </span>
        ) : (
          <span className="text-srapi-text-tertiary">—</span>
        ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminBillingLedger.title")}
        description={t("adminBillingLedger.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(r) => r.id}
        emptyIcon={Receipt}
        emptyTitle={t("adminBillingLedger.emptyTitle")}
        emptyBody={t("adminBillingLedger.emptyBody")}
        minWidth={760}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminBillingLedger.searchPlaceholder")}
            />
            <FilterSelect
              value={list.filters.type}
              onChange={(v) => list.setFilter("type", v)}
              options={LEDGER_TYPES.map((v) => ({ value: v, label: t(`adminBillingLedger.types.${v}`) }))}
              allLabel={t("adminBillingLedger.allTypes")}
            />
            <FilterSelect
              value={list.filters.reference_type}
              onChange={(v) => list.setFilter("reference_type", v)}
              options={referenceOptions.map((v) => ({ value: v, label: v }))}
              allLabel={t("adminBillingLedger.allReferences")}
            />
            <FilterSelect
              value={list.filters.window}
              onChange={(v) => list.setFilter("window", v)}
              options={LOG_WINDOW_PRESETS.map((p) => ({ value: p.value, label: t(p.labelKey) }))}
              allLabel={t(LOG_WINDOW_ALL_LABEL_KEY)}
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
