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
  useAdminGroups,
  useAdminModels,
  useAdminProviders,
  useAdminProxies,
  useSetAccountStatus,
  useTestAccount,
  useCreateAccount,
  useUpdateAccount,
  useClearAccountError,
  useRecoverAccount,
  useRefreshAccount,
  useResetAccountQuota,
  useBatchActionAccounts,
  useBatchDeleteAccounts,
  useBatchQuotaFetchAccounts,
  useBatchRefreshAccounts,
  useBatchUpdateAccountConcurrency,
  useBatchUpdateAccountCredentials,
  useBatchUpdateAccounts,
  useBulkUpdateAccounts,
  useDeleteAccount,
  useDiscoverAccountModels,
  useExportAccounts,
  useAccountsHealthSummary,
  useAccountsUsageTodayBatch,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ADMIN_ROUTES } from "@/lib/routes";
import { adminAccountHealthInvestigationHref } from "@/lib/admin-account-health-investigation";
import { adminErrorMessage } from "@/lib/admin-api";
import { formatInteger, formatMoney, formatPercent } from "@/lib/admin-format";
import {
  ACCOUNT_STATUSES,
  buildBatchAccountActionBody,
  runtimeClassLabel,
  type AccountBatchAction,
} from "@/lib/admin-account-form";
import { AccountImportDialog } from "@/components/admin/account-import-dialog";
import { BulkAddAccountsDialog } from "./bulk-add-dialog";
import type { Provider, ProviderAccount, ProviderAccountStatus } from "@/lib/sdk-types";
import { type AccountListMode, metadataString } from "./account-types";
import { AccountHealthCell, AccountQuotaCell, HealthSummaryStrip } from "./account-health-cells";
import { AccountStatusCell } from "./account-status-cell";
import { TokenExpiryChip } from "./token-expiry-chip";
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
  const groupFilter = list.filters.groupId || undefined;
  const accounts = useAdminAccounts({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
    provider_id: providerFilter,
    group_id: groupFilter,
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
  const refreshToken = useRefreshAccount();
  const resetQuota = useResetAccountQuota();
  const batchAction = useBatchActionAccounts();
  const batchDelete = useBatchDeleteAccounts();
  const batchConcurrency = useBatchUpdateAccountConcurrency();
  const batchUpdate = useBatchUpdateAccounts();
  const batchRefresh = useBatchRefreshAccounts();
  const batchRotateCreds = useBatchUpdateAccountCredentials();
  // sub2api parity — bulk-edit superset (status / priority / weight /
  // risk_level / max_concurrency) and batch quota-fetch (refresh-tier).
  const bulkUpdate = useBulkUpdateAccounts();
  const batchQuotaFetch = useBatchQuotaFetchAccounts();
  const deleteMut = useDeleteAccount();
  const discover = useDiscoverAccountModels();
  const exportMut = useExportAccounts();
  const healthSummary = useAccountsHealthSummary();
  const healthById = new Map(
    (healthSummary.data ?? []).map((h) => [h.account_id, h] as const),
  );
  // Group membership lookup: ProviderAccount carries group_ids only — resolve to
  // names for the table cell. Cheap to keep around as a Map; useAdminGroups is
  // already cached across the admin shell.
  const groups = useAdminGroups();
  const groupNameById = new Map(
    (groups.data?.data ?? []).map((g) => [String(g.id), g.name] as const),
  );
  // Today usage per row — batched into one call so the column is cheap even
  // when the page shows many accounts. Joined back by account_id below.
  const visibleAccountIds = (accounts.data?.data ?? []).map((a) => a.id);
  const usageToday = useAccountsUsageTodayBatch(visibleAccountIds);
  const todayByAccountId = new Map(
    (usageToday.data ?? []).map((t) => [t.account_id, t] as const),
  );

  const [formTarget, setFormTarget] = useState<ProviderAccount | "new" | null>(null);
  const [proxyTarget, setProxyTarget] = useState<ProviderAccount | null>(null);
  const [detailTarget, setDetailTarget] = useState<ProviderAccount | null>(null);
  const [testTarget, setTestTarget] = useState<ProviderAccount | null>(null);
  const [bulkDisableOpen, setBulkDisableOpen] = useState(false);
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false);
  const [bulkConcurrencyOpen, setBulkConcurrencyOpen] = useState(false);
  const [bulkCredentialOpen, setBulkCredentialOpen] = useState(false);
  // Mode tracks which target picker the dialog should write back to the
  // applyBulkEdit handler: "selected" sends account_ids, "filtered" sends
  // filters resolved server-side. Distinct from `bulkEditOpen` so closing
  // the dialog clears the toggle on next open.
  const [bulkEditOpen, setBulkEditOpen] = useState<false | "selected" | "filtered">(false);
  const [deleteTarget, setDeleteTarget] = useState<ProviderAccount | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [bulkAddOpen, setBulkAddOpen] = useState(false);
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
  const groupFilterOptions = (groups.data?.data ?? []).map((g) => ({
    value: String(g.id),
    label: g.name,
  }));
  const providerNameById = new Map(
    (providers.data?.data ?? []).map((p) => [String(p.id), p.display_name || p.name] as const),
  );
  const proxyOptions = (proxies.data?.data ?? []).map((p) => ({ value: p.id, label: p.name }));
  const isFiltered = Boolean(statusFilter || providerFilter);

  /** Apply a status to every selected account via the atomic PATCH
   * /admin/accounts/batch endpoint. Previously this fired N concurrent
   * single-item enable/disable calls (Promise.allSettled), which made the
   * audit log noisy + the result non-atomic on partial failure. The backend
   * already returns per-id errors so partial-failure feedback is preserved. */
  async function applyBulkStatus(status: "active" | "disabled") {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      const result = await batchUpdate.mutateAsync({ account_ids: ids, status });
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

  /** Soft-delete the selected accounts in one call. NotFound is idempotent
   *  (caller intent of "this id should not exist" is already true) so missing
   *  rows count as succeeded. Per-id store failures come back in result.errors
   *  and surface as a partial-batch toast. */
  async function applyBulkDelete() {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      const result = await batchDelete.mutateAsync(ids);
      list.clearSelection();
      const failedCount = result.errors.length;
      const succeededCount = result.deleted_count;
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

  /** Bulk-set max_concurrency on the selection — verbatim wiring for the
   *  port of sub2api's BatchUpdateConcurrency. NotFound is idempotent on
   *  the server side; per-id failures come back in result.errors[]. */
  async function applyBulkConcurrency(maxConcurrency: number) {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      const result = await batchConcurrency.mutateAsync(
        ids.map((account_id) => ({ account_id, max_concurrency: maxConcurrency })),
      );
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

  /** Bulk-refresh OAuth tokens across the selection — verbatim wiring for the
   *  port of sub2api's BatchRefresh. Filters the selection to OAuth runtime
   *  classes client-side so the operator gets immediate feedback when none of
   *  the selected rows would be refresh-eligible. The server-side
   *  /admin/accounts/batch-refresh endpoint already rejects non-OAuth rows
   *  per-row; the client-side guard exists so the button is correctly disabled
   *  in the toolbar when the selection is all api_key rows. */
  async function applyBulkRefresh() {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      const result = await batchRefresh.mutateAsync(ids);
      list.clearSelection();
      const failedCount = result.errors.length;
      const succeededCount = result.refreshed_count;
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

  /** Bulk-rotate credentials across the selection — verbatim wiring for the
   *  port of sub2api's BatchUpdateCredentials. The dialog hands the parsed
   *  items in already, so this method only consumes them and renders the
   *  toast. Server-side: NotFound is idempotent and per-row errors come back
   *  in result.errors[]. */
  async function applyBulkCredentialRotation(
    items: { account_id: string; credential: Record<string, unknown> }[],
  ) {
    if (items.length === 0) return;
    try {
      const result = await batchRotateCreds.mutateAsync(items);
      list.clearSelection();
      const failedCount = result.errors.length;
      const succeededCount = result.updated_count;
      if (failedCount > 0 && succeededCount > 0) {
        toast({
          title: t("feedback.batchPartial", { succeeded: succeededCount, failed: failedCount }),
          tone: "warning",
        });
      } else if (failedCount > 0) {
        toast({ title: t("feedback.batchAllFailed", { count: items.length }), tone: "error" });
      } else {
        toast({ title: t("feedback.batchAllSucceeded", { count: succeededCount }), tone: "success" });
      }
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  /** sub2api `BulkUpdate` parity — apply an arbitrary subset of editable
   *  fields to the selection in one server call. The modal hands the
   *  parsed body in (only fields the user actually changed are present).
   *  Per-row failures come back in result.errors and surface as a
   *  partial-batch toast — mirrors the toast pattern used by every other
   *  bulk action on this page. */
  async function applyBulkEdit(
    body: Parameters<typeof bulkUpdate.mutateAsync>[0],
  ) {
    if (!body.account_ids?.length && !body.filters) return;
    try {
      const result = await bulkUpdate.mutateAsync(body);
      list.clearSelection();
      const failedCount = result.errors.length;
      const succeededCount = result.updated_count;
      const totalCount = body.account_ids?.length ?? succeededCount + failedCount;
      if (failedCount > 0 && succeededCount > 0) {
        toast({
          title: t("feedback.batchPartial", { succeeded: succeededCount, failed: failedCount }),
          tone: "warning",
        });
      } else if (failedCount > 0) {
        toast({ title: t("feedback.batchAllFailed", { count: totalCount }), tone: "error" });
      } else {
        toast({ title: t("feedback.batchAllSucceeded", { count: succeededCount }), tone: "success" });
      }
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  /** sub2api `BatchRefreshTier` parity — fan out per-account quota
   *  refresh across the selection in one server call. Per-row failures
   *  come back in result.rows[] (outer call still returns 200), so
   *  surface them as a partial-batch toast. */
  async function applyBulkQuotaFetch() {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      const result = await batchQuotaFetch.mutateAsync(ids);
      list.clearSelection();
      const succeededCount = result.success;
      const failedCount = result.failed;
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
        <span className="text-2xs text-srapi-text-tertiary">{runtimeClassLabel(t, a.runtime_class)}</span>
      ),
    },
    {
      key: "groups",
      header: t("adminAccounts.groups"),
      hideOnMobile: true,
      render: (a) => {
        const ids = a.group_ids ?? [];
        if (ids.length === 0) {
          return <span className="text-2xs text-srapi-text-tertiary">{t("adminAccounts.ungrouped")}</span>;
        }
        return (
          <div className="flex flex-wrap gap-1">
            {ids.map((id) => {
              const name = groupNameById.get(String(id)) ?? `#${id}`;
              return (
                <span
                  key={String(id)}
                  className="inline-flex items-center rounded-md border border-srapi-border bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary"
                >
                  {name}
                </span>
              );
            })}
          </div>
        );
      },
    },
    {
      key: "status",
      header: t("common.active"),
      render: (a) => (
        <div className="flex flex-wrap items-center gap-2">
          <AccountStatusCell
            account={a}
            busy={setStatus.isPending}
            onToggle={readOnlyHealthView ? undefined : () => toggleAccountStatus(a)}
          />
          <TokenExpiryChip account={a} />
        </div>
      ),
    },
    {
      key: "health",
      header: t("adminAccounts.healthTitle"),
      hideOnMobile: true,
      sortValue: (a) => healthById.get(a.id)?.success_rate ?? -1,
      render: (a) => {
        const health = healthById.get(a.id);
        return (
          <AccountHealthCell
            health={health}
            investigationHref={adminAccountHealthInvestigationHref(health)}
          />
        );
      },
    },
    {
      key: "quota",
      header: t("adminAccounts.quotaTitle"),
      hideOnMobile: true,
      sortValue: (a) => healthById.get(a.id)?.quota_remaining_ratio ?? -1,
      render: (a) => <AccountQuotaCell health={healthById.get(a.id)} />,
    },
    {
      key: "today",
      header: t("adminAccounts.today"),
      hideOnMobile: true,
      sortValue: (a) => todayByAccountId.get(a.id)?.requests ?? -1,
      render: (a) => {
        const today = todayByAccountId.get(a.id);
        if (!today) {
          return (
            <span className="font-mono text-2xs text-srapi-text-tertiary">—</span>
          );
        }
        if (today.requests === 0) {
          return (
            <span className="font-mono text-2xs text-srapi-text-tertiary">
              {t("adminAccounts.todayIdle")}
            </span>
          );
        }
        return (
          <div className="flex flex-col gap-0.5">
            <span className="font-mono text-2xs text-srapi-text-secondary tabular">
              {formatInteger(today.requests)} · {formatMoney(today.cost, today.currency)}
            </span>
            <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
              {formatPercent(today.success_rate)}
            </span>
          </div>
        );
      },
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
      <Button
        variant="outline"
        size="sm"
        loading={batchConcurrency.isPending}
        onClick={() => setBulkConcurrencyOpen(true)}
      >
        {t("adminAccounts.bulkSetConcurrency")}
      </Button>
      {/* Bulk OAuth refresh — gated to selections whose rows are oauth_refresh
          or oauth_device_code runtime classes (the server already rejects
          other rows per-row, but disabling the button when nothing is
          eligible matches the operator's mental model). */}
      <Button
        variant="outline"
        size="sm"
        loading={batchRefresh.isPending}
        disabled={
          list.selected.size === 0 ||
          !(accounts.data?.data ?? []).some(
            (a) =>
              list.selected.has(a.id) &&
              (a.runtime_class === "oauth_refresh" || a.runtime_class === "oauth_device_code"),
          )
        }
        onClick={() => void applyBulkRefresh()}
        title={t("adminAccounts.bulkRefreshTokens") as string}
      >
        {t("adminAccounts.bulkRefreshTokens")}
      </Button>
      {/* Bulk credential rotation — opens a textarea modal. Operator pastes
          one row per line: account_id,key=value,key=value. The dialog parses
          this into the request body. */}
      <Button
        variant="outline"
        size="sm"
        loading={batchRotateCreds.isPending}
        disabled={list.selected.size === 0}
        onClick={() => setBulkCredentialOpen(true)}
      >
        {t("adminAccounts.bulkRotateCredentials")}
      </Button>
      {/* sub2api `BulkUpdate` superset — opens a modal that edits any
          subset of status / priority / weight / risk_level /
          max_concurrency / name / proxy_id / upstream_client /
          runtime_class across the selection. */}
      <Button
        variant="outline"
        size="sm"
        loading={bulkUpdate.isPending}
        disabled={list.selected.size === 0}
        onClick={() => setBulkEditOpen("selected")}
      >
        {t("adminAccounts.bulkEdit")}
      </Button>
      {/* sub2api `BulkUpdate` filter-mode entry — submits the request with
          `filters` instead of `account_ids` so the server-side resolver
          picks up every account matching the current filter state.
          Gated on isFiltered so the operator can't accidentally rewrite
          the entire fleet (the backend also rejects empty filters with
          400, but the disabled button matches the operator's mental
          model). */}
      <Button
        variant="outline"
        size="sm"
        loading={bulkUpdate.isPending}
        disabled={!isFiltered}
        onClick={() => setBulkEditOpen("filtered")}
        title={t("adminAccounts.bulkEditFilteredHint") as string}
      >
        {t("adminAccounts.bulkEditFiltered")}
      </Button>
      {/* sub2api `BatchRefreshTier` — fan-out per-account quota fetch
          across the selection. No modal — runs immediately. */}
      <Button
        variant="outline"
        size="sm"
        loading={batchQuotaFetch.isPending}
        disabled={list.selected.size === 0}
        onClick={() => void applyBulkQuotaFetch()}
      >
        {t("adminAccounts.bulkQuotaFetch")}
      </Button>
      <Button
        variant="outline"
        size="sm"
        loading={batchDelete.isPending}
        onClick={() => setBulkDeleteOpen(true)}
        className="border-srapi-error/40 text-srapi-error hover:bg-srapi-error/10"
      >
        {t("common.delete")}
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
      {groupFilterOptions.length > 0 ? (
        <FilterSelect
          value={groupFilter}
          onChange={(v) => list.setFilter("groupId", v)}
          options={groupFilterOptions}
          allLabel={t("adminAccounts.allGroups")}
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
          if (Number.isNaN(val)) {
            toast({ title: t("feedback.failed"), description: "Priority must be a valid number", tone: "error" });
            return;
          }
          void runAction(
            () => updateMut.mutateAsync({ id: a.id, body: { priority: val } }),
            t("feedback.saved"),
          );
        },
      },
      { label: t("adminAccounts.discoverModels"), onSelect: () => void runDiscover(a.id) },
      { label: t("adminAccounts.bindProxy"), onSelect: () => setProxyTarget(a) },
    );
    if (a.runtime_class === "oauth_refresh" || a.runtime_class === "oauth_device_code") {
      actions.push({
        label: t("adminAccounts.refreshTokenAction"),
        onSelect: () =>
          void runAction(
            () => refreshToken.mutateAsync(a.id),
            t("adminAccounts.refreshSuccess"),
          ),
      });
    }
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
                <Button variant="outline" size="sm" onClick={() => setBulkAddOpen(true)}>
                  {t("adminAccounts.bulkAdd")}
                </Button>
                <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
                  + {t("adminAccounts.create")}
                </Button>
              </>
            )}
          </div>
        }
      />
      <HealthSummaryStrip
        healthById={healthById}
        onSelectAccounts={readOnlyHealthView ? undefined : selectAccountIds}
      />

      {listMode === "cards" ? (
        <AccountsCardView
          query={accounts}
          providerNameById={providerNameById}
          healthById={healthById}
          healthInvestigationHref={adminAccountHealthInvestigationHref}
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
        open={bulkDeleteOpen}
        onOpenChange={setBulkDeleteOpen}
        title={t("adminAccounts.bulkDeleteTitle", { count: list.selected.size })}
        body={t("adminAccounts.bulkDeleteBody")}
        confirmLabel={t("common.delete")}
        onConfirm={() => applyBulkDelete()}
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

      <BulkAddAccountsDialog
        open={bulkAddOpen}
        onOpenChange={setBulkAddOpen}
        defaultProviderId={providers.data?.data?.[0]?.id ?? ""}
      />

      {bulkConcurrencyOpen ? (
        <BulkConcurrencyDialog
          count={list.selected.size}
          isPending={batchConcurrency.isPending}
          onSubmit={async (value) => {
            await applyBulkConcurrency(value);
            setBulkConcurrencyOpen(false);
          }}
          onClose={() => setBulkConcurrencyOpen(false)}
        />
      ) : null}

      {bulkCredentialOpen ? (
        <BulkCredentialRotateDialog
          selectedIds={[...list.selected]}
          isPending={batchRotateCreds.isPending}
          onSubmit={async (items) => {
            await applyBulkCredentialRotation(items);
            setBulkCredentialOpen(false);
          }}
          onClose={() => setBulkCredentialOpen(false)}
        />
      ) : null}

      {bulkEditOpen ? (
        <BulkEditAccountDialog
          mode={bulkEditOpen}
          count={bulkEditOpen === "selected" ? list.selected.size : -1}
          isPending={bulkUpdate.isPending}
          proxyOptions={proxyOptions}
          onSubmit={async (body) => {
            if (bulkEditOpen === "filtered") {
              await applyBulkEdit({
                filters: {
                  status: statusFilter || undefined,
                  provider_id: providerFilter || undefined,
                  group_id: groupFilter || undefined,
                },
                ...body,
              });
            } else {
              await applyBulkEdit({ account_ids: [...list.selected], ...body });
            }
            setBulkEditOpen(false);
          }}
          onClose={() => setBulkEditOpen(false)}
        />
      ) : null}
    </>
  );

  function selectAccountIds(ids: string[]) {
    if (ids.length === 0) return;
    list.togglePage(ids, true);
  }
}

// Bulk-rotate-credentials dialog. Operator pastes one row per line in the
// operator-friendly `account_id,key=value,key=value` format and the dialog
// parses each line into a {account_id, credential:{...}} item. Verbatim
// wiring for the port of sub2api's BatchUpdateCredentials — the server still
// validates each row (empty patch, invalid id) so the parser only has to
// reject syntactically malformed lines.
function BulkCredentialRotateDialog({
  selectedIds,
  isPending,
  onSubmit,
  onClose,
}: {
  selectedIds: string[];
  isPending: boolean;
  onSubmit: (items: { account_id: string; credential: Record<string, unknown> }[]) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [raw, setRaw] = useState(() => selectedIds.map((id) => `${id},`).join("\n"));
  const [error, setError] = useState<string | null>(null);

  function parseLine(line: string): { account_id: string; credential: Record<string, unknown> } | null {
    const parts = line.split(",").map((s) => s.trim()).filter((s) => s.length > 0);
    if (parts.length < 2) return null;
    const accountId = parts[0];
    const credential: Record<string, unknown> = {};
    for (const kv of parts.slice(1)) {
      const eq = kv.indexOf("=");
      if (eq <= 0) return null;
      const k = kv.slice(0, eq).trim();
      const v = kv.slice(eq + 1).trim();
      if (k.length === 0) return null;
      credential[k] = v;
    }
    return { account_id: accountId, credential };
  }

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const items: { account_id: string; credential: Record<string, unknown> }[] = [];
    const lines = raw.split(/\r?\n/);
    for (const line of lines) {
      const trimmed = line.trim();
      if (trimmed === "") continue;
      const item = parseLine(trimmed);
      if (!item) {
        setError(t("adminAccounts.bulkRotateBadLine") as string);
        return;
      }
      items.push(item);
    }
    if (items.length === 0) {
      setError(t("adminAccounts.bulkRotateEmpty") as string);
      return;
    }
    void onSubmit(items);
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("adminAccounts.bulkRotateTitle")}</DialogTitle>
          <DialogDescription>{t("adminAccounts.bulkRotateBody")}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-3">
          <Label htmlFor="bulk-rotate-textarea">
            {t("adminAccounts.bulkRotateInputLabel")}
          </Label>
          <textarea
            id="bulk-rotate-textarea"
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            rows={Math.min(12, Math.max(4, selectedIds.length))}
            className="w-full rounded border border-srapi-border bg-srapi-bg-secondary px-3 py-2 text-sm font-mono"
            placeholder="123,refresh_token=abc123,api_key=sk-..."
          />
          {error ? (
            <p className="text-xs text-srapi-error">{error}</p>
          ) : null}
          <DialogFooter>
            <Button variant="ghost" type="button" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" loading={isPending}>
              {t("adminAccounts.bulkRotateSubmit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// Small focused modal: one integer input + selected-count blurb. Submits an
// integer (0 = clear cap) to the parent's applyBulkConcurrency.
function BulkConcurrencyDialog({
  count,
  isPending,
  onSubmit,
  onClose,
}: {
  count: number;
  isPending: boolean;
  onSubmit: (value: number) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [value, setValue] = useState("4");
  const [error, setError] = useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const n = Number.parseInt(value.trim(), 10);
    if (!Number.isFinite(n) || n < 0) {
      setError(t("adminAccounts.bulkSetConcurrencyHint"));
      return;
    }
    void onSubmit(n);
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>
              {t("adminAccounts.bulkSetConcurrencyTitle", { count })}
            </DialogTitle>
            <DialogDescription>
              {t("adminAccounts.bulkSetConcurrencyHint")}
            </DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-3">
            <div>
              <Label htmlFor="bulk-concurrency">
                {t("adminAccounts.bulkSetConcurrency")}
              </Label>
              <Input
                id="bulk-concurrency"
                type="number"
                inputMode="numeric"
                min={0}
                value={value}
                disabled={isPending}
                aria-invalid={Boolean(error)}
                onChange={(e) => setValue(e.target.value)}
              />
            </div>
            {error ? (
              <p role="alert" className="text-2xs text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" disabled={isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={isPending}>
              {t("common.apply")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function hasQuotaError(account: ProviderAccount): boolean {
  return Boolean(metadataString(account.metadata, "last_quota_error_class"));
}

// sub2api `BulkEditAccountModal` port — exposes the full SRapi-schema
// editable surface (status / priority / weight / risk_level /
// max_concurrency / name / proxy_id / upstream_client / runtime_class).
// Each row has an "include this field?" toggle so only fields the
// operator explicitly chose to change land in the request body,
// matching sub2api's "delete-only-if-checked" semantics. Two target
// modes: `selected` (account_ids from list.selected) and `filtered`
// (filters mode resolved server-side from the page's current query).
function BulkEditAccountDialog({
  mode,
  count,
  isPending,
  proxyOptions,
  onSubmit,
  onClose,
}: {
  mode: "selected" | "filtered";
  count: number;
  isPending: boolean;
  proxyOptions: { value: string; label: string }[];
  onSubmit: (
    body: {
      name?: string;
      status?: ProviderAccountStatus;
      priority?: number;
      weight?: number;
      risk_level?: string;
      max_concurrency?: number;
      proxy_id?: string;
      upstream_client?: string;
      runtime_class?: string;
    },
  ) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [nameEnabled, setNameEnabled] = useState(false);
  const [nameValue, setNameValue] = useState("");
  const [statusEnabled, setStatusEnabled] = useState(false);
  const [statusValue, setStatusValue] = useState<ProviderAccountStatus>("active");
  const [priorityEnabled, setPriorityEnabled] = useState(false);
  const [priorityValue, setPriorityValue] = useState("0");
  const [weightEnabled, setWeightEnabled] = useState(false);
  const [weightValue, setWeightValue] = useState("1");
  const [riskEnabled, setRiskEnabled] = useState(false);
  const [riskValue, setRiskValue] = useState("normal");
  const [concurrencyEnabled, setConcurrencyEnabled] = useState(false);
  const [concurrencyValue, setConcurrencyValue] = useState("4");
  const [proxyEnabled, setProxyEnabled] = useState(false);
  const [proxyValue, setProxyValue] = useState("");
  const [upstreamEnabled, setUpstreamEnabled] = useState(false);
  const [upstreamValue, setUpstreamValue] = useState("");
  const [runtimeClassEnabled, setRuntimeClassEnabled] = useState(false);
  const [runtimeClassValue, setRuntimeClassValue] = useState("api_key");
  const [error, setError] = useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const body: {
      name?: string;
      status?: ProviderAccountStatus;
      priority?: number;
      weight?: number;
      risk_level?: string;
      max_concurrency?: number;
      proxy_id?: string;
      upstream_client?: string;
      runtime_class?: string;
    } = {};
    if (nameEnabled) {
      const v = nameValue.trim();
      if (!v) {
        setError(t("adminAccounts.bulkEditPickField"));
        return;
      }
      body.name = v;
    }
    if (statusEnabled) body.status = statusValue;
    if (priorityEnabled) {
      const n = Number.parseInt(priorityValue.trim(), 10);
      if (!Number.isFinite(n) || n < 0) {
        setError(t("adminAccounts.bulkEditNumberHint"));
        return;
      }
      body.priority = n;
    }
    if (weightEnabled) {
      const n = Number.parseFloat(weightValue.trim());
      if (!Number.isFinite(n) || n < 0) {
        setError(t("adminAccounts.bulkEditNumberHint"));
        return;
      }
      body.weight = n;
    }
    if (riskEnabled) {
      const v = riskValue.trim();
      if (!v) {
        setError(t("adminAccounts.bulkEditNumberHint"));
        return;
      }
      body.risk_level = v;
    }
    if (concurrencyEnabled) {
      const n = Number.parseInt(concurrencyValue.trim(), 10);
      if (!Number.isFinite(n) || n < 0) {
        setError(t("adminAccounts.bulkEditNumberHint"));
        return;
      }
      body.max_concurrency = n;
    }
    if (proxyEnabled) {
      // Empty string clears the binding (backend treats "" as "unbind").
      body.proxy_id = proxyValue;
    }
    if (upstreamEnabled) {
      // Empty string clears the override (matches the backend semantics).
      body.upstream_client = upstreamValue.trim();
    }
    if (runtimeClassEnabled) {
      body.runtime_class = runtimeClassValue;
    }
    if (Object.keys(body).length === 0) {
      setError(t("adminAccounts.bulkEditPickField"));
      return;
    }
    void onSubmit(body);
  }

  const titleKey =
    mode === "filtered"
      ? "adminAccounts.bulkEditFilteredTitle"
      : "adminAccounts.bulkEditTitle";
  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t(titleKey, { count })}</DialogTitle>
            <DialogDescription>{t("adminAccounts.bulkEditHint")}</DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-4">
            <BulkEditRow
              enabled={nameEnabled}
              onToggle={setNameEnabled}
              label={t("adminCommon.name")}
              disabled={isPending}
            >
              <Input
                value={nameValue}
                disabled={!nameEnabled || isPending}
                onChange={(e) => setNameValue(e.target.value)}
              />
            </BulkEditRow>
            <BulkEditRow
              enabled={statusEnabled}
              onToggle={setStatusEnabled}
              label={t("adminCommon.status")}
              disabled={isPending}
            >
              <select
                className="w-full rounded-md border border-srapi-border bg-srapi-card px-2 py-1.5 text-2xs"
                value={statusValue}
                disabled={!statusEnabled || isPending}
                onChange={(e) => setStatusValue(e.target.value as ProviderAccountStatus)}
              >
                <option value="active">active</option>
                <option value="disabled">disabled</option>
                <option value="needs_reauth">needs_reauth</option>
                <option value="suspended">suspended</option>
                <option value="dead">dead</option>
              </select>
            </BulkEditRow>
            <BulkEditRow
              enabled={priorityEnabled}
              onToggle={setPriorityEnabled}
              label={t("adminAccounts.priority")}
              disabled={isPending}
            >
              <Input
                type="number"
                inputMode="numeric"
                min={0}
                value={priorityValue}
                disabled={!priorityEnabled || isPending}
                onChange={(e) => setPriorityValue(e.target.value)}
              />
            </BulkEditRow>
            <BulkEditRow
              enabled={weightEnabled}
              onToggle={setWeightEnabled}
              label={t("adminAccounts.weight")}
              disabled={isPending}
            >
              <Input
                type="number"
                inputMode="decimal"
                min={0}
                step="0.1"
                value={weightValue}
                disabled={!weightEnabled || isPending}
                onChange={(e) => setWeightValue(e.target.value)}
              />
            </BulkEditRow>
            <BulkEditRow
              enabled={riskEnabled}
              onToggle={setRiskEnabled}
              label={t("adminAccounts.riskLevel")}
              disabled={isPending}
            >
              <Input
                value={riskValue}
                disabled={!riskEnabled || isPending}
                onChange={(e) => setRiskValue(e.target.value)}
                placeholder="normal"
              />
            </BulkEditRow>
            <BulkEditRow
              enabled={concurrencyEnabled}
              onToggle={setConcurrencyEnabled}
              label={t("adminAccounts.bulkSetConcurrency")}
              disabled={isPending}
            >
              <Input
                type="number"
                inputMode="numeric"
                min={0}
                value={concurrencyValue}
                disabled={!concurrencyEnabled || isPending}
                onChange={(e) => setConcurrencyValue(e.target.value)}
              />
            </BulkEditRow>
            <BulkEditRow
              enabled={proxyEnabled}
              onToggle={setProxyEnabled}
              label={t("adminAccounts.proxy")}
              disabled={isPending}
            >
              <select
                className="w-full rounded-md border border-srapi-border bg-srapi-card px-2 py-1.5 text-2xs"
                value={proxyValue}
                disabled={!proxyEnabled || isPending}
                onChange={(e) => setProxyValue(e.target.value)}
              >
                <option value="">{t("adminAccounts.proxyNone")}</option>
                {proxyOptions.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.label}
                  </option>
                ))}
              </select>
            </BulkEditRow>
            <BulkEditRow
              enabled={upstreamEnabled}
              onToggle={setUpstreamEnabled}
              label={t("adminAccounts.upstreamClient")}
              disabled={isPending}
            >
              <Input
                value={upstreamValue}
                disabled={!upstreamEnabled || isPending}
                onChange={(e) => setUpstreamValue(e.target.value)}
                placeholder={t("adminAccounts.upstreamClientPlaceholder") as string}
              />
            </BulkEditRow>
            <BulkEditRow
              enabled={runtimeClassEnabled}
              onToggle={setRuntimeClassEnabled}
              label={t("adminAccounts.runtimeClass")}
              disabled={isPending}
            >
              <select
                className="w-full rounded-md border border-srapi-border bg-srapi-card px-2 py-1.5 text-2xs"
                value={runtimeClassValue}
                disabled={!runtimeClassEnabled || isPending}
                onChange={(e) => setRuntimeClassValue(e.target.value)}
              >
                <option value="api_key">api_key</option>
                <option value="oauth_refresh">oauth_refresh</option>
                <option value="oauth_device_code">oauth_device_code</option>
                <option value="web_session_cookie">web_session_cookie</option>
                <option value="cli_client_token">cli_client_token</option>
                <option value="custom_reverse_proxy">custom_reverse_proxy</option>
              </select>
            </BulkEditRow>
            {error ? (
              <p role="alert" className="text-2xs text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" disabled={isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={isPending}>
              {t("common.apply")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function BulkEditRow({
  enabled,
  onToggle,
  label,
  disabled,
  children,
}: {
  enabled: boolean;
  onToggle: (next: boolean) => void;
  label: string;
  disabled: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-[auto_1fr_2fr] items-center gap-3">
      <input
        type="checkbox"
        checked={enabled}
        disabled={disabled}
        onChange={(e) => onToggle(e.target.checked)}
        className="size-4 rounded border-srapi-border"
        aria-label={label}
      />
      <Label className="text-2xs">{label}</Label>
      <div>{children}</div>
    </div>
  );
}
