"use client";

import { useState } from "react";
import { Cpu } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ADMIN_ROUTES } from "@/lib/routes";
import { PRESET_MODEL_NAMES } from "@/app/admin/quick-setup/presets";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useAdminModels,
  useAdminProviders,
  useCreateModel,
  useUpdateModel,
  useDeleteModel,
  useCreateModelAlias,
  useCreateModelMapping,
  useUpdateModelAlias,
  useUpdateModelMapping,
  useModelRateLimits,
  useUpsertModelRateLimit,
  useDeleteModelRateLimit,
} from "@/hooks/admin-queries";
import { RateLimitDialog } from "@/components/admin/rate-limit-dialog";
import { ModelDetailDialog } from "@/components/admin/model-detail-dialog";
import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { rateLimitSummary } from "@/lib/rate-limit-format";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import {
  MODEL_STATUSES,
  emptyModelForm,
  modelFormFromModel,
  buildCreateModelBody,
  buildUpdateModelBody,
  emptyModelAliasForm,
  buildCreateModelAliasBody,
  emptyModelMappingForm,
  buildCreateModelMappingBody,
  modelAliasFormFromRow,
  modelMappingFormFromRow,
  buildUpdateModelAliasBody,
  buildUpdateModelMappingBody,
  type ModelFormState,
  type ModelAliasFormState,
  type ModelMappingFormState,
} from "@/lib/admin-model-form";
import { MODEL_CAPABILITY_OPTIONS } from "@/lib/capabilities";
import type { Model, ModelAlias, ModelProviderMapping, ModelRateLimit } from "@/lib/sdk-types";

export default function AdminModelsPage() {
  return (
    <AdminShell>
      <ModelsContent />
    </AdminShell>
  );
}

function ModelsContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-models", []);
  const statusFilter = (list.filters.status as Model["status"]) || undefined;
  const models = useAdminModels({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
    q: list.search || undefined,
  });
  const createMut = useCreateModel();
  const updateMut = useUpdateModel();
  const rateLimits = useModelRateLimits();
  const upsertRl = useUpsertModelRateLimit();
  const deleteRl = useDeleteModelRateLimit();
  const aliasMut = useCreateModelAlias();
  const mappingMut = useCreateModelMapping();
  const aliasUpdateMut = useUpdateModelAlias();
  const mappingUpdateMut = useUpdateModelMapping();
  const deleteMut = useDeleteModel();
  // Provider picker for the mapping dialog (the registry is small; 200 covers it).
  const providers = useAdminProviders({ page: 1, page_size: 200 });
  const providerOptions = (providers.data?.data ?? []).filter((p) => p.status === "active").map((p) => ({
    value: p.id,
    label: p.display_name || p.name,
  }));
  const providerLabels = new Map(providerOptions.map((o) => [o.value, o.label]));
  const isFiltered = Boolean(statusFilter || list.search);

  const [formTarget, setFormTarget] = useState<Model | "new" | null>(null);
  const [rateLimitTarget, setRateLimitTarget] = useState<Model | null>(null);
  const [aliasTarget, setAliasTarget] = useState<Model | null>(null);
  const [mappingTarget, setMappingTarget] = useState<Model | null>(null);
  const [detailTarget, setDetailTarget] = useState<Model | null>(null);
  // Inline edit targets — when set the per-row Edit dialog opens with the row
  // pre-populated. Kept separate from the create targets above so an open
  // edit doesn't accidentally fall into a "create" code path.
  const [aliasEditTarget, setAliasEditTarget] = useState<{ model: Model; alias: ModelAlias } | null>(null);
  const [mappingEditTarget, setMappingEditTarget] = useState<{ model: Model; mapping: ModelProviderMapping } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Model | null>(null);
  const rateLimitByModel = new Map<number, ModelRateLimit>(
    (rateLimits.data?.data ?? []).map((rl) => [rl.model_id, rl]),
  );

  const sharedFields: FieldConfig<ModelFormState>[] = [
    { name: "displayName", label: t("adminModels.displayName"), required: true },
    { name: "family", label: t("adminModels.family"), help: t("adminModels.familyHelp"), placeholder: "gpt, claude, gemini" },
    { name: "contextWindow", label: t("adminModels.contextWindow"), help: t("adminModels.contextWindowHelp"), type: "number", placeholder: "128000" },
    { name: "maxOutputTokens", label: t("adminModels.maxOutput"), help: t("adminModels.maxOutputHelp"), type: "number", placeholder: "16384" },
    {
      name: "capabilities",
      label: t("adminModels.capabilities"),
      type: "multiselect",
      options: MODEL_CAPABILITY_OPTIONS,
      hint: t("adminModels.capabilitiesHint"),
    },
    { name: "qualityTier", label: t("adminModels.qualityTier"), help: t("adminModels.qualityTierHelp"), placeholder: "premium, standard", advanced: true },
    { name: "status", label: t("adminCommon.status"), type: "select", options: enumOptions(MODEL_STATUSES) },
  ];

  const createFields: FieldConfig<ModelFormState>[] = [
    {
      name: "canonicalName",
      label: t("adminModels.canonicalName"),
      placeholder: "gpt-4o-mini",
      hint: t("adminModels.canonicalHint"),
      required: true,
      suggestions: PRESET_MODEL_NAMES,
    },
    ...sharedFields,
  ];

  const aliasFields: FieldConfig<ModelAliasFormState>[] = [
    {
      name: "alias",
      label: t("adminModels.alias"),
      hint: t("adminModels.aliasHint"),
      required: true,
      placeholder: "gpt-4o",
      suggestions: PRESET_MODEL_NAMES,
    },
    { name: "status", label: t("adminCommon.status"), type: "select", options: enumOptions(MODEL_STATUSES) },
    {
      name: "strategyHint",
      label: t("adminModels.strategyHintLabel"),
      hint: t("adminModels.strategyHintHint"),
      advanced: true,
    },
    {
      name: "fallbackModelsText",
      label: t("adminModels.fallbackModels"),
      type: "textarea",
      hint: t("adminModels.fallbackModelsHint"),
      advanced: true,
    },
  ];

  const mappingFields: FieldConfig<ModelMappingFormState>[] = [
    {
      name: "providerId",
      label: t("adminModels.mappingProvider"),
      type: "select",
      options: providerOptions,
      required: true,
    },
    {
      name: "upstreamModelName",
      label: t("adminModels.upstreamModelName"),
      hint: t("adminModels.upstreamModelNameHint"),
      required: true,
      placeholder: "gpt-4o-2024-08-06",
      suggestions: PRESET_MODEL_NAMES,
    },
    { name: "status", label: t("adminCommon.status"), type: "select", options: enumOptions(MODEL_STATUSES) },
    {
      name: "capabilities",
      label: t("adminModels.capabilityOverride"),
      type: "multiselect",
      options: MODEL_CAPABILITY_OPTIONS,
      hint: t("adminModels.capabilityOverrideHint"),
      advanced: true,
    },
    {
      name: "pricingOverride",
      label: t("adminModels.pricingOverride"),
      type: "keyvalue",
      hint: t("adminModels.pricingOverrideHint"),
      advanced: true,
    },
  ];

  const columns: Column<Model>[] = [
    {
      key: "name",
      header: t("adminModels.canonicalName"),
      pinned: true,
      sortValue: (m) => m.canonical_name,
      render: (m) => (
        <div className="min-w-0">
          <div className="truncate text-srapi-text-primary">{m.display_name}</div>
          <div className="truncate font-mono text-2xs text-srapi-text-tertiary">{m.canonical_name}</div>
        </div>
      ),
    },
    {
      key: "family",
      header: t("adminModels.family"),
      hideOnMobile: true,
      sortValue: (m) => m.family ?? "",
      render: (m) => <span className="text-srapi-text-secondary">{m.family || "—"}</span>,
    },
    {
      key: "context",
      header: t("adminModels.contextWindow"),
      align: "right",
      hideOnMobile: true,
      sortValue: (m) => m.context_window ?? 0,
      render: (m) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {m.context_window != null ? m.context_window.toLocaleString() : "—"}
        </span>
      ),
    },
    {
      key: "ratelimit",
      header: t("adminRateLimit.column"),
      hideOnMobile: true,
      render: (m) => {
        const rl = rateLimitByModel.get(Number(m.id));
        if (!rl) {
          return <span className="text-2xs text-srapi-text-tertiary">{t("adminRateLimit.none")}</span>;
        }
        return (
          <span className="font-mono text-2xs text-srapi-text-secondary tabular">
            {rl.enabled ? rateLimitSummary(rl) : t("adminRateLimit.off")}
          </span>
        );
      },
    },
    {
      key: "status",
      header: t("common.active"),
      sortValue: (m) => m.status,
      render: (m) => <QuietBadge status={quietStatusFor(m.status)} label={statusLabel(t, m.status)} />,
    },
  ];

  const toggleColumns = columns
    .filter((c) => !c.pinned)
    .map((c) => ({ key: c.key, label: c.header }));

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminModels.title")}
        description={t("adminModels.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {models.data ? (
              <ListCount total={models.data.pagination?.total ?? models.data.data.length} />
            ) : null}
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminModels.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={models}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(m) => m.id}
        emptyIcon={Cpu}
        emptyTitle={t("adminModels.emptyTitle")}
        emptyBody={t("adminModels.emptyBody")}
        emptyAction={
          <div className="flex gap-2">
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminModels.create")}
            </Button>
            <Button variant="outline" size="sm" asChild>
              <a href={ADMIN_ROUTES.quickSetup}>{t("adminModels.emptyQuickSetup")}</a>
            </Button>
          </div>
        }
        minWidth={560}
        sort={list.sort}
        onSort={list.toggleSort}
        dimRow={(m) => m.status === "disabled"}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminCommon.search")}
            />
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={enumOptions(MODEL_STATUSES)}
              allLabel={t("adminCommon.allStatuses")}
            />
            <ColumnToggle columns={toggleColumns} visibility={colVis} />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: models.data?.pagination?.total ?? models.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(m) => (
          <RowActionsMenu
            actions={[
              { label: t("common.edit"), onSelect: () => setFormTarget(m) },
              { label: t("adminModels.addAlias"), onSelect: () => setAliasTarget(m) },
              { label: t("adminModels.addMapping"), onSelect: () => setMappingTarget(m) },
              { label: t("adminRateLimit.action"), onSelect: () => setRateLimitTarget(m) },
              { label: t("adminModels.manageRouting"), onSelect: () => setDetailTarget(m) },
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(m) },
            ]}
          />
        )}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("adminModels.deleteTitle")}
        body={t("adminModels.deleteBody")}
        confirmLabel={t("common.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteMut.isPending}
        onConfirm={async () => {
          if (deleteTarget) await deleteMut.mutateAsync(deleteTarget.id);
        }}
      />

      {formTarget === "new" ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={t("adminModels.create")}
          fields={createFields}
          initial={emptyModelForm()}
          buildBody={buildCreateModelBody}
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
          title={t("adminModels.edit")}
          description={formTarget.canonical_name}
          fields={sharedFields}
          initial={modelFormFromModel(formTarget)}
          buildBody={buildUpdateModelBody}
          submit={(body) => updateMut.mutateAsync({ id: formTarget.id, body })}
          successMessage={t("feedback.updated")}
          isPending={updateMut.isPending}
        />
      ) : null}

      {rateLimitTarget ? (
        <RateLimitDialog
          open
          onOpenChange={(open) => {
            if (!open) setRateLimitTarget(null);
          }}
          title={t("adminRateLimit.title", { name: rateLimitTarget.display_name })}
          current={rateLimitByModel.get(Number(rateLimitTarget.id))}
          onSubmit={(values) =>
            upsertRl.mutateAsync({ model_id: Number(rateLimitTarget.id), ...values })
          }
          onClear={
            rateLimitByModel.has(Number(rateLimitTarget.id))
              ? () => deleteRl.mutateAsync(rateLimitTarget.id)
              : undefined
          }
          isPending={upsertRl.isPending || deleteRl.isPending}
        />
      ) : null}

      {aliasTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setAliasTarget(null);
          }}
          title={t("adminModels.aliasTitle")}
          description={aliasTarget.canonical_name}
          fields={aliasFields}
          initial={emptyModelAliasForm()}
          buildBody={buildCreateModelAliasBody}
          submit={(body) => aliasMut.mutateAsync({ id: aliasTarget.id, body })}
          successMessage={t("feedback.created")}
          isPending={aliasMut.isPending}
        />
      ) : null}

      {mappingTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setMappingTarget(null);
          }}
          title={t("adminModels.mappingTitle")}
          description={mappingTarget.canonical_name}
          fields={mappingFields}
          initial={emptyModelMappingForm()}
          buildBody={buildCreateModelMappingBody}
          submit={(body) => mappingMut.mutateAsync({ id: mappingTarget.id, body })}
          successMessage={t("feedback.created")}
          isPending={mappingMut.isPending}
        />
      ) : null}

      {detailTarget ? (
        <ModelDetailDialog
          model={detailTarget}
          providerLabels={providerLabels}
          onClose={() => setDetailTarget(null)}
          onAddAlias={() => {
            const m = detailTarget;
            setDetailTarget(null);
            setAliasTarget(m);
          }}
          onAddMapping={() => {
            const m = detailTarget;
            setDetailTarget(null);
            setMappingTarget(m);
          }}
          onEditAlias={(alias) => {
            const m = detailTarget;
            setDetailTarget(null);
            setAliasEditTarget({ model: m, alias });
          }}
          onEditMapping={(mapping) => {
            const m = detailTarget;
            setDetailTarget(null);
            setMappingEditTarget({ model: m, mapping });
          }}
        />
      ) : null}

      {aliasEditTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setAliasEditTarget(null);
          }}
          title={t("adminModels.editAliasTitle")}
          description={aliasEditTarget.alias.alias}
          fields={aliasFields}
          initial={modelAliasFormFromRow(aliasEditTarget.alias)}
          buildBody={buildUpdateModelAliasBody}
          submit={(body) =>
            aliasUpdateMut.mutateAsync({
              id: aliasEditTarget.model.id,
              aliasId: aliasEditTarget.alias.id,
              body,
            })
          }
          successMessage={t("feedback.updated")}
          isPending={aliasUpdateMut.isPending}
        />
      ) : null}

      {mappingEditTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setMappingEditTarget(null);
          }}
          title={t("adminModels.editMappingTitle")}
          description={`${providerLabels.get(mappingEditTarget.mapping.provider_id) ?? mappingEditTarget.mapping.provider_id} · ${mappingEditTarget.mapping.upstream_model_name}`}
          // Provider cannot change on a PATCH (the backend rejects provider
          // reassignment to preserve audit history) — hide it on edit by
          // dropping the providerId field from the schema.
          fields={mappingFields.filter((f) => f.name !== "providerId")}
          initial={modelMappingFormFromRow(mappingEditTarget.mapping)}
          buildBody={buildUpdateModelMappingBody}
          submit={(body) =>
            mappingUpdateMut.mutateAsync({
              id: mappingEditTarget.model.id,
              mappingId: mappingEditTarget.mapping.id,
              body,
            })
          }
          successMessage={t("feedback.updated")}
          isPending={mappingUpdateMut.isPending}
        />
      ) : null}
    </>
  );
}
