"use client";

import { useState } from "react";
import { Plug } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { ADMIN_ROUTES } from "@/lib/routes";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { InlineDetailGrid, type InlineDetailSection } from "@/components/ui/inline-detail-grid";
import { SegmentedControl } from "@/components/ui/segmented-control";
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
import { cn } from "@/lib/cn";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { DataPill } from "@/components/ui/data-pill";
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
      options: enumOptions(PROVIDER_ADAPTER_TYPES, t),
      hint: t("adminProviders.adapterHint"),
    },
    {
      name: "protocol",
      label: t("adminProviders.protocol"),
      help: t("adminProviders.protocolHelp"),
      type: "select",
      options: enumOptions(PROVIDER_PROTOCOLS, t),
    },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(RESOURCE_STATUSES, t),
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
          <div className="truncate text-sm font-medium text-srapi-text-primary">
            {p.display_name || p.name}
          </div>
          <div className="truncate text-[11px] text-srapi-text-tertiary">{p.name}</div>
        </div>
      ),
    },
    {
      key: "adapterType",
      header: t("adminProviders.adapterType"),
      hideOnMobile: true,
      render: (p) => <DataPill tone="neutral">{p.adapter_type}</DataPill>,
    },
    {
      key: "protocol",
      header: t("adminProviders.protocol"),
      hideOnMobile: true,
      render: (p) => <DataPill tone="neutral">{p.protocol}</DataPill>,
    },
    {
      key: "accounts",
      header: t("dashboard.accounts"),
      hideOnMobile: true,
      sortValue: (p) => accountCountByProvider.get(p.id)?.total ?? 0,
      render: (p) => {
        const counts = accountCountByProvider.get(p.id);
        if (!counts)
          return <span className="text-xs tabular text-srapi-text-tertiary">0</span>;
        const inactive = Math.max(0, counts.total - counts.active);
        return (
          <DataTooltip
            title={t("dashboard.accounts")}
            primary={
              <span className="tabular">
                {counts.active}
                <span className="text-srapi-text-tertiary"> / {counts.total}</span>
              </span>
            }
            rows={[
              { label: t("common.active"), value: String(counts.active), tone: counts.active > 0 ? "success" : "muted" },
              { label: t("common.disabled"), value: String(inactive), tone: inactive > 0 ? "warning" : "muted" },
            ]}
          >
            <span className="flex items-center gap-1.5 text-xs tabular">
              <span className={cn("font-medium", counts.active > 0 ? "metric-strong-good" : "text-srapi-text-tertiary")}>
                {counts.active}
              </span>
              <span className="text-srapi-text-tertiary">/ {counts.total}</span>
            </span>
          </DataTooltip>
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
      <SectionHero
        eyebrow="Gateway · Providers"
        title={t("adminProviders.title")}
        description={t("adminProviders.subtitle")}
        metrics={
          providers.data
            ? [
                {
                  label: t("adminProviders.title"),
                  value: String(providers.data.pagination?.total ?? providers.data.data.length),
                },
              ]
            : undefined
        }
        actions={
          <>
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
          </>
        }
      />
      <AdminListView
        query={providers}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(p) => p.id}
        emptyIcon={Plug}
        rowSeverity={(p) => {
          if (p.status === "disabled" || p.status === "archived") return "warning";
          const counts = accountCountByProvider.get(p.id);
          if (counts && counts.total > 0 && counts.active === 0) return "warning";
          return undefined;
        }}
        expandRow={(p) => <ProviderDetailRow provider={p} />}
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
            <SegmentedControl<string>
              value={statusFilter ?? "__all__"}
              onChange={(v) => list.setFilter("status", v === "__all__" ? undefined : v)}
              ariaLabel={t("adminCommon.status")}
              size="sm"
              options={[
                { value: "__all__", label: t("common.all") },
                ...enumOptions(RESOURCE_STATUSES, t).map((opt) => ({
                  value: opt.value,
                  label: opt.label,
                })),
              ]}
            />
            <div className="ml-auto">
              <ColumnToggle
                columns={columns.map((c) => ({ key: c.key, label: c.header }))}
                visibility={colVis}
              />
            </div>
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

/**
 * Inline expansion content for a provider row. Surfaces identity / routing /
 * capability matrix / config schema as label-value pairs inside an
 * <InlineDetailGrid>. Replaces a per-row click→modal hop with at-a-glance
 * detail.
 */
function ProviderDetailRow({ provider }: { provider: Provider }) {
  const { t } = useLanguage();
  const capabilities = provider.capabilities as Record<string, unknown> | undefined;
  const configSchema = provider.config_schema as Record<string, unknown> | undefined;
  const authMethods = provider.auth_methods ?? [];

  const sections: InlineDetailSection[] = [
    {
      title: t("adminProviders.name"),
      rows: [
        { label: t("adminProviders.name"), value: provider.name, mono: true },
        { label: t("adminProviders.displayName"), value: provider.display_name },
        { label: t("adminProviders.adapterType"), value: provider.adapter_type, mono: true },
        { label: t("adminProviders.protocol"), value: provider.protocol, mono: true },
        ...(provider.platform_family
          ? [{ label: "platform", value: String(provider.platform_family), mono: true }]
          : []),
      ],
    },
    {
      title: t("adminProviders.capabilities"),
      rows:
        capabilities && Object.keys(capabilities).length > 0
          ? Object.entries(capabilities)
              .slice(0, 10)
              .map(([k, v]) => ({
                label: k,
                value: typeof v === "object" ? JSON.stringify(v) : String(v),
                mono: true,
              }))
          : [{ label: "—", value: t("adminCommon.noResults"), tone: "muted" as const }],
    },
  ];

  if (authMethods.length > 0) {
    sections.push({
      title: "auth methods",
      rows: authMethods.map((m) => ({ label: m, value: "✓", mono: true, tone: "success" as const })),
    });
  }

  if (configSchema && Object.keys(configSchema).length > 0) {
    sections.push({
      title: t("adminProviders.configSchema"),
      rows: Object.entries(configSchema)
        .slice(0, 10)
        .map(([k, v]) => ({
          label: k,
          value: typeof v === "object" ? JSON.stringify(v) : String(v),
          mono: true,
        })),
    });
  }

  return <InlineDetailGrid sections={sections} />;
}
