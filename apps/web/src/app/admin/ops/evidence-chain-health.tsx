"use client";

import Link from "next/link";
import type { ReactNode } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { Skeleton } from "@/components/ui/skeleton";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatInteger } from "@/lib/admin-format";
import { ADMIN_ROUTES } from "@/lib/routes";
import type { OpsSystemLogHealth } from "@/lib/sdk-types";

type EvidenceStatus = "healthy" | "degraded" | "unknown";

export function opsEvidenceChainStatus(
  health: OpsSystemLogHealth | undefined,
): { status: EvidenceStatus; tone: QuietStatus } {
  if (!health) {
    return { status: "unknown", tone: "disabled" };
  }
  const recorder = health.error_evidence_recorder;
  const degraded =
    health.degraded ||
    health.stale ||
    !health.writable ||
    !recorder.enabled ||
    recorder.degraded ||
    recorder.dropped_count > 0 ||
    recorder.write_failed_count > 0;
  return degraded ? { status: "degraded", tone: "error" } : { status: "healthy", tone: "active" };
}

export function OpsEvidenceChainHealth({
  health,
  loading,
}: {
  health: OpsSystemLogHealth | undefined;
  loading: boolean;
}) {
  const { t } = useLanguage();
  if (loading && !health) {
    return (
      <Card>
        <CardHeader className="flex-row items-center justify-between gap-3">
          <CardTitle>{t("adminOps.evidenceChain")}</CardTitle>
          <Skeleton className="h-6 w-24" />
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-4">
          <EvidenceMetricSkeleton />
          <EvidenceMetricSkeleton />
          <EvidenceMetricSkeleton />
          <EvidenceMetricSkeleton />
        </CardContent>
      </Card>
    );
  }

  const status = opsEvidenceChainStatus(health);
  const recorder = health?.error_evidence_recorder;

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-3">
        <CardTitle>{t("adminOps.evidenceChain")}</CardTitle>
        {loading && !health ? (
          <Skeleton className="h-6 w-24" />
        ) : (
          <QuietBadge
            status={status.tone}
            label={t(`adminOps.evidenceChainStatus.${status.status}`)}
          />
        )}
      </CardHeader>
      <CardContent className="grid gap-4 md:grid-cols-4">
        <EvidenceMetric
          label={t("adminOps.systemLogStore")}
          value={health?.storage_mode ?? "-"}
          footer={
            <span className="flex flex-wrap gap-1.5">
              <QuietBadge
                status={health?.writable ? "active" : "error"}
                label={
                  health?.writable
                    ? t("adminOpsSystemLogs.writable")
                    : t("adminOpsSystemLogs.readOnly")
                }
              />
              <QuietBadge
                status={health?.stale ? "limited" : "active"}
                label={
                  health?.stale ? t("adminOpsSystemLogs.stale") : t("adminOpsSystemLogs.fresh")
                }
              />
            </span>
          }
        />
        <EvidenceMetric
          label={t("adminOps.errorEvidence")}
          value={`${formatInteger(recorder?.queue_depth)}/${formatInteger(recorder?.queue_capacity)}`}
          valueTooltip={
            recorder ? (
              <DataTooltip
                title={t("adminOps.errorEvidence")}
                primary={`${formatInteger(recorder.queue_depth)}/${formatInteger(recorder.queue_capacity)}`}
                rows={[
                  { label: t("adminOps.recorded"), value: formatInteger(recorder.recorded_count), tone: "muted" },
                  { label: t("adminOps.processed"), value: formatInteger(recorder.processed_count), tone: "muted" },
                  {
                    label: t("adminOps.dropped"),
                    value: formatInteger(recorder.dropped_count),
                    tone: recorder.dropped_count > 0 ? "error" : "muted",
                  },
                  {
                    label: t("adminOps.failed"),
                    value: formatInteger(recorder.write_failed_count),
                    tone: recorder.write_failed_count > 0 ? "error" : "muted",
                  },
                ]}
              >
                <span className="cursor-help">
                  {formatInteger(recorder.queue_depth)}/{formatInteger(recorder.queue_capacity)}
                </span>
              </DataTooltip>
            ) : undefined
          }
          footer={
            <span className="flex flex-wrap gap-1.5">
              <QuietBadge
                status={!recorder?.enabled || recorder?.degraded ? "error" : "active"}
                label={
                  !recorder?.enabled
                    ? t("adminOps.recorderDisabled")
                    : recorder.degraded
                      ? t("adminOps.recorderDegraded")
                      : t("adminOps.recorderHealthy")
                }
              />
              <QuietBadge
                status={(recorder?.dropped_count ?? 0) > 0 ? "error" : "disabled"}
                label={`${t("adminOps.dropped")}:${formatInteger(recorder?.dropped_count ?? 0)}`}
              />
              <QuietBadge
                status={(recorder?.write_failed_count ?? 0) > 0 ? "error" : "disabled"}
                label={`${t("adminOps.failed")}:${formatInteger(recorder?.write_failed_count ?? 0)}`}
              />
            </span>
          }
        />
        <EvidenceMetric
          label={t("adminOpsSystemLogs.lastWrite")}
          value={formatDateTime(health?.last_log_at)}
          footer={
            <span className="flex flex-wrap gap-1.5">
              <QuietBadge
                status="disabled"
                label={`${t("adminOps.recorded")}:${formatInteger(recorder?.recorded_count ?? 0)}`}
              />
              <QuietBadge
                status="disabled"
                label={`${t("adminOps.processed")}:${formatInteger(recorder?.processed_count ?? 0)}`}
              />
            </span>
          }
        />
        <div className="min-w-0 space-y-2 md:border-l md:border-srapi-border md:pl-4">
          <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminOps.evidenceLinks")}
          </div>
          <div className="flex flex-wrap gap-2 text-[12px]">
            <Link
              href={ADMIN_ROUTES.opsSystemLogs}
              className="text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
            >
              {t("adminOps.openSystemLogs")}
            </Link>
            <Link
              href={ADMIN_ROUTES.errorLogs}
              className="text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
            >
              {t("adminOps.openErrorLogs")}
            </Link>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function EvidenceMetricSkeleton() {
  return (
    <div className="min-w-0 space-y-2 md:border-l md:border-srapi-border md:pl-4 md:first:border-l-0 md:first:pl-0">
      <Skeleton className="h-3 w-20" />
      <Skeleton className="h-5 w-24" />
      <Skeleton className="h-6 w-36" />
    </div>
  );
}

function EvidenceMetric({
  label,
  value,
  footer,
  valueTooltip,
}: {
  label: string;
  value: string;
  footer: ReactNode;
  /**
   * Optional drop-in replacement for the value cell — pass a DataTooltip-wrapped
   * span when the value benefits from a breakdown reveal on hover.
   */
  valueTooltip?: ReactNode;
}) {
  return (
    <div className="min-w-0 space-y-2 md:border-l md:border-srapi-border md:pl-4 md:first:border-l-0 md:first:pl-0">
      <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{label}</div>
      <div className="truncate text-sm font-medium tabular text-srapi-text-primary" title={value}>
        {valueTooltip ?? value}
      </div>
      <div className="text-[11px] text-srapi-text-tertiary">{footer}</div>
    </div>
  );
}
