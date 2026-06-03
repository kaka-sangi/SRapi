"use client";

import { useState } from "react";
import { Tag } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import {
  useAdminPricingRules,
  useAdminModels,
  useAdminProviders,
  useCreatePricingRule,
  useBulkImportPricingRules,
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
  buildCreatePricingRuleBody,
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
  const rules = useAdminPricingRules({ page: list.page, page_size: list.pageSize });
  const models = useAdminModels();
  const providers = useAdminProviders();
  const createMut = useCreatePricingRule();
  const bulkImportMut = useBulkImportPricingRules();
  const [creating, setCreating] = useState(false);
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
      setImportError("Invalid JSON");
      return;
    }
    if (!Array.isArray(parsed)) {
      setImportError("Invalid JSON");
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

  const modelOptions = (models.data?.data ?? []).map((m) => ({
    value: m.id,
    label: m.canonical_name ?? m.id,
  }));
  const providerOptions = (providers.data?.data ?? []).map((p) => ({
    value: p.id,
    label: p.display_name ?? p.id,
  }));

  const fields: FieldConfig<PricingRuleFormState>[] = [
    { name: "modelId", label: t("adminPricing.model"), type: "select", options: modelOptions },
    { name: "providerId", label: t("adminPricing.provider"), type: "select", options: providerOptions },
    { name: "inputPricePerMillionTokens", label: t("adminPricing.inputPrice") },
    { name: "outputPricePerMillionTokens", label: t("adminPricing.outputPrice") },
    { name: "cacheReadPricePerMillionTokens", label: t("adminPricing.cacheReadPrice") },
    { name: "cacheWritePricePerMillionTokens", label: t("adminPricing.cacheWritePrice") },
    { name: "currency", label: t("adminCommon.currency") },
    { name: "effectiveFromLocal", label: t("adminPricing.effectiveFrom"), type: "datetime" },
    { name: "effectiveToLocal", label: t("adminPricing.effectiveTo"), type: "datetime" },
  ];

  const columns: Column<PricingRule>[] = [
    {
      key: "model",
      header: t("adminPricing.model"),
      sortValue: (r) => r.model_id,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-primary">{r.model_id}</span>
      ),
    },
    {
      key: "provider",
      header: t("adminPricing.provider"),
      hideOnMobile: true,
      sortValue: (r) => r.provider_id,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-secondary">{r.provider_id}</span>
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
        getRowId={(r) => r.id}
        emptyIcon={Tag}
        emptyTitle={t("adminPricing.emptyTitle")}
        emptyBody={t("adminPricing.emptyBody")}
        minWidth={520}
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
