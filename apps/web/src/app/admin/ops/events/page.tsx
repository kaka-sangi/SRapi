"use client";

import { useState } from "react";
import { Webhook } from "lucide-react";
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
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useOutboxEvents } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, safeJson } from "@/lib/admin-format";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import type { DomainEventOutbox } from "@/lib/sdk-types";

const OUTBOX_STATUSES = ["pending", "published", "failed"];

export default function AdminOutboxPage() {
  return (
    <AdminShell>
      <OutboxContent />
    </AdminShell>
  );
}

/**
 * Domain-event outbox viewer — the transactional outbox that feeds webhook /
 * event delivery. Surfaces pending vs published vs failed entries (with attempt
 * counts, next-retry and last-error) so admins can see and debug delivery,
 * including the dead-letter tail. Read-only: the backend exposes no UI mutations.
 */
function OutboxContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-ops-events", []);
  const statusFilter = (list.filters.status as DomainEventOutbox["status"]) || undefined;
  const events = useOutboxEvents({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
  });
  const [detail, setDetail] = useState<DomainEventOutbox | null>(null);

  const columns: Column<DomainEventOutbox>[] = [
    {
      key: "time",
      header: t("adminOutbox.time"),
      pinned: true,
      render: (e) => (
        <span className="whitespace-nowrap text-[12px] tabular text-srapi-text-tertiary">
          {formatDateTime(e.created_at)}
        </span>
      ),
    },
    {
      key: "event",
      header: t("adminOutbox.event"),
      render: (e) => (
        <div className="min-w-0">
          <div className="truncate text-xs text-srapi-text-primary">{e.event_type}</div>
          <div className="truncate text-[11px] text-srapi-text-tertiary">
            {e.aggregate_type}
            {e.aggregate_id ? ` #${e.aggregate_id}` : ""}
          </div>
        </div>
      ),
    },
    {
      key: "producer",
      header: t("adminOutbox.producer"),
      hideOnMobile: true,
      render: (e) => (
        <span className="text-[12px] text-srapi-text-secondary">{e.producer_module}</span>
      ),
    },
    {
      key: "attempts",
      header: t("adminOutbox.attempts"),
      align: "right",
      hideOnMobile: true,
      render: (e) => (
        <span className="text-[12px] tabular text-srapi-text-tertiary">{e.attempt_count}</span>
      ),
    },
    {
      key: "diagnostic",
      header: t("adminOutbox.diagnostic"),
      hideOnMobile: true,
      className: "max-w-md",
      render: (e) => <OutboxDiagnosticSummary event={e} />,
    },
    {
      key: "status",
      header: t("adminCommon.status"),
      sortValue: (e) => e.status,
      render: (e) => <QuietBadge status={quietStatusFor(e.status)} label={statusLabel(t, e.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminOutbox.title")}
        description={t("adminOutbox.subtitle")}
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
        getRowId={(e) => e.id}
        emptyIcon={Webhook}
        emptyTitle={t("adminOutbox.emptyTitle")}
        emptyBody={t("adminOutbox.emptyBody")}
        minWidth={720}
        isFiltered={Boolean(statusFilter)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        dimRow={(e) => e.status === "published"}
        toolbar={
          <ListToolbar>
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={enumOptions(OUTBOX_STATUSES)}
              allLabel={t("adminCommon.allStatuses")}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: events.data?.pagination?.total ?? events.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(e) => (
          <RowActionsMenu
            actions={[{ label: t("adminOutbox.viewDetails"), onSelect: () => setDetail(e) }]}
          />
        )}
      />

      {detail ? (
        <Dialog open onOpenChange={(open) => !open && setDetail(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("adminOutbox.detailTitle")}</DialogTitle>
              <DialogDescription>
                {detail.event_type} · {detail.event_id}
              </DialogDescription>
            </DialogHeader>
            <div className="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pr-1">
              <Meta label={t("adminCommon.status")} value={statusLabel(t, detail.status)} />
              <Meta label={t("adminOutbox.producer")} value={detail.producer_module} />
              <Meta
                label={t("adminOutbox.aggregate")}
                value={`${detail.aggregate_type}${detail.aggregate_id ? ` #${detail.aggregate_id}` : ""}`}
              />
              <Meta label={t("adminOutbox.attempts")} value={String(detail.attempt_count)} />
              <Meta label={t("adminOutbox.correlation")} value={detail.correlation_id || "—"} />
              <Meta label={t("adminOutbox.idempotency")} value={detail.idempotency_key || "—"} />
              {detail.next_retry_at ? (
                <Meta label={t("adminOutbox.nextRetry")} value={formatDateTime(detail.next_retry_at)} />
              ) : null}
              {detail.published_at ? (
                <Meta label={t("adminOutbox.publishedAt")} value={formatDateTime(detail.published_at)} />
              ) : null}
              {detail.last_error ? (
                <Meta label={t("adminOutbox.lastError")} value={detail.last_error} />
              ) : null}
              <JsonBlock label={t("adminOutbox.payload")} value={detail.payload} />
              <JsonBlock label={t("adminCommon.metadata")} value={detail.metadata} />
            </div>
          </DialogContent>
        </Dialog>
      ) : null}
    </>
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

function OutboxDiagnosticSummary({ event }: { event: DomainEventOutbox }) {
  const { t } = useLanguage();
  const value =
    event.status === "failed"
      ? event.last_error || "—"
      : event.status === "pending"
        ? event.next_retry_at
          ? `${t("adminOutbox.nextRetry")} ${formatDateTime(event.next_retry_at)}`
          : "—"
        : event.published_at
          ? `${t("adminOutbox.publishedAt")} ${formatDateTime(event.published_at)}`
          : "—";

  return (
    <span className="block truncate text-[12px] text-srapi-text-secondary" title={value}>
      {value}
    </span>
  );
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  return (
    <div>
      <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{label}</span>
      <pre className="mt-1.5 max-h-48 overflow-auto rounded-lg bg-srapi-card-muted p-3 font-mono text-[11px] text-srapi-text-secondary">
        {safeJson(value)}
      </pre>
    </div>
  );
}
