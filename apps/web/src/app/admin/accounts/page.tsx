"use client";

import { useState } from "react";
import { Server } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu, type RowAction } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { enumOptions } from "@/components/admin/resource-form-dialog";
import { AccountFormDialog } from "@/components/admin/account-form-dialog";
import { BindProxyDialog } from "@/components/admin/bind-proxy-dialog";
import { AccountDetailSheet } from "@/components/admin/account-detail-sheet";
import {
  useAdminAccounts,
  useAdminProviders,
  useAdminProxies,
  useSetAccountStatus,
  useTestAccount,
  useCreateAccount,
  useUpdateAccount,
  useClearAccountError,
  useRecoverAccount,
  useDiscoverAccountModels,
  useExportAccounts,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { adminErrorMessage } from "@/lib/admin-api";
import { ACCOUNT_STATUSES } from "@/lib/admin-account-form";
import type { ProviderAccount } from "@/lib/sdk-types";

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
  const list = useAdminList();
  const statusFilter = (list.filters.status as ProviderAccount["status"]) || undefined;
  const providerFilter = list.filters.providerId || undefined;
  const accounts = useAdminAccounts({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
    provider_id: providerFilter,
  });
  const providers = useAdminProviders();
  const proxies = useAdminProxies();
  const setStatus = useSetAccountStatus();
  const test = useTestAccount();
  const createMut = useCreateAccount();
  const updateMut = useUpdateAccount();
  const clearErr = useClearAccountError();
  const recover = useRecoverAccount();
  const discover = useDiscoverAccountModels();
  const exportMut = useExportAccounts();

  const [formTarget, setFormTarget] = useState<ProviderAccount | "new" | null>(null);
  const [proxyTarget, setProxyTarget] = useState<ProviderAccount | null>(null);
  const [detailTarget, setDetailTarget] = useState<ProviderAccount | null>(null);
  const [bulkDisableOpen, setBulkDisableOpen] = useState(false);

  const providerOptions = (providers.data?.data ?? []).map((p) => ({
    value: p.id,
    label: p.display_name || p.name,
    platformFamily: p.platform_family ?? null,
    authMethods: p.auth_methods ?? null,
  }));
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
    list.clearSelection();
    if (failed > 0) {
      toast({ title: t("feedback.failed"), description: `${failed}/${ids.length}`, tone: "error" });
    } else {
      toast({ title: t("adminAccounts.bulkDone", { count: ids.length }), tone: "success" });
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

  async function runTest(id: string) {
    try {
      const result = await test.mutateAsync(id);
      toast({
        title: result.ok ? t("adminAccounts.testOk") : t("adminAccounts.testFailed"),
        description: result.message ?? undefined,
        tone: result.ok ? "success" : "error",
      });
    } catch (err) {
      toast({ title: t("adminAccounts.testFailed"), description: adminErrorMessage(err), tone: "error" });
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

  const columns: Column<ProviderAccount>[] = [
    {
      key: "name",
      header: t("adminAccounts.name"),
      sortValue: (a) => a.name,
      render: (a) => <span className="text-srapi-text-primary">{a.name}</span>,
    },
    {
      key: "provider",
      header: t("adminAccounts.provider"),
      sortValue: (a) => a.provider_id,
      render: (a) => (
        <span className="font-mono text-2xs text-srapi-text-secondary">{a.provider_id}</span>
      ),
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
      render: (a) => <QuietBadge status={quietStatusFor(a.status)} label={statusLabel(t, a.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminAccounts.title")}
        description={t("adminAccounts.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {accounts.data ? (
              <ListCount total={accounts.data.pagination?.total ?? accounts.data.data.length} />
            ) : null}
            <Button
              variant="outline"
              size="sm"
              loading={exportMut.isPending}
              onClick={() => void runExport()}
            >
              {t("adminAccounts.export")}
            </Button>
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminAccounts.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={accounts}
        columns={columns}
        getRowId={(a) => a.id}
        emptyIcon={Server}
        emptyTitle={t("adminAccounts.emptyTitle")}
        emptyBody={t("adminAccounts.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminAccounts.create")}
          </Button>
        }
        dimRow={(a) => a.status === "disabled"}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        toolbar={
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
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: accounts.data?.pagination?.total ?? accounts.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        selection={{
          selected: list.selected,
          onToggle: list.toggle,
          onTogglePage: list.togglePage,
          bulkActions: (
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
            </>
          ),
        }}
        rowActions={(a) => {
          const actions: RowAction[] = [
            { label: t("adminAccounts.details"), onSelect: () => setDetailTarget(a) },
            { label: t("adminAccounts.edit"), onSelect: () => setFormTarget(a) },
            { label: t("adminAccounts.test"), onSelect: () => void runTest(a.id) },
            { label: t("adminAccounts.discoverModels"), onSelect: () => void runDiscover(a.id) },
            { label: t("adminAccounts.bindProxy"), onSelect: () => setProxyTarget(a) },
          ];
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
          actions.push({
            label: a.status === "disabled" ? t("common.enable") : t("common.disable"),
            destructive: a.status !== "disabled",
            onSelect: () =>
              void runAction(
                () =>
                  setStatus.mutateAsync({
                    id: a.id,
                    status: a.status === "disabled" ? "active" : "disabled",
                  }),
                t("feedback.saved"),
              ),
          });
          return <RowActionsMenu actions={actions} />;
        }}
      />

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
    </>
  );
}
