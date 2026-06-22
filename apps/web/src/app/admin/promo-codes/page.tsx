"use client";

import { useState } from "react";
import { Ticket } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
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
  useCreatePromoCode,
  useUpdatePromoCode,
  useDeletePromoCode,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatDateTime } from "@/lib/admin-format";
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
      options: enumOptions(PROMO_DISCOUNT_TYPES),
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
      options: enumOptions(PROMO_CODE_STATUSES),
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
      render: (p) => (
        <span className="text-sm font-medium tabular text-srapi-text-primary">
          {p.discount_type === "percent" ? `${p.discount_value}%` : p.discount_value}
        </span>
      ),
    },
    {
      key: "uses",
      header: t("adminPromos.uses"),
      align: "right",
      hideOnMobile: true,
      render: (p) => (
        <span className="text-sm tabular text-srapi-text-secondary">
          {p.used_count ?? 0}
          {p.max_uses ? ` / ${p.max_uses}` : ""}
        </span>
      ),
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
        eyebrow="Commerce · Promo Codes"
        title={t("adminPromos.promoTitle")}
        description={t("adminPromos.promoSubtitle")}
        metrics={
          promos.data
            ? [
                {
                  label: "在用",
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
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={enumOptions(PROMO_CODE_STATUSES)}
              allLabel={t("adminCommon.allStatuses")}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: promos.data?.pagination?.total ?? promos.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
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
