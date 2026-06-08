"use client";

import { useMemo, useState } from "react";
import { CreditCard } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  useAdminSubscriptionPlans,
  useCreateSubscriptionPlan,
  useUpdateSubscriptionPlan,
  useDeleteSubscriptionPlan,
  useAdminModels,
  useAdminGroups,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney } from "@/lib/admin-format";
import {
  SUBSCRIPTION_PLAN_STATUSES,
  SCHEDULER_STRATEGIES,
  emptySubscriptionPlanForm,
  subscriptionPlanFormFromPlan,
  buildSubscriptionPlanBody,
  type SubscriptionPlanFormState,
} from "@/lib/admin-subscription-form";
import type { SubscriptionPlan } from "@/lib/sdk-types";

export default function OrderPlansPage() {
  return (
    <AdminShell>
      <PlansContent />
    </AdminShell>
  );
}

function PlansContent() {
  const { t } = useLanguage();
  const colVis = useColumnVisibility("admin-orders-plans", []);
  const plans = useAdminSubscriptionPlans();
  const models = useAdminModels();
  const groups = useAdminGroups();
  const createMut = useCreateSubscriptionPlan();
  const updateMut = useUpdateSubscriptionPlan();
  const deleteMut = useDeleteSubscriptionPlan();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<SubscriptionPlan | null>(null);
  const [toDelete, setToDelete] = useState<SubscriptionPlan | null>(null);

  // Options for the structured "allowed models" / "account group scope" pickers,
  // sourced live so an admin chooses real models/groups instead of typing keys.
  const modelOptions = useMemo(
    () =>
      (models.data?.data ?? []).map((m) => ({
        value: m.canonical_name,
        label: m.display_name || m.canonical_name,
      })),
    [models.data],
  );
  const groupOptions = useMemo(
    () => (groups.data?.data ?? []).map((g) => ({ value: String(g.id), label: g.name })),
    [groups.data],
  );

  const fields: FieldConfig<SubscriptionPlanFormState>[] = [
    { name: "name", label: t("adminCommon.name") },
    { name: "description", label: t("adminCommon.description"), type: "textarea" },
    { name: "price", label: t("adminCommon.price") },
    { name: "currency", label: t("adminCommon.currency") },
    { name: "validityDays", label: t("adminSubscriptions.validityDays"), type: "number" },
    {
      name: "allowedModels",
      label: t("adminSubscriptions.allowedModels"),
      type: "combobox",
      options: modelOptions,
      allowCustom: true,
      hint: t("adminSubscriptions.allowedModelsHint"),
    },
    {
      name: "monthlyTokenQuota",
      label: t("adminSubscriptions.monthlyTokenQuota"),
      type: "number",
      hint: t("adminSubscriptions.quotaUnlimitedHint"),
    },
    {
      name: "monthlyCostQuota",
      label: t("adminSubscriptions.monthlyCostQuota"),
      hint: t("adminSubscriptions.quotaUnlimitedHint"),
    },
    {
      name: "costQuotaMode",
      label: t("adminSubscriptions.costQuotaMode"),
      type: "select",
      options: [
        { value: "hard_cap", label: t("adminSubscriptions.costQuotaModeHardCap") },
        { value: "allowance", label: t("adminSubscriptions.costQuotaModeAllowance") },
      ],
      hint: t("adminSubscriptions.costQuotaModeHint"),
    },
    {
      name: "schedulerStrategy",
      label: t("adminSubscriptions.schedulerStrategy"),
      help: t("adminSubscriptions.schedulerStrategyHelp"),
      type: "select",
      options: SCHEDULER_STRATEGIES.map((v) => ({
        value: v,
        label: v === "default" ? t("adminSubscriptions.schedulerDefault") : v,
      })),
    },
    {
      name: "accountGroupScope",
      label: t("adminSubscriptions.accountGroupScope"),
      type: "combobox",
      options: groupOptions,
      hint: t("adminSubscriptions.accountGroupScopeHint"),
    },
    {
      name: "extraEntitlements",
      label: t("adminSubscriptions.extraEntitlements"),
      type: "keyvalue",
      advanced: true,
      hint: t("adminSubscriptions.extraEntitlementsHint"),
    },
    { name: "forSale", label: t("adminSubscriptions.forSale"), help: t("adminSubscriptions.forSaleHelp"), type: "switch" },
    { name: "sortOrder", label: t("adminSubscriptions.sortOrder"), help: t("adminSubscriptions.sortOrderHelp"), type: "number" },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(SUBSCRIPTION_PLAN_STATUSES),
    },
  ];

  const columns: Column<SubscriptionPlan>[] = [
    {
      key: "name",
      header: t("adminSubscriptions.plan"),
      pinned: true,
      render: (p) => <span className="text-srapi-text-primary">{p.name}</span>,
    },
    {
      key: "price",
      header: t("adminSubscriptions.price"),
      align: "right",
      render: (p) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(p.price, p.currency)} / {t("adminSubscriptions.validity", { days: p.validity_days })}
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
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminSubscriptions.tabPlans")}
        description={t("adminSubscriptions.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {plans.data ? (
              <ListCount total={plans.data.pagination?.total ?? plans.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
              ＋ {t("adminSubscriptions.createPlan")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={plans}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(p) => p.id}
        emptyIcon={CreditCard}
        emptyTitle={t("adminSubscriptions.emptyPlans")}
        emptyBody={t("adminSubscriptions.emptyPlansBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
            ＋ {t("adminSubscriptions.createPlan")}
          </Button>
        }
        minWidth={480}
        rowActions={(p) => (
          <RowActionsMenu
            actions={[
              { label: t("common.edit"), onSelect: () => setEditing(p) },
              { label: t("common.delete"), destructive: true, onSelect: () => setToDelete(p) },
            ]}
          />
        )}
      />

      <ConfirmDialog
        open={toDelete !== null}
        onOpenChange={(open) => {
          if (!open) setToDelete(null);
        }}
        title={t("adminSubscriptions.deletePlanTitle")}
        body={t("adminSubscriptions.deletePlanBody", { name: toDelete?.name ?? "" })}
        confirmLabel={t("common.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteMut.isPending}
        onConfirm={async () => {
          if (toDelete) await deleteMut.mutateAsync(toDelete.id);
        }}
      />

      {creating ? (
        <ResourceFormDialog
          open
          onOpenChange={setCreating}
          title={t("adminSubscriptions.createPlan")}
          fields={fields}
          initial={emptySubscriptionPlanForm()}
          buildBody={buildSubscriptionPlanBody}
          submit={(body) => createMut.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createMut.isPending}
        />
      ) : null}

      {editing ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setEditing(null);
          }}
          title={t("adminSubscriptions.editPlan")}
          fields={fields}
          initial={subscriptionPlanFormFromPlan(editing)}
          buildBody={buildSubscriptionPlanBody}
          submit={(body) => updateMut.mutateAsync({ id: editing.id, body })}
          successMessage={t("feedback.saved")}
          isPending={updateMut.isPending}
        />
      ) : null}
    </>
  );
}
