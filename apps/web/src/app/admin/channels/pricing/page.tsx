"use client";

import { useState } from "react";
import { Tag } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { RowActionsMenu, type RowAction } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { Card } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import {
  useAdminPricingRules,
  useAdminPricingRulePresets,
  useAdminModels,
  useAdminProviders,
  useCreatePricingRule,
  useUpdatePricingRule,
  useBulkImportPricingRules,
  useInstallPricingRulePresets,
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
import { formatInteger, formatMoney } from "@/lib/admin-format";
import {
  emptyPricingRuleForm,
  pricingRuleFormFromRule,
  buildCreatePricingRuleBody,
  buildUpdatePricingRuleBody,
  type PricingRuleFormState,
} from "@/lib/admin-subscription-form";
import type { PricingRule, PricingRulePreset } from "@/lib/sdk-types";

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
  const modelFilter = list.filters.modelId || undefined;
  const providerFilter = list.filters.providerId || undefined;
  const searchQuery = list.search || undefined;
  const rules = useAdminPricingRules({
    page: list.page,
    page_size: list.pageSize,
    q: searchQuery,
    model_id: modelFilter,
    provider_id: providerFilter,
  });
  const allRules = useAdminPricingRules();
  const presets = useAdminPricingRulePresets();
  const models = useAdminModels();
  const providers = useAdminProviders();
  const createMut = useCreatePricingRule();
  const updateMut = useUpdatePricingRule();
  const bulkImportMut = useBulkImportPricingRules();
  const installPresetsMut = useInstallPricingRulePresets();
  const deleteMut = useDeletePricingRule();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<PricingRule | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PricingRule | null>(null);
  const [importing, setImporting] = useState(false);
  const [importText, setImportText] = useState("");
  const [importError, setImportError] = useState<string | null>(null);
  const presetCoverage = buildPresetCoverage(presets.data ?? [], allRules.data?.data ?? []);

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

  async function installBuiltInPresets() {
    try {
      const result = await installPresetsMut.mutateAsync(undefined);
      toast({
        title: t("adminPricing.presetsInstalled", { count: result.created }),
        description: t("adminPricing.presetsInstalledHint", {
          requested: String(result.requested),
          validated: String(result.validated),
        }),
        tone: result.errors.length > 0 ? "warning" : "success",
      });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function installMissingPresets() {
    if (presetCoverage.missing.length === 0) return;
    try {
      const families = presetCoverage.missing.map((preset) => preset.model_family);
      const result = await installPresetsMut.mutateAsync({ families });
      toast({
        title: t("adminPricing.presetsInstalled", { count: result.created }),
        description: t("adminPricing.presetsInstalledHint", {
          requested: String(result.requested),
          validated: String(result.validated),
        }),
        tone: result.errors.length > 0 ? "warning" : "success",
      });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
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
  const isFiltered = Boolean(searchQuery || modelFilter || providerFilter);
  const billingModeOptions = [
    { value: "token", label: t("adminPricing.billingModeToken") },
    { value: "per_request", label: t("adminPricing.billingModePerRequest") },
    { value: "image", label: t("adminPricing.billingModeImage") },
  ];

  const priceFields: FieldConfig<PricingRuleFormState>[] = [
    {
      name: "billingMode",
      label: t("adminPricing.billingMode"),
      type: "select",
      options: billingModeOptions,
    },
    {
      name: "inputPricePerMillionTokens",
      label: t("adminPricing.inputPrice"),
      help: t("adminPricing.inputPriceHelp"),
    },
    {
      name: "outputPricePerMillionTokens",
      label: t("adminPricing.outputPrice"),
      help: t("adminPricing.outputPriceHelp"),
    },
    {
      name: "cacheReadPricePerMillionTokens",
      label: t("adminPricing.cacheReadPrice"),
      help: t("adminPricing.cacheReadPriceHelp"),
    },
    {
      name: "cacheWritePricePerMillionTokens",
      label: t("adminPricing.cacheWritePrice"),
      help: t("adminPricing.cacheWritePriceHelp"),
    },
    {
      name: "perRequestPrice",
      label: t("adminPricing.perRequestPrice"),
      help: t("adminPricing.perRequestPriceHelp"),
    },
    {
      name: "intervalsJson",
      label: t("adminPricing.intervals"),
      type: "textarea",
      help: t("adminPricing.intervalsHelp"),
    },
    { name: "currency", label: t("adminCommon.currency") },
    {
      name: "effectiveFromLocal",
      label: t("adminPricing.effectiveFrom"),
      help: t("adminPricing.effectiveFromHelp"),
      type: "datetime",
    },
    {
      name: "effectiveToLocal",
      label: t("adminPricing.effectiveTo"),
      help: t("adminPricing.effectiveToHelp"),
      type: "datetime",
    },
  ];

  const fields: FieldConfig<PricingRuleFormState>[] = [
    {
      name: "modelId",
      label: t("adminPricing.model"),
      type: "select",
      options: modelOptions,
      required: true,
    },
    {
      name: "providerId",
      label: t("adminPricing.provider"),
      type: "select",
      options: providerOptions,
      required: true,
    },
    ...priceFields,
  ];

  const editFields: FieldConfig<PricingRuleFormState>[] = priceFields;

  const columns: Column<PricingRule>[] = [
    {
      key: "model",
      header: t("adminPricing.model"),
      pinned: true,
      sortValue: (r) => pricingRuleModelLabel(r, modelMap, t),
      render: (r) => (
        <div className="min-w-0">
          <div className="text-2xs text-srapi-text-primary truncate">
            {pricingRuleModelLabel(r, modelMap, t)}
          </div>
          {String(r.model_id) === "0" ? (
            <div className="text-srapi-text-tertiary truncate text-2xs tracking-wide uppercase">
              {t("adminPricing.modelFamily")}
            </div>
          ) : null}
        </div>
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
            : (providerMap.get(String(r.provider_id)) ?? r.provider_id)}
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
        <span className="text-srapi-text-secondary tabular font-mono">
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
        <span className="text-srapi-text-secondary tabular font-mono">
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
        <span className="text-2xs text-srapi-text-tertiary tabular font-mono">
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
              columns={columns
                .filter((c) => !c.pinned)
                .map((c) => ({ key: c.key, label: c.header }))}
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
      <PricingPresetPanel
        presets={presets.data ?? []}
        coverage={presetCoverage}
        loading={presets.isLoading || allRules.isLoading}
        installing={installPresetsMut.isPending}
        totalRules={allRules.data?.pagination?.total ?? allRules.data?.data.length ?? 0}
        onInstallMissing={() => void installMissingPresets()}
        onInstallAll={() => void installBuiltInPresets()}
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
        isFiltered={isFiltered}
        noResultsTitle={t("adminPricing.emptyFilteredTitle")}
        noResultsBody={t("adminPricing.emptyFilteredBody")}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminPricing.searchPlaceholder")}
            />
            <FilterSelect
              value={modelFilter}
              onChange={(value) => list.setFilter("modelId", value)}
              options={[{ value: "0", label: t("adminPricing.anyModelFamily") }, ...modelOptions]}
              allLabel={t("adminPricing.allModels")}
            />
            <FilterSelect
              value={providerFilter}
              onChange={(value) => list.setFilter("providerId", value)}
              options={[{ value: "0", label: t("adminPricing.anyProvider") }, ...providerOptions]}
              allLabel={t("adminPricing.allProviders")}
            />
          </ListToolbar>
        }
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
          onOpenChange={(open) => {
            if (!open) setEditing(null);
          }}
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
            className="text-2xs min-h-48 font-mono"
            spellCheck={false}
          />
          {importError ? (
            <p role="alert" className="text-srapi-error text-sm">
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

interface PricingPresetCoverage {
  installed: PricingRulePreset[];
  missing: PricingRulePreset[];
}

function PricingPresetPanel({
  presets,
  coverage,
  loading,
  installing,
  totalRules,
  onInstallMissing,
  onInstallAll,
}: {
  presets: PricingRulePreset[];
  coverage: PricingPresetCoverage;
  loading: boolean;
  installing: boolean;
  totalRules: number;
  onInstallMissing: () => void;
  onInstallAll: () => void;
}) {
  const { t } = useLanguage();
  const total = presets.length;
  const installed = coverage.installed.length;
  const missing = coverage.missing.length;
  const missingPreview = coverage.missing
    .slice(0, 6)
    .map((preset) => preset.model_family)
    .join(", ");
  const samplePresets = presets.slice(0, 5);

  return (
    <Card className="mb-4 p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Tag className="text-srapi-text-tertiary size-4" />
            <h2 className="text-srapi-text-primary font-medium">
              {t("adminPricing.presetPanelTitle")}
            </h2>
            <QuietBadge
              status={missing === 0 && total > 0 ? "active" : "limited"}
              label={t("adminPricing.presetCoverage", {
                installed: formatInteger(installed),
                total: formatInteger(total),
              })}
            />
          </div>
          <p className="text-2xs text-srapi-text-tertiary mt-1 max-w-3xl">
            {t("adminPricing.presetPanelHint")}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="primary"
            size="sm"
            onClick={onInstallMissing}
            loading={installing}
            disabled={loading || installing || missing === 0}
          >
            {missing === 0
              ? t("adminPricing.presetAllCovered")
              : t("adminPricing.installMissingPresets")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={onInstallAll}
            loading={installing}
            disabled={loading || installing || total === 0}
          >
            {t("adminPricing.installAllPresets")}
          </Button>
        </div>
      </div>

      <div className="mt-3 grid gap-2 sm:grid-cols-3">
        <PresetMetric label={t("adminPricing.presetTotal")} value={formatInteger(total)} />
        <PresetMetric
          label={t("adminPricing.presetMissingFamilies")}
          value={formatInteger(missing)}
        />
        <PresetMetric
          label={t("adminPricing.presetRulesLoaded")}
          value={formatInteger(totalRules)}
        />
      </div>

      {loading ? (
        <p className="text-2xs text-srapi-text-tertiary mt-3">{t("adminPricing.presetLoading")}</p>
      ) : missing > 0 ? (
        <p className="text-2xs text-srapi-text-tertiary mt-3">
          {t("adminPricing.presetMissingPreview", { families: missingPreview })}
        </p>
      ) : (
        <p className="text-2xs text-srapi-success mt-3">{t("adminPricing.presetAllCoveredHint")}</p>
      )}

      {samplePresets.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {samplePresets.map((preset) => (
            <span
              key={preset.model_family}
              className="border-srapi-border bg-srapi-card-muted text-srapi-text-secondary text-2xs inline-flex max-w-full min-w-0 items-center gap-2 rounded-md border px-2.5 py-1 font-mono"
              title={t("adminPricing.presetSource", {
                source: preset.source ?? "built-in",
              })}
            >
              <span className="text-srapi-text-primary truncate">{preset.model_family}</span>
              <span className="text-srapi-text-tertiary">
                {t("adminPricing.presetTokenPrices", {
                  input: formatMoney(preset.input_price_per_million_tokens, preset.currency),
                  output: formatMoney(preset.output_price_per_million_tokens, preset.currency),
                })}
              </span>
            </span>
          ))}
        </div>
      ) : null}
    </Card>
  );
}

function PresetMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="border-srapi-border bg-srapi-card-muted rounded-md border px-3 py-2">
      <div className="text-srapi-text-tertiary text-2xs tracking-wide uppercase">{label}</div>
      <div className="text-srapi-text-primary mt-1 font-mono text-sm">{value}</div>
    </div>
  );
}

function buildPresetCoverage(
  presets: PricingRulePreset[],
  rules: PricingRule[],
): PricingPresetCoverage {
  const installedFamilies = new Set(
    rules
      .filter(
        (rule) =>
          String(rule.model_id) === "0" &&
          String(rule.provider_id) === "0" &&
          Boolean(rule.model_family),
      )
      .map((rule) => normalizeFamily(rule.model_family)),
  );
  const installed: PricingRulePreset[] = [];
  const missing: PricingRulePreset[] = [];
  for (const preset of presets) {
    if (installedFamilies.has(normalizeFamily(preset.model_family))) {
      installed.push(preset);
    } else {
      missing.push(preset);
    }
  }
  return { installed, missing };
}

function normalizeFamily(value: string | null | undefined): string {
  return (value ?? "").trim().toLowerCase();
}

function formatBillingMode(mode: PricingRule["billing_mode"], t: (key: string) => string): string {
  if (mode === "per_request") return t("adminPricing.billingModePerRequest");
  if (mode === "image") return t("adminPricing.billingModeImage");
  return t("adminPricing.billingModeToken");
}

function pricingRuleModelLabel(
  rule: PricingRule,
  modelMap: Map<string, string>,
  t: (key: string) => string,
): string {
  if (String(rule.model_id) === "0") {
    return rule.model_family || t("adminPricing.anyModelFamily");
  }
  return modelMap.get(String(rule.model_id)) ?? String(rule.model_id);
}
