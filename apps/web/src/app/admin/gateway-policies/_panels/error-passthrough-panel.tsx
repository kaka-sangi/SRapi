"use client";

import { useState } from "react";
import { ShieldAlert } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { DataPill } from "@/components/ui/data-pill";
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
  useErrorPassthroughRules,
  useCreateErrorPassthroughRule,
  useUpdateErrorPassthroughRule,
  useDeleteErrorPassthroughRule,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
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

export function ErrorPassthroughPanel() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-error-passthrough", []);
  const all = useErrorPassthroughRules();
  const { query, total } = useClientPagedList(all, list, { match: ruleMatch, compare: ruleCompare });

  const createMut = useCreateErrorPassthroughRule();
  const updateMut = useUpdateErrorPassthroughRule();
  const deleteMut = useDeleteErrorPassthroughRule();
  const [togglingId, setTogglingId] = useState<number | null>(null);

  async function toggleEnabled(rule: ErrorPassthroughRule) {
    if (togglingId === rule.id) return;
    setTogglingId(rule.id);
    try {
      await updateMut.mutateAsync({
        id: String(rule.id),
        body: { enabled: !rule.enabled },
      });
      toast({
        title: rule.enabled
          ? t("adminErrorPassthrough.toggleDisabled")
          : t("adminErrorPassthrough.toggleEnabled"),
        tone: "success",
      });
    } catch (err) {
      toast({ title: adminErrorMessage(err), tone: "error" });
    } finally {
      setTogglingId(null);
    }
  }

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
      pinned: true,
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
        <DataTooltip
          title={t("adminErrorPassthrough.priority")}
          primary={String(r.priority)}
          rows={[
            { label: t("adminErrorPassthrough.action"), value: actionLabel(r.action) },
            { label: t("adminErrorPassthrough.statusCodes"), value: String((r.status_codes ?? []).length), tone: "muted" },
            { label: t("adminErrorPassthrough.classes"), value: String((r.classes ?? []).length), tone: "muted" },
            { label: t("adminErrorPassthrough.keywords"), value: String((r.keywords ?? []).length), tone: "muted" },
          ]}
        >
          <span className="text-xs text-srapi-text-tertiary tabular">{r.priority}</span>
        </DataTooltip>
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
        <button
          type="button"
          onClick={() => void toggleEnabled(r)}
          disabled={togglingId === r.id}
          className="cursor-pointer disabled:cursor-wait disabled:opacity-60"
          title={
            r.enabled
              ? t("adminErrorPassthrough.clickToDisable")
              : t("adminErrorPassthrough.clickToEnable")
          }
        >
          <QuietBadge
            status={r.enabled ? "active" : "disabled"}
            label={r.enabled ? t("common.active") : t("common.disabled")}
          />
        </button>
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
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminErrorPassthrough.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        columnVisibility={colVis}
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
        enableKeyboardNav
        rowSeverity={(r) => {
          // Mask actions hide error details from clients — surface them as a
          // warning stripe so operators can immediately spot which errors
          // are being suppressed. Disabled rules stay muted via info.
          if (!r.enabled) return "info";
          if (r.action === "mask") return "warning";
          return undefined;
        }}
        expandRow={(r) => (
          <InlineDetailGrid
            sections={[
              {
                title: t("adminErrorPassthrough.statusCodes"),
                rows: (r.status_codes ?? []).length === 0
                  ? [{ label: t("adminErrorPassthrough.statusCodes"), value: "—", tone: "muted" }]
                  : (r.status_codes ?? []).slice(0, 6).map((s) => ({ label: String(s), value: "match", mono: true })),
              },
              {
                title: t("adminErrorPassthrough.classes") + " / " + t("adminErrorPassthrough.keywords"),
                rows: [
                  { label: t("adminErrorPassthrough.classes"), value: (r.classes ?? []).slice(0, 4).join(", ") || "—", mono: true, tone: (r.classes ?? []).length > 0 ? "default" : "muted" },
                  { label: t("adminErrorPassthrough.keywords"), value: (r.keywords ?? []).slice(0, 4).join(", ") || "—", mono: true, tone: (r.keywords ?? []).length > 0 ? "default" : "muted" },
                ],
              },
              {
                title: t("common.updated"),
                rows: [
                  { label: t("common.created"), value: r.created_at ? formatDateTime(r.created_at) : "—", tone: "muted" },
                  { label: t("common.updated"), value: r.updated_at ? formatDateTime(r.updated_at) : "—", tone: "muted" },
                  { label: t("adminErrorPassthrough.priority"), value: String(r.priority) },
                ],
              },
            ]}
          />
        )}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminErrorPassthrough.searchPlaceholder")}
            />
            <SegmentedControl<string>
              value={list.filters.action || "__all__"}
              onChange={(v) => list.setFilter("action", v === "__all__" ? undefined : v)}
              ariaLabel={t("adminErrorPassthrough.action")}
              size="sm"
              options={[
                { value: "__all__", label: t("adminErrorPassthrough.allActions") },
                ...ERROR_PASSTHROUGH_ACTIONS.map((value) => ({ value, label: actionLabel(value) })),
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
        <DataPill key={`${item}-${i}`} tone="neutral" size="sm">
          {item}
        </DataPill>
      ))}
      {extra > 0 ? (
        <span className="px-1 py-0.5 text-xs text-srapi-text-tertiary tabular">+{extra}</span>
      ) : null}
    </div>
  );
}
