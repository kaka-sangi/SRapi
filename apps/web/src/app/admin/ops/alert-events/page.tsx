"use client";

import { useState } from "react";
import Link from "next/link";
import { BellRing } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { enumOptions } from "@/components/admin/resource-form-dialog";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useOpsAlertEvents } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { buildOpsAlertEvidenceLinks, type OpsAlertEvidenceLinks } from "@/lib/admin-ops-alert-evidence";
import { formatDateTime, safeJson } from "@/lib/admin-format";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import type { OpsAlertEvent, OpsAlertSeverity, OpsAlertStatus } from "@/lib/sdk-types";

const ALERT_STATUSES: OpsAlertStatus[] = ["firing", "acknowledged", "resolved", "suppressed"];
const ALERT_SEVERITIES: OpsAlertSeverity[] = ["critical", "warning", "ticket"];

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
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
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
          <div className="truncate font-mono text-2xs text-srapi-text-tertiary">
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
          <QuietBadge status={quietStatusFor(event.status)} label={statusLabel(t, event.status)} />
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
      key: "updated",
      header: t("adminOpsAlertEvents.updated"),
      hideOnMobile: true,
      sortValue: (event) => event.updated_at,
      render: (event) => (
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(event.updated_at)}
        </span>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdminOps")}
        title={t("adminOpsAlertEvents.title")}
        description={t("adminOpsAlertEvents.subtitle")}
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
        minWidth={820}
        isFiltered={Boolean(statusFilter || severityFilter)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        dimRow={(event) => event.status === "resolved" || event.status === "suppressed"}
        toolbar={
          <ListToolbar>
            <FilterSelect
              value={statusFilter}
              onChange={(value) => list.setFilter("status", value)}
              options={enumOptions(ALERT_STATUSES)}
              allLabel={t("adminOpsAlertEvents.allStatuses")}
            />
            <FilterSelect
              value={severityFilter}
              onChange={(value) => list.setFilter("severity", value)}
              options={enumOptions(ALERT_SEVERITIES)}
              allLabel={t("adminOpsAlertEvents.allSeverities")}
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
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("adminOpsAlertEvents.detailTitle")}</DialogTitle>
              <DialogDescription>
                {detail.rule_id} · {detail.fingerprint}
              </DialogDescription>
            </DialogHeader>
            <div className="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pr-1">
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
              <div>
                <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                  {t("adminOpsAlertEvents.evidence")}
                </span>
                <div className="mt-2">
                  <AlertEvidenceLinks links={buildOpsAlertEvidenceLinks(detail.details)} />
                </div>
              </div>
              <JsonBlock label={t("adminOpsAlertEvents.details")} value={detail.details} />
            </div>
          </DialogContent>
        </Dialog>
      ) : null}
    </>
  );
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
    return <span className="font-mono text-2xs text-srapi-text-tertiary">—</span>;
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
      <span className="w-28 shrink-0 font-mono text-2xs uppercase text-srapi-text-tertiary">
        {label}
      </span>
      <span className="break-all font-mono text-xs text-srapi-text-secondary">{value}</span>
    </div>
  );
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div>
      <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">{label}</span>
      <pre className="mt-1.5 max-h-48 overflow-auto rounded-lg bg-srapi-card-muted p-3 font-mono text-2xs text-srapi-text-secondary">
        {safeJson(value)}
      </pre>
    </div>
  );
}
