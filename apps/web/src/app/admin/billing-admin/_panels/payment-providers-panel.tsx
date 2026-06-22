"use client";

import { useState } from "react";
import { Landmark } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { useClientPagedList } from "@/hooks/use-client-list";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import {
  useAdminPaymentProviders,
  useCreatePaymentProvider,
  useUpdatePaymentProvider,
  useTestPaymentProvider,
  useDeletePaymentProvider,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import {
  PAYMENT_PROVIDER_STATUSES,
  emptyPaymentProviderForm,
  buildCreatePaymentProviderBody,
  buildUpdatePaymentProviderBody,
  paymentProviderFormFromInstance,
  type PaymentProviderFormState,
} from "@/lib/admin-orders-form";
import type { PaymentProviderInstance } from "@/lib/sdk-types";

interface PaymentPreset {
  key: string;
  name: string;
  description: string;
  provider: string;
  methods: string[];
  feeRate: string;
  configTemplate: Record<string, unknown>;
}

const PAYMENT_PRESETS: PaymentPreset[] = [
  {
    key: "linuxdo",
    name: "Linux.do Credit",
    description: "LinuxDo 社区积分支付（EasyPay 协议）",
    provider: "linuxdo",
    methods: ["linuxdo"],
    feeRate: "0",
    configTemplate: {
      gateway_url: "https://credit.linux.do",
      merchant_id: "",
      signing_secret: "",
      exchange_rate: "1",
      notify_url: "",
      return_url: "",
    },
  },
  {
    key: "easypay",
    name: "EasyPay",
    description: "第三方聚合支付（支付宝 / 微信）",
    provider: "easypay",
    methods: ["alipay", "wxpay"],
    feeRate: "0.016",
    configTemplate: {
      gateway_url: "",
      merchant_id: "",
      signing_secret: "",
      notify_url: "",
      return_url: "",
      site_name: "SRapi",
    },
  },
  {
    key: "stripe",
    name: "Stripe",
    description: "国际卡支付 / Apple Pay / Google Pay",
    provider: "stripe",
    methods: ["card", "alipay", "wechat_pay", "link"],
    feeRate: "0.029",
    configTemplate: {},
  },
  {
    key: "alipay",
    name: "支付宝直连",
    description: "支付宝开放平台直接对接",
    provider: "alipay",
    methods: ["alipay"],
    feeRate: "0.006",
    configTemplate: {
      app_id: "",
      private_key: "",
      alipay_public_key: "",
      notify_url: "",
      return_url: "",
    },
  },
  {
    key: "wechat",
    name: "微信支付直连",
    description: "微信支付 APIv3 直接对接",
    provider: "wechat",
    methods: ["wxpay"],
    feeRate: "0.006",
    configTemplate: {
      mch_id: "",
      api_v3_key: "",
      serial_no: "",
      private_key: "",
      notify_url: "",
    },
  },
];

function providerMatch(
  provider: PaymentProviderInstance,
  term: string,
  filters: Record<string, string>,
): boolean {
  if (filters.status && provider.status !== filters.status) return false;
  if (!term) return true;
  return [provider.name, provider.provider, provider.status, ...provider.supported_methods]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

const providerCompare = (a: PaymentProviderInstance, b: PaymentProviderInstance) =>
  a.sort_order - b.sort_order || a.name.localeCompare(b.name);

export function PaymentProvidersPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-payment-providers", []);
  const all = useAdminPaymentProviders();
  const { query: providers, total } = useClientPagedList(all, list, {
    match: providerMatch,
    compare: providerCompare,
  });
  const { toast } = useToast();
  const createMut = useCreatePaymentProvider();
  const updateMut = useUpdatePaymentProvider();
  const testMut = useTestPaymentProvider();
  const deleteMut = useDeletePaymentProvider();

  const [formTarget, setFormTarget] = useState<PaymentProviderInstance | "new" | null>(null);
  const [selectedPreset, setSelectedPreset] = useState<PaymentPreset | null>(null);
  const [showPresetPicker, setShowPresetPicker] = useState(false);
  const [toDelete, setToDelete] = useState<PaymentProviderInstance | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);

  async function toggleStatus(p: PaymentProviderInstance) {
    if (togglingId === p.id) return;
    const next: PaymentProviderInstance["status"] =
      p.status === "active" ? "disabled" : "active";
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

  async function runTest(id: string) {
    try {
      const result = await testMut.mutateAsync(id);
      toast({
        title: result.ok ? t("feedback.acknowledged") : t("feedback.failed"),
        description:
          result.message ?? (result.latency_ms != null ? `${result.latency_ms} ms` : undefined),
        tone: result.ok ? "success" : "error",
      });
    } catch {
      toast({ title: t("feedback.failed"), tone: "error" });
    }
  }

  const fields: FieldConfig<PaymentProviderFormState>[] = [
    { name: "name", label: t("adminPayments.name") },
    { name: "provider", label: t("adminPayments.channel"), hint: t("adminPayments.channelHint") },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(PAYMENT_PROVIDER_STATUSES),
    },
    {
      name: "supportedMethodsText",
      label: t("adminPayments.supportedMethods"),
      type: "textarea",
      hint: t("adminPayments.supportedMethodsHint"),
    },
    {
      name: "feeRate",
      label: t("adminPayments.feeRate"),
      type: "number",
      hint: t("adminPayments.feeRateHint"),
    },
    {
      name: "weight",
      label: t("adminPayments.weight"),
      type: "number",
      hint: t("adminPayments.weightHint"),
    },
    { name: "config", label: t("adminPayments.config"), type: "keyvalue", hint: t("adminPayments.configHint") },
    {
      name: "limits",
      label: t("adminPayments.limits"),
      type: "keyvalue",
      advanced: true,
      hint: t("adminPayments.limitsHint"),
    },
    { name: "sortOrder", label: t("adminPayments.sortOrder"), type: "number", advanced: true },
    { name: "metadata", label: t("adminCommon.metadata"), type: "keyvalue", advanced: true },
  ];

  // The channel (provider) is immutable once created, and stored secrets aren't
  // returned, so edits drop the channel field and nudge config toward "leave blank".
  const editFields = fields
    .filter((f) => f.name !== "provider")
    .map((f) => (f.name === "config" ? { ...f, hint: t("adminPayments.configEditHint") } : f));

  const columns: Column<PaymentProviderInstance>[] = [
    {
      key: "name",
      header: t("adminPayments.name"),
      pinned: true,
      sortValue: (p) => p.name,
      render: (p) => <span className="text-srapi-text-primary">{p.name}</span>,
    },
    {
      key: "provider",
      header: t("adminPayments.channel"),
      sortValue: (p) => p.provider,
      render: (p) => (
        <span className="inline-flex items-center rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium uppercase tracking-wider text-srapi-text-secondary">
          {p.provider}
        </span>
      ),
    },
    {
      key: "status",
      header: t("adminCommon.status"),
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
    {
      key: "methods",
      header: t("adminPayments.config"),
      hideOnMobile: true,
      render: (p) => (
        <span className="text-xs text-srapi-text-tertiary">
          {p.supported_methods.length ? p.supported_methods.join(" · ") : "—"}
        </span>
      ),
    },
    {
      key: "fee",
      header: t("adminPayments.feeHeader"),
      hideOnMobile: true,
      align: "right",
      sortValue: (p) => Number(p.fee_rate),
      render: (p) => {
        const fee = Number(p.fee_rate);
        const perThousand = fee * 1000;
        return (
          <DataTooltip
            title={t("adminPayments.feeHeader")}
            primary={`${(fee * 100).toFixed(3)}%`}
            rows={[
              { label: "Decimal", value: fee.toFixed(5) },
              { label: "Per 1k", value: `${perThousand.toFixed(2)}` },
              { label: "On $100", value: `$${(fee * 100).toFixed(2)}` },
            ]}
          >
            <span className="text-xs tabular text-srapi-text-tertiary">
              {(fee * 100).toFixed(3)}%
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "weight",
      header: t("adminPayments.weight"),
      hideOnMobile: true,
      align: "right",
      sortValue: (p) => p.weight,
      render: (p) => (
        <span className="text-xs tabular text-srapi-text-tertiary">{p.weight}</span>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminPayments.title")}
        description={t("adminPayments.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setShowPresetPicker(true)}>
              ＋ {t("adminPayments.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={providers}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(p) => p.id}
        emptyIcon={Landmark}
        emptyTitle={t("adminPayments.emptyTitle")}
        emptyBody={t("adminPayments.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setShowPresetPicker(true)}>
            ＋ {t("adminPayments.create")}
          </Button>
        }
        minWidth={520}
        sort={list.sort}
        onSort={list.toggleSort}
        isFiltered={Boolean(list.search || list.filters.status)}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminPayments.searchPlaceholder")}
            />
            <SegmentedControl<string>
              value={(list.filters.status as string) ?? "all"}
              onChange={(v) => list.setFilter("status", v === "all" ? "" : v)}
              ariaLabel={t("adminCommon.allStatuses")}
              size="sm"
              options={[
                { value: "all", label: t("adminCommon.allStatuses") },
                { value: "active", label: t("common.active") },
                { value: "disabled", label: t("common.disabled") },
                { value: "archived", label: t("common.archived") },
              ]}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(p) => (
          <RowActionsMenu
            actions={[
              { label: t("adminPayments.edit"), onSelect: () => setFormTarget(p) },
              { label: t("adminPayments.test"), onSelect: () => void runTest(p.id) },
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
        title={t("adminPayments.deleteTitle")}
        body={t("adminPayments.deleteBody", { name: toDelete?.name ?? "" })}
        confirmLabel={t("common.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteMut.isPending}
        onConfirm={async () => {
          if (toDelete) await deleteMut.mutateAsync(toDelete.id);
        }}
      />

      {/* Preset picker */}
      {showPresetPicker && !formTarget ? (
        <PaymentPresetPicker
          onSelect={(preset) => {
            setSelectedPreset(preset);
            setShowPresetPicker(false);
            setFormTarget("new");
          }}
          onClose={() => setShowPresetPicker(false)}
        />
      ) : null}

      {formTarget === "new" ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) { setFormTarget(null); setSelectedPreset(null); }
          }}
          title={selectedPreset ? selectedPreset.name : t("adminPayments.create")}
          description={selectedPreset?.description}
          fields={fields}
          initial={selectedPreset ? presetToForm(selectedPreset) : emptyPaymentProviderForm()}
          buildBody={buildCreatePaymentProviderBody}
          submit={(body) => createMut.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createMut.isPending}
        />
      ) : formTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={t("adminPayments.edit")}
          description={formTarget.name}
          fields={editFields}
          initial={paymentProviderFormFromInstance(formTarget)}
          buildBody={buildUpdatePaymentProviderBody}
          submit={(body) => updateMut.mutateAsync({ id: formTarget.id, body })}
          successMessage={t("feedback.updated")}
          isPending={updateMut.isPending}
        />
      ) : null}
    </>
  );
}

function presetToForm(preset: PaymentPreset): PaymentProviderFormState {
  return {
    provider: preset.provider,
    name: preset.name,
    status: "active",
    supportedMethodsText: preset.methods.join("\n"),
    config: { ...preset.configTemplate },
    limits: {},
    metadata: {},
    sortOrder: "0",
    feeRate: preset.feeRate,
    weight: "1",
  };
}

function PaymentPresetPicker({
  onSelect,
  onClose,
}: {
  onSelect: (preset: PaymentPreset) => void;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg rounded-xl border border-srapi-border bg-srapi-card p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="space-y-1">
          <h2 className="text-lg font-semibold tracking-tight text-srapi-text-primary">
            {t("adminPayments.selectPreset")}
          </h2>
          <p className="text-sm text-srapi-text-secondary">{t("adminPayments.selectPresetHint")}</p>
        </div>
        <div className="mt-5 space-y-2.5">
          {PAYMENT_PRESETS.map((preset, idx) => (
            <button
              key={preset.key}
              type="button"
              onClick={() => onSelect(preset)}
              style={{ "--stagger-index": idx } as React.CSSProperties}
              className="group flex w-full items-center gap-4 rounded-xl border border-srapi-border bg-srapi-card px-4 py-3.5 text-left transition-all duration-200 hover:-translate-y-0.5 hover:border-srapi-primary/40 hover:bg-srapi-accent-soft/30 hover:shadow-md focus:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary/40"
            >
              <div className="grid size-10 shrink-0 place-items-center rounded-xl bg-srapi-accent-soft text-xs font-semibold uppercase tracking-wider text-srapi-primary">
                {preset.key.slice(0, 2).toUpperCase()}
              </div>
              <div className="min-w-0 flex-1">
                <div className="text-sm font-semibold tracking-tight text-srapi-text-primary">
                  {preset.name}
                </div>
                <div className="mt-0.5 text-xs text-srapi-text-tertiary">{preset.description}</div>
              </div>
              {preset.feeRate !== "0" ? (
                <span className="shrink-0 rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium tabular text-srapi-text-tertiary">
                  {(Number(preset.feeRate) * 100).toFixed(1)}%
                </span>
              ) : null}
            </button>
          ))}
        </div>
        <div className="mt-5 flex items-center justify-between border-t border-srapi-border/70 pt-4">
          <button
            type="button"
            onClick={() => {
              onSelect({
                key: "custom",
                name: "Custom",
                description: "",
                provider: "",
                methods: [],
                feeRate: "0",
                configTemplate: {},
              });
            }}
            className="text-xs font-medium text-srapi-text-tertiary transition-colors hover:text-srapi-primary"
          >
            {t("adminPayments.customProvider")}
          </button>
          <Button variant="ghost" size="sm" onClick={onClose}>
            {t("common.cancel")}
          </Button>
        </div>
      </div>
    </div>
  );
}
