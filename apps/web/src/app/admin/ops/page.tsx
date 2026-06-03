"use client";

import { useState } from "react";
import { Activity, Pencil } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { SloFormDialog } from "@/components/admin/slo-form-dialog";
import {
  useOpsSlos,
  useOpsAlerts,
  useAcknowledgeAlert,
  useOpsThroughput,
  useOpsErrorTrend,
} from "@/hooks/admin-queries";
import type { OpsSloDefinition } from "@/lib/sdk-types";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Sparkline } from "@/components/charts/sparkline";
import { QuotaNotchRail } from "@/components/ui/quota-notch-rail";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { EmptyState } from "@/components/ui/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { quietStatusFor } from "@/lib/status-badge";
import { formatDateTime } from "@/lib/admin-format";

export default function AdminOpsPage() {
  return (
    <AdminShell>
      <OpsContent />
    </AdminShell>
  );
}

function OpsContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const slos = useOpsSlos();
  const alerts = useOpsAlerts();
  const ackMut = useAcknowledgeAlert();

  async function ackAlert(id: string) {
    try {
      await ackMut.mutateAsync(id);
      toast({ title: t("feedback.acknowledged"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const throughput = useOpsThroughput();
  const errorTrend = useOpsErrorTrend();
  const [sloTarget, setSloTarget] = useState<OpsSloDefinition | "new" | null>(null);
  // Only firing alerts are "active". A resolved/suppressed alert also has no
  // acknowledged_at, so the old `!acknowledged_at` check wrongly kept them here.
  const activeAlerts = (alerts.data?.data ?? []).filter((a) => a.status === "firing");

  const throughputValues = (throughput.data?.points ?? []).map((p) => p.request_count);
  const errorValues = (errorTrend.data?.points ?? []).map((p) => p.error_rate * 100);

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminOps.title")}
        description={t("adminOps.subtitle")}
        actions={
          <Button variant="primary" size="sm" onClick={() => setSloTarget("new")}>
            ＋ {t("adminOps.createSlo")}
          </Button>
        }
      />

      {throughputValues.length > 0 || errorValues.length > 0 ? (
        <div className="grid gap-4 md:grid-cols-2">
          <Card>
            <CardContent>
              <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminOps.throughput")}
              </span>
              <div className="mt-3">
                <Sparkline values={throughputValues} ariaLabel={t("adminOps.throughput")} />
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent>
              <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminOps.errorRate")}
              </span>
              <div className="mt-3">
                <Sparkline values={errorValues} ariaLabel={t("adminOps.errorRate")} />
              </div>
            </CardContent>
          </Card>
        </div>
      ) : null}

      <PageQueryState
        query={slos}
        isEmpty={(d) => d.data.length === 0}
        skeleton={
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-36 rounded-xl" />
            ))}
          </div>
        }
      >
        {(result) =>
          result.data.length === 0 ? (
            <Card>
              <EmptyState
                icon={Activity}
                title={t("adminOps.emptySlo")}
                description={t("adminOps.emptySloBody")}
              />
            </Card>
          ) : (
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              {result.data.map((slo, i) => {
                const def = slo.definition;
                const objective = def?.objective ?? slo.evaluation?.objective ?? 0;
                const errorRate = slo.evaluation?.error_rate ?? 0;
                const availability = Math.max(0, (1 - errorRate) * 100);
                const objectivePct = objective > 1 ? objective : objective * 100;
                // Derive reliability health from the evaluation, not the SLO's
                // enable-state (`def.status` is "active"/"disabled", not health).
                const enabled = def?.status !== "disabled";
                const health = !enabled
                  ? "disabled"
                  : availability >= objectivePct
                    ? "healthy"
                    : "breached";
                const healthLabel = enabled ? t(`adminOps.${health}`) : t("common.disabled");
                return (
                  <Card key={def?.id ?? i}>
                    <CardHeader>
                      <CardTitle className="not-italic font-sans text-base text-srapi-text-primary">
                        {def?.name ?? t("adminOps.slo")}
                      </CardTitle>
                      <div className="flex items-center gap-2">
                        <QuietBadge
                          status={!enabled ? "disabled" : health === "healthy" ? "active" : "error"}
                          label={healthLabel}
                        />
                        {def ? (
                          <button
                            type="button"
                            onClick={() => setSloTarget(def)}
                            aria-label={t("adminOps.editSlo")}
                            className="text-srapi-text-tertiary transition-colors hover:text-srapi-text-primary"
                          >
                            <Pencil className="size-3.5" />
                          </button>
                        ) : null}
                      </div>
                    </CardHeader>
                    <CardContent className="space-y-4">
                      <div className="flex items-baseline justify-between">
                        <span className="text-xs text-srapi-text-secondary">
                          {t("adminOps.availability")}
                        </span>
                        <span className="font-serif text-2xl text-srapi-text-primary tabular">
                          {availability.toFixed(2)}%
                        </span>
                      </div>
                      <QuotaNotchRail value={availability} />
                      <div className="flex items-center justify-between font-mono text-2xs text-srapi-text-tertiary">
                        <span>
                          {t("adminOps.objective")} {objectivePct.toFixed(1)}%
                        </span>
                        <span>
                          {t("adminOps.window")} {def?.window_days ?? 30}d
                        </span>
                      </div>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )
        }
      </PageQueryState>

      {activeAlerts.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle className="not-italic font-sans text-base text-srapi-text-primary">
              {t("adminOps.alerts")}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {activeAlerts.map((alert) => (
              <div
                key={alert.id}
                className="flex items-center justify-between gap-4 border-t border-srapi-border py-2.5 first:border-t-0"
              >
                <div className="min-w-0">
                  <div className="truncate text-sm text-srapi-text-primary">{alert.summary}</div>
                  <div className="font-mono text-2xs text-srapi-text-tertiary tabular">
                    {formatDateTime(alert.started_at ?? alert.created_at)}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-3">
                  <QuietBadge status={quietStatusFor(alert.severity)} label={alert.severity} />
                  <Button
                    variant="outline"
                    size="sm"
                    loading={ackMut.isPending && ackMut.variables === alert.id}
                    onClick={() => ackAlert(alert.id)}
                  >
                    {t("adminOps.acknowledge")}
                  </Button>
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
      ) : null}

      {sloTarget !== null ? (
        <SloFormDialog
          key={sloTarget === "new" ? "new" : sloTarget.id}
          open
          target={sloTarget === "new" ? null : sloTarget}
          onOpenChange={(open) => {
            if (!open) setSloTarget(null);
          }}
        />
      ) : null}
    </>
  );
}
