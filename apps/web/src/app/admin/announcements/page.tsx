"use client";

import { useState } from "react";
import { Megaphone } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import {
  ResourceFormDialog,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
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

function AnnouncementsContent() {
  const { t } = useLanguage();
  const list = useAdminList();
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
  const isNew = formTarget === "new";

  const enumOptions = (values: readonly string[]) => values.map((v) => ({ value: v, label: v }));
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
  ];

  const columns: Column<Announcement>[] = [
    {
      key: "title",
      header: t("adminAnnouncements.headline"),
      sortValue: (a) => a.title,
      render: (a) => <span className="text-srapi-text-primary">{a.title}</span>,
    },
    {
      key: "published",
      header: t("adminAnnouncements.published"),
      hideOnMobile: true,
      render: (a) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(a.starts_at ?? a.created_at)}
        </span>
      ),
    },
    {
      key: "status",
      header: t("adminAnnouncements.state"),
      render: (a) => <QuietBadge status={quietStatusFor(a.status)} label={statusLabel(t, a.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminAnnouncements.title")}
        description={t("adminAnnouncements.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {items.data ? (
              <ListCount total={items.data.pagination?.total ?? items.data.data.length} />
            ) : null}
            <Button variant="primary" size="sm" onClick={() => setFormTarget("new")}>
              ＋ {t("adminAnnouncements.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={items}
        columns={columns}
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
        toolbar={
          <ListToolbar>
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={ANNOUNCEMENT_STATUSES.map((v) => ({ value: v, label: v }))}
              allLabel={t("adminCommon.allStatuses")}
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
    </>
  );
}
