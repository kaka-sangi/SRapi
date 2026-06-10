"use client";

import { useState } from "react";
import { ShieldAlert } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import {
  useContentSafetyConfig,
  useRiskStatus,
  useRiskLogs,
  useRiskConfig,
  useUpdateContentSafetyConfig,
  useUpdateRiskConfig,
} from "@/hooks/admin-queries";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatDateTime, formatInteger } from "@/lib/admin-format";
import {
  createRiskControlForm,
  buildRiskControlConfig,
  type RiskControlFormState,
} from "@/lib/admin-risk-control-form";
import {
  buildContentSafetyConfig,
  createContentSafetyForm,
  type ContentSafetyFormState,
} from "@/lib/admin-content-safety-form";
import { countryOptions } from "@/lib/countries";
import type { RiskControlLog } from "@/lib/sdk-types";

export default function AdminRiskControlPage() {
  return (
    <AdminShell>
      <RiskContent />
    </AdminShell>
  );
}

function RiskContent() {
  const { t, language } = useLanguage();
  const colVis = useColumnVisibility("admin-risk-control", []);
  const status = useRiskStatus();
  const logs = useRiskLogs();
  const config = useRiskConfig();
  const contentSafety = useContentSafetyConfig();
  const updateConfig = useUpdateRiskConfig();
  const updateContentSafety = useUpdateContentSafetyConfig();
  const [editing, setEditing] = useState(false);
  const [editingContentSafety, setEditingContentSafety] = useState(false);

  const configFields: FieldConfig<RiskControlFormState>[] = [
    { name: "enabled", label: t("adminRisk.enabled"), type: "switch" },
    {
      name: "mode",
      label: t("adminRisk.mode"),
      type: "select",
      options: [
        { value: "monitor", label: "monitor" },
        { value: "enforce", label: "enforce" },
      ],
      hint: t("adminRisk.modeHint"),
    },
    { name: "maxFailedRequestsPerMinute", label: t("adminRisk.maxFailed"), help: t("adminRisk.maxFailedHelp"), type: "number" },
    { name: "maxCostPerDay", label: t("adminRisk.maxCostPerDay"), help: t("adminRisk.maxCostPerDayHelp") },
    { name: "cooldownSeconds", label: t("adminRisk.cooldown"), help: t("adminRisk.cooldownHelp"), type: "number" },
    {
      name: "blockedCountries",
      label: t("adminRisk.blockedCountries"),
      type: "combobox",
      options: countryOptions(language),
      allowCustom: true,
      placeholder: t("adminRisk.blockedCountriesPlaceholder"),
      searchPlaceholder: t("adminCommon.search"),
      emptyText: t("adminCommon.noResults"),
      addCustomLabel: (q) => t("adminCommon.addValue", { value: q.toUpperCase() }),
      hint: t("adminRisk.blockedCountriesHint"),
    },
    {
      name: "blockedIps",
      label: t("adminRisk.blockedIps"),
      type: "tags",
      placeholder: t("adminRisk.blockedIpsPlaceholder"),
      hint: t("adminRisk.blockedIpsHint"),
    },
  ];

  const columns: Column<RiskControlLog>[] = [
    {
      key: "time",
      header: t("adminRisk.time"),
      pinned: true,
      render: (r) => (
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(r.created_at)}
        </span>
      ),
    },
    {
      key: "event",
      header: t("adminRisk.event"),
      render: (r) => <span className="text-srapi-text-primary">{r.action || "—"}</span>,
    },
    {
      key: "detail",
      header: t("adminRisk.detail"),
      hideOnMobile: true,
      render: (r) => <span className="text-srapi-text-secondary">{r.reason || "—"}</span>,
    },
    {
      key: "severity",
      header: t("adminRisk.severity"),
      render: (r) => <QuietBadge status={quietStatusFor(r.level)} label={statusLabel(t, r.level)} />,
    },
  ];

  const contentSafetyFields: FieldConfig<ContentSafetyFormState>[] = [
    { name: "enabled", label: t("adminRisk.contentSafetyEnabled"), type: "switch" },
    {
      name: "mode",
      label: t("adminRisk.contentSafetyMode"),
      type: "select",
      options: [
        { value: "monitor", label: "monitor" },
        { value: "enforce", label: "enforce" },
      ],
    },
    { name: "redactPii", label: t("adminRisk.redactPii"), type: "switch" },
    { name: "blockPii", label: t("adminRisk.blockPii"), type: "switch" },
    {
      name: "blockPromptInjection",
      label: t("adminRisk.blockPromptInjection"),
      type: "switch",
    },
    {
      name: "blockCustomKeywords",
      label: t("adminRisk.blockCustomKeywords"),
      type: "switch",
    },
    {
      name: "customKeywords",
      label: t("adminRisk.customKeywords"),
      type: "tags",
      placeholder: t("adminRisk.customKeywordsPlaceholder"),
    },
    {
      name: "modelScopes",
      label: t("adminRisk.modelScopes"),
      type: "tags",
      placeholder: t("adminRisk.modelScopesPlaceholder"),
      hint: t("adminRisk.modelScopesHint"),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminRisk.title")}
        description={t("adminRisk.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button
              variant="outline"
              size="sm"
              disabled={!config.data}
              onClick={() => setEditing(true)}
            >
              {t("adminRisk.editConfig")}
            </Button>
          </div>
        }
      />

      {/* current risk gauge */}
      {status.isLoading ? (
        <Card>
          <CardContent className="flex flex-wrap items-center gap-x-12 gap-y-4">
            <DialogListSkeleton rows={1} />
            <Skeleton className="h-8 w-12" />
            <Skeleton className="h-8 w-12" />
          </CardContent>
        </Card>
      ) : status.data ? (
        <Card>
          <CardContent className="flex flex-wrap items-center gap-x-12 gap-y-4">
            <div>
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminRisk.mode")}
              </div>
              <div className="mt-2">
                <QuietBadge status={quietStatusFor(status.data.mode)} label={statusLabel(t, status.data.mode)} />
              </div>
            </div>
            <div>
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminRisk.activeBlocks")}
              </div>
              <div className="mt-1 font-serif text-3xl text-srapi-text-primary tabular">
                {formatInteger(status.data.active_blocks)}
              </div>
            </div>
            <div>
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminRisk.recentEvents")}
              </div>
              <div className="mt-1 font-serif text-3xl text-srapi-text-primary tabular">
                {formatInteger(status.data.recent_events)}
              </div>
            </div>
            <div className="ml-auto text-right">
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminCommon.updated")}
              </div>
              <div className="mt-2 font-mono text-2xs text-srapi-text-secondary tabular">
                {formatDateTime(status.data.evaluated_at)}
              </div>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {contentSafety.isLoading ? (
        <Card>
          <CardContent className="flex flex-wrap items-center gap-x-12 gap-y-4">
            <DialogListSkeleton rows={1} />
            <Skeleton className="h-8 w-12" />
            <Skeleton className="h-8 w-12" />
          </CardContent>
        </Card>
      ) : contentSafety.data ? (
        <Card>
          <CardContent className="flex flex-wrap items-center gap-x-12 gap-y-4">
            <div>
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminRisk.contentSafetyTitle")}
              </div>
              <div className="mt-2">
                <QuietBadge
                  status={contentSafety.data.enabled ? "active" : "disabled"}
                  label={statusLabel(t, contentSafety.data.enabled ? "active" : "disabled")}
                />
              </div>
            </div>
            <div>
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminRisk.contentSafetyMode")}
              </div>
              <div className="mt-2">
                <QuietBadge
                  status={quietStatusFor(contentSafety.data.mode)}
                  label={statusLabel(t, contentSafety.data.mode)}
                />
              </div>
            </div>
            <div>
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminRisk.customKeywordCount")}
              </div>
              <div className="mt-1 font-serif text-3xl text-srapi-text-primary tabular">
                {formatInteger(contentSafety.data.custom_keywords.length)}
              </div>
            </div>
            <div>
              <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminRisk.modelScopeCount")}
              </div>
              <div className="mt-1 font-serif text-3xl text-srapi-text-primary tabular">
                {formatInteger(contentSafety.data.model_scopes.length)}
              </div>
            </div>
            <Button
              className="ml-auto"
              variant="outline"
              size="sm"
              onClick={() => setEditingContentSafety(true)}
            >
              {t("adminRisk.editContentSafety")}
            </Button>
          </CardContent>
        </Card>
      ) : null}

      <AdminListView
        query={logs}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(r) => r.id}
        emptyIcon={ShieldAlert}
        emptyTitle={t("adminRisk.emptyTitle")}
        emptyBody={t("adminRisk.emptyBody")}
        minWidth={640}
      />

      {editing && config.data ? (
        <ResourceFormDialog
          open
          onOpenChange={setEditing}
          title={t("adminRisk.editConfig")}
          fields={configFields}
          initial={createRiskControlForm(config.data)}
          buildBody={buildRiskControlConfig}
          submit={(body) => updateConfig.mutateAsync(body)}
          successMessage={t("feedback.saved")}
          isPending={updateConfig.isPending}
        />
      ) : null}

      {editingContentSafety && contentSafety.data ? (
        <ResourceFormDialog
          open
          onOpenChange={setEditingContentSafety}
          title={t("adminRisk.editContentSafety")}
          fields={contentSafetyFields}
          initial={createContentSafetyForm(contentSafety.data)}
          buildBody={buildContentSafetyConfig}
          submit={(body) => updateContentSafety.mutateAsync(body)}
          successMessage={t("feedback.saved")}
          isPending={updateContentSafety.isPending}
        />
      ) : null}
    </>
  );
}
