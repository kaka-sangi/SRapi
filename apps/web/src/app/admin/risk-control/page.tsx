"use client";

import { useState } from "react";
import { ShieldAlert } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
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
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
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

function riskLevelSeverity(
  level: RiskControlLog["level"],
): "info" | "success" | "warning" | "error" | "critical" | undefined {
  switch (level) {
    case "block":
      return "error";
    case "warn":
      return "warning";
    case "info":
      return "info";
    default:
      return undefined;
  }
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
        <span className="whitespace-nowrap text-[12px] tabular text-srapi-text-tertiary">
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
      sortValue: (r) => r.level,
      render: (r) => (
        <DataTooltip
          title={t("adminRisk.severity")}
          primary={statusLabel(t, r.level)}
          rows={[
            { label: t("adminRisk.event"), value: r.action || "—", tone: "muted" },
            ...(r.subject
              ? [{ label: t("adminRisk.detail"), value: r.subject, tone: "muted" as const }]
              : []),
            { label: t("adminRisk.time"), value: formatDateTime(r.created_at), tone: "muted" },
          ]}
        >
          <QuietBadge status={quietStatusFor(r.level)} label={statusLabel(t, r.level)} />
        </DataTooltip>
      ),
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
    {
      name: "moderationEnabled",
      label: t("adminRisk.moderationEnabled"),
      type: "switch",
      hint: t("adminRisk.moderationEnabledHint"),
    },
    {
      name: "moderationModel",
      label: t("adminRisk.moderationModel"),
      placeholder: "omni-moderation-latest",
    },
    {
      name: "moderationBaseUrl",
      label: t("adminRisk.moderationBaseUrl"),
      placeholder: "https://api.openai.com/v1",
    },
    {
      name: "moderationApiKey",
      label: t("adminRisk.moderationApiKey"),
      type: "password",
      placeholder: t("adminRisk.moderationApiKeyPlaceholder"),
      hint: t("adminRisk.moderationApiKeyHint"),
    },
    {
      name: "moderationBlockOnFlag",
      label: t("adminRisk.moderationBlockOnFlag"),
      type: "switch",
      hint: t("adminRisk.moderationBlockOnFlagHint"),
    },
    {
      name: "moderationTimeoutMs",
      label: t("adminRisk.moderationTimeoutMs"),
      type: "number",
    },
    {
      name: "moderationCacheTtlSeconds",
      label: t("adminRisk.moderationCacheTtlSeconds"),
      type: "number",
      hint: t("adminRisk.moderationCacheTtlHint"),
    },
  ];

  return (
    <>
      <SectionHero
        eyebrow={t("hero.eyebrowOpsRiskControl")}
        title={t("adminRisk.title")}
        description={t("adminRisk.subtitle")}
        metrics={
          status.data
            ? [
                {
                  label: t("adminRisk.activeBlocks"),
                  value: formatInteger(status.data.active_blocks),
                  tone: status.data.active_blocks > 0 ? "warning" : "default",
                },
                {
                  label: t("adminRisk.recentEvents"),
                  value: formatInteger(status.data.recent_events),
                },
              ]
            : undefined
        }
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
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminRisk.mode")}
              </div>
              <div className="mt-2">
                <QuietBadge status={quietStatusFor(status.data.mode)} label={statusLabel(t, status.data.mode)} />
              </div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminRisk.activeBlocks")}
              </div>
              <div
                className={
                  "mt-1 tabular " +
                  (status.data.active_blocks > 0 ? "metric-primary metric-strong-warn" : "metric-primary")
                }
              >
                {formatInteger(status.data.active_blocks)}
              </div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminRisk.recentEvents")}
              </div>
              <div className="mt-1 metric-secondary tabular">
                {formatInteger(status.data.recent_events)}
              </div>
            </div>
            <div className="ml-auto text-right">
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminCommon.updated")}
              </div>
              <div className="mt-2 text-[12px] tabular text-srapi-text-secondary">
                {formatDateTime(status.data.evaluated_at)}
              </div>
            </div>
          </CardContent>
        </Card>
      ) : status.isError ? (
        <Card>
          <IllustratedEmptyState
            illust="cog"
            title={t("common.error")}
            description={t("common.errorBody")}
            action={
              <Button variant="outline" size="sm" onClick={() => void status.refetch()}>
                {t("common.retry")}
              </Button>
            }
          />
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
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
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
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
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
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminRisk.customKeywordCount")}
              </div>
              <div className="mt-1 metric-secondary tabular">
                {formatInteger(contentSafety.data.custom_keywords.length)}
              </div>
            </div>
            <div>
              <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminRisk.modelScopeCount")}
              </div>
              <div className="mt-1 metric-tertiary tabular">
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
      ) : contentSafety.isError ? (
        <Card>
          <IllustratedEmptyState
            illust="cog"
            title={t("common.error")}
            description={t("common.errorBody")}
            action={
              <Button variant="outline" size="sm" onClick={() => void contentSafety.refetch()}>
                {t("common.retry")}
              </Button>
            }
          />
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
        rowSeverity={(r) => riskLevelSeverity(r.level)}
        expandRow={(r) => <RiskLogExpandDetail log={r} configMode={config.data?.mode} t={t} />}
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

/**
 * Inline risk-log expansion: shows the trigger rule expression (action),
 * full reason text, subject, the active enforcement mode, and any metadata
 * keys the recorder attached. Operators can correlate without reaching for
 * the JSON.
 */
function RiskLogExpandDetail({
  log,
  configMode,
  t,
}: {
  log: RiskControlLog;
  configMode: string | undefined;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const metaEntries = Object.entries(log.metadata ?? {}).slice(0, 12);

  const triggerRows: Array<{ label: string; value: string; mono?: boolean; tone?: "default" | "muted" }> = [
    { label: t("adminRisk.event"), value: log.action || "—", mono: true },
    { label: t("adminRisk.severity"), value: statusLabel(t, log.level), mono: true },
  ];
  if (log.subject) triggerRows.push({ label: t("adminRisk.detail"), value: log.subject, mono: true });
  if (configMode) triggerRows.push({ label: t("adminRisk.mode"), value: configMode, mono: true, tone: "muted" });
  triggerRows.push({
    label: t("adminRisk.time"),
    value: formatDateTime(log.created_at),
    tone: "muted",
  });

  return (
    <div>
      <InlineDetailGrid
        sections={[
          { title: t("adminRisk.event"), rows: triggerRows },
          ...(metaEntries.length > 0
            ? [
                {
                  title: t("adminCommon.metadata") || "Metadata",
                  rows: metaEntries.map(([k, v]) => ({
                    label: k,
                    value:
                      typeof v === "string" || typeof v === "number" || typeof v === "boolean"
                        ? String(v)
                        : JSON.stringify(v),
                    mono: true,
                  })),
                },
              ]
            : []),
        ]}
      />
      {log.reason ? (
        <div className="border-t border-srapi-border/60 bg-srapi-card-muted/30 px-6 py-4">
          <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminRisk.detail")}
          </div>
          <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-srapi-border bg-srapi-card-muted/60 p-3 text-[12px] text-srapi-text-secondary">
            {log.reason}
          </pre>
        </div>
      ) : null}
    </div>
  );
}
