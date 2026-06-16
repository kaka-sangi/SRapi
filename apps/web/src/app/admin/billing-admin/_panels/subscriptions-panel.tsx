"use client";

import { useState } from "react";
import { CreditCard } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import { useClientPagedList } from "@/hooks/use-client-list";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { RowActionsMenu, type RowAction } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useAdminSubscriptionPlans,
  useAdminSubscriptions,
  useAdminUsers,
  useCreateUserSubscription,
  useDeleteUserSubscription,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatDate } from "@/lib/admin-format";
import {
  SubscriptionUsageBars,
  type SubscriptionUsageLabels,
} from "@/components/features/subscription-usage-bars";
import {
  USER_SUBSCRIPTION_STATUSES,
  emptyUserSubscriptionForm,
  buildCreateUserSubscriptionBody,
  type UserSubscriptionFormState,
} from "@/lib/admin-subscription-form";
import type { UserSubscription } from "@/lib/sdk-types";

export function SubscriptionsPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-subscriptions", []);
  const plans = useAdminSubscriptionPlans();
  const allSubs = useAdminSubscriptions();
  // Server endpoint accepts page/page_size/user_id only — no status filter —
  // so the operator-side filtering happens client-side via the shared list
  // helper. The full list ships in one shot today, so the lift is small.
  const { query: subs, total } = useClientPagedList(allSubs, list, {
    match: (row, _term, filters) =>
      !filters.status || row.status === filters.status,
    compare: (a, b) => (b.starts_at ?? "").localeCompare(a.starts_at ?? ""),
  });
  const statusFilter = list.filters.status as UserSubscription["status"] | undefined;
  const users = useAdminUsers();
  const userLookup = useUserEmailLookup();
  const createSub = useCreateUserSubscription();
  const deleteSub = useDeleteUserSubscription();
  const [creatingSub, setCreatingSub] = useState(false);
  const [subToDelete, setSubToDelete] = useState<UserSubscription | null>(null);

  const subFields: FieldConfig<UserSubscriptionFormState>[] = [
    {
      name: "userId",
      label: t("adminSubscriptions.user"),
      type: "select",
      required: true,
      options: (users.data?.data ?? []).map((u) => ({ value: u.id, label: u.email })),
    },
    {
      name: "planId",
      label: t("adminSubscriptions.plan"),
      type: "select",
      required: true,
      options: (plans.data?.data ?? []).map((p) => ({ value: p.id, label: p.name })),
    },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(USER_SUBSCRIPTION_STATUSES),
    },
    { name: "startsAtLocal", label: t("adminCommon.startsAt"), type: "datetime" },
    { name: "expiresAtLocal", label: t("adminCommon.expiresAt"), type: "datetime" },
  ];

  const subColumns: Column<UserSubscription>[] = [
    {
      key: "user",
      header: t("adminSubscriptions.user"),
      pinned: true,
      render: (s) => <span className="text-srapi-text-secondary">{userLookup.get(s.user_id)}</span>,
    },
    {
      key: "plan",
      header: t("adminSubscriptions.plan"),
      pinned: true,
      render: (s) => <span className="font-mono text-2xs text-srapi-text-secondary">{s.plan_id}</span>,
    },
    {
      key: "period",
      header: t("adminSubscriptions.period"),
      hideOnMobile: true,
      render: (s) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDate(s.starts_at)} – {formatDate(s.expires_at)}
        </span>
      ),
    },
    {
      key: "usage",
      header: t("adminSubscriptions.usage"),
      hideOnMobile: true,
      render: (s) => <SubscriptionUsageBars subscription={s} labels={subscriptionUsageLabels(t)} />,
    },
    {
      key: "status",
      header: t("common.active"),
      render: (s) => <QuietBadge status={quietStatusFor(s.status)} label={statusLabel(t, s.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminSubscriptions.title")}
        description={t("adminSubscriptions.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {allSubs.data ? <ListCount total={total} /> : null}
            <Button variant="primary" size="sm" onClick={() => setCreatingSub(true)}>
              ＋ {t("adminSubscriptions.createSubscription")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={subs}
        columns={subColumns}
        columnVisibility={colVis}
        getRowId={(s) => s.id}
        emptyIcon={CreditCard}
        emptyTitle={t("adminSubscriptions.emptySubs")}
        emptyBody={t("adminSubscriptions.emptySubsBody")}
        minWidth={560}
        isFiltered={Boolean(statusFilter)}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <FilterSelect
              value={list.filters.status}
              onChange={(v) => list.setFilter("status", v)}
              options={USER_SUBSCRIPTION_STATUSES.map((s) => ({
                value: s,
                label: statusLabel(t, s),
              }))}
              allLabel={t("adminCommon.allStatuses")}
            />
            <ColumnToggle
              columns={subColumns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(s) => {
          const actions: RowAction[] = [
            { label: t("common.delete"), destructive: true, onSelect: () => setSubToDelete(s) },
          ];
          return <RowActionsMenu actions={actions} />;
        }}
      />

      <ConfirmDialog
        open={subToDelete !== null}
        onOpenChange={(open) => {
          if (!open) setSubToDelete(null);
        }}
        title={t("adminSubscriptions.deleteSubTitle")}
        body={t("adminSubscriptions.deleteSubBody")}
        confirmLabel={t("common.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteSub.isPending}
        onConfirm={async () => {
          if (subToDelete) await deleteSub.mutateAsync(subToDelete.id);
        }}
      />

      {creatingSub ? (
        <ResourceFormDialog
          open
          onOpenChange={setCreatingSub}
          title={t("adminSubscriptions.createSubscription")}
          fields={subFields}
          initial={emptyUserSubscriptionForm()}
          buildBody={buildCreateUserSubscriptionBody}
          submit={(body) => createSub.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createSub.isPending}
        />
      ) : null}
    </>
  );
}

function subscriptionUsageLabels(t: ReturnType<typeof useLanguage>["t"]): SubscriptionUsageLabels {
  return {
    daily: t("adminSubscriptions.dailyUsage"),
    weekly: t("adminSubscriptions.weeklyUsage"),
    monthly: t("adminSubscriptions.monthlyUsage"),
    noQuota: t("adminSubscriptions.noCostQuota"),
  };
}
