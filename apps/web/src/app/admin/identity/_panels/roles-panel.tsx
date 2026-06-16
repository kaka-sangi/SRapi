"use client";

import { useState } from "react";
import { ShieldCheck } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { Button } from "@/components/ui/button";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useClientPagedList } from "@/hooks/use-client-list";
import {
  useAdminRoles,
  useAdminPermissionCatalog,
  useCreateAdminRole,
  useUpdateAdminRole,
  useDeleteAdminRole,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import {
  emptyRoleForm,
  roleFormFromRole,
  buildRoleBody,
  buildRoleUpdateBody,
  isBuiltInRole,
  type RoleFormState,
} from "@/lib/admin-role-form";
import type { Role } from "@/lib/sdk-types";

function roleMatch(role: Role, term: string): boolean {
  if (!term) return true;
  return [role.name, role.description, ...(role.permissions ?? [])]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

const roleCompare = (a: Role, b: Role) => a.name.localeCompare(b.name);

export function RolesPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-roles", ["created"]);
  const all = useAdminRoles();
  const permissionCatalog = useAdminPermissionCatalog();
  const { query, total } = useClientPagedList(all, list, { match: roleMatch, compare: roleCompare });
  const createMut = useCreateAdminRole();
  const updateMut = useUpdateAdminRole();
  const deleteMut = useDeleteAdminRole();
  const [creating, setCreating] = useState(false);
  const [editTarget, setEditTarget] = useState<Role | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Role | null>(null);
  const isFiltered = Boolean(list.search);

  const permissionsField: FieldConfig<RoleFormState> = {
    name: "permissions",
    label: t("adminRoles.permissions"),
    type: "combobox",
    options: (permissionCatalog.data ?? []).map((permission) => ({
      value: permission.permission,
      label: `${permission.permission} · ${permission.description}`,
    })),
    placeholder: t("adminRoles.permissionsPlaceholder"),
    searchPlaceholder: t("adminRoles.permissionsPlaceholder"),
    emptyText: t("adminCommon.noResults"),
    hint: t("adminRoles.permissionsHint"),
  };

  const fields: FieldConfig<RoleFormState>[] = [
    {
      name: "name",
      label: t("adminRoles.name"),
      required: true,
      placeholder: t("adminRoles.namePlaceholder"),
    },
    { name: "description", label: t("adminCommon.description") },
    permissionsField,
  ];

  // The role name is the immutable identity, so it's omitted from the edit form.
  const editFields: FieldConfig<RoleFormState>[] = [
    { name: "description", label: t("adminCommon.description") },
    permissionsField,
  ];

  const columns: Column<Role>[] = [
    {
      key: "name",
      header: t("adminRoles.name"),
      pinned: true,
      render: (r) => (
        <div className="flex items-center gap-2">
          <span className="font-mono text-xs text-srapi-text-primary">{r.name}</span>
          {isBuiltInRole(r.name) ? (
            <span className="rounded border border-srapi-border px-1 py-px text-[10px] uppercase tracking-wide text-srapi-text-tertiary">
              {t("adminRoles.builtIn")}
            </span>
          ) : null}
        </div>
      ),
      sortValue: (r) => r.name,
    },
    {
      key: "description",
      header: t("adminCommon.description"),
      hideOnMobile: true,
      render: (r) =>
        r.description ? (
          <span className="block max-w-[24rem] truncate text-srapi-text-secondary">
            {r.description}
          </span>
        ) : (
          <span className="text-srapi-text-tertiary">—</span>
        ),
    },
    {
      key: "permissions",
      header: t("adminRoles.permissions"),
      render: (r) => {
        const perms = r.permissions ?? [];
        if (perms.length === 0) return <span className="text-srapi-text-tertiary">—</span>;
        return (
          <div className="flex flex-wrap items-center gap-1">
            {perms.slice(0, 3).map((p) => (
              <span
                key={p}
                className="rounded-md border border-srapi-border px-1.5 py-0.5 font-mono text-2xs text-srapi-text-secondary"
              >
                {p}
              </span>
            ))}
            {perms.length > 3 ? (
              <span className="font-mono text-2xs text-srapi-text-tertiary">
                +{perms.length - 3}
              </span>
            ) : null}
          </div>
        );
      },
    },
    {
      key: "created",
      header: t("adminCommon.created"),
      hideOnMobile: true,
      align: "right",
      render: (r) => (
        <span className="font-mono text-2xs tabular text-srapi-text-tertiary">
          {new Date(r.created_at).toLocaleDateString()}
        </span>
      ),
      sortValue: (r) => r.created_at,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminRoles.title")}
        description={t("adminRoles.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
              ＋ {t("adminRoles.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(r) => String(r.id)}
        emptyIcon={ShieldCheck}
        emptyTitle={t("adminRoles.emptyTitle")}
        emptyBody={t("adminRoles.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
            ＋ {t("adminRoles.create")}
          </Button>
        }
        minWidth={620}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminRoles.searchPlaceholder")}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(r) =>
          isBuiltInRole(r.name) ? null : (
            <RowActionsMenu
              actions={[
                { label: t("common.edit"), onSelect: () => setEditTarget(r) },
                {
                  label: t("common.delete"),
                  destructive: true,
                  onSelect: () => setDeleteTarget(r),
                },
              ]}
            />
          )
        }
      />

      {creating && permissionCatalog.data ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setCreating(false);
          }}
          title={t("adminRoles.create")}
          description={t("adminRoles.subtitle")}
          fields={fields}
          initial={emptyRoleForm()}
          buildBody={buildRoleBody}
          submit={(body) => createMut.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createMut.isPending}
        />
      ) : null}

      {editTarget && permissionCatalog.data ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setEditTarget(null);
          }}
          title={t("adminRoles.edit")}
          description={editTarget.name}
          fields={editFields}
          initial={roleFormFromRole(editTarget)}
          buildBody={buildRoleUpdateBody}
          submit={(body) => updateMut.mutateAsync({ id: String(editTarget.id), body })}
          successMessage={t("feedback.updated")}
          isPending={updateMut.isPending}
        />
      ) : null}

      {deleteTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("adminRoles.deleteTitle")}
          body={t("adminRoles.deleteBody", { name: deleteTarget.name })}
          confirmLabel={t("common.delete")}
          onConfirm={() => deleteMut.mutateAsync(String(deleteTarget.id))}
          successMessage={t("feedback.deleted")}
          isPending={deleteMut.isPending}
        />
      ) : null}
    </>
  );
}
