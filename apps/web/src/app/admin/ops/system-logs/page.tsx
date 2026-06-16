"use client";

import { useState } from "react";
import { FileText } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, SearchInput, FilterSelect } from "@/components/admin/list-toolbar";
import { OpsLogCleanupDialog } from "@/components/admin/ops-log-cleanup-dialog";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { useAdminList } from "@/hooks/use-admin-list";
import { useOpsSystemLogs } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, safeJson } from "@/lib/admin-format";
import type { OpsSystemLog, OpsSystemLogLevel } from "@/lib/sdk-types";

const LOG_LEVELS: OpsSystemLogLevel[] = ["debug", "info", "warn", "error"];

// Admin view of the application's structured log buffer. Backed by the existing
// /admin/ops/system-logs endpoint that has had real rows since launch but no
// page to render them. Read-only for now — bounded cleanup (cleanupOpsSystemLogs)
// is wired in the SDK and can be added behind a confirm dialog later.
export default function AdminOpsSystemLogsPage() {
  return (
    <AdminShell>
      <Content />
    </AdminShell>
  );
}

function Content() {
  const { t } = useLanguage();
  const list = useAdminList();
  const level = (list.filters.level as OpsSystemLogLevel | undefined) ?? undefined;
  const source = list.filters.source || undefined;
  const logs = useOpsSystemLogs({
    page: list.page,
    page_size: list.pageSize,
    level,
    source,
    q: list.search || undefined,
  });
  const [detail, setDetail] = useState<OpsSystemLog | null>(null);
  const [showCleanup, setShowCleanup] = useState(false);

  const columns: Column<OpsSystemLog>[] = [
    {
      key: "time",
      header: t("adminOpsSystemLogs.time"),
      render: (row) => (
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(row.created_at)}
        </span>
      ),
    },
    {
      key: "level",
      header: t("adminOpsSystemLogs.level"),
      render: (row) => <QuietBadge status={levelTone(row.level)} label={row.level} />,
    },
    {
      key: "source",
      header: t("adminOpsSystemLogs.source"),
      hideOnMobile: true,
      render: (row) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{row.source || "—"}</span>
      ),
    },
    {
      key: "message",
      header: t("adminOpsSystemLogs.message"),
      render: (row) => (
        <span className="block max-w-2xl truncate text-srapi-text-secondary" title={row.message}>
          {row.message}
        </span>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminOpsSystemLogs.title")}
        description={t("adminOpsSystemLogs.subtitle")}
        actions={
          <div className="flex items-center gap-2">
            {logs.data ? (
              <ListCount total={logs.data.pagination?.total ?? logs.data.data.length} />
            ) : null}
            <Button type="button" variant="outline" size="sm" onClick={() => setShowCleanup(true)}>
              {t("adminOpsSystemLogs.cleanup")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={logs}
        columns={columns}
        getRowId={(r) => r.id}
        emptyIcon={FileText}
        emptyTitle={t("adminOpsSystemLogs.emptyTitle")}
        emptyBody={t("adminOpsSystemLogs.emptyBody")}
        minWidth={720}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminOpsSystemLogs.searchPlaceholder")}
            />
            <FilterSelect
              value={level}
              onChange={(value) => list.setFilter("level", value)}
              allLabel={t("adminOpsSystemLogs.allLevels")}
              options={LOG_LEVELS.map((l) => ({ value: l, label: l }))}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: logs.data?.pagination?.total ?? logs.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(row) => (
          <RowActionsMenu
            actions={[{ label: t("adminOpsSystemLogs.viewDetails"), onSelect: () => setDetail(row) }]}
          />
        )}
      />

      <Dialog
        open={detail !== null}
        onOpenChange={(open) => {
          if (!open) setDetail(null);
        }}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{detail?.message || t("adminOpsSystemLogs.viewDetails")}</DialogTitle>
            {detail ? (
              <DialogDescription>
                {formatDateTime(detail.created_at)} · {detail.source} ·{" "}
                <span className="font-mono">{detail.level}</span>
              </DialogDescription>
            ) : null}
          </DialogHeader>
          {detail ? (
            <div className="space-y-3">
              {detail.request_id ? (
                <KV label={t("adminOpsSystemLogs.requestId")} value={detail.request_id} />
              ) : null}
              {detail.trace_id ? (
                <KV label={t("adminOpsSystemLogs.traceId")} value={detail.trace_id} />
              ) : null}
              {detail.metadata && Object.keys(detail.metadata).length > 0 ? (
                <div>
                  <div className="text-2xs uppercase text-srapi-text-tertiary">
                    {t("adminOpsSystemLogs.metadata")}
                  </div>
                  <pre className="mt-1 max-h-96 overflow-auto rounded-lg border border-srapi-border bg-srapi-card-muted px-3 py-2 text-2xs text-srapi-text-secondary">
                    {safeJson(detail.metadata)}
                  </pre>
                </div>
              ) : null}
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

      <OpsLogCleanupDialog open={showCleanup} onOpenChange={setShowCleanup} />
    </>
  );
}

function KV({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-3">
      <dt className="w-24 shrink-0 text-2xs uppercase text-srapi-text-tertiary">{label}</dt>
      <dd className="font-mono text-2xs text-srapi-text-secondary break-all">{value}</dd>
    </div>
  );
}

function levelTone(level: OpsSystemLogLevel): "active" | "limited" | "disabled" | "error" {
  switch (level) {
    case "error":
      return "error";
    case "warn":
      return "limited";
    case "info":
      return "active";
    default:
      return "disabled";
  }
}
