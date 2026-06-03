"use client";

import { useState } from "react";
import { ShieldAlert } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { ResourceFormDialog, type FieldConfig } from "@/components/admin/resource-form-dialog";
import { useRiskStatus, useRiskLogs, useRiskConfig, useUpdateRiskConfig } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatDateTime, formatInteger } from "@/lib/admin-format";
import {
  createRiskControlForm,
  buildRiskControlConfig,
  type RiskControlFormState,
} from "@/lib/admin-risk-control-form";
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
  const status = useRiskStatus();
  const logs = useRiskLogs();
  const config = useRiskConfig();
  const updateConfig = useUpdateRiskConfig();
  const [editing, setEditing] = useState(false);

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
    { name: "maxFailedRequestsPerMinute", label: t("adminRisk.maxFailed"), type: "number" },
    { name: "maxCostPerDay", label: t("adminRisk.maxCostPerDay") },
    { name: "cooldownSeconds", label: t("adminRisk.cooldown"), type: "number" },
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

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminRisk.title")}
        description={t("adminRisk.subtitle")}
        actions={
          <Button
            variant="outline"
            size="sm"
            disabled={!config.data}
            onClick={() => setEditing(true)}
          >
            {t("adminRisk.editConfig")}
          </Button>
        }
      />

      {/* current risk gauge */}
      {status.isLoading ? (
        <Skeleton className="h-24 rounded-xl" />
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

      <AdminListView
        query={logs}
        columns={columns}
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
    </>
  );
}
