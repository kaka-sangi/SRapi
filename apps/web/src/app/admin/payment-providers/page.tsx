"use client";

import { useState } from "react";
import { Landmark } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { useAdminList } from "@/hooks/use-admin-list";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { useAdminPaymentProviders, useCreatePaymentProvider } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import {
  PAYMENT_PROVIDER_STATUSES,
  emptyPaymentProviderForm,
  buildCreatePaymentProviderBody,
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
  const providers = useAdminPaymentProviders({
    page: list.page,
    page_size: list.pageSize,
  });
  const createMut = useCreatePaymentProvider();

  const [showCreate, setShowCreate] = useState(false);

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

  const columns: Column<PaymentProviderInstance>[] = [
    {
      key: "name",
      header: t("adminPayments.name"),
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
            <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
              ＋ {t("adminPayments.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={providers}
        columns={columns}
        getRowId={(p) => p.id}
        emptyIcon={Landmark}
        emptyTitle={t("adminPayments.emptyTitle")}
        emptyBody={t("adminPayments.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setShowCreate(true)}>
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
      />

      {showCreate ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setShowCreate(false);
          }}
          title={t("adminPayments.create")}
          fields={fields}
          initial={emptyPaymentProviderForm()}
          buildBody={buildCreatePaymentProviderBody}
          submit={(body) => createMut.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createMut.isPending}
        />
      ) : null}
    </>
  );
}
