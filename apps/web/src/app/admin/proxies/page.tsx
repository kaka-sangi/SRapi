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
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
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
  useTestProxy,
  useBatchDeleteProxies,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
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
  const colVis = useColumnVisibility("admin-proxies", []);
  const statusFilter = (list.filters.status as ProxyDefinition["status"]) || undefined;
  const proxies = useAdminProxies({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
  });
  const createMut = useCreateProxy();
  const updateMut = useUpdateProxy();
  const deleteMut = useDeleteProxy();
  const testMut = useTestProxy();
  const [bulkTesting, setBulkTesting] = useState(false);

  async function runTest(id: string) {
    try {
      const result = await testMut.mutateAsync({ id });
      if (result.ok) {
        toast({
          title: t("adminProxies.testOk", { latency: result.latency_ms }),
          description: t("adminProxies.testTarget", { target: result.target_url }),
          tone: "success",
        });
      } else {
        toast({
          title: t("adminProxies.testFailed", {
            // Show the categorised error class verbatim — useful for triage.
            reason: result.error_class,
          }),
          description: t("adminProxies.testTarget", { target: result.target_url }),
          tone: "error",
        });
      }
    } catch (err) {
      toast({
        title: t("feedback.failed"),
        description: err instanceof Error ? err.message : String(err),
        tone: "error",
      });
    }
  }
  const batchDeleteMut = useBatchDeleteProxies();
  const { toast } = useToast();

  const [formTarget, setFormTarget] = useState<ProxyDefinition | "new" | null>(null);
  const [toDelete, setToDelete] = useState<ProxyDefinition | null>(null);
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);
  const isNew = formTarget === "new";

  // Bulk-test the selection. Each proxy.test call hits the network, so we cap
  // the concurrency at PROXY_TEST_CONCURRENCY to avoid lighting up the box
  // with N parallel TLS handshakes on big selections. allSettled keeps the
  // loop alive even if one mutation throws — we summarise everything at the
  // end rather than aborting on the first failure.
  async function bulkTest() {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    setBulkTesting(true);
    let okCount = 0;
    let failCount = 0;
    const PROXY_TEST_CONCURRENCY = 4;
    try {
      for (let i = 0; i < ids.length; i += PROXY_TEST_CONCURRENCY) {
        const slice = ids.slice(i, i + PROXY_TEST_CONCURRENCY);
        const results = await Promise.allSettled(
          slice.map((id) => testMut.mutateAsync({ id })),
        );
        for (const r of results) {
          if (r.status === "fulfilled" && r.value.ok) okCount += 1;
          else failCount += 1;
        }
      }
      if (failCount === 0) {
        toast({
          title: t("adminProxies.bulkTestOk", { count: okCount }),
          tone: "success",
        });
      } else if (okCount === 0) {
        toast({
          title: t("adminProxies.bulkTestAllFailed", { count: failCount }),
          tone: "error",
        });
      } else {
        toast({
          title: t("adminProxies.bulkTestPartial", { ok: okCount, fail: failCount }),
          tone: "warning",
        });
      }
    } finally {
      setBulkTesting(false);
    }
  }

  /** Atomic bulk-soft-delete via /admin/proxies/batch-delete. Per-id outcome
   * means partial failures show in the toast rather than silently rolling
   * back the whole call — same UX pattern as the accounts bulk-status flow. */
  async function applyBulkDelete() {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      const result = await batchDeleteMut.mutateAsync(ids);
      list.clearSelection();
      setBulkDeleteOpen(false);
      const failed = result.errors.length;
      const succeeded = result.deleted_count;
      if (failed > 0 && succeeded > 0) {
        toast({ title: t("feedback.batchPartial", { succeeded, failed }), tone: "warning" });
      } else if (failed > 0) {
        toast({ title: t("feedback.batchAllFailed", { count: ids.length }), tone: "error" });
      } else {
        toast({ title: t("feedback.batchAllSucceeded", { count: succeeded }), tone: "success" });
      }
    } catch (err) {
      toast({ title: t("feedback.failed"), description: String(err), tone: "error" });
    }
  }

  const fields: FieldConfig<ProxyFormState>[] = [
    { name: "name", label: t("adminProxies.name"), required: true },
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
    { name: "metadata", label: t("adminCommon.metadata"), help: t("adminCommon.metadataHelp"), type: "keyvalue", advanced: true },
  ];

  const columns: Column<ProxyDefinition>[] = [
    {
      key: "name",
      header: t("adminProxies.name"),
      pinned: true,
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
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminProxies.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={proxies}
        columns={columns}
        columnVisibility={colVis}
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
        selection={{
          selected: list.selected,
          onToggle: list.toggle,
          onTogglePage: list.togglePage,
          bulkActions: (
            <>
              <Button
                variant="outline"
                size="sm"
                loading={bulkTesting}
                onClick={() => void bulkTest()}
              >
                {t("adminProxies.bulkTest")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                loading={batchDeleteMut.isPending}
                onClick={() => setBulkDeleteOpen(true)}
              >
                {t("common.delete")}
              </Button>
            </>
          ),
        }}
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
              { label: t("adminProxies.test"), onSelect: () => void runTest(p.id) },
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

      <ConfirmDialog
        open={bulkDeleteOpen}
        onOpenChange={setBulkDeleteOpen}
        title={t("adminProxies.bulkDeleteTitle")}
        body={t("adminProxies.bulkDeleteBody", { count: list.selected.size })}
        confirmLabel={t("common.delete")}
        isPending={batchDeleteMut.isPending}
        onConfirm={applyBulkDelete}
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
