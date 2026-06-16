"use client";

import { useMemo, useState } from "react";
import { CreditCard } from "lucide-react";
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
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
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

export function PlansPanel() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const colVis = useColumnVisibility("admin-orders-plans", []);
  const plans = useAdminSubscriptionPlans();
  const [togglingId, setTogglingId] = useState<string | null>(null);

  async function toggleStatus(p: SubscriptionPlan) {
    if (togglingId === p.id) return;
    const next: SubscriptionPlan["status"] = p.status === "active" ? "disabled" : "active";
    setTogglingId(p.id);
    try {
      await updateMut.mutateAsync({ id: p.id, body: { status: next } });
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: adminErrorMessage(err), tone: "error" });
    } finally {
      setTogglingId(null);
    }
  }
  const models = useAdminModels();
  const groups = useAdminGroups();
  const createMut = useCreateSubscriptionPlan();
  const updateMut = useUpdateSubscriptionPlan();
  const deleteMut = useDeleteSubscriptionPlan();
  const [creating, setCreating] = useState(false);
  const [showPresets, setShowPresets] = useState(false);
  const [planPreset, setPlanPreset] = useState<SubscriptionPlanFormState | null>(null);
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
      name: "dailyCostQuota",
      label: t("adminSubscriptions.dailyCostQuota"),
      hint: t("adminSubscriptions.quotaUnlimitedHint"),
    },
    {
      name: "weeklyCostQuota",
      label: t("adminSubscriptions.weeklyCostQuota"),
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
      render: (p) => {
        const canToggle = p.status === "active" || p.status === "disabled";
        const badge = (
          <QuietBadge status={quietStatusFor(p.status)} label={statusLabel(t, p.status)} />
        );
        if (!canToggle) return badge;
        return (
          <button
            type="button"
            onClick={() => void toggleStatus(p)}
            disabled={togglingId === p.id}
            className="cursor-pointer disabled:cursor-wait disabled:opacity-60"
            title={p.status === "active" ? t("common.disable") : t("common.enable")}
          >
            {badge}
          </button>
        );
      },
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
            <Button variant="primary" size="sm" onClick={() => setShowPresets(true)}>
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
          <Button variant="primary" size="sm" onClick={() => setShowPresets(true)}>
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

      {showPresets && !creating ? (
        <PlanPresetPicker
          onSelect={(preset) => {
            setPlanPreset(preset);
            setShowPresets(false);
            setCreating(true);
          }}
          onClose={() => setShowPresets(false)}
          t={t}
        />
      ) : null}

      {creating ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => { setCreating(open); if (!open) setPlanPreset(null); }}
          title={t("adminSubscriptions.createPlan")}
          fields={fields}
          initial={planPreset ?? emptySubscriptionPlanForm()}
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

const PLAN_PRESETS: { key: string; name: string; descKey: string; form: Partial<SubscriptionPlanFormState> }[] = [
  {
    key: "free",
    name: "Free",
    descKey: "adminSubscriptions.presetFreeDesc",
    form: { name: "Free", price: "0", currency: "USD", validityDays: "30", monthlyCostQuota: "1.00", costQuotaMode: "hard_cap", forSale: true, status: "active" },
  },
  {
    key: "basic",
    name: "Basic",
    descKey: "adminSubscriptions.presetBasicDesc",
    form: { name: "Basic", price: "9.90", currency: "USD", validityDays: "30", monthlyCostQuota: "10.00", costQuotaMode: "hard_cap", forSale: true, status: "active" },
  },
  {
    key: "pro",
    name: "Pro",
    descKey: "adminSubscriptions.presetProDesc",
    form: { name: "Pro", price: "29.90", currency: "USD", validityDays: "30", monthlyCostQuota: "50.00", costQuotaMode: "hard_cap", forSale: true, status: "active" },
  },
  {
    key: "enterprise",
    name: "Enterprise",
    descKey: "adminSubscriptions.presetEnterpriseDesc",
    form: { name: "Enterprise", price: "99.90", currency: "USD", validityDays: "30", monthlyCostQuota: "", costQuotaMode: "hard_cap", forSale: true, status: "active" },
  },
];

function PlanPresetPicker({
  onSelect,
  onClose,
  t,
}: {
  onSelect: (form: SubscriptionPlanFormState) => void;
  onClose: () => void;
  t: (key: string) => string;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40" onClick={onClose}>
      <div className="w-full max-w-md rounded-xl border border-srapi-border bg-srapi-card p-6 shadow-lg" onClick={(e) => e.stopPropagation()}>
        <h2 className="font-serif text-xl text-srapi-text-primary">{t("adminSubscriptions.selectTemplate")}</h2>
        <p className="mt-1 text-sm text-srapi-text-secondary">{t("adminSubscriptions.selectTemplateHint")}</p>
        <div className="mt-5 grid grid-cols-2 gap-2">
          {PLAN_PRESETS.map((p) => (
            <button
              key={p.key}
              type="button"
              onClick={() => onSelect({ ...emptySubscriptionPlanForm(), ...p.form })}
              className="rounded-lg border border-srapi-border bg-srapi-card px-4 py-3 text-left transition-colors hover:border-srapi-border-strong hover:bg-srapi-card-muted"
            >
              <div className="text-sm font-medium text-srapi-text-primary">{p.name}</div>
              <div className="mt-0.5 text-xs text-srapi-text-tertiary">{t(p.descKey)}</div>
              {p.form.price && p.form.price !== "0" ? (
                <div className="mt-2 font-mono text-sm text-srapi-text-secondary">${p.form.price}/mo</div>
              ) : (
                <div className="mt-2 font-mono text-sm text-srapi-text-tertiary">Free</div>
              )}
            </button>
          ))}
        </div>
        <div className="mt-4 flex justify-between">
          <button
            type="button"
            onClick={() => onSelect(emptySubscriptionPlanForm())}
            className="text-xs text-srapi-text-tertiary transition-colors hover:text-srapi-text-secondary"
          >
            {t("adminPayments.customProvider")}
          </button>
          <Button variant="ghost" size="sm" onClick={onClose}>{t("common.cancel")}</Button>
        </div>
      </div>
    </div>
  );
}
