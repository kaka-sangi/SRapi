"use client";

import { useState } from "react";
import { Fingerprint } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import { formatDateTime } from "@/lib/admin-format";
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
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  TLS_TEMPLATES,
  HTTP_VERSION_POLICIES,
  emptyTlsProfileForm,
  tlsProfileFormFromProfile,
  buildTlsProfileBody,
  type TlsProfileFormState,
} from "@/lib/admin-tls-profile-form";
import type { TlsProfile } from "@/lib/sdk-types";

function profileMatch(profile: TlsProfile, term: string, filters: Record<string, string>): boolean {
  // Status filter rides client-side because /admin/tls-profiles already pages
  // in-memory via useClientPagedList — the whole roster is local so we can
  // narrow without an API change.
  if (filters.enabled === "on" && !profile.enabled) return false;
  if (filters.enabled === "off" && profile.enabled) return false;
  if (!term) return true;
  return [profile.name, profile.tls_template, profile.http_version_policy, profile.user_agent]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

const profileCompare = (a: TlsProfile, b: TlsProfile) => a.name.localeCompare(b.name);

export function TlsProfilesPanel() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-tls-profiles", []);
  const all = useTlsProfiles();
  const { query, total } = useClientPagedList(all, list, {
    match: profileMatch,
    compare: profileCompare,
  });

  const createMut = useCreateTlsProfile();
  const updateMut = useUpdateTlsProfile();
  const deleteMut = useDeleteTlsProfile();

  const [formTarget, setFormTarget] = useState<TlsProfile | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<TlsProfile | null>(null);
  const [togglingId, setTogglingId] = useState<number | null>(null);
  const isNew = formTarget === "new";
  const isFiltered = Boolean(list.search || list.filters.enabled);

  async function toggleEnabled(profile: TlsProfile) {
    if (togglingId === profile.id) return;
    setTogglingId(profile.id);
    try {
      await updateMut.mutateAsync({
        id: String(profile.id),
        body: { enabled: !profile.enabled },
      });
      toast({
        title: profile.enabled
          ? t("adminTlsProfiles.toggleDisabled")
          : t("adminTlsProfiles.toggleEnabled"),
        tone: "success",
      });
    } catch (err) {
      toast({ title: adminErrorMessage(err), tone: "error" });
    } finally {
      setTogglingId(null);
    }
  }

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
    {
      name: "extra_headers",
      label: t("adminTlsProfiles.extraHeaders"),
      help: t("adminTlsProfiles.extraHeadersHelp"),
      type: "keyvalue",
      advanced: true,
    },
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
        <DataTooltip
          title={t("adminTlsProfiles.template")}
          primary={p.tls_template || "default"}
          rows={[
            { label: t("adminTlsProfiles.httpPolicy"), value: p.http_version_policy || "—" },
            { label: t("adminTlsProfiles.userAgent"), value: p.user_agent ? p.user_agent.slice(0, 60) : "—", tone: "muted" },
            { label: t("adminTlsProfiles.extraHeaders"), value: String(Object.keys(p.extra_headers ?? {}).length), tone: "muted" },
          ]}
        >
          <span className="text-srapi-text-secondary text-sm tabular">
            {p.tls_template || "default"}
          </span>
        </DataTooltip>
      ),
    },
    {
      key: "httpPolicy",
      header: t("adminTlsProfiles.httpPolicy"),
      hideOnMobile: true,
      render: (p) => (
        <span className="text-xs text-srapi-text-tertiary tabular">{p.http_version_policy}</span>
      ),
    },
    {
      key: "userAgent",
      header: t("adminTlsProfiles.userAgent"),
      hideOnMobile: true,
      render: (p) =>
        p.user_agent ? (
          <span className="text-xs text-srapi-text-tertiary block max-w-[20rem] truncate tabular">
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
        <button
          type="button"
          onClick={() => void toggleEnabled(p)}
          disabled={togglingId === p.id}
          className="cursor-pointer disabled:cursor-wait disabled:opacity-60"
          aria-label={
            p.enabled ? t("adminTlsProfiles.clickToDisable") : t("adminTlsProfiles.clickToEnable")
          }
          title={
            p.enabled ? t("adminTlsProfiles.clickToDisable") : t("adminTlsProfiles.clickToEnable")
          }
        >
          <QuietBadge
            status={p.enabled ? "active" : "disabled"}
            label={p.enabled ? t("common.active") : t("common.disabled")}
          />
        </button>
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
              columns={columns
                .filter((c) => !c.pinned)
                .map((c) => ({ key: c.key, label: c.header }))}
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
        enableKeyboardNav
        rowSeverity={(p) => (p.enabled ? undefined : "info")}
        expandRow={(p) => {
          const headerKeys = Object.keys(p.extra_headers ?? {});
          return (
            <InlineDetailGrid
              sections={[
                {
                  title: t("adminTlsProfiles.template"),
                  rows: [
                    { label: t("adminTlsProfiles.template"), value: p.tls_template || "default" },
                    { label: t("adminTlsProfiles.httpPolicy"), value: p.http_version_policy || "—" },
                    { label: t("adminTlsProfiles.enabled"), value: p.enabled ? t("common.active") : t("common.disabled"), tone: p.enabled ? "success" : "muted" },
                  ],
                },
                {
                  title: t("adminTlsProfiles.userAgent"),
                  rows: [
                    { label: "UA", value: p.user_agent || "—", mono: true, tone: p.user_agent ? "default" : "muted" },
                  ],
                },
                {
                  title: t("adminTlsProfiles.extraHeaders"),
                  rows: headerKeys.length === 0
                    ? [{ label: t("adminTlsProfiles.extraHeaders"), value: "—", tone: "muted" }]
                    : headerKeys.slice(0, 5).map((key) => ({
                        label: key,
                        value: String(p.extra_headers?.[key] ?? ""),
                        mono: true,
                      })),
                },
              ]}
              actions={
                <span className="text-[11px] text-srapi-text-tertiary tabular">
                  {t("common.updated")}: {p.updated_at ? formatDateTime(p.updated_at) : "—"}
                </span>
              }
            />
          );
        }}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminTlsProfiles.searchPlaceholder")}
            />
            <SegmentedControl<string>
              value={list.filters.enabled || "__all__"}
              onChange={(v) => list.setFilter("enabled", v === "__all__" ? undefined : v)}
              ariaLabel={t("adminTlsProfiles.enabled")}
              size="sm"
              options={[
                { value: "__all__", label: t("adminCommon.allStatuses") },
                { value: "on", label: t("common.active") },
                { value: "off", label: t("common.disabled") },
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
