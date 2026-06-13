"use client";

import { useState } from "react";
import { Tag } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { RowActionsMenu, type RowAction } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  useAdminPricingRules,
  useAdminModels,
  useAdminProviders,
  useCreatePricingRule,
  useUpdatePricingRule,
  useBulkImportPricingRules,
  useDeletePricingRule,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { adminErrorMessage } from "@/lib/admin-api";
import { formatMoney } from "@/lib/admin-format";
import {
  emptyPricingRuleForm,
  pricingRuleFormFromRule,
  buildCreatePricingRuleBody,
  buildUpdatePricingRuleBody,
  type PricingRuleFormState,
} from "@/lib/admin-subscription-form";
import type { PricingRule } from "@/lib/sdk-types";

export default function ChannelPricingPage() {
  return (
    <AdminShell>
      <PricingContent />
    </AdminShell>
  );
}

function PricingContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-channel-pricing", []);
  const rules = useAdminPricingRules({ page: list.page, page_size: list.pageSize });
  const models = useAdminModels();
  const providers = useAdminProviders();
  const createMut = useCreatePricingRule();
  const updateMut = useUpdatePricingRule();
  const bulkImportMut = useBulkImportPricingRules();
  const deleteMut = useDeletePricingRule();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<PricingRule | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PricingRule | null>(null);
  const [importing, setImporting] = useState(false);
  const [importText, setImportText] = useState("");
  const [importError, setImportError] = useState<string | null>(null);

  function openImport(open: boolean) {
    setImporting(open);
    if (!open) {
      setImportText("");
      setImportError(null);
    }
  }

  async function submitImport() {
    setImportError(null);
    let parsed: unknown;
    try {
      parsed = JSON.parse(importText);
    } catch {
      setImportError(t("adminPricing.bulkImportInvalidJson"));
      return;
    }
    if (!Array.isArray(parsed)) {
      setImportError(t("adminPricing.bulkImportInvalidJson"));
      return;
    }
    try {
      const result = await bulkImportMut.mutateAsync({ items: parsed });
      toast({
        title: t("adminPricing.importResult", { count: result.created }),
        tone: "success",
      });
      openImport(false);
    } catch (err) {
      setImportError(adminErrorMessage(err));
    }
  }

  const modelList = models.data?.data ?? [];
  const providerList = providers.data?.data ?? [];
  const modelMap = new Map(modelList.map((m) => [String(m.id), m.canonical_name ?? m.id]));
  const providerMap = new Map(providerList.map((p) => [String(p.id), p.display_name ?? p.id]));
  const modelOptions = modelList.map((m) => ({
    value: m.id,
    label: m.canonical_name ?? m.id,
  }));
  const providerOptions = providerList.map((p) => ({
    value: p.id,
    label: p.display_name ?? p.id,
  }));
  const billingModeOptions = [
    { value: "token", label: t("adminPricing.billingModeToken") },
    { value: "per_request", label: t("adminPricing.billingModePerRequest") },
    { value: "image", label: t("adminPricing.billingModeImage") },
  ];

  const priceFields: FieldConfig<PricingRuleFormState>[] = [
    { name: "billingMode", label: t("adminPricing.billingMode"), type: "select", options: billingModeOptions },
    { name: "inputPricePerMillionTokens", label: t("adminPricing.inputPrice"), help: t("adminPricing.inputPriceHelp") },
    { name: "outputPricePerMillionTokens", label: t("adminPricing.outputPrice"), help: t("adminPricing.outputPriceHelp") },
    { name: "cacheReadPricePerMillionTokens", label: t("adminPricing.cacheReadPrice"), help: t("adminPricing.cacheReadPriceHelp") },
    { name: "cacheWritePricePerMillionTokens", label: t("adminPricing.cacheWritePrice"), help: t("adminPricing.cacheWritePriceHelp") },
    { name: "perRequestPrice", label: t("adminPricing.perRequestPrice"), help: t("adminPricing.perRequestPriceHelp") },
    { name: "intervalsJson", label: t("adminPricing.intervals"), type: "textarea", help: t("adminPricing.intervalsHelp") },
    { name: "currency", label: t("adminCommon.currency") },
    { name: "effectiveFromLocal", label: t("adminPricing.effectiveFrom"), help: t("adminPricing.effectiveFromHelp"), type: "datetime" },
    { name: "effectiveToLocal", label: t("adminPricing.effectiveTo"), help: t("adminPricing.effectiveToHelp"), type: "datetime" },
  ];

  const fields: FieldConfig<PricingRuleFormState>[] = [
    { name: "modelId", label: t("adminPricing.model"), type: "select", options: modelOptions },
    { name: "providerId", label: t("adminPricing.provider"), type: "select", options: providerOptions },
    ...priceFields,
  ];

  const editFields: FieldConfig<PricingRuleFormState>[] = priceFields;

  const columns: Column<PricingRule>[] = [
    {
      key: "model",
      header: t("adminPricing.model"),
      pinned: true,
      sortValue: (r) => modelMap.get(String(r.model_id)) ?? String(r.model_id),
      render: (r) => (
        <span className="text-2xs text-srapi-text-primary">
          {modelMap.get(String(r.model_id)) ?? r.model_id}
        </span>
      ),
    },
    {
      key: "provider",
      header: t("adminPricing.provider"),
      hideOnMobile: true,
      sortValue: (r) => providerMap.get(String(r.provider_id)) ?? String(r.provider_id),
      render: (r) => (
        <span className="text-2xs text-srapi-text-secondary">
          {String(r.provider_id) === "0"
            ? t("adminPricing.anyProvider")
            : providerMap.get(String(r.provider_id)) ?? r.provider_id}
        </span>
      ),
    },
    {
      key: "mode",
      header: t("adminPricing.billingMode"),
      hideOnMobile: true,
      sortValue: (r) => r.billing_mode,
      render: (r) => (
        <span className="text-2xs text-srapi-text-secondary">
          {formatBillingMode(r.billing_mode, t)}
        </span>
      ),
    },
    {
      key: "input",
      header: t("adminPricing.inputPrice"),
      align: "right",
      render: (r) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(r.input_price_per_million_tokens, r.currency)}
        </span>
      ),
    },
    {
      key: "output",
      header: t("adminPricing.outputPrice"),
      align: "right",
      hideOnMobile: true,
      render: (r) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(r.output_price_per_million_tokens, r.currency)}
        </span>
      ),
    },
    {
      key: "intervals",
      header: t("adminPricing.intervals"),
      align: "right",
      hideOnMobile: true,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {r.intervals.length}
        </span>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminPricing.title")}
        description={t("adminPricing.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {rules.data ? (
              <ListCount total={rules.data.pagination?.total ?? rules.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="outline" size="sm" onClick={() => openImport(true)}>
              {t("adminPricing.bulkImport")}
            </Button>
            <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
              ＋ {t("adminPricing.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={rules}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(r) => r.id}
        emptyIcon={Tag}
        emptyTitle={t("adminPricing.emptyTitle")}
        emptyBody={t("adminPricing.emptyBody")}
        minWidth={520}
        rowActions={(r) => {
          const actions: RowAction[] = [
            { label: t("common.edit"), onSelect: () => setEditing(r) },
            { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(r) },
          ];
          return <RowActionsMenu actions={actions} />;
        }}
        sort={list.sort}
        onSort={list.toggleSort}
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: rules.data?.pagination?.total ?? rules.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
      />

      {creating ? (
        <ResourceFormDialog
          open
          onOpenChange={setCreating}
          title={t("adminPricing.create")}
          fields={fields}
          initial={emptyPricingRuleForm()}
          buildBody={buildCreatePricingRuleBody}
          submit={(body) => createMut.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createMut.isPending}
        />
      ) : null}

      {editing ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => { if (!open) setEditing(null); }}
          title={t("adminPricing.edit")}
          fields={editFields}
          initial={pricingRuleFormFromRule(editing)}
          buildBody={buildUpdatePricingRuleBody}
          submit={(body) => updateMut.mutateAsync({ id: editing.id, body })}
          successMessage={t("feedback.updated")}
          isPending={updateMut.isPending}
        />
      ) : null}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("adminPricing.deleteTitle")}
        body={t("adminPricing.deleteBody")}
        confirmLabel={t("common.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteMut.isPending}
        onConfirm={async () => {
          if (deleteTarget) await deleteMut.mutateAsync(deleteTarget.id);
        }}
      />

      <Dialog open={importing} onOpenChange={openImport}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("adminPricing.bulkImport")}</DialogTitle>
            <DialogDescription>{t("adminPricing.bulkImportHint")}</DialogDescription>
          </DialogHeader>
          <Textarea
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            placeholder={`[\n  { "model_id": "...", "provider_id": "..." }\n]`}
            className="min-h-48 font-mono text-2xs"
            spellCheck={false}
          />
          {importError ? (
            <p role="alert" className="text-sm text-srapi-error">
              {importError}
            </p>
          ) : null}
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => openImport(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              loading={bulkImportMut.isPending}
              onClick={submitImport}
            >
              {t("adminPricing.bulkImport")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function formatBillingMode(mode: PricingRule["billing_mode"], t: (key: string) => string): string {
  if (mode === "per_request") return t("adminPricing.billingModePerRequest");
  if (mode === "image") return t("adminPricing.billingModeImage");
  return t("adminPricing.billingModeToken");
}
