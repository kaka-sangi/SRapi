"use client";

import { useCallback, useMemo, useState } from "react";
import { Receipt } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DensityToggle, type DensityValue } from "@/components/ui/density-toggle";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
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

// Parse the decimal-as-string `amount` (and `balance_after`) sent by the API
// into a number for sign comparisons. Falls back to 0 on a non-finite parse so
// rendering never crashes on a malformed row.
function ledgerAmountNumber(raw: string | number | null | undefined): number {
  if (raw === null || raw === undefined || raw === "") return 0;
  const v = typeof raw === "number" ? raw : Number(raw);
  return Number.isFinite(v) ? v : 0;
}

// Severity for the row stripe: positive credits (money in) → success, debits
// (money out via usage_charge) → warning, neutral adjustments → info. Refunds
// are flagged warning since they signal a chargeback-class event.
function ledgerSeverity(type: LedgerType, amount: string | number | null | undefined): "info" | "success" | "warning" {
  if (type === "usage_charge") return "warning";
  if (type === "refund") return "warning";
  if (type === "payment_credit" || type === "redeem_code_credit" || type === "compensation") {
    return "success";
  }
  if (ledgerAmountNumber(amount) < 0) return "warning";
  return "info";
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
  const [density, setDensity] = useState<DensityValue>("regular");
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
      if (filters.severity && ledgerSeverity(row.type, row.amount) !== filters.severity) return false;
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
  const severityFilter = list.filters.severity;
  const isFiltered = Boolean(
    list.search ||
      list.filters.type ||
      list.filters.reference_type ||
      list.filters.window ||
      severityFilter,
  );

  const columns: Column<BillingLedgerEntry>[] = [
    {
      key: "time",
      header: t("adminBillingLedger.time"),
      pinned: true,
      render: (r) => (
        <span className="whitespace-nowrap text-[12px] tabular text-srapi-text-tertiary">
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
      // Wrapped in DataTooltip so hovering the amount reveals the directional
      // breakdown (sign, type, currency, balance impact) without claiming a
      // dedicated column.
      render: (r) => {
        const numericAmount = ledgerAmountNumber(r.amount);
        const sign = numericAmount >= 0 ? "+" : "−";
        const toneClass =
          numericAmount >= 0 ? "metric-strong-good" : "metric-strong-warn";
        return (
          <DataTooltip
            title={t(`adminBillingLedger.types.${r.type}`)}
            primary={`${sign}${formatMoney(Math.abs(numericAmount), r.currency)}`}
            rows={[
              { label: t("common.direction"), value: numericAmount >= 0 ? t("common.credit") : t("common.debit"), tone: numericAmount >= 0 ? "success" : "warning" },
              { label: t("adminCommon.currency"), value: r.currency || "—" },
              { label: t("adminBillingLedger.balanceAfter"), value: formatMoney(r.balance_after, r.currency) },
              { label: t("adminBillingLedger.type"), value: r.type },
            ]}
            footer={r.reference_type ? `${r.reference_type}${r.reference_id ? ` #${r.reference_id}` : ""}` : undefined}
          >
            <span className={`tabular font-medium ${toneClass}`}>
              {formatMoney(r.amount, r.currency)}
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "balance",
      header: t("adminBillingLedger.balanceAfter"),
      align: "right",
      hideOnMobile: true,
      render: (r) => {
        const numericAmount = ledgerAmountNumber(r.amount);
        return (
          <DataTooltip
            title={t("adminBillingLedger.balanceAfter")}
            primary={formatMoney(r.balance_after, r.currency)}
            rows={[
              { label: t("adminBillingLedger.amount"), value: formatMoney(r.amount, r.currency), tone: numericAmount >= 0 ? "success" : "warning" },
              { label: "Δ", value: `${numericAmount >= 0 ? "+" : ""}${formatMoney(r.amount, r.currency)}` },
              { label: t("adminCommon.currency"), value: r.currency || "—" },
            ]}
          >
            <span className="text-[12px] tabular text-srapi-text-tertiary metric-tertiary">
              {formatMoney(r.balance_after, r.currency)}
            </span>
          </DataTooltip>
        );
      },
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
              <span className="ml-1 text-[11px] text-srapi-text-tertiary">#{r.reference_id}</span>
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
            <DensityToggle value={density} onChange={setDensity} />
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
        density={density}
        enableKeyboardNav
        // Credits → success, charges/refunds → warning, neutral adjustments →
        // info. The stripe lets ops eyeball «what moved in/out» without
        // reading every amount.
        rowSeverity={(r) => ledgerSeverity(r.type, r.amount)}
        // Inline ledger entry detail: user/type/amount on the left, reference on
        // the right. Keeps cognitive context inside the table.
        expandRow={(r) => (
          <InlineDetailGrid
            sections={[
              {
                title: t("adminBillingLedger.user"),
                rows: [
                  { label: t("adminBillingLedger.user"), value: userLookup.get(r.user_id) || `#${r.user_id ?? "—"}` },
                  { label: t("adminBillingLedger.time"), value: formatDateTime(r.created_at), mono: true },
                ],
              },
              {
                title: t("adminBillingLedger.amount"),
                rows: [
                  { label: t("adminBillingLedger.type"), value: t(`adminBillingLedger.types.${r.type}`) },
                  { label: t("adminBillingLedger.amount"), value: formatMoney(r.amount, r.currency), mono: true, tone: ledgerAmountNumber(r.amount) >= 0 ? "success" : "warning" },
                  { label: t("adminBillingLedger.balanceAfter"), value: formatMoney(r.balance_after, r.currency), mono: true },
                  { label: t("adminCommon.currency"), value: r.currency || "—" },
                ],
              },
              {
                title: t("adminBillingLedger.reference"),
                rows: r.reference_type
                  ? [
                      { label: t("adminBillingLedger.reference"), value: r.reference_type, mono: true },
                      { label: "ID", value: r.reference_id ? `#${r.reference_id}` : "—", mono: true, tone: "muted" },
                    ]
                  : [{ label: t("adminBillingLedger.reference"), value: "—", tone: "muted" }],
              },
            ]}
          />
        )}
        toolbar={
          <>
            {/* Severity chip strip — one-glance pivot for credit vs debit feeds. */}
            <div className="flex items-center gap-3 border-b border-srapi-border/60 bg-srapi-card-muted/40 px-4 py-2">
              <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("common.severity")}
              </span>
              <SegmentedControl
                value={severityFilter === "success" ? "success" : severityFilter === "warning" ? "warning" : severityFilter === "info" ? "info" : "all"}
                onChange={(v) => list.setFilter("severity", v === "all" ? undefined : v)}
                options={[
                  { value: "all", label: t("common.all") },
                  { value: "success", label: t("adminBillingLedger.credits") },
                  { value: "warning", label: t("adminBillingLedger.charges") },
                  { value: "info", label: t("adminBillingLedger.adjustments") },
                ]}
                size="sm"
                ariaLabel="ledger severity filter"
              />
            </div>
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
          </>
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
