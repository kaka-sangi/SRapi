"use client";

import { useState } from "react";
import { Tags } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { DataPill } from "@/components/ui/data-pill";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { formatDateTime } from "@/lib/admin-format";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useClientPagedList } from "@/hooks/use-client-list";
import {
  useUserAttributeDefinitions,
  useCreateUserAttributeDefinition,
  useUpdateUserAttributeDefinition,
  useDeleteUserAttributeDefinition,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import {
  USER_ATTRIBUTE_DATA_TYPES,
  emptyUserAttributeForm,
  userAttributeFormFromDefinition,
  buildUserAttributeBody,
  type UserAttributeFormState,
} from "@/lib/admin-user-attribute-form";
import type { UserAttributeDefinition } from "@/lib/sdk-types";

type EnabledFilter = "__all__" | "enabled" | "disabled";

function definitionMatch(
  definition: UserAttributeDefinition,
  term: string,
  filters: Record<string, string>,
): boolean {
  const status = (filters.status as EnabledFilter | undefined) ?? "__all__";
  if (status === "enabled" && !definition.enabled) return false;
  if (status === "disabled" && definition.enabled) return false;
  if (!term) return true;
  return [definition.key, definition.name, definition.data_type]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

// Surface in the order operators see them on the profile form.
const definitionCompare = (a: UserAttributeDefinition, b: UserAttributeDefinition) =>
  (a.display_order ?? 0) - (b.display_order ?? 0) || a.name.localeCompare(b.name);

export function UserAttributesPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-user-attributes", []);
  const all = useUserAttributeDefinitions();
  const { query, total } = useClientPagedList(all, list, {
    match: definitionMatch,
    compare: definitionCompare,
  });

  const createMut = useCreateUserAttributeDefinition();
  const updateMut = useUpdateUserAttributeDefinition();
  const deleteMut = useDeleteUserAttributeDefinition();

  const [formTarget, setFormTarget] = useState<UserAttributeDefinition | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<UserAttributeDefinition | null>(null);
  const isNew = formTarget === "new";
  const isFiltered = Boolean(list.search || list.filters.status);

  // `key` is the stable API identifier — editable only at creation time.
  const fields: FieldConfig<UserAttributeFormState>[] = [
    ...(isNew
      ? [{ name: "key" as const, label: t("adminUserAttributes.key"), hint: t("adminUserAttributes.keyHint") }]
      : []),
    { name: "name", label: t("adminUserAttributes.name") },
    {
      name: "data_type",
      label: t("adminUserAttributes.dataType"),
      type: "select",
      options: USER_ATTRIBUTE_DATA_TYPES.map((value) => ({ value, label: value })),
    },
    {
      name: "options",
      label: t("adminUserAttributes.options"),
      type: "tags",
      hint: t("adminUserAttributes.optionsHint"),
    },
    { name: "required", label: t("adminUserAttributes.required"), type: "switch" },
    { name: "display_order", label: t("adminUserAttributes.order"), type: "number" },
    { name: "enabled", label: t("adminUserAttributes.enabled"), type: "switch" },
  ];

  const columns: Column<UserAttributeDefinition>[] = [
    {
      key: "key",
      header: t("adminUserAttributes.key"),
      pinned: true,
      render: (d) => <span className="font-mono text-xs text-srapi-text-secondary">{d.key}</span>,
    },
    {
      key: "name",
      header: t("adminUserAttributes.name"),
      render: (d) => <span className="text-srapi-text-primary">{d.name}</span>,
    },
    {
      key: "dataType",
      header: t("adminUserAttributes.dataType"),
      render: (d) => {
        const optionCount = d.data_type === "select" ? (d.options ?? []).length : 0;
        return (
          <DataTooltip
            title={t("adminUserAttributes.dataType")}
            primary={d.data_type}
            rows={
              d.data_type === "select"
                ? [
                    {
                      label: t("adminUserAttributes.optionsCount"),
                      value: String(optionCount),
                      tone: optionCount > 0 ? undefined : "warning",
                    },
                    {
                      label: t("adminUserAttributes.required"),
                      value: d.required ? t("common.active") : t("common.disabled"),
                      tone: d.required ? "warning" : "muted",
                    },
                  ]
                : [
                    {
                      label: t("adminUserAttributes.required"),
                      value: d.required ? t("common.active") : t("common.disabled"),
                      tone: d.required ? "warning" : "muted",
                    },
                  ]
            }
          >
            <DataPill tone="neutral" size="sm">
              {d.data_type}
            </DataPill>
          </DataTooltip>
        );
      },
    },
    {
      key: "required",
      header: t("adminUserAttributes.required"),
      hideOnMobile: true,
      render: (d) =>
        d.required ? (
          <span className="text-srapi-text-secondary">{t("adminUserAttributes.required")}</span>
        ) : (
          <span className="text-srapi-text-tertiary">—</span>
        ),
    },
    {
      key: "order",
      header: t("adminUserAttributes.order"),
      align: "right",
      hideOnMobile: true,
      render: (d) => (
        <span className="text-[12px] tabular text-srapi-text-tertiary">{d.display_order}</span>
      ),
    },
    {
      key: "enabled",
      header: t("adminUserAttributes.enabled"),
      render: (d) => (
        <QuietBadge
          status={d.enabled ? "active" : "disabled"}
          label={d.enabled ? t("common.active") : t("common.disabled")}
        />
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminUserAttributes.title")}
        description={t("adminUserAttributes.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminUserAttributes.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(d) => String(d.id)}
        emptyIcon={Tags}
        emptyTitle={t("adminUserAttributes.emptyTitle")}
        emptyBody={t("adminUserAttributes.emptyBody")}
        emptyContent={
          <IllustratedEmptyState
            illust="users"
            title={t("adminUserAttributes.emptyTitle")}
            description={t("adminUserAttributes.emptyBody")}
            action={
              <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
                ＋ {t("adminUserAttributes.create")}
              </Button>
            }
          />
        }
        minWidth={680}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        enableKeyboardNav
        // Disabled attributes are hidden from the profile form but still
        // visible to operators — surface that with a muted info stripe.
        rowSeverity={(d) => (d.enabled ? undefined : "info")}
        expandRow={(d) => {
          const options = d.options ?? [];
          return (
            <>
              <InlineDetailGrid
                sections={[
                  {
                    title: t("adminUserAttributes.schema"),
                    rows: [
                      { label: t("adminUserAttributes.key"), value: d.key, mono: true },
                      { label: t("adminUserAttributes.name"), value: d.name },
                      { label: t("adminUserAttributes.dataType"), value: d.data_type, mono: true },
                    ],
                  },
                  {
                    title: t("adminUserAttributes.required"),
                    rows: [
                      {
                        label: t("adminUserAttributes.required"),
                        value: d.required ? t("common.active") : t("common.disabled"),
                        tone: d.required ? "warning" : "muted",
                      },
                      {
                        label: t("adminUserAttributes.enabled"),
                        value: d.enabled ? t("common.active") : t("common.disabled"),
                        tone: d.enabled ? "success" : "muted",
                      },
                      {
                        label: t("adminUserAttributes.order"),
                        value: String(d.display_order ?? 0),
                        tone: "muted",
                      },
                    ],
                  },
                  {
                    title: t("adminUserAttributes.metadata"),
                    rows: [
                      { label: t("common.created"), value: d.created_at ? formatDateTime(d.created_at) : "—", tone: "muted" },
                      { label: t("common.updated"), value: d.updated_at ? formatDateTime(d.updated_at) : "—", tone: "muted" },
                      {
                        label: t("adminUserAttributes.optionsCount"),
                        value: String(options.length),
                        tone: "muted",
                      },
                    ],
                  },
                ]}
              />
              {d.data_type === "select" ? (
                <div className="border-t border-srapi-border/60 bg-srapi-card-muted/30 px-6 py-4">
                  <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                    {t("adminUserAttributes.options")}
                  </div>
                  {options.length === 0 ? (
                    <p className="text-xs text-srapi-text-tertiary">
                      {t("adminUserAttributes.noOptions")}
                    </p>
                  ) : (
                    <div className="flex flex-wrap gap-1.5">
                      {options.map((opt) => (
                        <DataPill key={opt} tone="neutral" size="sm">
                          {opt}
                        </DataPill>
                      ))}
                    </div>
                  )}
                </div>
              ) : null}
            </>
          );
        }}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminUserAttributes.searchPlaceholder")}
            />
            <SegmentedControl<EnabledFilter>
              value={(list.filters.status as EnabledFilter | undefined) ?? "__all__"}
              onChange={(v) => list.setFilter("status", v === "__all__" ? undefined : v)}
              ariaLabel={t("adminUserAttributes.enabled")}
              size="sm"
              options={[
                { value: "__all__", label: t("adminUserAttributes.filter_all") },
                { value: "enabled", label: t("adminUserAttributes.filter_enabled") },
                { value: "disabled", label: t("adminUserAttributes.filter_disabled") },
              ]}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(d) => (
          <RowActionsMenu
            actions={[
              { label: t("common.edit"), onSelect: () => setFormTarget(d) },
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(d) },
            ]}
          />
        )}
      />

      {formTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={isNew ? t("adminUserAttributes.create") : t("adminUserAttributes.edit")}
          fields={fields}
          initial={isNew ? emptyUserAttributeForm() : userAttributeFormFromDefinition(formTarget)}
          buildBody={buildUserAttributeBody}
          submit={
            isNew
              ? (body) => createMut.mutateAsync(body)
              : (body) => {
                  // `key` is immutable and absent from the update DTO; the backend
                  // strictly rejects unknown fields, so strip it on edit.
                  const { key: _key, ...updateBody } = body;
                  return updateMut.mutateAsync({ id: String(formTarget.id), body: updateBody });
                }
          }
          successMessage={isNew ? t("feedback.created") : t("feedback.updated")}
          isPending={createMut.isPending || updateMut.isPending}
        />
      ) : null}

      {deleteTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("feedback.confirmDeleteTitle", { name: deleteTarget.name })}
          body={t("feedback.confirmDeleteBody")}
          confirmLabel={t("common.delete")}
          confirmPhrase={deleteTarget.key}
          onConfirm={() => deleteMut.mutateAsync(String(deleteTarget.id))}
          successMessage={t("feedback.deleted")}
          isPending={deleteMut.isPending}
        />
      ) : null}
    </>
  );
}
