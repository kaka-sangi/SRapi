"use client";

import { useCallback, useMemo, useState } from "react";
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
import { CopyButton } from "@/components/ui/copy-button";
import { DensityToggle, type DensityValue } from "@/components/ui/density-toggle";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import { useClientPagedList } from "@/hooks/use-client-list";
import { useAuditLogs } from "@/hooks/admin-queries";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, safeJson } from "@/lib/admin-format";
import type { AuditLog } from "@/lib/sdk-types";
import {
  LOG_WINDOW_PRESETS,
  LOG_WINDOW_ALL_LABEL_KEY,
  logWindowSince,
} from "@/lib/log-window-filter";

const auditCompare = (a: AuditLog, b: AuditLog) => (b.created_at ?? "").localeCompare(a.created_at ?? "");

function distinct(values: Array<string | null | undefined>): string[] {
  return [...new Set(values.filter((v): v is string => Boolean(v)))].sort();
}

// Audit actions split into severity buckets for the toolbar chip strip. Destructive
// verbs ("delete", "destroy", "purge", "revoke", "ban", "remove", "force") read as
// «warning»; everything else stays neutral «info». Used by both the rowSeverity
// stripe and the SegmentedControl.
function auditSeverity(action?: string | null): "info" | "warning" {
  if (!action) return "info";
  const a = action.toLowerCase();
  if (/delete|destroy|purge|revoke|ban|remove|force|reset/.test(a)) return "warning";
  return "info";
}

export function AuditLogsPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-audit-logs", []);
  const all = useAuditLogs();
  const [detail, setDetail] = useState<AuditLog | null>(null);
  const [density, setDensity] = useState<DensityValue>("regular");
  const userLookup = useUserEmailLookup();
  // Closure variant of auditMatch — same upgrade iter-78/79 applied to
  // /admin/orders and billing-ledger. Operators search by email when
  // investigating "who did what".
  const match = useCallback(
    (row: AuditLog, term: string, filters: Record<string, string>): boolean => {
      if (filters.action && row.action !== filters.action) return false;
      if (filters.resource_type && row.resource_type !== filters.resource_type) return false;
      if (filters.actor_user_id && String(row.actor_user_id ?? "") !== filters.actor_user_id) return false;
      if (filters.severity && auditSeverity(row.action) !== filters.severity) return false;
      if (filters.window) {
        const since = logWindowSince(filters.window);
        if (since && row.created_at && new Date(row.created_at) < since) return false;
      }
      if (!term) return true;
      const email = userLookup.map.get(String(row.actor_user_id)) ?? "";
      return [row.actor_user_id, email, row.resource_id, row.resource_type, row.action, row.ip, row.trace_id]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(term);
    },
    [userLookup.map],
  );
  const { query, total } = useClientPagedList(all, list, {
    match,
    compare: auditCompare,
  });

  const rows = useMemo(() => all.data?.data ?? [], [all.data]);
  const actionOptions = useMemo(() => distinct(rows.map((r) => r.action)), [rows]);
  const resourceOptions = useMemo(() => distinct(rows.map((r) => r.resource_type)), [rows]);
  // Reuse the iter-15/error-logs pattern: 200-user lookup for actor labels
  // gives us readable {email} options in the dropdown. Larger installs whose
  // actors are past row 200 still see the actor_user_id in the table.
  const actorOptions = useMemo(
    () =>
      (userLookup.query.data?.data ?? []).map((u) => ({
        value: String(u.id),
        label: u.email,
      })),
    [userLookup.query.data],
  );
  const severityFilter = list.filters.severity;
  const isFiltered = Boolean(
    list.search ||
      list.filters.action ||
      list.filters.resource_type ||
      list.filters.actor_user_id ||
      list.filters.window ||
      severityFilter,
  );

  const columns: Column<AuditLog>[] = [
    {
      key: "time",
      header: t("adminAudit.time"),
      pinned: true,
      render: (a) => (
        <span className="whitespace-nowrap text-[12px] tabular text-srapi-text-tertiary">
          {formatDateTime(a.created_at)}
        </span>
      ),
    },
    {
      key: "actor",
      header: t("adminAudit.actor"),
      hideOnMobile: true,
      render: (a) => (
        <span className="text-srapi-text-secondary">{userLookup.get(a.actor_user_id)}</span>
      ),
    },
    {
      key: "action",
      header: t("adminAudit.action"),
      render: (a) => <span className="text-xs text-srapi-text-primary">{a.action}</span>,
    },
    {
      key: "resource",
      header: t("adminAudit.resource"),
      render: (a) => (
        <span className="text-srapi-text-secondary">
          {a.resource_type}
          {a.resource_id ? (
            <span className="ml-1 text-[11px] text-srapi-text-tertiary">#{a.resource_id}</span>
          ) : null}
        </span>
      ),
    },
    {
      key: "ip",
      header: t("adminAudit.ip"),
      hideOnMobile: true,
      render: (a) => <span className="text-[12px] text-srapi-text-tertiary">{a.ip || "—"}</span>,
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
            <DensityToggle value={density} onChange={setDensity} />
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
        density={density}
        enableKeyboardNav
        // Destructive actions (delete/revoke/reset) get a warning stripe; other
        // ops stay neutral info. The 2.5px left rule lets operators skim down a
        // page of audit entries and pick out the «scary» rows instantly.
        rowSeverity={(a) => auditSeverity(a.action)}
        // Inline detail: actor/resource/network on the left, before/after JSON on
        // the right. The dialog still exists for deep-link, but click-to-expand
        // covers 90% of the "what changed" need without losing list context.
        expandRow={(a) => <AuditExpandedDetail entry={a} email={userLookup.get(a.actor_user_id)} />}
        toolbar={
          <>
            {/* Severity chip strip — chooses the 2.5px row stripe at-a-glance.
                The 「Action」 FilterSelect remains below for verb-level pinpointing. */}
            <div className="flex items-center gap-3 border-b border-srapi-border/60 bg-srapi-card-muted/40 px-4 py-2">
              <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                Severity
              </span>
              <SegmentedControl
                value={severityFilter === "warning" ? "warning" : severityFilter === "info" ? "info" : "all"}
                onChange={(v) => list.setFilter("severity", v === "all" ? undefined : v)}
                options={[
                  { value: "all", label: "All" },
                  { value: "warning", label: "Destructive" },
                  { value: "info", label: "Routine" },
                ]}
                size="sm"
                ariaLabel="audit severity filter"
              />
            </div>
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
                options={LOG_WINDOW_PRESETS.map((p) => ({ value: p.value, label: t(p.labelKey) }))}
                allLabel={t(LOG_WINDOW_ALL_LABEL_KEY)}
              />
            </ListToolbar>
          </>
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
              <DialogTitle className="text-lg font-semibold tracking-tight">{t("adminAudit.detailTitle")}</DialogTitle>
              <DialogDescription>
                {detail.action} · {detail.resource_type}
                {detail.resource_id ? ` #${detail.resource_id}` : ""}
              </DialogDescription>
            </DialogHeader>
            <div className="mt-4 max-h-[60vh] space-y-4 overflow-y-auto pr-1">
              <AuditMeta label={t("adminAudit.actor")} value={userLookup.get(detail.actor_user_id)} />
              <AuditMeta label={t("adminAudit.ip")} value={detail.ip || "—"} copyable />
              <AuditMeta label={t("adminAudit.trace")} value={detail.trace_id || "—"} copyable />
              <AuditMeta label={t("adminAudit.userAgent")} value={detail.user_agent || "—"} copyable />
              <JsonBlock label={t("adminAudit.before")} value={detail.before} />
              <JsonBlock label={t("adminAudit.after")} value={detail.after} />
            </div>
          </DialogContent>
        </Dialog>
      ) : null}
    </>
  );
}

function AuditExpandedDetail({ entry, email }: { entry: AuditLog; email: string }) {
  const { t } = useLanguage();
  const severity = auditSeverity(entry.action);
  const beforeJson = safeJson(entry.before);
  const afterJson = safeJson(entry.after);
  const hasBefore = beforeJson && beforeJson !== "null" && beforeJson !== "{}" && beforeJson !== "[]";
  const hasAfter = afterJson && afterJson !== "null" && afterJson !== "{}" && afterJson !== "[]";
  return (
    <div className="space-y-3">
      <InlineDetailGrid
        sections={[
          {
            title: t("adminAudit.actor"),
            rows: [
              { label: t("adminAudit.actor"), value: email || "—" },
              { label: t("adminAudit.action"), value: entry.action, mono: true, tone: severity === "warning" ? "warning" : "default" },
              { label: t("adminAudit.resource"), value: `${entry.resource_type}${entry.resource_id ? ` #${entry.resource_id}` : ""}`, mono: true },
            ],
          },
          {
            title: t("adminAudit.ip"),
            rows: [
              { label: t("adminAudit.ip"), value: entry.ip || "—", mono: true },
              { label: t("adminAudit.trace"), value: entry.trace_id || "—", mono: true, tone: "muted" },
              { label: t("adminAudit.userAgent"), value: entry.user_agent || "—", mono: true, tone: "muted" },
            ],
          },
          {
            title: t("adminAudit.time"),
            rows: [
              { label: t("adminAudit.time"), value: formatDateTime(entry.created_at), mono: true },
            ],
          },
        ]}
      />
      {hasBefore || hasAfter ? (
        <div className="grid gap-3 px-6 pb-4 sm:grid-cols-2">
          {hasBefore ? (
            <div>
              <div className="mb-1 flex items-center gap-2">
                <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("adminAudit.before")}
                </span>
                <CopyButton value={beforeJson} size="inline" />
              </div>
              <pre className="max-h-48 overflow-auto rounded-lg bg-srapi-card-muted p-3 font-mono text-[11px] text-srapi-text-secondary">
                {beforeJson}
              </pre>
            </div>
          ) : null}
          {hasAfter ? (
            <div>
              <div className="mb-1 flex items-center gap-2">
                <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("adminAudit.after")}
                </span>
                <CopyButton value={afterJson} size="inline" />
              </div>
              <pre className="max-h-48 overflow-auto rounded-lg bg-srapi-card-muted p-3 font-mono text-[11px] text-srapi-text-secondary">
                {afterJson}
              </pre>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function AuditMeta({
  label,
  value,
  copyable,
}: {
  label: string;
  value: string;
  copyable?: boolean;
}) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="w-24 shrink-0 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
        {label}
      </span>
      <span className="flex min-w-0 items-center gap-1.5">
        <span className="break-all text-xs text-srapi-text-secondary">{value}</span>
        {copyable && value && value !== "—" ? <CopyButton value={value} size="inline" /> : null}
      </span>
    </div>
  );
}

function JsonBlock({ label, value }: { label: string; value: unknown }) {
  const json = safeJson(value);
  // A bare {} or empty payload would put a useless copy button on every audit
  // entry — suppress it for those cases.
  const hasContent = json && json !== "null" && json !== "{}" && json !== "[]";
  return (
    <div>
      <div className="flex items-center gap-2">
        <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{label}</span>
        {hasContent ? <CopyButton value={json} size="inline" /> : null}
      </div>
      <pre className="mt-1.5 max-h-48 overflow-auto rounded-lg bg-srapi-card-muted p-3 font-mono text-[11px] text-srapi-text-secondary">
        {json}
      </pre>
    </div>
  );
}
