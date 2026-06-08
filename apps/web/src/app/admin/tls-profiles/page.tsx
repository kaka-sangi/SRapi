"use client";

import { useState } from "react";
import { Fingerprint } from "lucide-react";
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
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useClientPagedList } from "@/hooks/use-client-list";
import {
  useTlsProfiles,
  useCreateTlsProfile,
  useUpdateTlsProfile,
  useDeleteTlsProfile,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import {
  TLS_TEMPLATES,
  HTTP_VERSION_POLICIES,
  emptyTlsProfileForm,
  tlsProfileFormFromProfile,
  buildTlsProfileBody,
  type TlsProfileFormState,
} from "@/lib/admin-tls-profile-form";
import type { TlsProfile } from "@/lib/sdk-types";

function profileMatch(profile: TlsProfile, term: string): boolean {
  if (!term) return true;
  return [profile.name, profile.tls_template, profile.http_version_policy, profile.user_agent]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

const profileCompare = (a: TlsProfile, b: TlsProfile) => a.name.localeCompare(b.name);

export default function AdminTlsProfilesPage() {
  return (
    <AdminShell>
      <TlsProfilesContent />
    </AdminShell>
  );
}

function TlsProfilesContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-tls-profiles", []);
  const all = useTlsProfiles();
  const { query, total } = useClientPagedList(all, list, { match: profileMatch, compare: profileCompare });

  const createMut = useCreateTlsProfile();
  const updateMut = useUpdateTlsProfile();
  const deleteMut = useDeleteTlsProfile();

  const [formTarget, setFormTarget] = useState<TlsProfile | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<TlsProfile | null>(null);
  const isNew = formTarget === "new";
  const isFiltered = Boolean(list.search);

  const fields: FieldConfig<TlsProfileFormState>[] = [
    { name: "name", label: t("adminTlsProfiles.name") },
    {
      name: "tls_template",
      label: t("adminTlsProfiles.template"),
      help: t("adminTlsProfiles.templateHelp"),
      type: "select",
      options: TLS_TEMPLATES.map((value) => ({ value, label: value })),
    },
    {
      name: "http_version_policy",
      label: t("adminTlsProfiles.httpPolicy"),
      help: t("adminTlsProfiles.httpPolicyHelp"),
      type: "select",
      options: HTTP_VERSION_POLICIES.map((value) => ({ value, label: value })),
    },
    {
      name: "user_agent",
      label: t("adminTlsProfiles.userAgent"),
      hint: t("adminTlsProfiles.userAgentHint"),
    },
    { name: "enabled", label: t("adminTlsProfiles.enabled"), type: "switch" },
    { name: "extra_headers", label: t("adminTlsProfiles.extraHeaders"), help: t("adminTlsProfiles.extraHeadersHelp"), type: "keyvalue", advanced: true },
  ];

  const columns: Column<TlsProfile>[] = [
    {
      key: "name",
      header: t("adminTlsProfiles.name"),
      pinned: true,
      render: (p) => <span className="text-srapi-text-primary">{p.name}</span>,
    },
    {
      key: "template",
      header: t("adminTlsProfiles.template"),
      render: (p) => (
        <span className="font-mono text-xs text-srapi-text-secondary">
          {p.tls_template || "default"}
        </span>
      ),
    },
    {
      key: "httpPolicy",
      header: t("adminTlsProfiles.httpPolicy"),
      hideOnMobile: true,
      render: (p) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{p.http_version_policy}</span>
      ),
    },
    {
      key: "userAgent",
      header: t("adminTlsProfiles.userAgent"),
      hideOnMobile: true,
      render: (p) =>
        p.user_agent ? (
          <span className="block max-w-[20rem] truncate font-mono text-2xs text-srapi-text-tertiary">
            {p.user_agent}
          </span>
        ) : (
          <span className="text-srapi-text-tertiary">—</span>
        ),
    },
    {
      key: "enabled",
      header: t("adminTlsProfiles.enabled"),
      render: (p) => (
        <QuietBadge
          status={p.enabled ? "active" : "disabled"}
          label={p.enabled ? t("common.active") : t("common.disabled")}
        />
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminTlsProfiles.title")}
        description={t("adminTlsProfiles.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminTlsProfiles.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(p) => String(p.id)}
        emptyIcon={Fingerprint}
        emptyTitle={t("adminTlsProfiles.emptyTitle")}
        emptyBody={t("adminTlsProfiles.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminTlsProfiles.create")}
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
              placeholder={t("adminTlsProfiles.searchPlaceholder")}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(p) => (
          <RowActionsMenu
            actions={[
              { label: t("common.edit"), onSelect: () => setFormTarget(p) },
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(p) },
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
          title={isNew ? t("adminTlsProfiles.create") : t("adminTlsProfiles.edit")}
          fields={fields}
          initial={isNew ? emptyTlsProfileForm() : tlsProfileFormFromProfile(formTarget)}
          buildBody={buildTlsProfileBody}
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
          confirmPhrase={deleteTarget.name}
          onConfirm={() => deleteMut.mutateAsync(String(deleteTarget.id))}
          successMessage={t("feedback.deleted")}
          isPending={deleteMut.isPending}
        />
      ) : null}
    </>
  );
}
