"use client";

import { useState } from "react";
import { Landmark } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
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
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import {
  PAYMENT_PROVIDER_STATUSES,
  emptyPaymentProviderForm,
  buildCreatePaymentProviderBody,
  buildUpdatePaymentProviderBody,
  paymentProviderFormFromInstance,
  type PaymentProviderFormState,
} from "@/lib/admin-orders-form";
import type { PaymentProviderInstance } from "../../../../../../packages/sdk/typescript/src/types.gen";

export default function AdminPaymentProvidersPage() {
  return (
    <AdminShell>
      <PaymentProvidersContent />
    </AdminShell>
  );
}

function PaymentProvidersContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-payment-providers", []);
  const providers = useAdminPaymentProviders({
    page: list.page,
    page_size: list.pageSize,
  });
  const { toast } = useToast();
  const createMut = useCreatePaymentProvider();
  const updateMut = useUpdatePaymentProvider();
  const testMut = useTestPaymentProvider();
  const deleteMut = useDeletePaymentProvider();

  const [formTarget, setFormTarget] = useState<PaymentProviderInstance | "new" | null>(null);
  const [toDelete, setToDelete] = useState<PaymentProviderInstance | null>(null);

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
      label: "Fee rate",
      type: "number",
      hint: "Decimal channel fee rate, for example 0.006 means 0.6%.",
    },
    {
      name: "weight",
      label: "Weight",
      type: "number",
      hint: "Positive round-robin weight used when multiple active channels support the same method.",
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
        <span className="font-mono text-2xs uppercase text-srapi-text-secondary">{p.provider}</span>
      ),
    },
    {
      key: "status",
      header: t("adminCommon.status"),
      render: (p) => <QuietBadge status={quietStatusFor(p.status)} label={statusLabel(t, p.status)} />,
    },
    {
      key: "methods",
      header: t("adminPayments.config"),
      hideOnMobile: true,
      render: (p) => (
        <span className="text-2xs text-srapi-text-tertiary">
          {p.supported_methods.length ? p.supported_methods.join(" · ") : "—"}
        </span>
      ),
    },
    {
      key: "fee",
      header: "Fee",
      hideOnMobile: true,
      align: "right",
      sortValue: (p) => Number(p.fee_rate),
      render: (p) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {(Number(p.fee_rate) * 100).toFixed(3)}%
        </span>
      ),
    },
    {
      key: "weight",
      header: "Weight",
      hideOnMobile: true,
      align: "right",
      sortValue: (p) => p.weight,
      render: (p) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">{p.weight}</span>
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
            {providers.data ? (
              <ListCount total={providers.data.pagination?.total ?? providers.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
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
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminPayments.create")}
          </Button>
        }
        minWidth={520}
        sort={list.sort}
        onSort={list.toggleSort}
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: providers.data?.pagination?.total ?? providers.data?.data.length ?? 0,
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

      {formTarget === "new" ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={t("adminPayments.create")}
          fields={fields}
          initial={emptyPaymentProviderForm()}
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
