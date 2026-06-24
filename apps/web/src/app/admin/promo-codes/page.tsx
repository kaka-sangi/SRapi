"use client";

import React, { useState } from "react";
import { Ticket } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { PromoCodeUsagesDialog } from "@/components/admin/promo-code-usages-dialog";
import {
  useAdminPromoCodes,
  useAdminPromoCodeUsages,
  useCreatePromoCode,
  useUpdatePromoCode,
  useDeletePromoCode,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import {
  PROMO_DISCOUNT_TYPES,
  PROMO_CODE_STATUSES,
  emptyPromoCodeForm,
  promoFormFromCode,
  buildPromoCodeBody,
  type PromoCodeFormState,
} from "@/lib/admin-commerce-code-form";
import type { PromoCode } from "@/lib/sdk-types";

export default function AdminPromoCodesPage() {
  return (
    <AdminShell>
      <PromoContent />
    </AdminShell>
  );
}

function PromoContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-promo-codes", []);
  const statusFilter = (list.filters.status as PromoCode["status"]) || undefined;
  const codeSearch = list.search || undefined;
  const promos = useAdminPromoCodes({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
    code: codeSearch,
  });
  const createMut = useCreatePromoCode();
  const updateMut = useUpdatePromoCode();
  const deleteMut = useDeletePromoCode();

  const [formTarget, setFormTarget] = useState<PromoCode | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PromoCode | null>(null);
  const [usagesTarget, setUsagesTarget] = useState<PromoCode | null>(null);
  const isNew = formTarget === "new";

  const fields: FieldConfig<PromoCodeFormState>[] = [
    { name: "code", label: t("adminPromos.code") },
    {
      name: "discountType",
      label: t("adminPromos.discountType"),
      type: "select",
      options: enumOptions(PROMO_DISCOUNT_TYPES, t),
    },
    { name: "discountValue", label: t("adminPromos.value") },
    { name: "currency", label: t("adminCommon.currency") },
    { name: "maxUses", label: t("adminPromos.maxUses"), type: "number" },
    { name: "perUserLimit", label: t("adminPromos.perUserLimit"), type: "number" },
    { name: "minOrderAmount", label: t("adminPromos.minOrderAmount") },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(PROMO_CODE_STATUSES, t),
    },
    { name: "startsAtLocal", label: t("adminCommon.startsAt"), type: "datetime" },
    { name: "expiresAtLocal", label: t("adminCommon.expiresAt"), type: "datetime" },
  ];

  const columns: Column<PromoCode>[] = [
    {
      key: "code",
      header: t("adminPromos.code"),
      pinned: true,
      sortValue: (p) => p.code,
      render: (p) => (
        <span className="text-sm font-semibold tracking-tight tabular text-srapi-text-primary">
          {p.code}
        </span>
      ),
    },
    {
      key: "value",
      header: t("adminPromos.value"),
      render: (p) => {
        const isPercent = p.discount_type === "percent";
        const display = isPercent ? `${p.discount_value}%` : p.discount_value;
        return (
          <DataTooltip
            title={t("adminPromos.value")}
            primary={display}
            rows={
              isPercent
                ? [
                    { label: t("adminPromo.type"), value: t("adminPromo.percent") },
                    { label: t("adminPromo.value"), value: `${p.discount_value}%` },
                    ...(p.min_order_amount
                      ? [{ label: t("adminPromo.minOrder"), value: formatMoney(p.min_order_amount, p.currency || "USD"), tone: "muted" as const }]
                      : []),
                  ]
                : [
                    { label: t("adminPromo.type"), value: t("adminPromo.fixed") },
                    { label: t("adminCommon.currency"), value: (p.currency || "USD").toUpperCase() },
                    { label: t("adminPromo.value"), value: formatMoney(p.discount_value, p.currency || "USD") },
                    ...(p.min_order_amount
                      ? [{ label: t("adminPromo.minOrder"), value: formatMoney(p.min_order_amount, p.currency || "USD"), tone: "muted" as const }]
                      : []),
                  ]
            }
          >
            <span className="text-sm font-medium tabular text-srapi-text-primary">{display}</span>
          </DataTooltip>
        );
      },
    },
    {
      key: "uses",
      header: t("adminPromos.uses"),
      align: "right",
      hideOnMobile: true,
      render: (p) => {
        const used = p.used_count ?? 0;
        const max = p.max_uses ?? 0;
        const remaining = max > 0 ? Math.max(0, max - used) : null;
        const pct = max > 0 ? Math.min(100, Math.round((used / max) * 100)) : null;
        return (
          <DataTooltip
            title={t("adminPromos.uses")}
            primary={
              <span>
                {used}
                {max ? <span className="text-srapi-text-tertiary"> / {max}</span> : null}
              </span>
            }
            rows={[
              { label: t("adminPromo.used"), value: String(used) },
              ...(max > 0
                ? [
                    { label: t("adminPromo.cap"), value: String(max) },
                    { label: t("adminPromo.remaining"), value: String(remaining ?? 0), tone: "muted" as const },
                    { label: t("adminPromo.utilization"), value: `${pct}%` },
                  ]
                : [{ label: t("adminPromo.cap"), value: t("adminPromo.unlimited"), tone: "muted" as const }]),
              ...(p.per_user_limit
                ? [{ label: t("adminPromo.perUser"), value: String(p.per_user_limit), tone: "muted" as const }]
                : []),
            ]}
          >
            <span className="text-sm tabular text-srapi-text-secondary">
              {used}
              {max ? ` / ${max}` : ""}
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "limits",
      header: t("adminPromos.limits"),
      hideOnMobile: true,
      render: (p) => (
        <span className="text-xs tabular text-srapi-text-tertiary">
          {p.per_user_limit ? `${p.per_user_limit}/user` : "—"}
          {p.min_order_amount ? ` · ≥ ${p.min_order_amount}` : ""}
        </span>
      ),
    },
    {
      key: "expires",
      header: t("adminCommon.expiresAt"),
      hideOnMobile: true,
      // Empty string sortValue keeps unset expiries at the bottom on
      // ascending sort, matching the "when does this expire?" mental model.
      sortValue: (p) => p.expires_at ?? "",
      render: (p) => (
        <span className="text-[12px] tabular text-srapi-text-tertiary">
          {p.expires_at ? formatDateTime(p.expires_at) : "—"}
        </span>
      ),
    },
    {
      key: "status",
      header: t("common.active"),
      render: (p) => <QuietBadge status={quietStatusFor(p.status)} label={statusLabel(t, p.status)} />,
    },
  ];

  return (
    <>
      <SectionHero
        eyebrow={t("hero.eyebrowCommercePromo")}
        title={t("adminPromos.promoTitle")}
        description={t("adminPromos.promoSubtitle")}
        metrics={
          promos.data
            ? [
                {
                  label: t("adminPromos.activeCount"),
                  value: String(
                    promos.data.data.filter((p) => p.status === "active").length,
                  ),
                },
              ]
            : undefined
        }
        actions={
          <div className="flex items-center gap-3">
            {promos.data ? (
              <ListCount total={promos.data.pagination?.total ?? promos.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminPromos.createPromo")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={promos}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(p) => p.id}
        emptyIcon={Ticket}
        emptyTitle={t("adminPromos.emptyPromo")}
        emptyBody={t("adminPromos.emptyPromoBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminPromos.createPromo")}
          </Button>
        }
        minWidth={520}
        isFiltered={Boolean(statusFilter || codeSearch)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminPromos.searchPlaceholder")}
            />
            <SegmentedControl<string>
              value={(statusFilter as string) ?? "all"}
              onChange={(v) => list.setFilter("status", v === "all" ? "" : v)}
              ariaLabel={t("adminCommon.allStatuses")}
              size="sm"
              options={[
                { value: "all", label: t("adminCommon.allStatuses") },
                ...enumOptions(PROMO_CODE_STATUSES, t).map((o) => ({
                  value: o.value,
                  label: statusLabel(t, o.value as PromoCode["status"]),
                })),
              ]}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: promos.data?.pagination?.total ?? promos.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        expandRow={(p) => <PromoUsageInlineDetail promo={p} />}
        rowActions={(p) => (
          <RowActionsMenu
            actions={[
              { label: t("adminPromos.usagesAction"), onSelect: () => setUsagesTarget(p) },
              { label: t("common.edit"), onSelect: () => setFormTarget(p) },
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(p) },
            ]}
          />
        )}
      />

      {formTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={isNew ? t("adminPromos.createPromo") : t("adminPromos.editPromo")}
          fields={fields}
          initial={isNew ? emptyPromoCodeForm() : promoFormFromCode(formTarget)}
          buildBody={buildPromoCodeBody}
          submit={
            isNew
              ? (body) => createMut.mutateAsync(body)
              : (body) => updateMut.mutateAsync({ id: formTarget.id, body })
          }
          successMessage={isNew ? t("feedback.created") : t("feedback.updated")}
          isPending={createMut.isPending || updateMut.isPending}
        />
      ) : null}

      {deleteTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("feedback.confirmDeleteTitle", { name: deleteTarget.code })}
          body={t("feedback.confirmDeleteBody")}
          confirmLabel={t("common.delete")}
          confirmPhrase={deleteTarget.code}
          onConfirm={() => deleteMut.mutateAsync(deleteTarget.id)}
          successMessage={t("feedback.deleted")}
          isPending={deleteMut.isPending}
        />
      ) : null}

      <PromoCodeUsagesDialog
        promoId={usagesTarget?.id ?? null}
        code={usagesTarget?.code ?? ""}
        open={usagesTarget !== null}
        onOpenChange={(open) => {
          if (!open) setUsagesTarget(null);
        }}
      />
    </>
  );
}

// Inline detail rendered when a promo row is expanded. Surfaces the top 5
// redeemers, the total uses, and the 5 most recent uses so the operator can
// scan a code's distribution without opening the full usages dialog. The
// query is gated by `enabled` so we only fetch on expansion.
function PromoUsageInlineDetail({ promo }: { promo: PromoCode }) {
  const { t } = useLanguage();
  const usages = useAdminPromoCodeUsages(promo.id, true);
  const list = usages.data?.data ?? [];

  // Rank users by usage count (top 5 redeemers).
  const byUser = new Map<string, number>();
  for (const u of list) {
    const key = String(u.user_id);
    byUser.set(key, (byUser.get(key) ?? 0) + 1);
  }
  const topRedeemers = [...byUser.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 5);

  // Most-recent 5 by applied_at.
  const recent = [...list]
    .sort((a, b) => (b.applied_at ?? "").localeCompare(a.applied_at ?? ""))
    .slice(0, 5);

  const total = list.length;

  return (
    <div className="border-t border-srapi-border/60 bg-srapi-card-muted/30 px-6 py-4">
      {usages.isLoading ? (
        <p className="text-sm text-srapi-text-tertiary">{t("common.loading")}</p>
      ) : total === 0 ? (
        <p className="text-sm text-srapi-text-tertiary">{t("adminPromos.usagesEmpty")}</p>
      ) : (
        <div className="grid gap-x-8 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
          <div>
            <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              {t("adminPromos.uses")}
            </div>
            <div className="metric-primary tabular">{total}</div>
            <div className="mt-1 text-[11px] text-srapi-text-tertiary">
              {promo.max_uses ? `cap ${promo.max_uses}` : "unlimited cap"}
            </div>
          </div>
          <div>
            <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              Top redeemers
            </div>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5">
              {topRedeemers.map(([userId, count]) => (
                <React.Fragment key={userId}>
                  <dt className="text-[12px] tabular text-srapi-text-secondary">#{userId}</dt>
                  <dd className="text-right text-[12px] font-medium tabular text-srapi-text-primary">
                    {count}
                  </dd>
                </React.Fragment>
              ))}
            </dl>
          </div>
          <div>
            <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
              Recent uses
            </div>
            <ul className="space-y-1.5 text-[12px] tabular">
              {recent.map((u) => (
                <li
                  key={u.id}
                  className="flex items-center justify-between gap-2 text-srapi-text-secondary"
                >
                  <span className="truncate">{formatDateTime(u.applied_at)}</span>
                  <span className="text-srapi-text-primary">
                    -{formatMoney(u.discount_amount, u.currency)}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </div>
  );
}
