"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { RefreshCw, Server } from "lucide-react";
import { useAutoRefresh } from "@/hooks/use-auto-refresh";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu, type RowAction } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { enumOptions } from "@/components/admin/resource-form-dialog";
import { AccountFormDialog } from "@/components/admin/account-form-dialog";
import { BindProxyDialog } from "@/components/admin/bind-proxy-dialog";
import { AccountDetailSheet } from "@/components/admin/account-detail-sheet";
import { AccountTestDialog } from "@/components/features/account-test-dialog";
import {
  useAdminAccounts,
  useAdminModels,
  useAdminProviders,
  useAdminProxies,
  useSetAccountStatus,
  useTestAccount,
  useCreateAccount,
  useUpdateAccount,
  useClearAccountError,
  useRecoverAccount,
  useResetAccountQuota,
  useBatchActionAccounts,
  useDeleteAccount,
  useDiscoverAccountModels,
  useExportAccounts,
  useAccountsHealthSummary,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { ADMIN_ROUTES } from "@/lib/routes";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  ACCOUNT_STATUSES,
  buildBatchAccountActionBody,
  type AccountBatchAction,
} from "@/lib/admin-account-form";
import { AccountImportDialog } from "@/components/admin/account-import-dialog";
import type { Provider, ProviderAccount } from "@/lib/sdk-types";
import { type AccountListMode, metadataString } from "./account-types";
import { AccountHealthCell, AccountQuotaCell, HealthSummaryStrip } from "./account-health-cells";
import { AccountStatusCell } from "./account-status-cell";
import { AutoRefreshButton, ViewModeToggle } from "./accounts-toolbar";
import { AccountsCardView } from "./account-card";

function extractAccountTemplate(p: Provider) {
  const schema = p.config_schema as Record<string, unknown> | undefined;
  const tpl = schema?.account_template;
  return tpl && typeof tpl === "object" ? (tpl as Record<string, unknown>) : null;
}

export default function AdminAccountsPage() {
  return (
    <AdminShell>
      <AccountsContent />
    </AdminShell>
  );
}

/** Statuses that represent an error/parked account that an admin can clear or recover. */
function isRecoverable(status: ProviderAccount["status"]): boolean {
  return status !== "active" && status !== "disabled";
}

function downloadJson(filename: string, data: unknown) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

function AccountsContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const qc = useQueryClient();
  const list = useAdminList();
  const searchParams = useSearchParams();
  const readOnlyHealthView = searchParams.get("view") === "health";
  const colVis = useColumnVisibility("admin-accounts", ["created_at", "updated_at", "notes"]);

  const autoRefresh = useAutoRefresh(
    () => void qc.invalidateQueries({ queryKey: ["admin"] }),
    { storageKey: "admin-accounts", defaultInterval: 30 },
  );

  const statusFilter =
    (list.filters.status as ProviderAccount["status"]) || (readOnlyHealthView ? "active" : undefined);
  const providerFilter = list.filters.providerId || undefined;
  const accounts = useAdminAccounts({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
    provider_id: providerFilter,
  });
  const models = useAdminModels({ page: 1, page_size: 200, status: "active" });
  const providers = useAdminProviders();
  const proxies = useAdminProxies();
  const setStatus = useSetAccountStatus();
  const test = useTestAccount();
  const createMut = useCreateAccount();
  const updateMut = useUpdateAccount();
  const clearErr = useClearAccountError();
  const recover = useRecoverAccount();
  const resetQuota = useResetAccountQuota();
  const batchAction = useBatchActionAccounts();
  const deleteMut = useDeleteAccount();
  const discover = useDiscoverAccountModels();
  const exportMut = useExportAccounts();
  const healthSummary = useAccountsHealthSummary();
  const healthById = new Map(
    (healthSummary.data ?? []).map((h) => [h.account_id, h] as const),
  );

  const [formTarget, setFormTarget] = useState<ProviderAccount | "new" | null>(null);
  const [proxyTarget, setProxyTarget] = useState<ProviderAccount | null>(null);
  const [detailTarget, setDetailTarget] = useState<ProviderAccount | null>(null);
  const [testTarget, setTestTarget] = useState<ProviderAccount | null>(null);
  const [bulkDisableOpen, setBulkDisableOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ProviderAccount | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [listMode, setListMode] = useState<AccountListMode>("cards");

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (e.key === "n" && !readOnlyHealthView) { e.preventDefault(); setFormTarget("new"); }
      if (e.key === "r") { e.preventDefault(); void qc.invalidateQueries({ queryKey: ["admin"] }); }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [qc, readOnlyHealthView]);

  const providerOptions = (providers.data?.data ?? []).map((p) => ({
    value: p.id,
    label: p.display_name || p.name,
    platformFamily: p.platform_family ?? null,
    authMethods: p.auth_methods ?? null,
    adapterType: p.adapter_type,
    accountTemplate: extractAccountTemplate(p),
  }));
  const providerNameById = new Map(
    (providers.data?.data ?? []).map((p) => [String(p.id), p.display_name || p.name] as const),
  );
  const proxyOptions = (proxies.data?.data ?? []).map((p) => ({ value: p.id, label: p.name }));
  const isFiltered = Boolean(statusFilter || providerFilter);

  /** Apply a status to every selected account, reporting partial failures. */
  async function applyBulkStatus(status: "active" | "disabled") {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    const results = await Promise.allSettled(
      ids.map((id) => setStatus.mutateAsync({ id, status })),
    );
    const failed = results.filter((r) => r.status === "rejected").length;
    const succeeded = ids.length - failed;
    list.clearSelection();
    if (failed > 0 && succeeded > 0) {
      toast({ title: t("feedback.batchPartial", { succeeded, failed }), tone: "warning" });
    } else if (failed > 0) {
      toast({ title: t("feedback.batchAllFailed", { count: ids.length }), tone: "error" });
    } else {
      toast({ title: t("feedback.batchAllSucceeded", { count: ids.length }), tone: "success" });
    }
  }

  /** Run a per-account maintenance action (clear_error / recover) across the selection. */
  async function applyBulkAction(action: AccountBatchAction) {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      const result = await batchAction.mutateAsync(buildBatchAccountActionBody({ accountIds: ids, action }));
      list.clearSelection();
      const failedCount = result.errors.length;
      const succeededCount = result.updated_count;
      if (failedCount > 0 && succeededCount > 0) {
        toast({
          title: t("feedback.batchPartial", { succeeded: succeededCount, failed: failedCount }),
          tone: "warning",
        });
      } else if (failedCount > 0) {
        toast({ title: t("feedback.batchAllFailed", { count: ids.length }), tone: "error" });
      } else {
        toast({ title: t("feedback.batchAllSucceeded", { count: succeededCount }), tone: "success" });
      }
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function runAction(fn: () => Promise<unknown>, okTitle: string) {
    try {
      await fn();
      toast({ title: okTitle, tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function runDiscover(id: string) {
    try {
      const result = await discover.mutateAsync({ id });
      toast({
        title: t("adminAccounts.discoverDone", { count: result.model_ids.length }),
        tone: "success",
      });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function runExport() {
    try {
      const data = await exportMut.mutateAsync();
      downloadJson("srapi-accounts.json", data);
      toast({ title: t("adminAccounts.exportDone", { count: data.length }), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  function toggleAccountStatus(account: ProviderAccount) {
    void runAction(
      () =>
        setStatus.mutateAsync({
          id: account.id,
          status: account.status === "disabled" ? "active" : "disabled",
        }),
      t("feedback.saved"),
    );
  }

  const columns: Column<ProviderAccount>[] = [
    {
      key: "name",
      header: t("adminAccounts.name"),
      pinned: true,
      sortValue: (a) => a.name,
      render: (a) => <span className="text-srapi-text-primary">{a.name}</span>,
    },
    {
      key: "provider",
      header: t("adminAccounts.provider"),
      sortValue: (a) => providerNameById.get(String(a.provider_id)) || a.provider_id,
      render: (a) => (
        <span className="text-srapi-text-secondary">
          {providerNameById.get(String(a.provider_id)) || a.provider_id}
        </span>
      ),
    },
    {
      key: "base_url",
      header: t("adminAccounts.baseUrl"),
      hideOnMobile: true,
      sortValue: (a) => metadataString(a.metadata, "base_url"),
      render: (a) => {
        const url = metadataString(a.metadata, "base_url");
        return url ? (
          <span className="max-w-[12rem] truncate font-mono text-2xs text-srapi-text-tertiary" title={url}>{url}</span>
        ) : <span className="text-2xs text-srapi-text-tertiary">—</span>;
      },
    },
    {
      key: "type",
      header: t("adminAccounts.type"),
      hideOnMobile: true,
      render: (a) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{a.runtime_class}</span>
      ),
    },
    {
      key: "status",
      header: t("common.active"),
      render: (a) => (
        <AccountStatusCell
          account={a}
          busy={setStatus.isPending}
          onToggle={readOnlyHealthView ? undefined : () => toggleAccountStatus(a)}
        />
      ),
    },
    {
      key: "health",
      header: t("adminAccounts.healthTitle"),
      hideOnMobile: true,
      sortValue: (a) => healthById.get(a.id)?.success_rate ?? -1,
      render: (a) => <AccountHealthCell health={healthById.get(a.id)} />,
    },
    {
      key: "quota",
      header: t("adminAccounts.quotaTitle"),
      hideOnMobile: true,
      sortValue: (a) => healthById.get(a.id)?.quota_remaining_ratio ?? -1,
      render: (a) => <AccountQuotaCell health={healthById.get(a.id)} />,
    },
  ];

  const bulkActions = (
    <>
      <Button
        variant="outline"
        size="sm"
        loading={setStatus.isPending}
        onClick={() => void applyBulkStatus("active")}
      >
        {t("common.enable")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        loading={setStatus.isPending}
        onClick={() => setBulkDisableOpen(true)}
      >
        {t("common.disable")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        loading={batchAction.isPending}
        onClick={() => void applyBulkAction("clear_error")}
      >
        {t("adminAccounts.clearError")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        loading={batchAction.isPending}
        onClick={() => void applyBulkAction("recover")}
      >
        {t("adminAccounts.recover")}
      </Button>
    </>
  );

  const selection = readOnlyHealthView
    ? undefined
    : {
        selected: list.selected,
        onToggle: list.toggle,
        onTogglePage: list.togglePage,
        bulkActions,
      };

  const pagination = {
    page: list.page,
    pageSize: list.pageSize,
    total: accounts.data?.pagination?.total ?? accounts.data?.data.length ?? 0,
    onPageChange: list.setPage,
  };

  const toolbar = (
    <ListToolbar>
      <FilterSelect
        value={statusFilter}
        onChange={(v) => list.setFilter("status", v)}
        options={enumOptions(ACCOUNT_STATUSES)}
        allLabel={t("adminCommon.allStatuses")}
      />
      {providerOptions.length > 0 ? (
        <FilterSelect
          value={providerFilter}
          onChange={(v) => list.setFilter("providerId", v)}
          options={providerOptions}
          allLabel={t("adminAccounts.allProviders")}
        />
      ) : null}
      <div className="flex items-center gap-2 sm:ml-auto">
        <ViewModeToggle mode={listMode} onChange={setListMode} />
        {listMode === "table" ? (
          <ColumnToggle
            columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
            visibility={colVis}
          />
        ) : null}
      </div>
    </ListToolbar>
  );

  function renderRowActions(a: ProviderAccount) {
    const actions: RowAction[] = [
      { label: t("adminAccounts.details"), onSelect: () => setDetailTarget(a) },
      {
        label: t("adminAccounts.test"),
        onSelect: () => {
          test.reset();
          setTestTarget(a);
        },
      },
    ];
    if (readOnlyHealthView) {
      return <RowActionsMenu actions={actions} />;
    }
    actions.splice(1, 0, { label: t("adminAccounts.edit"), onSelect: () => setFormTarget(a) });
    actions.push(
      {
        label: t("adminAccounts.setPriority"),
        onSelect: () => {
          const input = prompt(t("adminAccounts.setPriorityPrompt"), String(a.priority ?? 0));
          if (input === null) return;
          const val = parseInt(input, 10);
          if (Number.isNaN(val)) return;
          void runAction(
            () => updateMut.mutateAsync({ id: a.id, body: { priority: val } }),
            t("feedback.saved"),
          );
        },
      },
      { label: t("adminAccounts.discoverModels"), onSelect: () => void runDiscover(a.id) },
      { label: t("adminAccounts.bindProxy"), onSelect: () => setProxyTarget(a) },
    );
    if (isRecoverable(a.status)) {
      actions.push(
        {
          label: t("adminAccounts.clearError"),
          onSelect: () =>
            void runAction(() => clearErr.mutateAsync(a.id), t("feedback.saved")),
        },
        {
          label: t("adminAccounts.recover"),
          onSelect: () =>
            void runAction(() => recover.mutateAsync(a.id), t("feedback.saved")),
        },
      );
    }
    if (hasQuotaError(a)) {
      actions.push({
        label: t("adminAccounts.quotaReset"),
        onSelect: () =>
          void runAction(() => resetQuota.mutateAsync(a.id), t("adminAccounts.quotaResetDone")),
      });
    }
    actions.push({
      label: t("common.delete"),
      destructive: true,
      onSelect: () => setDeleteTarget(a),
    });
    return <RowActionsMenu actions={actions} />;
  }

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminAccounts.title")}
        description={readOnlyHealthView ? t("adminAccounts.healthViewSubtitle") : t("adminAccounts.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {accounts.data ? (
              <ListCount total={accounts.data.pagination?.total ?? accounts.data.data.length} />
            ) : null}
            {accounts.isFetching ? (
              <RefreshCw className="size-3 animate-spin text-srapi-text-tertiary" />
            ) : null}
            <AutoRefreshButton autoRefresh={autoRefresh} />
            {readOnlyHealthView ? (
              <QuietBadge status="active" label={t("adminAccounts.healthView")} />
            ) : (
              <>
                <Button
                  variant="outline"
                  size="sm"
                  loading={exportMut.isPending}
                  onClick={() => void runExport()}
                >
                  {t("adminAccounts.export")}
                </Button>
                <Button variant="outline" size="sm" onClick={() => setImportOpen(true)}>
                  {t("batchAdd.tab")} / {t("adminAccounts.importAction")}
                </Button>
                <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
                  + {t("adminAccounts.create")}
                </Button>
              </>
            )}
          </div>
        }
      />
      <HealthSummaryStrip healthById={healthById} total={accounts.data?.data.length ?? 0} />

      {listMode === "cards" ? (
        <AccountsCardView
          query={accounts}
          providerNameById={providerNameById}
          healthById={healthById}
          toolbar={toolbar}
          selection={selection}
          pagination={pagination}
          isFiltered={isFiltered}
          onClearFilters={list.clearFilters}
          emptyAction={
            readOnlyHealthView ? undefined : (
              <div className="flex gap-2">
                <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
                  + {t("adminAccounts.create")}
                </Button>
                <Button variant="outline" size="sm" asChild>
                  <a href={ADMIN_ROUTES.quickSetup}>{t("adminAccounts.emptyQuickSetup")}</a>
                </Button>
              </div>
            )
          }
          onDetail={setDetailTarget}
          renderActions={renderRowActions}
          renderStatus={(a) => (
            <AccountStatusCell
              account={a}
              busy={setStatus.isPending}
              onToggle={readOnlyHealthView ? undefined : () => toggleAccountStatus(a)}
            />
          )}
        />
      ) : (
        <AdminListView
          query={accounts}
          columns={columns}
          columnVisibility={colVis}
          getRowId={(a) => a.id}
          emptyIcon={Server}
          emptyTitle={t("adminAccounts.emptyTitle")}
          emptyBody={t("adminAccounts.emptyBody")}
          emptyAction={
            readOnlyHealthView ? undefined : (
              <div className="flex gap-2">
                <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
                  + {t("adminAccounts.create")}
                </Button>
                <Button variant="outline" size="sm" asChild>
                  <a href={ADMIN_ROUTES.quickSetup}>{t("adminAccounts.emptyQuickSetup")}</a>
                </Button>
              </div>
            )
          }
          dimRow={(a) => a.status === "disabled"}
          isFiltered={isFiltered}
          onClearFilters={list.clearFilters}
          sort={list.sort}
          onSort={list.toggleSort}
          toolbar={toolbar}
          pagination={pagination}
          selection={selection}
          rowActions={renderRowActions}
        />
      )}

      {formTarget === "new" ? (
        <AccountFormDialog
          open
          mode="create"
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          providerOptions={providerOptions}
          defaultProviderId={providers.data?.data?.[0]?.id ?? ""}
          submit={(body) =>
            createMut.mutateAsync(body as Parameters<typeof createMut.mutateAsync>[0])
          }
          isPending={createMut.isPending}
        />
      ) : formTarget ? (
        <AccountFormDialog
          open
          mode="edit"
          target={formTarget}
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          providerOptions={providerOptions}
          defaultProviderId={formTarget.provider_id}
          submit={(body) =>
            updateMut.mutateAsync({
              id: formTarget.id,
              body: body as Parameters<typeof updateMut.mutateAsync>[0]["body"],
            })
          }
          isPending={updateMut.isPending}
        />
      ) : null}

      {testTarget ? (
        <AccountTestDialog
          open
          onOpenChange={(open) => {
            if (!open) {
              setTestTarget(null);
              test.reset();
            }
          }}
          accountName={testTarget.name}
          models={models.data?.data ?? []}
          onRun={(body) => test.mutate({ id: testTarget.id, body })}
          result={test.data}
          errorMessage={test.error instanceof Error ? test.error.message : null}
          isPending={test.isPending}
        />
      ) : null}

      {proxyTarget ? (
        <BindProxyDialog
          open
          account={proxyTarget}
          proxyOptions={proxyOptions}
          onOpenChange={(open) => {
            if (!open) setProxyTarget(null);
          }}
        />
      ) : null}

      <AccountDetailSheet
        account={detailTarget}
        onOpenChange={(open) => {
          if (!open) setDetailTarget(null);
        }}
      />

      <ConfirmDialog
        open={bulkDisableOpen}
        onOpenChange={setBulkDisableOpen}
        title={t("adminAccounts.bulkDisableTitle", { count: list.selected.size })}
        body={t("adminAccounts.bulkDisableBody")}
        confirmLabel={t("common.disable")}
        onConfirm={() => applyBulkStatus("disabled")}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("adminAccounts.deleteTitle")}
        body={t("adminAccounts.deleteBody")}
        confirmLabel={t("common.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteMut.isPending}
        onConfirm={async () => {
          if (deleteTarget) await deleteMut.mutateAsync(deleteTarget.id);
        }}
      />

      <AccountImportDialog
        open={importOpen}
        onOpenChange={setImportOpen}
        providerOptions={providerOptions.map((o) => ({ value: o.value, label: o.label }))}
        codexProviderOptions={providerOptions
          .filter((o) => o.adapterType === "reverse-proxy-codex-cli")
          .map((o) => ({ value: o.value, label: o.label }))}
        defaultProviderId={providers.data?.data?.[0]?.id ?? ""}
      />
    </>
  );
}

function hasQuotaError(account: ProviderAccount): boolean {
  return Boolean(metadataString(account.metadata, "last_quota_error_class"));
}
