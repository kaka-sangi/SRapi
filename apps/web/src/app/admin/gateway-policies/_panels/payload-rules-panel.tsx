"use client";

import { useState } from "react";
import { SlidersHorizontal } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useClientPagedList } from "@/hooks/use-client-list";
import {
  usePayloadRules,
  useCreatePayloadRule,
  useUpdatePayloadRule,
  useDeletePayloadRule,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  PAYLOAD_RULE_ACTIONS,
  emptyPayloadRuleForm,
  payloadRuleFormFromRule,
  buildPayloadRuleBody,
  type PayloadRuleFormState,
} from "@/lib/admin-payload-rule-form";
import type { PayloadRule } from "@/lib/sdk-types";

const ACTION_TONE: Record<PayloadRule["action"], QuietStatus> = {
  override: "active",
  default: "limited",
  filter: "disabled",
};

function ruleMatch(rule: PayloadRule, term: string, filters: Record<string, string>): boolean {
  if (filters.action && rule.action !== filters.action) return false;
  if (!term) return true;
  return [rule.name, rule.action, rule.match_model, rule.match_protocol, ...Object.keys(rule.params ?? {})]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

// Lowest priority number wins first, mirroring gateway evaluation order.
const ruleCompare = (a: PayloadRule, b: PayloadRule) =>
  (a.priority ?? 0) - (b.priority ?? 0) || a.name.localeCompare(b.name);

export function PayloadRulesPanel() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-payload-rules", []);
  const all = usePayloadRules();
  const { query, total } = useClientPagedList(all, list, { match: ruleMatch, compare: ruleCompare });

  const createMut = useCreatePayloadRule();
  const updateMut = useUpdatePayloadRule();
  const deleteMut = useDeletePayloadRule();

  const [formTarget, setFormTarget] = useState<PayloadRule | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PayloadRule | null>(null);
  const [togglingId, setTogglingId] = useState<number | null>(null);

  async function toggleEnabled(rule: PayloadRule) {
    if (togglingId === rule.id) return;
    setTogglingId(rule.id);
    try {
      await updateMut.mutateAsync({
        id: String(rule.id),
        body: { enabled: !rule.enabled },
      });
      toast({
        title: rule.enabled
          ? t("adminPayloadRules.toggleDisabled")
          : t("adminPayloadRules.toggleEnabled"),
        tone: "success",
      });
    } catch (err) {
      toast({ title: adminErrorMessage(err), tone: "error" });
    } finally {
      setTogglingId(null);
    }
  }
  const isNew = formTarget === "new";
  const isFiltered = Boolean(list.search || list.filters.action);

  const actionLabel = (action: PayloadRule["action"]) => t(`adminPayloadRules.action_${action}`);

  const fields: FieldConfig<PayloadRuleFormState>[] = [
    { name: "name", label: t("adminPayloadRules.name"), required: true },
    {
      name: "action",
      label: t("adminPayloadRules.action"),
      type: "select",
      options: PAYLOAD_RULE_ACTIONS.map((value) => ({ value, label: actionLabel(value) })),
      hint: t("adminPayloadRules.actionHint"),
    },
    {
      name: "matchModel",
      label: t("adminPayloadRules.matchModel"),
      placeholder: "gpt-*",
      hint: t("adminPayloadRules.matchModelHint"),
    },
    {
      name: "matchProtocol",
      label: t("adminPayloadRules.matchProtocol"),
      placeholder: "openai-compatible",
      hint: t("adminPayloadRules.matchProtocolHint"),
    },
    { name: "priority", label: t("adminPayloadRules.priority"), help: t("adminPayloadRules.priorityHelp"), type: "number" },
    { name: "enabled", label: t("adminPayloadRules.enabled"), type: "switch" },
    {
      name: "params",
      label: t("adminPayloadRules.params"),
      type: "keyvalue",
      hint: t("adminPayloadRules.paramsHint"),
      required: true,
    },
  ];

  const columns: Column<PayloadRule>[] = [
    {
      key: "name",
      header: t("adminPayloadRules.name"),
      pinned: true,
      render: (r) => <span className="text-srapi-text-primary">{r.name}</span>,
    },
    {
      key: "action",
      header: t("adminPayloadRules.action"),
      render: (r) => <QuietBadge status={ACTION_TONE[r.action]} label={actionLabel(r.action)} />,
    },
    {
      key: "match",
      header: t("adminPayloadRules.match"),
      hideOnMobile: true,
      render: (r) => (
        <span className="text-xs text-srapi-text-tertiary tabular">
          {r.match_model || "*"}
          {" · "}
          {r.match_protocol || t("adminPayloadRules.anyProtocol")}
        </span>
      ),
    },
    {
      key: "paths",
      header: t("adminPayloadRules.params"),
      hideOnMobile: true,
      render: (r) => <PathChips params={r.params} />,
    },
    {
      key: "priority",
      header: t("adminPayloadRules.priority"),
      align: "right",
      render: (r) => (
        <span className="text-xs text-srapi-text-tertiary tabular">{r.priority}</span>
      ),
    },
    {
      key: "enabled",
      header: t("adminPayloadRules.enabled"),
      render: (r) => (
        <button
          type="button"
          onClick={() => void toggleEnabled(r)}
          disabled={togglingId === r.id}
          className="cursor-pointer disabled:cursor-wait disabled:opacity-60"
          title={
            r.enabled
              ? t("adminPayloadRules.clickToDisable")
              : t("adminPayloadRules.clickToEnable")
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
        title={t("adminPayloadRules.title")}
        description={t("adminPayloadRules.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminPayloadRules.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(r) => String(r.id)}
        emptyIcon={SlidersHorizontal}
        emptyTitle={t("adminPayloadRules.emptyTitle")}
        emptyBody={t("adminPayloadRules.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminPayloadRules.create")}
          </Button>
        }
        minWidth={760}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminPayloadRules.searchPlaceholder")}
            />
            <FilterSelect
              value={list.filters.action}
              onChange={(v) => list.setFilter("action", v)}
              options={PAYLOAD_RULE_ACTIONS.map((value) => ({ value, label: actionLabel(value) }))}
              allLabel={t("adminPayloadRules.allActions")}
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
          title={isNew ? t("adminPayloadRules.create") : t("adminPayloadRules.edit")}
          fields={fields}
          initial={isNew ? emptyPayloadRuleForm() : payloadRuleFormFromRule(formTarget)}
          buildBody={buildPayloadRuleBody}
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

function PathChips({ params }: { params: PayloadRule["params"] }) {
  const paths = Object.keys(params ?? {});
  if (paths.length === 0) {
    return <span className="text-srapi-text-tertiary">—</span>;
  }
  const shown = paths.slice(0, 4);
  const extra = paths.length - shown.length;
  return (
    <div className="flex flex-wrap gap-1">
      {shown.map((path) => (
        <code
          key={path}
          className="rounded-full bg-srapi-card-muted px-2.5 py-1 font-mono text-[11px] font-medium text-srapi-text-secondary"
        >
          {path}
        </code>
      ))}
      {extra > 0 ? (
        <span className="px-1 py-0.5 text-xs text-srapi-text-tertiary tabular">+{extra}</span>
      ) : null}
    </div>
  );
}
