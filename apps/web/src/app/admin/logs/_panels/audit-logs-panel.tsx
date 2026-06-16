"use client";

import { useMemo, useState } from "react";
import { ScrollText } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useClientPagedList } from "@/hooks/use-client-list";
import { useAuditLogs, useAdminUsers } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, safeJson } from "@/lib/admin-format";
import type { AuditLog } from "@/lib/sdk-types";

// Preset values are minutes-back-from-now. "all" (or empty) means no bound;
// the auditMatch check below skips the comparison when the preset isn't set.
const WINDOW_PRESETS: { value: string; labelKey: string; minutes: number }[] = [
  { value: "24h", labelKey: "adminAudit.window24h", minutes: 24 * 60 },
  { value: "7d", labelKey: "adminAudit.window7d", minutes: 7 * 24 * 60 },
  { value: "30d", labelKey: "adminAudit.window30d", minutes: 30 * 24 * 60 },
];

function windowSinceFromFilter(value: string | undefined): Date | null {
  if (!value) return null;
  const preset = WINDOW_PRESETS.find((p) => p.value === value);
  if (!preset) return null;
  return new Date(Date.now() - preset.minutes * 60 * 1000);
}

function auditMatch(row: AuditLog, term: string, filters: Record<string, string>): boolean {
  if (filters.action && row.action !== filters.action) return false;
  if (filters.resource_type && row.resource_type !== filters.resource_type) return false;
  if (filters.actor_user_id && String(row.actor_user_id ?? "") !== filters.actor_user_id) return false;
  if (filters.window) {
    const since = windowSinceFromFilter(filters.window);
    if (since && row.created_at && new Date(row.created_at) < since) return false;
  }
  if (!term) return true;
  return [row.actor_user_id, row.resource_id, row.resource_type, row.action, row.ip, row.trace_id]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

const auditCompare = (a: AuditLog, b: AuditLog) => (b.created_at ?? "").localeCompare(a.created_at ?? "");

function distinct(values: Array<string | null | undefined>): string[] {
  return [...new Set(values.filter((v): v is string => Boolean(v)))].sort();
}

export function AuditLogsPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-audit-logs", []);
  const all = useAuditLogs();
  const { query, total } = useClientPagedList(all, list, {
    match: auditMatch,
    compare: auditCompare,
  });
  const [detail, setDetail] = useState<AuditLog | null>(null);

  const rows = useMemo(() => all.data?.data ?? [], [all.data]);
  const actionOptions = useMemo(() => distinct(rows.map((r) => r.action)), [rows]);
  const resourceOptions = useMemo(() => distinct(rows.map((r) => r.resource_type)), [rows]);
  // Reuse the iter-15/error-logs pattern: 200-user lookup for actor labels
  // gives us readable {email} options in the dropdown. Larger installs whose
  // actors are past row 200 still see the actor_user_id in the table.
  const users = useAdminUsers({ page: 1, page_size: 200 });
  const actorOptions = useMemo(
    () =>
      (users.data?.data ?? []).map((u) => ({
        value: String(u.id),
        label: u.email,
      })),
    [users.data],
  );
  const isFiltered = Boolean(
    list.search ||
      list.filters.action ||
      list.filters.resource_type ||
      list.filters.actor_user_id ||
      list.filters.window,
  );

  const columns: Column<AuditLog>[] = [
    {
      key: "time",
      header: t("adminAudit.time"),
      pinned: true,
      render: (a) => (
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(a.created_at)}
        </span>
      ),
    },
    {
      key: "actor",
      header: t("adminAudit.actor"),
      hideOnMobile: true,
      render: (a) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{a.actor_user_id || "—"}</span>
      ),
    },
    {
      key: "action",
      header: t("adminAudit.action"),
      render: (a) => <span className="font-mono text-xs text-srapi-text-primary">{a.action}</span>,
    },
    {
      key: "resource",
      header: t("adminAudit.resource"),
      render: (a) => (
        <span className="text-srapi-text-secondary">
          {a.resource_type}
          {a.resource_id ? (
            <span className="ml-1 font-mono text-2xs text-srapi-text-tertiary">#{a.resource_id}</span>
          ) : null}
        </span>
      ),
    },
    {
      key: "ip",
      header: t("adminAudit.ip"),
      hideOnMobile: true,
      render: (a) => <span className="font-mono text-2xs text-srapi-text-tertiary">{a.ip || "—"}</span>,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminAudit.title")}
        description={t("adminAudit.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
          </div>
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(a) => a.id}
        emptyIcon={ScrollText}
        emptyTitle={t("adminAudit.emptyTitle")}
        emptyBody={t("adminAudit.emptyBody")}
        minWidth={760}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminAudit.searchPlaceholder")}
            />
            <FilterSelect
              value={list.filters.action}
              onChange={(v) => list.setFilter("action", v)}
              options={actionOptions.map((v) => ({ value: v, label: v }))}
              allLabel={t("adminAudit.allActions")}
            />
            <FilterSelect
              value={list.filters.resource_type}
              onChange={(v) => list.setFilter("resource_type", v)}
              options={resourceOptions.map((v) => ({ value: v, label: v }))}
              allLabel={t("adminAudit.allResources")}
            />
            <FilterSelect
              value={list.filters.actor_user_id}
              onChange={(v) => list.setFilter("actor_user_id", v)}
              options={actorOptions}
              allLabel={t("adminAudit.allActors")}
            />
            <FilterSelect
              value={list.filters.window}
              onChange={(v) => list.setFilter("window", v)}
              options={WINDOW_PRESETS.map((p) => ({ value: p.value, label: t(p.labelKey) }))}
              allLabel={t("adminAudit.allTime")}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(a) => (
          <RowActionsMenu
            actions={[{ label: t("adminAudit.viewDetails"), onSelect: () => setDetail(a) }]}
          />
        )}
      />

      {detail ? (
        <Dialog open onOpenChange={(open) => !open && setDetail(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("adminAudit.detailTitle")}</DialogTitle>
              <DialogDescription>
                {detail.action} · {detail.resource_type}
                {detail.resource_id ? ` #${detail.resource_id}` : ""}
              </DialogDescription>
            </DialogHeader>
            <div className="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pr-1">
              <AuditMeta label={t("adminAudit.actor")} value={detail.actor_user_id || "—"} />
              <AuditMeta label={t("adminAudit.ip")} value={detail.ip || "—"} />
              <AuditMeta label={t("adminAudit.trace")} value={detail.trace_id || "—"} />
              <AuditMeta label={t("adminAudit.userAgent")} value={detail.user_agent || "—"} />
              <JsonBlock label={t("adminAudit.before")} value={detail.before} />
              <JsonBlock label={t("adminAudit.after")} value={detail.after} />
            </div>
          </DialogContent>
        </Dialog>
      ) : null}
    </>
  );
}

function AuditMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="w-24 shrink-0 font-mono text-2xs uppercase text-srapi-text-tertiary">
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
