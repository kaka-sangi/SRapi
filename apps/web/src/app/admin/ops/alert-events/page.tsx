"use client";

import { useState, type ReactNode } from "react";
import Link from "next/link";
import { BellRing } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar } from "@/components/admin/list-toolbar";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import { OpsAlertRunbookSteps } from "@/components/admin/ops-alert-runbook-steps";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useAdminErrorLogFingerprints, useOpsAlertEvents } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import {
  buildOpsAlertEvidenceLinks,
  buildOpsAlertRunbookSteps,
  type OpsAlertEvidenceLinks,
} from "@/lib/admin-ops-alert-evidence";
import { adminErrorLogDetailHref, adminRequestEvidenceHref } from "@/lib/admin-log-links";
import { formatDateTime, formatInteger, formatLatency, formatPercent, safeJson } from "@/lib/admin-format";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import type {
  JsonObject,
  OpsAlertEvent,
  OpsAlertSeverity,
  OpsAlertStatus,
  OpsErrorFingerprint,
} from "@/lib/sdk-types";

const ALERT_STATUSES: OpsAlertStatus[] = ["firing", "acknowledged", "resolved", "suppressed"];
const ALERT_SEVERITIES: OpsAlertSeverity[] = ["critical", "warning", "ticket"];

function alertSeverityTone(
  severity: OpsAlertSeverity | undefined,
  status: OpsAlertStatus,
): "info" | "success" | "warning" | "error" | "critical" | undefined {
  // Resolved/suppressed alerts mute — they don't need to scream.
  if (status === "resolved") return "success";
  if (status === "suppressed") return "info";
  switch (severity) {
    case "critical":
      return "critical";
    case "warning":
      return "warning";
    case "ticket":
      return "info";
    default:
      return undefined;
  }
}

export default function AdminOpsAlertEventsPage() {
  return (
    <AdminShell>
      <AlertEventsContent />
    </AdminShell>
  );
}

function AlertEventsContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-ops-alert-events", []);
  const statusFilter = (list.filters.status as OpsAlertStatus) || undefined;
  const severityFilter = (list.filters.severity as OpsAlertSeverity) || undefined;
  const events = useOpsAlertEvents({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
    severity: severityFilter,
  });
  const [detail, setDetail] = useState<OpsAlertEvent | null>(null);

  const columns: Column<OpsAlertEvent>[] = [
    {
      key: "time",
      header: t("adminOpsAlertEvents.time"),
      pinned: true,
      render: (event) => (
        <span className="whitespace-nowrap text-[12px] tabular text-srapi-text-tertiary">
          {formatDateTime(event.started_at)}
        </span>
      ),
    },
    {
      key: "alert",
      header: t("adminOpsAlertEvents.alert"),
      className: "max-w-xl",
      render: (event) => (
        <div className="min-w-0">
          <div className="truncate text-sm text-srapi-text-primary">{event.summary}</div>
          <div className="truncate text-[11px] text-srapi-text-tertiary">
            {event.rule_id} · {event.fingerprint}
          </div>
        </div>
      ),
    },
    {
      key: "status",
      header: t("adminCommon.status"),
      sortValue: (event) => event.status,
      render: (event) => (
        <div className="flex flex-wrap items-center gap-1.5">
          <DataTooltip
            title={t("adminCommon.status")}
            primary={statusLabel(t, event.status)}
            rows={[
              { label: t("adminOpsAlertEvents.startedAt"), value: formatDateTime(event.started_at) },
              { label: t("adminOpsAlertEvents.updated"), value: formatDateTime(event.updated_at) },
              ...(event.acknowledged_at
                ? [
                    {
                      label: t("adminOpsAlertEvents.acknowledgedAt"),
                      value: formatDateTime(event.acknowledged_at),
                      tone: "muted" as const,
                    },
                  ]
                : []),
              ...(event.resolved_at
                ? [
                    {
                      label: t("adminOpsAlertEvents.resolvedAt"),
                      value: formatDateTime(event.resolved_at),
                      tone: "success" as const,
                    },
                  ]
                : []),
            ]}
            footer={event.fingerprint}
          >
            <QuietBadge status={quietStatusFor(event.status)} label={statusLabel(t, event.status)} />
          </DataTooltip>
          <QuietBadge status={quietStatusFor(event.severity)} label={event.severity} />
        </div>
      ),
    },
    {
      key: "evidence",
      header: t("adminOpsAlertEvents.evidence"),
      hideOnMobile: true,
      render: (event) => <AlertEvidenceLinks links={buildOpsAlertEvidenceLinks(event.details)} />,
    },
    {
      key: "runbook",
      header: t("adminOps.runbook.title"),
      hideOnMobile: true,
      render: (event) => (
        <OpsAlertRunbookSteps steps={buildOpsAlertRunbookSteps(event.details)} compact />
      ),
    },
    {
      key: "updated",
      header: t("adminOpsAlertEvents.updated"),
      hideOnMobile: true,
      sortValue: (event) => event.updated_at,
      render: (event) => (
        <span className="whitespace-nowrap text-[12px] tabular text-srapi-text-tertiary">
          {formatDateTime(event.updated_at)}
        </span>
      ),
    },
  ];

  return (
    <>
      <SectionHero
        eyebrow={t("hero.eyebrowOpsAlerts")}
        title={t("adminOpsAlertEvents.title")}
        description={t("adminOpsAlertEvents.subtitle")}
        metrics={(() => {
          const rows = events.data?.data ?? [];
          const firing = rows.filter((e) => e.status === "firing").length;
          const unack = rows.filter(
            (e) => e.status === "firing" && !e.acknowledged_at,
          ).length;
          const critical = rows.filter(
            (e) => e.severity === "critical" && e.status === "firing",
          ).length;
          return [
            {
              label: t("adminOpsAlertEvents.unacknowledged"),
              value: formatInteger(unack),
              tone: unack > 0 ? "warning" : "default",
            },
            {
              label: t("adminOpsAlertEvents.firingCount"),
              value: formatInteger(firing),
              tone: firing > 0 ? "error" : "default",
            },
            {
              label: t("adminOpsAlertEvents.criticalCount"),
              value: formatInteger(critical),
              tone: critical > 0 ? "error" : "default",
            },
          ];
        })()}
        actions={
          <div className="flex items-center gap-3">
            {events.data ? (
              <ListCount total={events.data.pagination?.total ?? events.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
          </div>
        }
      />
      <AdminListView
        query={events}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(event) => event.id}
        emptyIcon={BellRing}
        emptyTitle={t("adminOpsAlertEvents.emptyTitle")}
        emptyBody={t("adminOpsAlertEvents.emptyBody")}
        minWidth={1040}
        isFiltered={Boolean(statusFilter || severityFilter)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        dimRow={(event) => event.status === "resolved" || event.status === "suppressed"}
        rowSeverity={(event) => alertSeverityTone(event.severity, event.status)}
        expandRow={(event) => (
          <AlertExpandDetail event={event} t={t} />
        )}
        toolbar={
          <ListToolbar>
            <SegmentedControl<string>
              value={(severityFilter as string) ?? "all"}
              onChange={(v) => list.setFilter("severity", v === "all" ? "" : v)}
              ariaLabel={t("adminOpsAlertEvents.allSeverities")}
              size="sm"
              options={[
                { value: "all", label: t("adminOpsAlertEvents.allSeverities") },
                ...ALERT_SEVERITIES.map((v) => ({ value: v, label: v })),
              ]}
            />
            <SegmentedControl<string>
              value={(statusFilter as string) ?? "all"}
              onChange={(v) => list.setFilter("status", v === "all" ? "" : v)}
              ariaLabel={t("adminOpsAlertEvents.allStatuses")}
              size="sm"
              options={[
                { value: "all", label: t("adminOpsAlertEvents.allStatuses") },
                ...ALERT_STATUSES.map((v) => ({ value: v, label: v })),
              ]}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: events.data?.pagination?.total ?? events.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(event) => (
          <RowActionsMenu
            actions={[{ label: t("adminOpsAlertEvents.viewDetails"), onSelect: () => setDetail(event) }]}
          />
        )}
      />

      {detail ? (
        <Dialog open onOpenChange={(open) => !open && setDetail(null)}>
          <DialogContent className="max-w-4xl">
            <DialogHeader>
              <DialogTitle>{t("adminOpsAlertEvents.detailTitle")}</DialogTitle>
              <DialogDescription>
                {detail.rule_id} · {detail.fingerprint}
              </DialogDescription>
            </DialogHeader>
            <div className="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pr-1">
              <div className="grid gap-2 sm:grid-cols-2">
                <Meta label={t("adminCommon.status")} value={statusLabel(t, detail.status)} />
                <Meta label={t("adminOpsAlertEvents.severity")} value={detail.severity} />
                <Meta label={t("adminOpsAlertEvents.startedAt")} value={formatDateTime(detail.started_at)} />
                <Meta label={t("adminOpsAlertEvents.updated")} value={formatDateTime(detail.updated_at)} />
                {detail.resolved_at ? (
                  <Meta label={t("adminOpsAlertEvents.resolvedAt")} value={formatDateTime(detail.resolved_at)} />
                ) : null}
                {detail.acknowledged_at ? (
                  <Meta label={t("adminOpsAlertEvents.acknowledgedAt")} value={formatDateTime(detail.acknowledged_at)} />
                ) : null}
                {detail.suppressed_by ? (
                  <Meta label={t("adminOpsAlertEvents.suppressedBy")} value={detail.suppressed_by} />
                ) : null}
              </div>
              <AlertSignalSummary details={detail.details} />
              <AlertFingerprintPanel details={detail.details} />
              <div>
                <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("adminOpsAlertEvents.evidence")}
                </span>
                <div className="mt-2">
                  <AlertEvidenceLinks links={buildOpsAlertEvidenceLinks(detail.details)} />
                </div>
              </div>
              <OpsAlertRunbookSteps steps={buildOpsAlertRunbookSteps(detail.details)} />
              <JsonBlock label={t("adminOpsAlertEvents.details")} value={detail.details} />
            </div>
          </DialogContent>
        </Dialog>
      ) : null}
    </>
  );
}

function AlertFingerprintPanel({ details }: { details?: JsonObject }) {
  const { t } = useLanguage();
  const query = buildAlertFingerprintQuery(details);
  const fingerprints = useAdminErrorLogFingerprints(query ?? undefined, Boolean(query));
  if (!query) return null;
  if (fingerprints.isLoading) {
    return (
      <section>
        <SectionLabel>{t("adminOpsAlertEvents.fingerprintAttribution")}</SectionLabel>
        <div className="text-[11px] text-srapi-text-tertiary">
          {t("adminOpsAlertEvents.fingerprintsLoading")}
        </div>
      </section>
    );
  }
  if (fingerprints.isError) {
    return (
      <section>
        <SectionLabel>{t("adminOpsAlertEvents.fingerprintAttribution")}</SectionLabel>
        <div className="text-[11px] text-srapi-error">
          {t("adminOpsAlertEvents.fingerprintsFailed")}
        </div>
      </section>
    );
  }

  const data = fingerprints.data?.data ?? [];
  const meta = fingerprints.data?.meta;
  return (
    <section>
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <SectionLabel>{t("adminOpsAlertEvents.fingerprintAttribution")}</SectionLabel>
        {meta ? (
          <span className="text-[11px] text-srapi-text-tertiary">
            {t("adminOpsAlertEvents.fingerprintsMeta", {
              total: meta.total,
              scanned: meta.scanned,
            })}
            {meta.truncated ? ` · ${t("adminOpsAlertEvents.fingerprintsTruncated")}` : ""}
          </span>
        ) : null}
      </div>
      {data.length === 0 ? (
        <div className="text-[11px] text-srapi-text-tertiary">
          {t("adminOpsAlertEvents.fingerprintsEmpty")}
        </div>
      ) : (
        <div className="space-y-2">
          {data.slice(0, 5).map((item) => (
            <AlertFingerprintItem key={item.fingerprint} item={item} />
          ))}
        </div>
      )}
    </section>
  );
}

function AlertFingerprintItem({ item }: { item: OpsErrorFingerprint }) {
  const { t } = useLanguage();
  const exampleHref = adminErrorLogDetailHref({ id: item.example_error_log_id });
  const requestEvidenceHref = adminRequestEvidenceHref({ request_id: item.example_request_id });
  return (
    <div className="rounded border border-srapi-border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate text-xs font-medium text-srapi-text-primary">
            {item.message_pattern || item.error_class || item.fingerprint}
          </div>
          <div className="mt-1 truncate text-[11px] text-srapi-text-tertiary">
            {[
              item.source_endpoint,
              item.model,
              item.error_class,
              item.error_owner,
              item.status_class,
            ]
              .filter(Boolean)
              .join(" · ")}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-1.5">
          <QuietBadge status="error" label={t("adminOpsAlertEvents.fingerprintCount", { count: item.count })} />
          {item.open_count > 0 ? (
            <QuietBadge status="limited" label={t("adminOpsAlertEvents.fingerprintOpen", { count: item.open_count })} />
          ) : null}
          {item.investigating_count > 0 ? (
            <QuietBadge
              status="limited"
              label={t("adminOpsAlertEvents.fingerprintInvestigating", {
                count: item.investigating_count,
              })}
            />
          ) : null}
        </div>
      </div>
      <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-[11px] text-srapi-text-tertiary">
        <span className="tabular">
          {formatDateTime(item.first_occurred_at)} → {formatDateTime(item.last_occurred_at)}
        </span>
        <div className="flex flex-wrap items-center gap-1">
          {requestEvidenceHref ? (
            <Button asChild variant="ghost" size="sm">
              <Link href={requestEvidenceHref}>{t("adminOps.evidence.requestEvidence")}</Link>
            </Button>
          ) : null}
          {exampleHref ? (
            <Button asChild variant="ghost" size="sm">
              <Link href={exampleHref}>{t("adminOpsAlertEvents.openFingerprintExample")}</Link>
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}

interface SignalItem {
  label: string;
  value: string;
}

interface SignalSection {
  title: string;
  items: SignalItem[];
}

function AlertSignalSummary({ details }: { details?: JsonObject }) {
  const { t } = useLanguage();
  const sections = buildAlertSignalSections(details, t);
  if (sections.length === 0) return null;

  return (
    <div className="grid gap-3 lg:grid-cols-2">
      {sections.map((section) => (
        <div key={section.title} className="rounded border border-srapi-border p-3">
          <div className="mb-2 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {section.title}
          </div>
          <div className="grid gap-1.5">
            {section.items.map((item) => (
              <div key={`${section.title}:${item.label}`} className="grid grid-cols-[7rem_1fr] gap-2 text-xs">
                <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {item.label}
                </span>
                <span className="min-w-0 break-words font-mono text-srapi-text-secondary">
                  {item.value}
                </span>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function buildAlertSignalSections(
  details: JsonObject | undefined,
  t: (key: string, values?: Record<string, string | number>) => string,
): SignalSection[] {
  if (!details) return [];
  const metricType = detailString(details, "metric_type", "metricType");
  const operator = detailString(details, "operator");
  const sections: SignalSection[] = [];

  const triggerItems = compactSignalItems([
    signalString(t("adminOpsAlertEvents.ruleName"), detailString(details, "rule_name", "ruleName")),
    signalString(t("adminOpsAlertEvents.sloName"), detailString(details, "slo_name", "sloName")),
    signalString(t("adminOpsAlertEvents.metric"), metricType),
    metricType && operator && detailNumber(details, "threshold") !== null
      ? {
          label: t("adminOpsAlertEvents.condition"),
          value: `${metricType} ${operator} ${formatAlertNumber("threshold", detailNumber(details, "threshold"), metricType)}`,
        }
      : null,
    signalNumber(t("adminOpsAlertEvents.observed"), "observed_value", details, metricType),
    signalNumber(t("adminOpsAlertEvents.objective"), "objective", details),
    signalNumber(t("adminOpsAlertEvents.burnRateThreshold"), "burn_rate_threshold", details),
  ]);
  if (triggerItems.length > 0) {
    sections.push({ title: t("adminOpsAlertEvents.signalTrigger"), items: triggerItems });
  }

  const trafficItems = compactSignalItems([
    signalNumber(t("adminOpsAlertEvents.totalRequests"), "total_requests", details),
    signalNumber(t("adminOpsAlertEvents.goodRequests"), "good_requests", details),
    signalNumber(t("adminOpsAlertEvents.badRequests"), "bad_requests", details),
    signalNumber(t("adminOpsAlertEvents.minRequestCount"), "min_request_count", details),
    signalNumber(t("adminOpsAlertEvents.errorRate"), "error_rate", details),
    signalNumber(t("adminOpsAlertEvents.successRate"), "success_rate", details),
    signalNumber(t("adminOpsAlertEvents.latencyP95"), "latency_p95_ms", details),
  ]);
  if (trafficItems.length > 0) {
    sections.push({ title: t("adminOpsAlertEvents.signalTraffic"), items: trafficItems });
  }

  const burnRateItems = compactSignalItems([
    signalNumber(t("adminOpsAlertEvents.longBurnRate"), "long_burn_rate", details),
    signalNumber(t("adminOpsAlertEvents.shortBurnRate"), "short_burn_rate", details),
    signalNumber(t("adminOpsAlertEvents.errorBudgetConsumed"), "error_budget_consumed", details),
    signalNumber(t("adminOpsAlertEvents.longTotalRequests"), "long_total_requests", details),
    signalNumber(t("adminOpsAlertEvents.shortTotalRequests"), "short_total_requests", details),
    signalNumber(t("adminOpsAlertEvents.longBadRequests"), "long_bad_requests", details),
    signalNumber(t("adminOpsAlertEvents.shortBadRequests"), "short_bad_requests", details),
  ]);
  if (burnRateItems.length > 0) {
    sections.push({ title: t("adminOpsAlertEvents.signalBurnRate"), items: burnRateItems });
  }

  const windowItems = compactSignalItems([
    signalNumber(t("adminOpsAlertEvents.windowSize"), "window_seconds", details),
    signalNumber(t("adminOpsAlertEvents.longWindow"), "long_window_seconds", details),
    signalNumber(t("adminOpsAlertEvents.shortWindow"), "short_window_seconds", details),
    signalString(t("adminOpsAlertEvents.windowStart"), detailString(details, "window_start", "windowStart")),
    signalString(t("adminOpsAlertEvents.windowEnd"), detailString(details, "window_end", "windowEnd")),
  ]);
  if (windowItems.length > 0) {
    sections.push({ title: t("adminOpsAlertEvents.signalWindow"), items: windowItems });
  }

  const scopeItems = compactSignalItems([
    signalString(t("adminOpsAlertEvents.requestId"), detailString(details, "request_id", "requestId")),
    signalString(t("adminOpsAlertEvents.accountId"), detailString(details, "account_id", "accountId")),
    signalString(t("adminOpsAlertEvents.providerId"), detailString(details, "provider_id", "providerId")),
    signalString(t("adminOpsAlertEvents.sourceEndpoint"), detailString(details, "source_endpoint", "sourceEndpoint")),
    signalString(t("adminOpsAlertEvents.model"), detailString(details, "model", "canonical_model", "model_alias")),
    signalString(t("adminOpsAlertEvents.errorClass"), detailString(details, "error_class", "errorClass")),
    signalString(t("adminOpsAlertEvents.errorOwnerExclude"), detailString(details, "error_owner_exclude", "errorOwnerExclude")),
  ]);
  if (scopeItems.length > 0) {
    sections.push({ title: t("adminOpsAlertEvents.signalScope"), items: scopeItems });
  }

  return sections;
}

function buildAlertFingerprintQuery(details: JsonObject | undefined) {
  if (!details) return null;
  const query = {
    account_id: detailString(details, "account_id", "accountId") || undefined,
    provider_id: detailString(details, "provider_id", "providerId") || undefined,
    model: detailString(details, "model", "canonical_model", "model_alias") || undefined,
    error_class: detailString(details, "error_class", "errorClass") || undefined,
    source_endpoint: detailString(details, "source_endpoint", "sourceEndpoint") || undefined,
    start: detailString(details, "window_start", "windowStart") || undefined,
    end: detailString(details, "window_end", "windowEnd") || undefined,
    limit: 5,
  };
  const hasScope = Boolean(
    query.account_id ||
      query.provider_id ||
      query.model ||
      query.error_class ||
      query.source_endpoint ||
      query.start ||
      query.end,
  );
  return hasScope ? query : null;
}

function compactSignalItems(items: Array<SignalItem | null>): SignalItem[] {
  return items.filter((item): item is SignalItem => Boolean(item));
}

function signalString(label: string, value: string | null): SignalItem | null {
  if (!value) return null;
  return { label, value };
}

function signalNumber(
  label: string,
  key: string,
  details: JsonObject,
  metricType?: string | null,
): SignalItem | null {
  const value = detailNumber(details, key);
  if (value === null) return null;
  return { label, value: formatAlertNumber(key, value, metricType) };
}

function detailString(details: JsonObject | undefined, ...keys: string[]): string | null {
  for (const key of keys) {
    const value = details?.[key];
    if (typeof value === "string" && value.trim()) return value.trim();
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
    if (Array.isArray(value) && value.length > 0) {
      const parts = value
        .map((item) => (typeof item === "string" || typeof item === "number" ? String(item).trim() : ""))
        .filter(Boolean);
      if (parts.length > 0) return parts.join(", ");
    }
  }
  return null;
}

function detailNumber(details: JsonObject | undefined, key: string): number | null {
  const value = details?.[key];
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return null;
}

function formatAlertNumber(key: string, value: number | null, metricType?: string | null): string {
  if (value === null) return "-";
  const normalizedKey = key.toLowerCase();
  const normalizedMetric = (metricType || "").toLowerCase();
  if (normalizedKey.includes("burn_rate")) return `${formatDecimal(value)}x`;
  if (
    normalizedKey.includes("rate") ||
    normalizedKey === "objective" ||
    normalizedKey === "error_budget_consumed" ||
    normalizedMetric === "error_rate" ||
    normalizedMetric === "success_rate"
  ) {
    return formatPercent(value);
  }
  if (normalizedKey.includes("latency") || normalizedMetric === "latency_p95") {
    return formatLatency(value);
  }
  if (normalizedKey.includes("seconds")) {
    return formatDurationSeconds(value);
  }
  if (Number.isInteger(value)) return formatInteger(value);
  return formatDecimal(value);
}

function formatDurationSeconds(value: number): string {
  if (!Number.isFinite(value)) return "-";
  if (value % 86400 === 0) return `${formatInteger(value / 86400)}d`;
  if (value % 3600 === 0) return `${formatInteger(value / 3600)}h`;
  if (value % 60 === 0) return `${formatInteger(value / 60)}m`;
  return `${formatInteger(value)}s`;
}

function formatDecimal(value: number): string {
  return value.toLocaleString(undefined, { maximumFractionDigits: 4 });
}

function AlertEvidenceLinks({ links }: { links: OpsAlertEvidenceLinks }) {
  const { t } = useLanguage();
  const actions = [
    { href: links.errorLogs, label: t("adminOps.evidence.errorLogs") },
    { href: links.requestEvidence, label: t("adminOps.evidence.requestEvidence") },
    { href: links.schedulerDecision, label: t("adminOps.evidence.schedulerDecision") },
    { href: links.accountHealth, label: t("adminOps.evidence.accountHealth") },
  ].filter((item): item is { href: string; label: string } => Boolean(item.href));
  if (actions.length === 0) {
    return <span className="text-[11px] text-srapi-text-tertiary">—</span>;
  }
  return (
    <div className="flex flex-wrap gap-1">
      {actions.map((action) => (
        <Button key={`${action.label}:${action.href}`} asChild variant="ghost" size="sm">
          <Link href={action.href}>{action.label}</Link>
        </Button>
      ))}
    </div>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="w-28 shrink-0 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
        {label}
      </span>
      <span className="break-all text-xs text-srapi-text-secondary">{value}</span>
    </div>
  );
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div>
      <SectionLabel>{label}</SectionLabel>
      <pre className="mt-1.5 max-h-48 overflow-auto rounded-lg bg-srapi-card-muted p-3 font-mono text-[11px] text-srapi-text-secondary">
        {safeJson(value)}
      </pre>
    </div>
  );
}

function SectionLabel({ children }: { children: string }) {
  return <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{children}</span>;
}

/**
 * Inline expansion rendered under an alert row. Surfaces the canonical fields
 * (severity, lifecycle timestamps, scope), the evidence-link strip and the
 * runbook checklist — i.e. everything the dialog has minus the deep JSON dump,
 * so an operator can triage without opening the modal.
 */
function AlertExpandDetail({
  event,
  t,
}: {
  event: OpsAlertEvent;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const ruleName = detailString(event.details, "rule_name", "ruleName") ?? event.rule_id;
  const metricType = detailString(event.details, "metric_type", "metricType");
  const observed = detailNumber(event.details, "observed_value");
  const threshold = detailNumber(event.details, "threshold");
  const operator = detailString(event.details, "operator");
  const runbookSteps = buildOpsAlertRunbookSteps(event.details);
  const evidence = buildOpsAlertEvidenceLinks(event.details);

  const lifecycle: Array<{ label: string; value: ReactNode; mono?: boolean; tone?: "muted" | "success" | "warning" | "error" }> = [
    { label: t("adminOpsAlertEvents.startedAt"), value: formatDateTime(event.started_at) },
    { label: t("adminOpsAlertEvents.updated"), value: formatDateTime(event.updated_at), tone: "muted" },
  ];
  if (event.acknowledged_at)
    lifecycle.push({
      label: t("adminOpsAlertEvents.acknowledgedAt"),
      value: formatDateTime(event.acknowledged_at),
      tone: "muted",
    });
  if (event.resolved_at)
    lifecycle.push({
      label: t("adminOpsAlertEvents.resolvedAt"),
      value: formatDateTime(event.resolved_at),
      tone: "success",
    });
  if (event.suppressed_by)
    lifecycle.push({
      label: t("adminOpsAlertEvents.suppressedBy"),
      value: event.suppressed_by,
      mono: true,
      tone: "muted",
    });

  const trigger: Array<{ label: string; value: ReactNode; mono?: boolean }> = [
    { label: t("adminOpsAlertEvents.ruleName"), value: ruleName, mono: true },
    { label: t("adminOpsAlertEvents.severity"), value: event.severity },
  ];
  if (metricType) trigger.push({ label: t("adminOpsAlertEvents.metric"), value: metricType, mono: true });
  if (metricType && operator && threshold !== null)
    trigger.push({
      label: t("adminOpsAlertEvents.condition"),
      value: `${metricType} ${operator} ${formatAlertNumber("threshold", threshold, metricType)}`,
      mono: true,
    });
  if (observed !== null)
    trigger.push({
      label: t("adminOpsAlertEvents.observed"),
      value: formatAlertNumber("observed_value", observed, metricType),
      mono: true,
    });

  const scope: Array<{ label: string; value: ReactNode; mono?: boolean }> = [];
  const accountId = detailString(event.details, "account_id", "accountId");
  const providerId = detailString(event.details, "provider_id", "providerId");
  const model = detailString(event.details, "model", "canonical_model", "model_alias");
  const sourceEndpoint = detailString(event.details, "source_endpoint", "sourceEndpoint");
  const errorClass = detailString(event.details, "error_class", "errorClass");
  if (accountId) scope.push({ label: t("adminOpsAlertEvents.accountId"), value: accountId, mono: true });
  if (providerId) scope.push({ label: t("adminOpsAlertEvents.providerId"), value: providerId, mono: true });
  if (model) scope.push({ label: t("adminOpsAlertEvents.model"), value: model, mono: true });
  if (sourceEndpoint)
    scope.push({ label: t("adminOpsAlertEvents.sourceEndpoint"), value: sourceEndpoint, mono: true });
  if (errorClass)
    scope.push({ label: t("adminOpsAlertEvents.errorClass"), value: errorClass, mono: true });
  if (scope.length === 0) scope.push({ label: t("adminOpsAlertEvents.fingerprint"), value: event.fingerprint, mono: true });

  return (
    <InlineDetailGrid
      sections={[
        { title: t("adminOpsAlertEvents.signalTrigger"), rows: trigger },
        { title: t("adminCommon.status"), rows: lifecycle },
        { title: t("adminOpsAlertEvents.signalScope"), rows: scope },
      ]}
      actions={
        <div className="flex w-full flex-col gap-3">
          {runbookSteps.length > 0 ? (
            <div>
              <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminOps.runbook.title")}
              </div>
              <OpsAlertRunbookSteps steps={runbookSteps} compact />
            </div>
          ) : null}
          <div className="flex flex-wrap items-center justify-end gap-2">
            <AlertEvidenceLinks links={evidence} />
          </div>
        </div>
      }
    />
  );
}
