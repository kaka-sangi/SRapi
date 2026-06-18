"use client";

import { useState, type ReactNode } from "react";
import { FileText } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, SearchInput, FilterSelect } from "@/components/admin/list-toolbar";
import { OpsLogCleanupDialog } from "@/components/admin/ops-log-cleanup-dialog";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { useAdminList } from "@/hooks/use-admin-list";
import { useOpsSystemLogHealth, useOpsSystemLogs } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatInteger, safeJson } from "@/lib/admin-format";
import type { OpsSystemLog, OpsSystemLogHealth, OpsSystemLogLevel } from "@/lib/sdk-types";

const LOG_LEVELS: OpsSystemLogLevel[] = ["debug", "info", "warn", "error"];
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
  const health = useOpsSystemLogHealth();
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
      <SystemLogEvidencePanel health={health.data} loading={health.isLoading} />
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

function SystemLogEvidencePanel({
  health,
  loading,
}: {
  health: OpsSystemLogHealth | undefined;
  loading: boolean;
}) {
  const { t } = useLanguage();
  if (loading && !health) {
    return (
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <EvidenceTileSkeleton />
        <EvidenceTileSkeleton />
        <EvidenceTileSkeleton />
        <EvidenceTileSkeleton />
      </div>
    );
  }
  const total = health?.total_count ?? 0;
  const lastErrorHint = health?.last_error_at
    ? [formatDateTime(health.last_error_at), health.last_error_source].filter(Boolean).join(" · ")
    : t("adminOpsSystemLogs.noRecentError");
  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
      <EvidenceTile
        label={t("adminOpsSystemLogs.backend")}
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
            {health?.degraded ? (
              <QuietBadge status="error" label={t("adminOpsSystemLogs.degraded")} />
            ) : (
              <QuietBadge status="active" label={t("adminOpsSystemLogs.healthy")} />
            )}
          </span>
        }
      />
      <EvidenceTile
        label={t("adminOpsSystemLogs.lastWrite")}
        value={formatDateTime(health?.last_log_at)}
        footer={
          <QuietBadge
            status={health?.stale ? "limited" : "active"}
            label={health?.stale ? t("adminOpsSystemLogs.stale") : t("adminOpsSystemLogs.fresh")}
          />
        }
      />
      <EvidenceTile
        label={t("adminOpsSystemLogs.lastError")}
        value={health?.last_error_message || "-"}
        footer={
          <span className="block truncate" title={lastErrorHint}>
            {lastErrorHint}
          </span>
        }
      />
      <EvidenceTile
        label={t("adminOpsSystemLogs.total")}
        value={formatInteger(total)}
        footer={<LevelCounts counts={health?.level_counts} />}
      />
    </div>
  );
}

function EvidenceTile({
  label,
  value,
  footer,
}: {
  label: string;
  value: string;
  footer: ReactNode;
}) {
  return (
    <Card className="flex min-h-28 flex-col justify-between p-4">
      <div>
        <div className="font-mono text-2xs uppercase text-srapi-text-tertiary">{label}</div>
        <div className="mt-2 line-clamp-2 break-words text-sm font-medium text-srapi-text-primary">
          {value}
        </div>
      </div>
      <div className="mt-3 font-mono text-2xs text-srapi-text-tertiary">{footer}</div>
    </Card>
  );
}

function EvidenceTileSkeleton() {
  return (
    <Card className="flex min-h-28 flex-col justify-between p-4">
      <div>
        <Skeleton className="h-3 w-24" />
        <Skeleton className="mt-3 h-4 w-32" />
      </div>
      <Skeleton className="mt-4 h-5 w-40" />
    </Card>
  );
}

function LevelCounts({ counts }: { counts: OpsSystemLogHealth["level_counts"] | undefined }) {
  return (
    <span className="flex flex-wrap gap-1.5">
      {LOG_LEVELS.map((level) => (
        <QuietBadge
          key={level}
          status={levelTone(level)}
          label={`${level}:${formatInteger(counts?.[level] ?? 0)}`}
        />
      ))}
    </span>
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
