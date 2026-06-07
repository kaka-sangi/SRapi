"use client";

import { useState } from "react";
import { Network } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useAdminProxies,
  useCreateProxy,
  useUpdateProxy,
  useDeleteProxy,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import {
  PROXY_TYPES,
  PROXY_STATUSES,
  emptyProxyForm,
  proxyFormFromProxy,
  buildCreateProxyBody,
  buildUpdateProxyBody,
  type ProxyFormState,
} from "@/lib/admin-proxy-form";
import type { ProxyDefinition } from "@/lib/sdk-types";

export default function AdminProxiesPage() {
  return (
    <AdminShell>
      <ProxiesContent />
    </AdminShell>
  );
}

function ProxiesContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const statusFilter = (list.filters.status as ProxyDefinition["status"]) || undefined;
  const proxies = useAdminProxies({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
  });
  const createMut = useCreateProxy();
  const updateMut = useUpdateProxy();
  const deleteMut = useDeleteProxy();

  const [formTarget, setFormTarget] = useState<ProxyDefinition | "new" | null>(null);
  const [toDelete, setToDelete] = useState<ProxyDefinition | null>(null);
  const isNew = formTarget === "new";

  const fields: FieldConfig<ProxyFormState>[] = [
    { name: "name", label: t("adminProxies.name") },
    {
      name: "type",
      label: t("adminProxies.protocol"),
      type: "select",
      options: enumOptions(PROXY_TYPES),
    },
    {
      name: "url",
      label: t("adminProxies.url"),
      placeholder: "http://user:pass@host:port",
      hint: isNew ? undefined : t("adminProxies.urlEditHint"),
    },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(PROXY_STATUSES),
      advanced: true,
    },
    { name: "metadata", label: t("adminCommon.metadata"), type: "keyvalue", advanced: true },
  ];

  const columns: Column<ProxyDefinition>[] = [
    {
      key: "name",
      header: t("adminProxies.name"),
      sortValue: (p) => p.name,
      render: (p) => <span className="text-srapi-text-primary">{p.name}</span>,
    },
    {
      key: "protocol",
      header: t("adminProxies.protocol"),
      render: (p) => (
        <span className="font-mono text-2xs uppercase text-srapi-text-secondary">{p.type}</span>
      ),
    },
    {
      key: "url",
      header: t("adminProxies.url"),
      hideOnMobile: true,
      render: (p) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">
          {p.url_configured ? "✓" : "—"}
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
        title={t("adminProxies.title")}
        description={t("adminProxies.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {proxies.data ? (
              <ListCount total={proxies.data.pagination?.total ?? proxies.data.data.length} />
            ) : null}
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminProxies.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={proxies}
        columns={columns}
        getRowId={(p) => p.id}
        emptyIcon={Network}
        emptyTitle={t("adminProxies.emptyTitle")}
        emptyBody={t("adminProxies.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminProxies.create")}
          </Button>
        }
        minWidth={520}
        isFiltered={Boolean(statusFilter)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        toolbar={
          <ListToolbar>
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={enumOptions(PROXY_STATUSES)}
              allLabel={t("adminCommon.allStatuses")}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: proxies.data?.pagination?.total ?? proxies.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(p) => (
          <RowActionsMenu
            actions={[
              { label: t("common.edit"), onSelect: () => setFormTarget(p) },
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
        title={t("adminProxies.deleteTitle")}
        body={t("adminProxies.deleteBody", { name: toDelete?.name ?? "" })}
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
          title={t("adminProxies.create")}
          fields={fields}
          initial={emptyProxyForm()}
          buildBody={buildCreateProxyBody}
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
          title={t("adminProxies.edit")}
          fields={fields}
          initial={proxyFormFromProxy(formTarget)}
          buildBody={buildUpdateProxyBody}
          submit={(body) => updateMut.mutateAsync({ id: formTarget.id, body })}
          successMessage={t("feedback.updated")}
          isPending={updateMut.isPending}
        />
      ) : null}
    </>
  );
}
