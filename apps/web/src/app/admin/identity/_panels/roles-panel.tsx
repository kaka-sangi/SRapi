"use client";

import { useMemo, useState } from "react";
import { ShieldCheck } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
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
import type { PermissionDefinition, Role } from "@/lib/sdk-types";

type RoleKind = "__all__" | "builtIn" | "custom";

function roleMatch(role: Role, term: string, filters: Record<string, string>): boolean {
  const kind = (filters.kind as RoleKind | undefined) ?? "__all__";
  if (kind === "builtIn" && !isBuiltInRole(role.name)) return false;
  if (kind === "custom" && isBuiltInRole(role.name)) return false;
  if (!term) return true;
  return [role.name, role.description, ...(role.permissions ?? [])]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

const roleCompare = (a: Role, b: Role) => a.name.localeCompare(b.name);

// Split a `resource:action` permission key into its parts so the expand-row
// can render a compact `resource · action` matrix grouped by resource.
function parsePermission(key: string): { resource: string; action: string } {
  const idx = key.indexOf(":");
  if (idx < 0) return { resource: key, action: "*" };
  return { resource: key.slice(0, idx), action: key.slice(idx + 1) };
}

function countActions(
  perms: ReadonlyArray<string>,
  catalog: ReadonlyArray<PermissionDefinition> | undefined,
): { read: number; write: number } {
  // Prefer the catalog's authoritative action mapping; fall back to the
  // trailing segment of the permission key when the catalog hasn't loaded.
  const byKey = new Map(catalog?.map((p) => [p.permission, p.action]) ?? []);
  let read = 0;
  let write = 0;
  for (const p of perms) {
    const action = byKey.get(p) ?? parsePermission(p).action;
    if (action === "read") read += 1;
    else if (action === "write") write += 1;
  }
  return { read, write };
}

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
  const isFiltered = Boolean(list.search || list.filters.kind);

  // Index the permission catalog once so per-row tooltips can resolve a key's
  // description / action without scanning the array on every render.
  const permissionIndex = useMemo(() => {
    const map = new Map<string, PermissionDefinition>();
    for (const p of permissionCatalog.data ?? []) map.set(p.permission, p);
    return map;
  }, [permissionCatalog.data]);

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
            <DataPill tone="neutral" size="sm">
              {t("adminRoles.builtIn")}
            </DataPill>
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
        const { read, write } = countActions(perms, permissionCatalog.data);
        if (perms.length === 0) {
          return <span className="text-srapi-text-tertiary">—</span>;
        }
        return (
          <DataTooltip
            title={t("adminRoles.permissionsBreakdown")}
            primary={String(perms.length)}
            rows={[
              { label: t("adminRoles.permissionsReadCount"), value: String(read), tone: "muted" },
              { label: t("adminRoles.permissionsWriteCount"), value: String(write), tone: write > 0 ? "warning" : "muted" },
              { label: t("adminRoles.permissions"), value: perms.slice(0, 3).join(", ") + (perms.length > 3 ? "…" : ""), tone: "muted" },
            ]}
            footer={isBuiltInRole(r.name) ? t("adminRoles.builtIn") : undefined}
          >
            <div className="flex flex-wrap items-center gap-1">
              {perms.slice(0, 3).map((p) => (
                <span
                  key={p}
                  className="rounded-full bg-srapi-card-muted px-2 py-0.5 font-mono text-[11px] font-medium text-srapi-text-secondary"
                >
                  {p}
                </span>
              ))}
              {perms.length > 3 ? (
                <span className="text-[11px] font-medium text-srapi-text-tertiary tabular">
                  +{perms.length - 3}
                </span>
              ) : null}
            </div>
          </DataTooltip>
        );
      },
    },
    {
      key: "created",
      header: t("adminCommon.created"),
      hideOnMobile: true,
      align: "right",
      render: (r) => (
        <span className="text-[12px] tabular text-srapi-text-tertiary">
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
        emptyContent={
          <IllustratedEmptyState
            illust="users"
            title={t("adminRoles.emptyTitle")}
            description={t("adminRoles.emptyBody")}
            action={
              <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
                ＋ {t("adminRoles.create")}
              </Button>
            }
          />
        }
        minWidth={620}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        enableKeyboardNav
        // Built-in roles can't be deleted — surface that with a quiet info
        // stripe so operators see at a glance which rows are read-only.
        rowSeverity={(r) => (isBuiltInRole(r.name) ? "info" : undefined)}
        expandRow={(r) => {
          const perms = r.permissions ?? [];
          const { read, write } = countActions(perms, permissionCatalog.data);
          // Group permissions by resource so a 12-permission role reads as
          // "users: read,write · usage: read" rather than a wall of chips.
          const grouped = new Map<string, string[]>();
          for (const key of perms) {
            const { resource, action } = parsePermission(key);
            const list = grouped.get(resource) ?? [];
            list.push(action);
            grouped.set(resource, list);
          }
          const resources = [...grouped.keys()].sort();
          return (
            <>
              <InlineDetailGrid
                sections={[
                  {
                    title: t("adminRoles.permissionsBreakdown"),
                    rows: [
                      { label: t("adminRoles.permissions"), value: String(perms.length) },
                      { label: t("adminRoles.permissionsReadCount"), value: String(read), tone: "muted" },
                      { label: t("adminRoles.permissionsWriteCount"), value: String(write), tone: write > 0 ? "warning" : "muted" },
                    ],
                  },
                  {
                    title: t("adminCommon.description"),
                    rows: [
                      { label: t("adminRoles.name"), value: r.name, mono: true },
                      {
                        label: t("adminCommon.description"),
                        value: r.description || "—",
                        tone: r.description ? undefined : "muted",
                      },
                      {
                        label: t("adminRoles.builtIn"),
                        value: isBuiltInRole(r.name) ? t("common.active") : t("common.disabled"),
                        tone: isBuiltInRole(r.name) ? undefined : "muted",
                      },
                    ],
                  },
                  {
                    title: t("adminRoles.metadata"),
                    rows: [
                      { label: t("common.created"), value: r.created_at ? formatDateTime(r.created_at) : "—", tone: "muted" },
                      { label: t("common.updated"), value: r.updated_at ? formatDateTime(r.updated_at) : "—", tone: "muted" },
                    ],
                  },
                ]}
              />
              <div className="border-t border-srapi-border/60 bg-srapi-card-muted/30 px-6 py-4">
                <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("adminRoles.permissionMatrix")}
                </div>
                {resources.length === 0 ? (
                  <p className="text-xs text-srapi-text-tertiary">
                    {t("adminRoles.noPermissions")}
                  </p>
                ) : (
                  <div className="grid gap-x-6 gap-y-2 sm:grid-cols-2 lg:grid-cols-3">
                    {resources.map((resource) => {
                      const actions = grouped.get(resource) ?? [];
                      return (
                        <div key={resource} className="flex items-center gap-2 text-xs">
                          <span className="min-w-[6rem] truncate font-mono text-srapi-text-secondary">
                            {resource}
                          </span>
                          <div className="flex flex-wrap gap-1">
                            {actions.map((action) => {
                              const fullKey = `${resource}:${action}`;
                              const def = permissionIndex.get(fullKey);
                              return (
                                <DataTooltip
                                  key={action}
                                  title={fullKey}
                                  primary={action}
                                  rows={
                                    def
                                      ? [
                                          { label: t("adminCommon.description"), value: def.description, tone: "muted" },
                                          { label: "action", value: def.action, tone: def.action === "write" ? "warning" : "muted" },
                                        ]
                                      : undefined
                                  }
                                >
                                  <DataPill
                                    tone={action === "write" ? "warning" : "neutral"}
                                    size="sm"
                                  >
                                    {action}
                                  </DataPill>
                                </DataTooltip>
                              );
                            })}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </>
          );
        }}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminRoles.searchPlaceholder")}
            />
            <SegmentedControl<RoleKind>
              value={(list.filters.kind as RoleKind | undefined) ?? "__all__"}
              onChange={(v) => list.setFilter("kind", v === "__all__" ? undefined : v)}
              ariaLabel={t("adminRoles.title")}
              size="sm"
              options={[
                { value: "__all__", label: t("adminRoles.filter_all") },
                { value: "builtIn", label: t("adminRoles.filter_builtIn") },
                { value: "custom", label: t("adminRoles.filter_custom") },
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
