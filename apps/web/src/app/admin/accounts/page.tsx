"use client";

import { useEffect, useState, type ReactNode } from "react";
import type { UseQueryResult } from "@tanstack/react-query";
import { useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { LayoutGrid, List, RefreshCw, SearchX, Server, Timer } from "lucide-react";
import { useAutoRefresh } from "@/hooks/use-auto-refresh";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu, type RowAction } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { PageQueryState } from "@/components/layout/page-query-state";
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
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { EmptyState } from "@/components/ui/empty-state";
import { Pagination } from "@/components/ui/pagination";
import { Skeleton } from "@/components/ui/skeleton";
import { ADMIN_ROUTES } from "@/lib/routes";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { adminErrorMessage, type AdminListResult } from "@/lib/admin-api";
import {
  ACCOUNT_STATUSES,
  buildBatchAccountActionBody,
  type AccountBatchAction,
} from "@/lib/admin-account-form";
import { AccountImportDialog } from "@/components/admin/account-import-dialog";
import type { Provider, ProviderAccount, AccountHealthSnapshot } from "@/lib/sdk-types";
import { cn } from "@/lib/cn";
import { latestQuotaWindows, quotaWindowDisplayLabel, quotaWindowTiming } from "@/lib/quota-display";

type AccountListMode = "cards" | "table";

interface AccountSelection {
  selected: Set<string>;
  onToggle: (id: string) => void;
  onTogglePage: (ids: string[], checked: boolean) => void;
  bulkActions?: ReactNode;
}

interface AccountPagination {
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
}

const EMPTY_FILL = "min-h-[55vh] justify-center";

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
        defaultProviderId={providers.data?.data?.[0]?.id ?? ""}
      />
    </>
  );
}

function HealthSummaryStrip({
  healthById,
  total,
}: {
  healthById: Map<string, AccountHealthSnapshot>;
  total: number;
}) {
  if (healthById.size === 0 || total === 0) return null;
  const entries = [...healthById.values()];
  const healthy = entries.filter((h) => h.circuit_state === "closed" && h.success_rate >= 0.9).length;
  const degraded = entries.filter((h) => h.circuit_state === "closed" && h.success_rate < 0.9 && h.success_rate > 0).length;
  const tripped = entries.filter((h) => h.circuit_state !== "closed").length;
  const pctH = total > 0 ? (healthy / total) * 100 : 0;
  const pctD = total > 0 ? (degraded / total) * 100 : 0;
  const pctT = total > 0 ? (tripped / total) * 100 : 0;
  return (
    <div className="mb-4 flex items-center gap-3">
      <div className="relative h-2 flex-1 overflow-hidden rounded-full bg-srapi-border">
        <div
          className="absolute inset-y-0 left-0 rounded-full bg-srapi-success transition-all"
          style={{ width: `${pctH}%` }}
        />
        <div
          className="absolute inset-y-0 rounded-full bg-srapi-warning transition-all"
          style={{ left: `${pctH}%`, width: `${pctD}%` }}
        />
        <div
          className="absolute inset-y-0 rounded-full bg-srapi-error transition-all"
          style={{ left: `${pctH + pctD}%`, width: `${pctT}%` }}
        />
      </div>
      <span className="flex shrink-0 items-center gap-2 font-mono text-2xs text-srapi-text-tertiary">
        <span className="flex items-center gap-1">
          <span className="size-1.5 rounded-full bg-srapi-success" />{healthy}
        </span>
        {degraded > 0 && (
          <span className="flex items-center gap-1">
            <span className="size-1.5 rounded-full bg-srapi-warning" />{degraded}
          </span>
        )}
        {tripped > 0 && (
          <span className="flex items-center gap-1">
            <span className="size-1.5 rounded-full bg-srapi-error" />{tripped}
          </span>
        )}
      </span>
    </div>
  );
}

function AccountHealthCell({ health }: { health?: AccountHealthSnapshot }) {
  if (!health) return <span className="text-2xs text-srapi-text-tertiary">—</span>;
  const rate = health.success_rate;
  const circuit = health.circuit_state;
  const isOpen = circuit === "open";
  const isHalfOpen = circuit === "half-open";
  const p50 = Math.round(health.latency_p50_ms);
  return (
    <span className="flex items-center gap-1.5">
      <span
        className={cn(
          "inline-block size-1.5 rounded-full",
          isOpen ? "bg-srapi-error" : isHalfOpen ? "bg-srapi-warning" : rate >= 0.95 ? "bg-srapi-success" : rate >= 0.8 ? "bg-srapi-warning" : "bg-srapi-error",
        )}
      />
      <span className="font-mono text-2xs tabular text-srapi-text-secondary">
        {Math.round(rate * 100)}%
      </span>
      {p50 > 0 ? (
        <span className="font-mono text-[10px] tabular text-srapi-text-tertiary">{p50}ms</span>
      ) : null}
      {health.error_class ? (
        <span className="max-w-[6rem] truncate text-2xs text-srapi-error">{health.error_class}</span>
      ) : null}
      {health.cooldown_until ? (
        <span className="text-[10px] text-srapi-warning" title={health.cooldown_reason ?? undefined}>
          {health.cooldown_reason ?? "cooldown"}
        </span>
      ) : null}
    </span>
  );
}

function ViewModeToggle({
  mode,
  onChange,
}: {
  mode: AccountListMode;
  onChange: (mode: AccountListMode) => void;
}) {
  const { t } = useLanguage();
  return (
    <div className="inline-flex h-9 rounded-lg border border-srapi-border-strong bg-srapi-card p-0.5">
      <Button
        type="button"
        variant={mode === "cards" ? "outline" : "ghost"}
        size="sm"
        className={cn(
          "h-7 rounded-md border-0 px-2.5 shadow-none",
          mode !== "cards" && "text-srapi-text-secondary",
        )}
        aria-pressed={mode === "cards"}
        onClick={() => onChange("cards")}
      >
        <LayoutGrid className="size-3.5" />
        <span className="hidden sm:inline">{t("adminAccounts.viewCards")}</span>
      </Button>
      <Button
        type="button"
        variant={mode === "table" ? "outline" : "ghost"}
        size="sm"
        className={cn(
          "h-7 rounded-md border-0 px-2.5 shadow-none",
          mode !== "table" && "text-srapi-text-secondary",
        )}
        aria-pressed={mode === "table"}
        onClick={() => onChange("table")}
      >
        <List className="size-3.5" />
        <span className="hidden sm:inline">{t("adminAccounts.viewTable")}</span>
      </Button>
    </div>
  );
}

function AutoRefreshButton({
  autoRefresh,
}: {
  autoRefresh: ReturnType<typeof useAutoRefresh>;
}) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  return (
    <div className="relative">
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setOpen((v) => !v)}
        className={cn(autoRefresh.enabled && "border-srapi-success/40")}
      >
        {autoRefresh.enabled ? (
          <RefreshCw className="size-3.5 animate-spin text-srapi-success" style={{ animationDuration: `${autoRefresh.interval}s` }} />
        ) : (
          <Timer className="size-3.5" />
        )}
        <span className="hidden sm:inline">
          {autoRefresh.enabled
            ? `${autoRefresh.timeUntilRefresh}s`
            : t("common.autoRefresh")}
        </span>
      </Button>
      {open ? (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute right-0 z-50 mt-1.5 w-44 rounded-lg border border-srapi-border bg-srapi-card p-1.5 shadow-lg">
            <button
              type="button"
              onClick={() => { autoRefresh.toggle(); setOpen(false); }}
              className="flex w-full items-center justify-between rounded-md px-3 py-2 text-xs text-srapi-text-secondary transition-colors hover:bg-srapi-card-muted"
            >
              <span>{autoRefresh.enabled ? t("common.off") : t("common.autoRefresh")}</span>
              {autoRefresh.enabled ? (
                <span className="size-1.5 rounded-full bg-srapi-success" />
              ) : null}
            </button>
            <div className="my-1 border-t border-srapi-border" />
            {autoRefresh.intervalOptions.map((sec) => (
              <button
                key={sec}
                type="button"
                onClick={() => { autoRefresh.setInterval(sec); if (!autoRefresh.enabled) autoRefresh.toggle(); setOpen(false); }}
                className={cn(
                  "flex w-full items-center justify-between rounded-md px-3 py-1.5 text-xs transition-colors hover:bg-srapi-card-muted",
                  autoRefresh.interval === sec && autoRefresh.enabled
                    ? "font-medium text-srapi-text-primary"
                    : "text-srapi-text-tertiary",
                )}
              >
                <span>{sec}s</span>
                {autoRefresh.interval === sec && autoRefresh.enabled ? (
                  <span className="text-2xs text-srapi-success">&#10003;</span>
                ) : null}
              </button>
            ))}
          </div>
        </>
      ) : null}
    </div>
  );
}

function AccountsCardView({
  query,
  providerNameById,
  healthById,
  toolbar,
  selection,
  pagination,
  isFiltered,
  onClearFilters,
  emptyAction,
  onDetail,
  renderActions,
  renderStatus,
}: {
  query: UseQueryResult<AdminListResult<ProviderAccount>>;
  providerNameById: Map<string, string>;
  healthById: Map<string, AccountHealthSnapshot>;
  toolbar: ReactNode;
  selection?: AccountSelection;
  pagination: AccountPagination;
  isFiltered: boolean;
  onClearFilters: () => void;
  emptyAction?: ReactNode;
  onDetail?: (account: ProviderAccount) => void;
  renderActions: (account: ProviderAccount) => ReactNode;
  renderStatus: (account: ProviderAccount) => ReactNode;
}) {
  const { t } = useLanguage();

  return (
    <Card className="anim-rise-sm overflow-hidden">
      {toolbar}
      {selection && selection.selected.size > 0 ? (
        <AccountBulkBar
          count={selection.selected.size}
          onClear={() => selection.onTogglePage([...selection.selected], false)}
        >
          {selection.bulkActions}
        </AccountBulkBar>
      ) : null}
      <PageQueryState
        query={query}
        isEmpty={(d) => d.data.length === 0}
        skeleton={<AccountCardSkeleton />}
      >
        {(data) =>
          data.data.length === 0 ? (
            isFiltered ? (
              <EmptyState
                className={EMPTY_FILL}
                icon={SearchX}
                title={t("adminCommon.noResults")}
                description={t("adminCommon.noResultsBody")}
                action={
                  <Button variant="outline" size="sm" onClick={onClearFilters}>
                    {t("adminCommon.clearFilters")}
                  </Button>
                }
              />
            ) : (
              <EmptyState
                className={EMPTY_FILL}
                icon={Server}
                title={t("adminAccounts.emptyTitle")}
                description={t("adminAccounts.emptyBody")}
                action={emptyAction}
              />
            )
          ) : (
            <AccountCardGrid
              accounts={data.data}
              providerNameById={providerNameById}
              healthById={healthById}
              selection={selection}
              onDetail={onDetail}
              renderActions={renderActions}
              renderStatus={renderStatus}
            />
          )
        }
      </PageQueryState>
      {pagination.total > pagination.pageSize ? (
        <div className="border-t border-srapi-border">
          <Pagination
            page={pagination.page}
            pageSize={pagination.pageSize}
            total={pagination.total}
            onPageChange={pagination.onPageChange}
            labelFor={(from, to, total) => t("adminCommon.pageLabel", { from, to, total })}
          />
        </div>
      ) : null}
    </Card>
  );
}

function AccountCardGrid({
  accounts,
  providerNameById,
  healthById,
  selection,
  onDetail,
  renderActions,
  renderStatus,
}: {
  accounts: ProviderAccount[];
  providerNameById: Map<string, string>;
  healthById: Map<string, AccountHealthSnapshot>;
  selection?: AccountSelection;
  onDetail?: (account: ProviderAccount) => void;
  renderActions: (account: ProviderAccount) => ReactNode;
  renderStatus: (account: ProviderAccount) => ReactNode;
}) {
  const pageIds = accounts.map((a) => a.id);
  const allOnPage = pageIds.length > 0 && pageIds.every((id) => selection?.selected.has(id));
  const someOnPage = pageIds.some((id) => selection?.selected.has(id));

  return (
    <div>
      {selection ? (
        <div className="flex items-center gap-2 border-b border-srapi-border px-4 py-2.5">
          <Checkbox
            aria-label="select all"
            checked={allOnPage}
            indeterminate={!allOnPage && someOnPage}
            onChange={(e) => selection.onTogglePage(pageIds, e.target.checked)}
          />
        </div>
      ) : null}
      <div className="grid gap-3 p-3 sm:grid-cols-2 xl:grid-cols-3">
        {accounts.map((account) => (
          <AccountCard
            key={account.id}
            account={account}
            providerName={providerNameById.get(String(account.provider_id)) || account.provider_id}
            health={healthById.get(account.id)}
            selected={selection?.selected.has(account.id) ?? false}
            onSelect={selection ? () => selection.onToggle(account.id) : undefined}
            onDetail={onDetail ? () => onDetail(account) : undefined}
            actions={renderActions(account)}
            status={renderStatus(account)}
          />
        ))}
      </div>
    </div>
  );
}

const PROVIDER_ACCENT: Record<string, string> = {
  anthropic: "border-l-orange-400",
  openai: "border-l-emerald-500",
  deepseek: "border-l-blue-500",
  gemini: "border-l-sky-400",
  groq: "border-l-amber-500",
  mistral: "border-l-indigo-400",
  openrouter: "border-l-violet-500",
  codex: "border-l-emerald-400",
  grok: "border-l-slate-400",
  kimi: "border-l-teal-400",
  moonshot: "border-l-teal-400",
  qwen: "border-l-purple-400",
  together: "border-l-rose-400",
  cerebras: "border-l-cyan-400",
};

function providerAccent(name: string): string {
  const lower = name.toLowerCase();
  for (const [key, cls] of Object.entries(PROVIDER_ACCENT)) {
    if (lower.includes(key)) return cls;
  }
  return "border-l-srapi-border";
}

function AccountCard({
  account,
  providerName,
  health,
  selected,
  onSelect,
  onDetail,
  actions,
  status,
}: {
  account: ProviderAccount;
  providerName: string;
  health?: AccountHealthSnapshot;
  selected: boolean;
  onSelect?: () => void;
  onDetail?: () => void;
  actions: ReactNode;
  status: ReactNode;
}) {
  const { t } = useLanguage();
  const hasPriority = account.priority != null && account.priority !== 0;
  const hasWeight = account.weight != null && account.weight !== 1;
  return (
    <article
      className={cn(
        "rounded-lg border border-srapi-border border-l-[3px] bg-srapi-card px-4 py-3.5 transition-colors",
        providerAccent(providerName),
        account.status === "disabled" && "opacity-60",
        selected && "border-srapi-primary/50 bg-srapi-card-muted",
        onDetail && "cursor-pointer hover:bg-srapi-card-muted/50",
      )}
      onClick={(e) => {
        if (!onDetail) return;
        const target = e.target as HTMLElement;
        if (target.closest("button, input, a, [role=menuitem]")) return;
        onDetail();
      }}
    >
      <div className="flex items-start gap-3">
        {onSelect ? (
          <Checkbox
            aria-label="select row"
            checked={selected}
            onChange={() => onSelect()}
            className="mt-1"
          />
        ) : null}
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h3 className="truncate text-sm font-medium text-srapi-text-primary">{account.name}</h3>
              <p className="mt-1 truncate text-xs text-srapi-text-secondary">{providerName}</p>
              {metadataString(account.metadata, "base_url") ? (
                <p className="mt-0.5 truncate font-mono text-2xs text-srapi-text-tertiary">
                  {metadataString(account.metadata, "base_url")}
                </p>
              ) : null}
            </div>
            <div className="shrink-0">{actions}</div>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            {status}
            <span className="font-mono text-2xs text-srapi-text-tertiary">{account.runtime_class}</span>
            {hasPriority || hasWeight ? (
              <span className="rounded-md bg-srapi-bg-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary">
                {hasPriority ? `P${account.priority}` : ""}
                {hasPriority && hasWeight ? " · " : ""}
                {hasWeight ? `W${account.weight}` : ""}
              </span>
            ) : null}
          </div>
        </div>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-3 border-t border-srapi-border/70 pt-3">
        <AccountCardMetric label={t("adminAccounts.healthTitle")}>
          <AccountHealthCell health={health} />
        </AccountCardMetric>
        <AccountCardMetric label={t("adminAccounts.quotaTitle")}>
          <AccountQuotaCell health={health} />
        </AccountCardMetric>
      </div>
    </article>
  );
}

function AccountCardMetric({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="mb-1.5 text-2xs text-srapi-text-tertiary">{label}</div>
      {children}
    </div>
  );
}

function AccountBulkBar({
  count,
  onClear,
  children,
}: {
  count: number;
  onClear: () => void;
  children?: ReactNode;
}) {
  const { t } = useLanguage();
  return (
    <div className="anim-rise-sm flex flex-wrap items-center gap-3 border-b border-srapi-border bg-srapi-card-muted px-4 py-2.5">
      <span className="font-mono text-2xs text-srapi-text-secondary">
        {t("adminCommon.selectedCount", { count })}
      </span>
      <button
        type="button"
        onClick={onClear}
        className="text-2xs text-srapi-text-tertiary underline-offset-2 hover:text-srapi-text-primary hover:underline"
      >
        {t("adminCommon.clearSelection")}
      </button>
      <div className="ml-auto flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

function AccountCardSkeleton() {
  return (
    <div className="min-h-[55vh] p-3">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="rounded-lg border border-l-[3px] border-srapi-border px-4 py-3.5">
            <div className="flex items-start justify-between gap-3">
              <div className="space-y-2">
                <Skeleton className="h-4 w-36" />
                <Skeleton className="h-3 w-24" />
              </div>
              <Skeleton className="h-8 w-8" />
            </div>
            <div className="mt-3 flex gap-2">
              <Skeleton className="h-6 w-16" />
              <Skeleton className="h-6 w-20" />
            </div>
            <div className="mt-4 grid grid-cols-2 gap-3 border-t border-srapi-border/70 pt-3">
              <Skeleton className="h-8 w-full" />
              <Skeleton className="h-8 w-full" />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function AccountStatusCell({
  account,
  busy = false,
  onToggle,
}: {
  account: ProviderAccount;
  busy?: boolean;
  onToggle?: () => void;
}) {
  const { t } = useLanguage();
  const quotaClass = metadataString(account.metadata, "last_quota_error_class");
  const validationURL = metadataString(account.metadata, "validation_url");
  const tone = quietStatusFor(account.status);
  const label = statusLabel(t, account.status);
  const actionLabel = account.status === "disabled" ? t("common.enable") : t("common.disable");

  const statusBadge = onToggle ? (
    <button
      type="button"
      disabled={busy}
      aria-label={actionLabel}
      title={actionLabel}
      onClick={onToggle}
      className={cn(
        "inline-flex h-7 items-center gap-1.5 rounded-md border border-srapi-border px-2 font-mono text-2xs text-srapi-text-secondary transition-colors hover:border-srapi-text-tertiary hover:bg-srapi-card-muted hover:text-srapi-text-primary disabled:pointer-events-none disabled:opacity-50",
      )}
    >
      <span
        aria-hidden
        className={cn(
          "text-[0.7em] leading-none",
          tone === "active"
            ? "text-srapi-success"
            : tone === "limited"
              ? "text-srapi-warning"
              : tone === "error"
                ? "text-srapi-error"
                : "text-srapi-text-tertiary",
        )}
      >
        {tone === "active" ? "●" : tone === "disabled" ? "○" : "■"}
      </span>
      {label}
    </button>
  ) : (
    <QuietBadge status={tone} label={label} />
  );

  return (
    <span className="flex flex-wrap items-center gap-1.5">
      {statusBadge}
      {quotaClass ? (
        <QuietBadge
          status={quotaClass === "validation_required" ? "limited" : "error"}
          label={quotaClass === "validation_required" ? t("adminAccounts.validationRequired") : quotaClass}
        />
      ) : null}
      {validationURL ? (
        <a
          href={validationURL}
          target="_blank"
          rel="noreferrer"
          className="text-2xs text-srapi-primary hover:underline"
        >
          {t("adminAccounts.validationLink")}
        </a>
      ) : null}
    </span>
  );
}

function AccountQuotaCell({ health }: { health?: AccountHealthSnapshot }) {
  const { t } = useLanguage();
  if (!health) return <span className="text-2xs text-srapi-text-tertiary">—</span>;
  const windows = latestQuotaWindows(health.quota_windows ?? []);
  if (windows.length > 0) {
    const title = windows
      .map(
        (window) =>
          `${quotaWindowDisplayLabel(window, t)} ${Math.round(window.remainingPercent)}% · ${quotaWindowTiming(window, t)}`,
      )
      .join("\n");
    return (
      <span className="flex min-w-44 flex-col gap-1" title={title}>
        {windows.map((window) => {
          const ratio = window.remainingPercent / 100;
          const exhausted = window.remainingPercent <= 0;
          const pct = Math.round(window.remainingPercent);
          return (
            <span
              key={window.snapshot.quota_type}
              className="grid grid-cols-[3rem_minmax(4rem,1fr)_2.5rem] items-center gap-1.5"
            >
              <span className="truncate font-mono text-[10px] uppercase leading-none text-srapi-text-tertiary">
                {quotaWindowDisplayLabel(window, t)}
              </span>
              <span className="relative h-1.5 min-w-16 overflow-hidden rounded-full bg-srapi-border">
                <span
                  className={cn(
                    "absolute inset-y-0 left-0 rounded-full transition-all",
                    exhausted
                      ? "bg-srapi-error"
                      : ratio <= 0.2
                        ? "bg-srapi-warning"
                        : "bg-srapi-success",
                  )}
                  style={{ width: `${Math.max(pct, 2)}%` }}
                />
              </span>
              <span
                className={cn(
                  "text-right font-mono text-[10px] leading-none tabular text-srapi-text-tertiary",
                  exhausted
                    ? "text-srapi-error"
                    : window.remainingPercent <= 20
                      ? "text-srapi-warning"
                      : undefined,
                )}
              >
                {pct}%
              </span>
            </span>
          );
        })}
      </span>
    );
  }
  const ratio = health.quota_remaining_ratio;
  const exhausted = health.quota_exhausted;
  const pct = Math.round(ratio * 100);
  return (
    <span className="flex items-center gap-1.5">
      <span className="relative h-1.5 w-12 overflow-hidden rounded-full bg-srapi-border">
        <span
          className={cn(
            "absolute inset-y-0 left-0 rounded-full transition-all",
            exhausted ? "bg-srapi-error" : ratio <= 0.2 ? "bg-srapi-warning" : "bg-srapi-success",
          )}
          style={{ width: `${Math.max(pct, 2)}%` }}
        />
      </span>
      <span className={cn(
        "font-mono text-2xs tabular",
        exhausted ? "text-srapi-error" : ratio <= 0.2 ? "text-srapi-warning" : "text-srapi-text-secondary",
      )}>
        {pct}%
      </span>
    </span>
  );
}

function hasQuotaError(account: ProviderAccount): boolean {
  return Boolean(metadataString(account.metadata, "last_quota_error_class"));
}

function metadataString(metadata: ProviderAccount["metadata"], key: string): string {
  const value = metadata?.[key];
  return typeof value === "string" ? value.trim() : "";
}
