"use client";

import { useState } from "react";
import { ShieldAlert } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { useAdminList } from "@/hooks/use-admin-list";
import { useClientPagedList } from "@/hooks/use-client-list";
import {
  useErrorPassthroughRules,
  useCreateErrorPassthroughRule,
  useUpdateErrorPassthroughRule,
  useDeleteErrorPassthroughRule,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import {
  ERROR_PASSTHROUGH_ACTIONS,
  emptyErrorPassthroughForm,
  errorPassthroughFormFromRule,
  buildErrorPassthroughBody,
  type ErrorPassthroughFormState,
} from "@/lib/admin-error-passthrough-form";
import type { ErrorPassthroughRule } from "@/lib/sdk-types";

const ACTION_TONE: Record<ErrorPassthroughRule["action"], QuietStatus> = {
  expose: "active",
  mask: "disabled",
};

function ruleMatch(
  rule: ErrorPassthroughRule,
  term: string,
  filters: Record<string, string>,
): boolean {
  if (filters.action && rule.action !== filters.action) return false;
  if (!term) return true;
  return [rule.name, ...(rule.classes ?? []), ...(rule.keywords ?? []), ...(rule.status_codes ?? []).map(String)]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

// Lowest priority number wins first, mirroring how the gateway evaluates rules.
const ruleCompare = (a: ErrorPassthroughRule, b: ErrorPassthroughRule) =>
  (a.priority ?? 0) - (b.priority ?? 0) || a.name.localeCompare(b.name);

export default function AdminErrorPassthroughPage() {
  return (
    <AdminShell>
      <ErrorPassthroughContent />
    </AdminShell>
  );
}

function ErrorPassthroughContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const all = useErrorPassthroughRules();
  const { query, total } = useClientPagedList(all, list, { match: ruleMatch, compare: ruleCompare });

  const createMut = useCreateErrorPassthroughRule();
  const updateMut = useUpdateErrorPassthroughRule();
  const deleteMut = useDeleteErrorPassthroughRule();

  const [formTarget, setFormTarget] = useState<ErrorPassthroughRule | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ErrorPassthroughRule | null>(null);
  const isNew = formTarget === "new";
  const isFiltered = Boolean(list.search || list.filters.action);

  const actionLabel = (action: ErrorPassthroughRule["action"]) =>
    t(action === "expose" ? "adminErrorPassthrough.actionExpose" : "adminErrorPassthrough.actionMask");

  const fields: FieldConfig<ErrorPassthroughFormState>[] = [
    { name: "name", label: t("adminErrorPassthrough.name") },
    {
      name: "action",
      label: t("adminErrorPassthrough.action"),
      type: "select",
      options: ERROR_PASSTHROUGH_ACTIONS.map((value) => ({ value, label: actionLabel(value) })),
    },
    { name: "priority", label: t("adminErrorPassthrough.priority"), type: "number" },
    { name: "enabled", label: t("adminErrorPassthrough.enabled"), type: "switch" },
    {
      name: "statusCodes",
      label: t("adminErrorPassthrough.statusCodes"),
      type: "tags",
      placeholder: "429",
      hint: t("adminErrorPassthrough.statusCodesHint"),
    },
    { name: "classes", label: t("adminErrorPassthrough.classes"), type: "tags" },
    {
      name: "keywords",
      label: t("adminErrorPassthrough.keywords"),
      type: "tags",
      hint: t("adminErrorPassthrough.keywordsHint"),
    },
  ];

  const columns: Column<ErrorPassthroughRule>[] = [
    {
      key: "name",
      header: t("adminErrorPassthrough.name"),
      render: (r) => <span className="text-srapi-text-primary">{r.name}</span>,
    },
    {
      key: "action",
      header: t("adminErrorPassthrough.action"),
      render: (r) => <QuietBadge status={ACTION_TONE[r.action]} label={actionLabel(r.action)} />,
    },
    {
      key: "priority",
      header: t("adminErrorPassthrough.priority"),
      align: "right",
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">{r.priority}</span>
      ),
    },
    {
      key: "match",
      header: t("adminErrorPassthrough.match"),
      hideOnMobile: true,
      render: (r) => <MatchChips rule={r} />,
    },
    {
      key: "enabled",
      header: t("adminErrorPassthrough.enabled"),
      render: (r) => (
        <QuietBadge
          status={r.enabled ? "active" : "disabled"}
          label={r.enabled ? t("common.active") : t("common.disabled")}
        />
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminErrorPassthrough.title")}
        description={t("adminErrorPassthrough.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminErrorPassthrough.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        getRowId={(r) => String(r.id)}
        emptyIcon={ShieldAlert}
        emptyTitle={t("adminErrorPassthrough.emptyTitle")}
        emptyBody={t("adminErrorPassthrough.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminErrorPassthrough.create")}
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
              placeholder={t("adminErrorPassthrough.searchPlaceholder")}
            />
            <FilterSelect
              value={list.filters.action}
              onChange={(v) => list.setFilter("action", v)}
              options={ERROR_PASSTHROUGH_ACTIONS.map((value) => ({ value, label: actionLabel(value) }))}
              allLabel={t("adminErrorPassthrough.allActions")}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(r) => (
          <RowActionsMenu
            actions={[
              { label: t("common.edit"), onSelect: () => setFormTarget(r) },
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(r) },
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
          title={isNew ? t("adminErrorPassthrough.create") : t("adminErrorPassthrough.edit")}
          fields={fields}
          initial={isNew ? emptyErrorPassthroughForm() : errorPassthroughFormFromRule(formTarget)}
          buildBody={buildErrorPassthroughBody}
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

function MatchChips({ rule }: { rule: ErrorPassthroughRule }) {
  const items = [
    ...(rule.status_codes ?? []).map(String),
    ...(rule.classes ?? []),
    ...(rule.keywords ?? []),
  ];
  if (items.length === 0) {
    return <span className="text-srapi-text-tertiary">—</span>;
  }
  const shown = items.slice(0, 6);
  const extra = items.length - shown.length;
  return (
    <div className="flex flex-wrap gap-1">
      {shown.map((item, i) => (
        <span
          key={`${item}-${i}`}
          className="rounded border border-srapi-border px-1.5 py-0.5 font-mono text-2xs text-srapi-text-secondary"
        >
          {item}
        </span>
      ))}
      {extra > 0 ? (
        <span className="px-1 py-0.5 font-mono text-2xs text-srapi-text-tertiary">+{extra}</span>
      ) : null}
    </div>
  );
}
