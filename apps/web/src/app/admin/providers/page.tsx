"use client";

import { useState } from "react";
import { Plug } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ADMIN_ROUTES } from "@/lib/routes";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useAdminProviders,
  useCreateProvider,
  useUpdateProvider,
  useTestProvider,
  useDeleteProvider,
  useInstallProviderPresets,
  useAccountsHealthSummary,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import {
  PROVIDER_ADAPTER_TYPES,
  PROVIDER_PROTOCOLS,
  RESOURCE_STATUSES,
  emptyProviderForm,
  providerFormFromProvider,
  buildCreateProviderBody,
  buildUpdateProviderBody,
  type ProviderFormState,
} from "@/lib/admin-provider-form";
import type { Provider } from "@/lib/sdk-types";

export default function AdminProvidersPage() {
  return (
    <AdminShell>
      <ProvidersContent />
    </AdminShell>
  );
}

function ProvidersContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-providers", []);
  const statusFilter = (list.filters.status as Provider["status"]) || undefined;
  const searchQuery = list.search || undefined;
  const providers = useAdminProviders({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
    q: searchQuery,
  });
  const createMut = useCreateProvider();
  const updateMut = useUpdateProvider();
  const testMut = useTestProvider();
  const deleteMut = useDeleteProvider();
  const installMut = useInstallProviderPresets();
  const healthSummary = useAccountsHealthSummary();
  const accountCountByProvider = new Map<string, { active: number; total: number }>();
  for (const h of healthSummary.data ?? []) {
    const pid = String(h.provider_id);
    const prev = accountCountByProvider.get(pid) ?? { active: 0, total: 0 };
    prev.total++;
    if (h.status === "active") prev.active++;
    accountCountByProvider.set(pid, prev);
  }

  const [formTarget, setFormTarget] = useState<Provider | "new" | null>(null);
  const [toDelete, setToDelete] = useState<Provider | null>(null);

  async function runInstallPresets() {
    try {
      const r = await installMut.mutateAsync();
      const skipped = Math.max(0, r.requested - r.succeeded - r.failed);
      toast({
        title: r.succeeded > 0 ? t("feedback.created") : t("adminProviders.presetsNone"),
        description:
          r.succeeded > 0
            ? t("adminProviders.presetsInstalled", {
                created: String(r.succeeded),
                skipped: String(skipped),
              })
            : undefined,
        tone: "success",
      });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
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

  async function toggleStatus(p: Provider) {
    const next = p.status === "disabled" ? "active" : "disabled";
    try {
      await updateMut.mutateAsync({
        id: p.id,
        body: buildUpdateProviderBody({ ...providerFormFromProvider(p), status: next }),
      });
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const endpointCapabilityOptions = [
    { value: "auto", label: t("adminProviders.capabilityAuto") },
    { value: "on", label: t("adminProviders.capabilityOn") },
    { value: "off", label: t("adminProviders.capabilityOff") },
  ];

  // The provider slug (`name`) is immutable after creation, so it only appears
  // on the create form; edits keep the identity stable.
  const sharedFields: FieldConfig<ProviderFormState>[] = [
    { name: "displayName", label: t("adminProviders.displayName") },
    {
      name: "adapterType",
      label: t("adminProviders.adapterType"),
      type: "select",
      options: enumOptions(PROVIDER_ADAPTER_TYPES),
      hint: t("adminProviders.adapterHint"),
    },
    {
      name: "protocol",
      label: t("adminProviders.protocol"),
      help: t("adminProviders.protocolHelp"),
      type: "select",
      options: enumOptions(PROVIDER_PROTOCOLS),
    },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(RESOURCE_STATUSES),
    },
    {
      name: "chatCompletionsCapability",
      label: t("adminProviders.endpointChatCompletions"),
      type: "select",
      options: endpointCapabilityOptions,
      hint: t("adminProviders.endpointCapabilityHint"),
    },
    {
      name: "responsesCapability",
      label: t("adminProviders.endpointResponses"),
      type: "select",
      options: endpointCapabilityOptions,
      hint: t("adminProviders.endpointCapabilityHint"),
    },
    {
      name: "responsesCompactCapability",
      label: t("adminProviders.endpointResponsesCompact"),
      type: "select",
      options: endpointCapabilityOptions,
      hint: t("adminProviders.endpointCapabilityHint"),
    },
    {
      name: "responsesInputItemsCapability",
      label: t("adminProviders.endpointResponsesInputItems"),
      type: "select",
      options: endpointCapabilityOptions,
      hint: t("adminProviders.endpointCapabilityHint"),
    },
    {
      name: "messagesCapability",
      label: t("adminProviders.endpointMessages"),
      type: "select",
      options: endpointCapabilityOptions,
      hint: t("adminProviders.endpointCapabilityHint"),
    },
    {
      name: "supportedModels",
      label: t("adminProviders.supportedModels"),
      type: "tags",
      placeholder: "gpt-4o, claude-*",
      hint: t("adminProviders.supportedModelsHint"),
    },
    {
      name: "excludedModels",
      label: t("adminProviders.excludedModels"),
      type: "tags",
      placeholder: "gpt-4.1, o1-*",
      hint: t("adminProviders.excludedModelsHint"),
    },
    {
      name: "capabilities",
      label: t("adminProviders.capabilities"),
      help: t("adminProviders.capabilitiesHelp"),
      type: "keyvalue",
      advanced: true,
    },
    {
      name: "configSchema",
      label: t("adminProviders.configSchema"),
      help: t("adminProviders.configSchemaHelp"),
      type: "keyvalue",
      advanced: true,
    },
  ];

  const createFields: FieldConfig<ProviderFormState>[] = [
    {
      name: "name",
      label: t("adminProviders.name"),
      placeholder: "deepseek",
      hint: t("adminProviders.nameHint"),
    },
    ...sharedFields,
  ];

  const columns: Column<Provider>[] = [
    {
      key: "name",
      header: t("adminProviders.name"),
      pinned: true,
      sortValue: (p) => p.display_name || p.name,
      render: (p) => (
        <div className="min-w-0">
          <div className="text-srapi-text-primary truncate">{p.display_name || p.name}</div>
          <div className="text-2xs text-srapi-text-tertiary truncate font-mono">{p.name}</div>
        </div>
      ),
    },
    {
      key: "adapterType",
      header: t("adminProviders.adapterType"),
      hideOnMobile: true,
      render: (p) => (
        <span className="text-2xs text-srapi-text-secondary font-mono">{p.adapter_type}</span>
      ),
    },
    {
      key: "protocol",
      header: t("adminProviders.protocol"),
      hideOnMobile: true,
      render: (p) => (
        <span className="text-2xs text-srapi-text-tertiary font-mono">{p.protocol}</span>
      ),
    },
    {
      key: "accounts",
      header: t("dashboard.accounts"),
      hideOnMobile: true,
      sortValue: (p) => accountCountByProvider.get(p.id)?.total ?? 0,
      render: (p) => {
        const counts = accountCountByProvider.get(p.id);
        if (!counts) return <span className="text-2xs text-srapi-text-tertiary">0</span>;
        return (
          <span className="text-2xs flex items-center gap-1.5 font-mono">
            <span className="text-srapi-success">{counts.active}</span>
            <span className="text-srapi-text-tertiary">/ {counts.total}</span>
          </span>
        );
      },
    },
    {
      key: "status",
      header: t("common.active"),
      sortValue: (p) => p.status,
      render: (p) => {
        // Inline toggle only flips between the two operator-meaningful
        // states — pending/archived are kept as a read-only badge so a
        // misclick can't silently regress them.
        const canToggle = p.status === "active" || p.status === "disabled";
        const badge = (
          <QuietBadge status={quietStatusFor(p.status)} label={statusLabel(t, p.status)} />
        );
        if (!canToggle) return badge;
        return (
          <button
            type="button"
            onClick={() => void toggleStatus(p)}
            disabled={updateMut.isPending}
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
        title={t("adminProviders.title")}
        description={t("adminProviders.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {providers.data ? (
              <ListCount total={providers.data.pagination?.total ?? providers.data.data.length} />
            ) : null}
            <Button
              variant="outline"
              size="sm"
              onClick={() => void runInstallPresets()}
              disabled={installMut.isPending}
            >
              {installMut.isPending
                ? t("adminProviders.installingPresets")
                : t("adminProviders.installPresets")}
            </Button>
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminProviders.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={providers}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(p) => p.id}
        emptyIcon={Plug}
        emptyTitle={t("adminProviders.emptyTitle")}
        emptyBody={t("adminProviders.emptyBody")}
        emptyAction={
          <div className="flex gap-2">
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminProviders.create")}
            </Button>
            <Button variant="outline" size="sm" asChild>
              <a href={ADMIN_ROUTES.quickSetup}>{t("adminProviders.emptyQuickSetup")}</a>
            </Button>
          </div>
        }
        minWidth={560}
        isFiltered={Boolean(statusFilter || searchQuery)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        dimRow={(p) => p.status === "disabled"}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminProviders.searchPlaceholder")}
            />
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={enumOptions(RESOURCE_STATUSES)}
              allLabel={t("adminCommon.allStatuses")}
            />
            <ColumnToggle
              columns={columns.map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: providers.data?.pagination?.total ?? providers.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(p) => (
          <RowActionsMenu
            actions={[
              { label: t("common.edit"), onSelect: () => setFormTarget(p) },
              { label: t("adminProviders.test"), onSelect: () => void runTest(p.id) },
              {
                label: p.status === "disabled" ? t("common.enable") : t("common.disable"),
                onSelect: () => void toggleStatus(p),
              },
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
        title={t("adminProviders.deleteTitle")}
        body={t("adminProviders.deleteBody", {
          name: toDelete?.display_name || toDelete?.name || "",
        })}
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
          title={t("adminProviders.create")}
          fields={createFields}
          initial={emptyProviderForm()}
          buildBody={buildCreateProviderBody}
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
          title={t("adminProviders.edit")}
          description={formTarget.name}
          fields={sharedFields}
          initial={providerFormFromProvider(formTarget)}
          buildBody={buildUpdateProviderBody}
          submit={(body) => updateMut.mutateAsync({ id: formTarget.id, body })}
          successMessage={t("feedback.updated")}
          isPending={updateMut.isPending}
        />
      ) : null}
    </>
  );
}
