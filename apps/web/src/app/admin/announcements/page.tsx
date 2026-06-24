"use client";

import { useState } from "react";
import { Megaphone } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar } from "@/components/admin/list-toolbar";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  ResourceFormDialog,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { AnnouncementReadStatusDialog } from "@/components/admin/announcement-read-status-dialog";
import {
  useAdminAnnouncements,
  useCreateAnnouncement,
  useUpdateAnnouncement,
  useDeleteAnnouncement,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatDateTime } from "@/lib/admin-format";
import {
  ANNOUNCEMENT_STATUSES,
  ANNOUNCEMENT_SEVERITIES,
  ANNOUNCEMENT_AUDIENCES,
  ANNOUNCEMENT_SEGMENT_ROLES,
  emptyAnnouncementForm,
  announcementFormFromAnnouncement,
  buildAnnouncementBody,
  type AnnouncementFormState,
} from "@/lib/admin-announcement-form";
import type { Announcement } from "@/lib/sdk-types";

export default function AdminAnnouncementsPage() {
  return (
    <AdminShell>
      <AnnouncementsContent />
    </AdminShell>
  );
}

function announcementSeverityTone(
  a: Announcement,
): "info" | "success" | "warning" | "error" | "critical" | undefined {
  // Draft/archived announcements should fade; status carries weight over severity.
  if (a.status === "draft") return undefined;
  if (a.status === "archived") return undefined;
  switch (a.severity) {
    case "critical":
      return "critical";
    case "warning":
      return "warning";
    case "info":
      return "info";
    default:
      return undefined;
  }
}

function AnnouncementsContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-announcements", []);
  const statusFilter = (list.filters.status as Announcement["status"]) || undefined;
  const items = useAdminAnnouncements({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
  });
  const createMut = useCreateAnnouncement();
  const updateMut = useUpdateAnnouncement();
  const deleteMut = useDeleteAnnouncement();

  const [formTarget, setFormTarget] = useState<Announcement | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Announcement | null>(null);
  const [readsTarget, setReadsTarget] = useState<Announcement | null>(null);
  const isNew = formTarget === "new";

  const enumOptions = (values: readonly string[]) => values.map((v) => ({ value: v, label: t(`common.${v}`) }));
  const fields: FieldConfig<AnnouncementFormState>[] = [
    { name: "title", label: t("adminAnnouncements.headline") },
    { name: "content", label: t("adminAnnouncements.content"), type: "textarea" },
    {
      name: "status",
      label: t("adminAnnouncements.state"),
      type: "select",
      options: enumOptions(ANNOUNCEMENT_STATUSES),
    },
    {
      name: "severity",
      label: t("adminAnnouncements.severity"),
      type: "select",
      options: enumOptions(ANNOUNCEMENT_SEVERITIES),
    },
    {
      name: "audience",
      label: t("adminAnnouncements.audience"),
      type: "select",
      options: enumOptions(ANNOUNCEMENT_AUDIENCES),
    },
    {
      name: "startsAt",
      label: t("adminAnnouncements.startsAt"),
      type: "datetime",
      hint: t("adminAnnouncements.startsAtHint"),
    },
    {
      name: "endsAt",
      label: t("adminAnnouncements.endsAt"),
      type: "datetime",
      hint: t("adminAnnouncements.endsAtHint"),
    },
    {
      name: "segmentRoles",
      label: t("adminAnnouncements.segmentRoles"),
      type: "multiselect",
      options: enumOptions(ANNOUNCEMENT_SEGMENT_ROLES),
      hint: t("adminAnnouncements.segmentRolesHint"),
      advanced: true,
    },
    {
      name: "segmentEmailDomains",
      label: t("adminAnnouncements.segmentEmailDomains"),
      type: "tags",
      advanced: true,
    },
    {
      name: "segmentUserIds",
      label: t("adminAnnouncements.segmentUserIds"),
      type: "tags",
      advanced: true,
    },
  ];

  const columns: Column<Announcement>[] = [
    {
      key: "title",
      header: t("adminAnnouncements.headline"),
      pinned: true,
      sortValue: (a) => a.title,
      render: (a) => (
        <div className="min-w-0">
          <div className="truncate text-srapi-text-primary">{a.title}</div>
          {a.audience ? (
            <div className="truncate text-[11px] text-srapi-text-tertiary">
              {a.audience} · {a.severity}
            </div>
          ) : null}
        </div>
      ),
    },
    {
      key: "published",
      header: t("adminAnnouncements.published"),
      hideOnMobile: true,
      sortValue: (a) => a.starts_at ?? a.created_at,
      render: (a) => (
        <DataTooltip
          title={t("adminAnnouncements.published")}
          primary={formatDateTime(a.starts_at ?? a.created_at)}
          rows={[
            { label: t("adminCommon.created"), value: formatDateTime(a.created_at), tone: "muted" },
            ...(a.starts_at
              ? [{ label: t("adminAnnouncements.startsAt"), value: formatDateTime(a.starts_at) }]
              : []),
            ...(a.ends_at
              ? [{ label: t("adminAnnouncements.endsAt"), value: formatDateTime(a.ends_at), tone: "muted" as const }]
              : []),
          ]}
        >
          <span className="text-[12px] tabular text-srapi-text-tertiary">
            {formatDateTime(a.starts_at ?? a.created_at)}
          </span>
        </DataTooltip>
      ),
    },
    {
      key: "status",
      header: t("adminAnnouncements.state"),
      sortValue: (a) => a.status,
      render: (a) => (
        <div className="flex flex-wrap items-center gap-1.5">
          <QuietBadge status={quietStatusFor(a.status)} label={statusLabel(t, a.status)} />
          <QuietBadge status={quietStatusFor(a.severity)} label={a.severity} />
        </div>
      ),
    },
  ];

  return (
    <>
      <SectionHero
        eyebrow="Ops · Announcements"
        title={t("adminAnnouncements.title")}
        description={t("adminAnnouncements.subtitle")}
        metrics={(() => {
          const rows = items.data?.data ?? [];
          const published = rows.filter((a) => a.status === "published").length;
          return [{ label: "已发布", value: String(published) }];
        })()}
        actions={
          <div className="flex items-center gap-3">
            {items.data ? (
              <ListCount total={items.data.pagination?.total ?? items.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminAnnouncements.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={items}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(a) => a.id}
        emptyIcon={Megaphone}
        emptyTitle={t("adminAnnouncements.emptyTitle")}
        emptyBody={t("adminAnnouncements.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
            ＋ {t("adminAnnouncements.create")}
          </Button>
        }
        minWidth={480}
        isFiltered={Boolean(statusFilter)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        rowSeverity={(a) => announcementSeverityTone(a)}
        dimRow={(a) => a.status === "draft" || a.status === "archived"}
        expandRow={(a) => <AnnouncementExpandDetail announcement={a} t={t} />}
        toolbar={
          <ListToolbar>
            <SegmentedControl<string>
              value={(statusFilter as string) ?? "all"}
              onChange={(v) => list.setFilter("status", v === "all" ? "" : v)}
              ariaLabel={t("adminCommon.allStatuses")}
              size="sm"
              options={[
                { value: "all", label: t("adminCommon.allStatuses") },
                ...ANNOUNCEMENT_STATUSES.map((v) => ({ value: v, label: v })),
              ]}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: items.data?.pagination?.total ?? items.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(a) => (
          <RowActionsMenu
            actions={[
              { label: t("adminAnnouncements.readStatus"), onSelect: () => setReadsTarget(a) },
              { label: t("common.edit"), onSelect: () => setFormTarget(a) },
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(a) },
            ]}
          />
        )}
      />

      {formTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setFormTarget(null);
          }}
          title={isNew ? t("adminAnnouncements.create") : t("adminAnnouncements.edit")}
          fields={fields}
          initial={isNew ? emptyAnnouncementForm() : announcementFormFromAnnouncement(formTarget)}
          buildBody={buildAnnouncementBody}
          submit={
            isNew
              ? (body) => createMut.mutateAsync(body)
              : (body) => updateMut.mutateAsync({ id: formTarget.id, body })
          }
          successMessage={isNew ? t("feedback.created") : t("feedback.updated")}
          isPending={createMut.isPending || updateMut.isPending}
        />
      ) : null}

      {deleteTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("feedback.confirmDeleteTitle", { name: deleteTarget.title })}
          body={t("feedback.confirmDeleteBody")}
          confirmLabel={t("common.delete")}
          confirmPhrase={deleteTarget.title}
          onConfirm={() => deleteMut.mutateAsync(deleteTarget.id)}
          successMessage={t("feedback.deleted")}
          isPending={deleteMut.isPending}
        />
      ) : null}

      <AnnouncementReadStatusDialog
        announcementId={readsTarget?.id ?? null}
        title={readsTarget?.title ?? ""}
        open={readsTarget !== null}
        onOpenChange={(open) => {
          if (!open) setReadsTarget(null);
        }}
      />
    </>
  );
}

/**
 * Inline announcement expansion — a body preview (no truncation), the lifecycle
 * window, and audience targeting. Read-rate stats live behind the «Read status»
 * action menu because they hit a separate endpoint; we surface a hint here
 * pointing operators to that dialog rather than blocking row hover on a fetch.
 */
function AnnouncementExpandDetail({
  announcement,
  t,
}: {
  announcement: Announcement;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const lifecycle: Array<{ label: string; value: string; mono?: boolean; tone?: "default" | "muted" | "success" }> = [
    { label: t("adminCommon.created"), value: formatDateTime(announcement.created_at), tone: "muted" },
    { label: t("adminCommon.updated"), value: formatDateTime(announcement.updated_at), tone: "muted" },
  ];
  if (announcement.starts_at)
    lifecycle.push({
      label: t("adminAnnouncements.startsAt"),
      value: formatDateTime(announcement.starts_at),
      tone: "success",
    });
  if (announcement.ends_at)
    lifecycle.push({
      label: t("adminAnnouncements.endsAt"),
      value: formatDateTime(announcement.ends_at),
    });

  const audience: Array<{ label: string; value: string; mono?: boolean; tone?: "default" | "muted" }> = [
    { label: t("adminAnnouncements.audience"), value: announcement.audience, mono: true },
    { label: t("adminAnnouncements.severity"), value: announcement.severity, mono: true },
    { label: t("adminAnnouncements.state"), value: statusLabel(t, announcement.status), mono: true },
  ];
  const segments = announcement.segments ?? [];
  for (const seg of segments) {
    if (seg.roles?.length)
      audience.push({
        label: t("adminAnnouncements.segmentRoles"),
        value: seg.roles.join(", "),
        mono: true,
        tone: "muted",
      });
    if (seg.email_domains?.length)
      audience.push({
        label: t("adminAnnouncements.segmentEmailDomains"),
        value: seg.email_domains.join(", "),
        mono: true,
        tone: "muted",
      });
    if (seg.user_ids?.length)
      audience.push({
        label: t("adminAnnouncements.segmentUserIds"),
        value: seg.user_ids.map(String).join(", "),
        mono: true,
        tone: "muted",
      });
  }

  const body = announcement.content?.trim() || "";
  // Markdown preview is intentionally text-only — operators read what users will
  // see in the in-product modal, no need to evaluate markdown here.
  const preview = body.length > 800 ? body.slice(0, 800) + "…" : body;

  return (
    <div>
      <InlineDetailGrid
        sections={[
          { title: t("adminAnnouncements.audience"), rows: audience },
          { title: t("adminAnnouncements.published"), rows: lifecycle },
        ]}
      />
      {preview ? (
        <div className="border-t border-srapi-border/60 bg-srapi-card-muted/30 px-6 py-4">
          <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminAnnouncements.content")}
          </div>
          <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-srapi-border bg-srapi-card-muted/60 p-3 text-[12px] text-srapi-text-secondary">
            {preview}
          </pre>
        </div>
      ) : null}
    </div>
  );
}
