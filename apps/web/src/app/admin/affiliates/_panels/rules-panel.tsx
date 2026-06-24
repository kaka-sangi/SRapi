"use client";

import { useState } from "react";
import { Percent } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { useAffiliateRules, useCreateAffiliateRule, useUpdateAffiliateRule } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { Button } from "@/components/ui/button";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatDateTime } from "@/lib/admin-format";
import {
  AFFILIATE_RULE_STATUSES,
  AFFILIATE_RULE_TRIGGER_TYPES,
  affiliateRuleFormFromRule,
  buildAffiliateRuleBody,
  emptyAffiliateRuleForm,
  type AffiliateRuleFormState,
} from "@/lib/admin-affiliate-rule-form";
import type { AffiliateRule } from "@/lib/sdk-types";

export function RulesPanel() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-affiliate-rules", []);
  const rules = useAffiliateRules({ page: list.page, page_size: list.pageSize });
  const createMut = useCreateAffiliateRule();
  const updateMut = useUpdateAffiliateRule();
  const [formTarget, setFormTarget] = useState<AffiliateRule | "new" | null>(null);
  const [togglingId, setTogglingId] = useState<string | null>(null);
  const isNew = formTarget === "new";

  async function toggleStatus(rule: AffiliateRule) {
    if (togglingId === rule.id) return;
    const next: AffiliateRule["status"] = rule.status === "active" ? "disabled" : "active";
    setTogglingId(rule.id);
    try {
      await updateMut.mutateAsync({ id: rule.id, body: { status: next } });
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: adminErrorMessage(err), tone: "error" });
    } finally {
      setTogglingId(null);
    }
  }

  const fields: FieldConfig<AffiliateRuleFormState>[] = [
    { name: "name", label: t("adminAffiliates.ruleName"), required: true },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(AFFILIATE_RULE_STATUSES, t),
    },
    {
      name: "triggerType",
      label: t("adminAffiliates.triggerType"),
      type: "select",
      options: enumOptions(AFFILIATE_RULE_TRIGGER_TYPES, t),
    },
    { name: "rate", label: t("adminAffiliates.rate"), required: true },
    { name: "fixedAmount", label: t("adminAffiliates.fixedAmount"), required: true },
    { name: "currency", label: t("adminCommon.currency"), required: true },
    { name: "maxRebateAmount", label: t("adminAffiliates.maxRebate"), required: true },
    { name: "validFromLocal", label: t("adminCommon.startsAt"), type: "datetime" },
    { name: "validToLocal", label: t("adminCommon.expiresAt"), type: "datetime" },
  ];

  const columns: Column<AffiliateRule>[] = [
    {
      key: "name",
      header: t("adminAffiliates.ruleName"),
      pinned: true,
      sortValue: (rule) => rule.name,
      render: (rule) => (
        <span className="font-medium text-srapi-text-primary">{rule.name}</span>
      ),
    },
    {
      key: "rate",
      header: t("adminAffiliates.rate"),
      render: (rule) => {
        const ratePct = Number(rule.rate) * 100;
        const fixed = Number(rule.fixed_amount);
        const maxRebate = Number(rule.max_rebate_amount);
        return (
          <DataTooltip
            title={t("adminAffiliates.rate")}
            primary={
              <span>
                {rule.rate}
                {rule.fixed_amount !== "0.00000000" ? ` + ${rule.fixed_amount}` : ""}
              </span>
            }
            rows={[
              {
                label: "Rate",
                value: Number.isFinite(ratePct) ? `${ratePct.toFixed(3)}%` : rule.rate,
              },
              ...(fixed > 0
                ? [{ label: t("adminAffiliate.fixedBonus"), value: `${rule.fixed_amount} ${rule.currency}` }]
                : []),
              { label: t("adminCommon.currency"), value: rule.currency.toUpperCase() },
              ...(maxRebate > 0
                ? [
                    {
                      label: t("adminAffiliate.maxRebate"),
                      value: `${rule.max_rebate_amount} ${rule.currency}`,
                      tone: "muted" as const,
                    },
                  ]
                : [{ label: t("adminAffiliate.maxRebate"), value: t("adminAffiliate.uncapped"), tone: "muted" as const }]),
            ]}
          >
            <span className="text-sm font-medium tabular text-srapi-text-primary">
              {rule.rate}
              {rule.fixed_amount !== "0.00000000" ? ` + ${rule.fixed_amount}` : ""}
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "currency",
      header: t("adminCommon.currency"),
      hideOnMobile: true,
      render: (rule) => (
        <span className="text-xs tabular text-srapi-text-tertiary">
          {rule.currency}
          {rule.max_rebate_amount !== "0.00000000" ? ` · ${rule.max_rebate_amount}` : ""}
        </span>
      ),
    },
    {
      key: "validity",
      header: t("adminAffiliates.validity"),
      hideOnMobile: true,
      render: (rule) => (
        <span className="text-[12px] tabular text-srapi-text-tertiary">
          {rule.valid_from ? formatDateTime(rule.valid_from) : "—"}
          {" / "}
          {rule.valid_to ? formatDateTime(rule.valid_to) : "—"}
        </span>
      ),
    },
    {
      key: "status",
      header: t("adminCommon.status"),
      render: (rule) => {
        const canToggle = rule.status === "active" || rule.status === "disabled";
        const badge = (
          <QuietBadge status={quietStatusFor(rule.status)} label={statusLabel(t, rule.status)} />
        );
        if (!canToggle) return badge;
        return (
          <button
            type="button"
            onClick={() => void toggleStatus(rule)}
            disabled={togglingId === rule.id}
            className="cursor-pointer disabled:cursor-wait disabled:opacity-60"
            title={rule.status === "active" ? t("common.disable") : t("common.enable")}
          >
            {badge}
          </button>
        );
      },
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminAffiliates.rulesTitle")}
        description={t("adminAffiliates.rulesSubtitle")}
        actions={
          <div className="flex items-center gap-3">
            {rules.data ? (
              <ListCount total={rules.data.pagination?.total ?? rules.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminAffiliates.createRule")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={rules}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(rule) => rule.id}
        emptyIcon={Percent}
        emptyTitle={t("adminAffiliates.emptyRules")}
        emptyBody={t("adminAffiliates.emptyRulesBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminAffiliates.createRule")}
          </Button>
        }
        minWidth={680}
        sort={list.sort}
        onSort={list.toggleSort}
        rowSeverity={(rule) => {
          // Approval-state stripe: active = success, disabled = info,
          // archived = warning so deprecated rules are visually demoted.
          switch (rule.status) {
            case "active":
              return "success";
            case "disabled":
              return "info";
            case "archived":
              return "warning";
            default:
              return undefined;
          }
        }}
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: rules.data?.pagination?.total ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(rule) => (
          <RowActionsMenu
            actions={[{ label: t("common.edit"), onSelect: () => setFormTarget(rule) }]}
          />
        )}
      />

      {formTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={isNew ? t("adminAffiliates.createRule") : t("adminAffiliates.editRule")}
          fields={fields}
          initial={isNew ? emptyAffiliateRuleForm() : affiliateRuleFormFromRule(formTarget)}
          buildBody={buildAffiliateRuleBody}
          submit={
            isNew
              ? (body) => createMut.mutateAsync(body)
              : (body) => updateMut.mutateAsync({ id: formTarget.id, body })
          }
          successMessage={isNew ? t("feedback.created") : t("feedback.updated")}
          isPending={createMut.isPending || updateMut.isPending}
        />
      ) : null}
    </>
  );
}
