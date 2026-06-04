"use client";

import { useState } from "react";
import { ShieldCheck } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { Button } from "@/components/ui/button";
import { useAdminList } from "@/hooks/use-admin-list";
import { useClientPagedList } from "@/hooks/use-client-list";
import { useAdminRoles, useCreateAdminRole } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { emptyRoleForm, buildRoleBody, type RoleFormState } from "@/lib/admin-role-form";
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

export default function AdminRolesPage() {
  return (
    <AdminShell>
      <RolesContent />
    </AdminShell>
  );
}

function RolesContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const all = useAdminRoles();
  const { query, total } = useClientPagedList(all, list, { match: roleMatch, compare: roleCompare });
  const createMut = useCreateAdminRole();
  const [creating, setCreating] = useState(false);
  const isFiltered = Boolean(list.search);

  const fields: FieldConfig<RoleFormState>[] = [
    {
      name: "name",
      label: t("adminRoles.name"),
      required: true,
      placeholder: t("adminRoles.namePlaceholder"),
    },
    { name: "description", label: t("adminCommon.description") },
    {
      name: "permissions",
      label: t("adminRoles.permissions"),
      type: "tags",
      placeholder: t("adminRoles.permissionsPlaceholder"),
      hint: t("adminRoles.permissionsHint"),
    },
  ];

  const columns: Column<Role>[] = [
    {
      key: "name",
      header: t("adminRoles.name"),
      render: (r) => <span className="font-mono text-xs text-srapi-text-primary">{r.name}</span>,
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
            <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
              ＋ {t("adminRoles.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
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
      />

      {creating ? (
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
    </>
  );
}
