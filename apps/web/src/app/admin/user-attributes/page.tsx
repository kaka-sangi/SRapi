"use client";

import { useState } from "react";
import { Tags } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { useAdminList } from "@/hooks/use-admin-list";
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

function definitionMatch(definition: UserAttributeDefinition, term: string): boolean {
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

export default function AdminUserAttributesPage() {
  return (
    <AdminShell>
      <UserAttributesContent />
    </AdminShell>
  );
}

function UserAttributesContent() {
  const { t } = useLanguage();
  const list = useAdminList();
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
  const isFiltered = Boolean(list.search);

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
      render: (d) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{d.data_type}</span>
      ),
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
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">{d.display_order}</span>
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
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminUserAttributes.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        getRowId={(d) => String(d.id)}
        emptyIcon={Tags}
        emptyTitle={t("adminUserAttributes.emptyTitle")}
        emptyBody={t("adminUserAttributes.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminUserAttributes.create")}
          </Button>
        }
        minWidth={680}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminUserAttributes.searchPlaceholder")}
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
              : (body) => updateMut.mutateAsync({ id: String(formTarget.id), body })
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
