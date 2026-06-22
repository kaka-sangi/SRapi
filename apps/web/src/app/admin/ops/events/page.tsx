"use client";

import { useState } from "react";
import { Webhook } from "lucide-react";
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
import { SegmentedControl } from "@/components/ui/segmented-control";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useOutboxEvents } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, safeJson } from "@/lib/admin-format";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import type { DomainEventOutbox } from "@/lib/sdk-types";

const OUTBOX_STATUSES = ["pending", "published", "failed"];

function outboxSeverity(
  status: DomainEventOutbox["status"],
  attemptCount: number,
): "info" | "success" | "warning" | "error" | "critical" | undefined {
  switch (status) {
    case "published":
      return "success";
    case "pending":
      // Repeatedly-deferred deliveries deserve a warning stripe.
      return attemptCount > 1 ? "warning" : "info";
    case "failed":
      // Dead-letter rows (high attempt count) push to critical.
      return attemptCount >= 5 ? "critical" : "error";
    default:
      return undefined;
  }
}

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
      sortValue: (e) => e.attempt_count,
      render: (e) => (
        <DataTooltip
          title={t("adminOutbox.attempts")}
          primary={String(e.attempt_count)}
          rows={[
            { label: t("adminOutbox.producer"), value: e.producer_module, tone: "muted" },
            ...(e.next_retry_at
              ? [
                  {
                    label: t("adminOutbox.nextRetry"),
                    value: formatDateTime(e.next_retry_at),
                    tone: "warning" as const,
                  },
                ]
              : []),
            ...(e.published_at
              ? [
                  {
                    label: t("adminOutbox.publishedAt"),
                    value: formatDateTime(e.published_at),
                    tone: "success" as const,
                  },
                ]
              : []),
            ...(e.last_error
              ? [{ label: t("adminOutbox.lastError"), value: e.last_error, tone: "error" as const }]
              : []),
          ]}
          footer={e.idempotency_key || e.event_id}
        >
          <span
            className={
              e.attempt_count > 3
                ? "metric-strong-warn text-[12px] tabular"
                : "text-[12px] tabular text-srapi-text-tertiary"
            }
          >
            {e.attempt_count}
          </span>
        </DataTooltip>
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
      <SectionHero
        eyebrow="Ops · Events"
        title={t("adminOutbox.title")}
        description={t("adminOutbox.subtitle")}
        metrics={(() => {
          const rows = events.data?.data ?? [];
          const failed = rows.filter((e) => e.status === "failed").length;
          const pending = rows.filter((e) => e.status === "pending").length;
          return [
            { label: "失败", value: String(failed), tone: failed > 0 ? "error" : "default" },
            { label: "待发布", value: String(pending), tone: pending > 0 ? "warning" : "default" },
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
        rowSeverity={(e) => outboxSeverity(e.status, e.attempt_count)}
        expandRow={(e) => <OutboxExpandDetail event={e} t={t} />}
        toolbar={
          <ListToolbar>
            <SegmentedControl<string>
              value={(statusFilter as string) ?? "all"}
              onChange={(v) => list.setFilter("status", v === "all" ? "" : v)}
              ariaLabel={t("adminCommon.allStatuses")}
              size="sm"
              options={[
                { value: "all", label: t("adminCommon.allStatuses") },
                ...OUTBOX_STATUSES.map((v) => ({ value: v, label: v })),
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

/**
 * Inline outbox expansion: routing/payload preview + retry trail. Inline beats
 * the modal — operators triaging a delivery storm scrub rows quickly without
 * losing scroll position, then promote to the dialog only for the full dump.
 */
function OutboxExpandDetail({
  event,
  t,
}: {
  event: DomainEventOutbox;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const routing = [
    { label: t("adminOutbox.event"), value: event.event_type, mono: true },
    { label: "event_id", value: event.event_id, mono: true },
    {
      label: t("adminOutbox.aggregate"),
      value: `${event.aggregate_type}${event.aggregate_id ? ` #${event.aggregate_id}` : ""}`,
      mono: true,
    },
    { label: t("adminOutbox.producer"), value: event.producer_module, mono: true },
    {
      label: t("adminOutbox.correlation"),
      value: event.correlation_id || "—",
      mono: true,
      tone: (event.correlation_id ? "default" : "muted") as "default" | "muted",
    },
    {
      label: t("adminOutbox.idempotency"),
      value: event.idempotency_key || "—",
      mono: true,
      tone: (event.idempotency_key ? "default" : "muted") as "default" | "muted",
    },
  ];

  const retry: Array<{
    label: string;
    value: string;
    mono?: boolean;
    tone?: "default" | "muted" | "success" | "warning" | "error";
  }> = [
    { label: t("adminOutbox.attempts"), value: String(event.attempt_count) },
    {
      label: t("adminCommon.status"),
      value: statusLabel(t, event.status),
      tone:
        event.status === "failed"
          ? "error"
          : event.status === "pending"
            ? "warning"
            : "success",
    },
    { label: t("adminCommon.created"), value: formatDateTime(event.created_at), tone: "muted" },
  ];
  if (event.next_retry_at)
    retry.push({
      label: t("adminOutbox.nextRetry"),
      value: formatDateTime(event.next_retry_at),
      tone: "warning",
    });
  if (event.published_at)
    retry.push({
      label: t("adminOutbox.publishedAt"),
      value: formatDateTime(event.published_at),
      tone: "success",
    });
  if (event.last_error)
    retry.push({
      label: t("adminOutbox.lastError"),
      value: event.last_error,
      tone: "error",
    });

  const payloadPreview = safeJson(event.payload);
  const truncated = payloadPreview.length > 480 ? payloadPreview.slice(0, 480) + "…" : payloadPreview;

  return (
    <div>
      <InlineDetailGrid
        sections={[
          { title: t("adminOutbox.event"), rows: routing },
          { title: t("adminOutbox.attempts"), rows: retry },
        ]}
      />
      <div className="border-t border-srapi-border/60 bg-srapi-card-muted/30 px-6 py-4">
        <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
          {t("adminOutbox.payload")}
        </div>
        <pre className="max-h-40 overflow-auto rounded-lg border border-srapi-border bg-srapi-card-muted/60 p-3 font-mono text-[11px] text-srapi-text-secondary">
          {truncated}
        </pre>
      </div>
    </div>
  );
}
