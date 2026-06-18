"use client";

import Link from "next/link";
import { Activity, Bug, FileText, ScrollText } from "lucide-react";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { FilterSelect, ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { PageHeader } from "@/components/layout/page-header";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { useAdminList } from "@/hooks/use-admin-list";
import { useOpsRequestEvidence } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatLatency } from "@/lib/admin-format";
import {
  adminErrorLogsHref,
  adminRequestDumpsHref,
  adminSystemLogsHref,
} from "@/lib/admin-log-links";
import {
  LOG_WINDOW_PRESETS,
  LOG_WINDOW_ALL_LABEL_KEY,
  logWindowSince,
} from "@/lib/log-window-filter";
import type { RequestEvidenceRow } from "@/lib/sdk-types";

export function RequestEvidencePanel() {
  const { t } = useLanguage();
  const list = useAdminList({ pageSize: 50 });
  const kind = list.filters.kind || undefined;
  const source = list.filters.source || undefined;
  const windowFilter = list.filters.window || "1h";
  const requestID = list.filters.request_id || undefined;
  const start = logWindowSince(windowFilter)?.toISOString();
  const query = useOpsRequestEvidence({
    page: list.page,
    page_size: list.pageSize,
    request_id: requestID,
    kind: kind as "success" | "error" | "unknown" | undefined,
    evidence_source: source as "usage" | "ops_error" | "request_dump" | undefined,
    start,
    q: list.search || undefined,
  });
  const total = query.data?.pagination?.total ?? query.data?.data.length ?? 0;
  const isFiltered = Boolean(kind || source || requestID || list.search || list.filters.window);

  const columns: Column<RequestEvidenceRow>[] = [
    {
      key: "time",
      header: t("adminRequestEvidence.time"),
      pinned: true,
      render: (row) => (
        <span className="text-2xs text-srapi-text-tertiary tabular font-mono whitespace-nowrap">
          {formatDateTime(row.created_at)}
        </span>
      ),
    },
    {
      key: "result",
      header: t("adminRequestEvidence.result"),
      render: (row) => <QuietBadge status={kindTone(row.kind)} label={kindLabel(t, row.kind)} />,
    },
    {
      key: "request",
      header: t("adminRequestEvidence.request"),
      pinned: true,
      render: (row) => (
        <div className="min-w-0">
          <div
            className="text-srapi-text-primary truncate font-mono text-xs"
            title={row.request_id}
          >
            {row.request_id}
          </div>
          <div
            className="text-2xs text-srapi-text-tertiary truncate font-mono"
            title={row.model ?? ""}
          >
            {row.model || "—"}
          </div>
        </div>
      ),
    },
    {
      key: "route",
      header: t("adminRequestEvidence.route"),
      hideOnMobile: true,
      render: (row) => (
        <div className="min-w-0">
          <div
            className="text-2xs text-srapi-text-secondary truncate font-mono"
            title={row.source_endpoint ?? ""}
          >
            {row.source_endpoint || "—"}
          </div>
          <div className="text-2xs text-srapi-text-tertiary truncate font-mono">
            {row.source_protocol || "—"}
            {row.target_protocol ? ` -> ${row.target_protocol}` : ""}
          </div>
        </div>
      ),
    },
    {
      key: "actor",
      header: t("adminRequestEvidence.actor"),
      hideOnMobile: true,
      render: (row) => (
        <div className="text-2xs text-srapi-text-tertiary space-y-0.5 font-mono">
          <div>
            {t("adminRequestEvidence.userShort")}: {row.user_id ?? "—"}
          </div>
          <div>
            {t("adminRequestEvidence.keyShort")}: {row.api_key_id ?? "—"}
          </div>
          <div>
            {t("adminRequestEvidence.accountShort")}: {row.account_id ?? "—"}
          </div>
        </div>
      ),
    },
    {
      key: "status",
      header: t("adminRequestEvidence.status"),
      align: "right",
      render: (row) => (
        <div className="space-y-0.5 text-right">
          <div className={statusClass(row.status_code)}>{row.status_code ?? "—"}</div>
          <div className="text-2xs text-srapi-text-tertiary font-mono">
            {row.attempt_no ? `#${row.attempt_no}` : "—"}
          </div>
        </div>
      ),
    },
    {
      key: "error",
      header: t("adminRequestEvidence.error"),
      hideOnMobile: true,
      render: (row) => (
        <div className="min-w-0">
          <div
            className="text-srapi-error truncate font-mono text-xs"
            title={row.error_class ?? ""}
          >
            {row.error_class || "—"}
          </div>
          <div className="text-2xs text-srapi-text-tertiary line-clamp-2 break-words">
            {row.error_message || row.error_phase || "—"}
          </div>
        </div>
      ),
    },
    {
      key: "latency",
      header: t("adminRequestEvidence.latency"),
      align: "right",
      hideOnMobile: true,
      render: (row) => (
        <span className="text-2xs text-srapi-text-tertiary tabular font-mono">
          {typeof row.latency_ms === "number" ? formatLatency(row.latency_ms) : "—"}
        </span>
      ),
    },
    {
      key: "tokens",
      header: t("adminRequestEvidence.tokens"),
      align: "right",
      hideOnMobile: true,
      render: (row) => (
        <span className="text-2xs text-srapi-text-tertiary tabular font-mono">
          {row.total_tokens ?? "—"}
        </span>
      ),
    },
    {
      key: "source",
      header: t("adminRequestEvidence.source"),
      hideOnMobile: true,
      render: (row) => (
        <div className="flex flex-wrap gap-1">
          {row.has_usage_log ? <SourceChip label={t("adminRequestEvidence.usage")} /> : null}
          {row.has_ops_error_log ? (
            <SourceChip label={t("adminRequestEvidence.opsError")} tone="error" />
          ) : null}
          {row.has_request_dump ? (
            <SourceChip
              label={`${t("adminRequestEvidence.dump")} ${row.request_dump_count}`}
              tone="info"
            />
          ) : null}
        </div>
      ),
    },
    {
      key: "evidence",
      header: t("adminRequestEvidence.evidence"),
      className: "w-48",
      render: (row) => <EvidenceLinks row={row} />,
    },
  ];

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("adminRequestEvidence.title")}
        description={t("adminRequestEvidence.subtitle")}
        actions={
          <AutoRefreshControl
            onRefresh={async () => {
              await query.refetch();
            }}
            isRefreshing={query.isFetching}
            storageKey="admin-request-evidence-refresh"
            defaultSec={0}
          />
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        getRowId={(row) =>
          `${row.evidence_source}:${row.request_id}:${row.usage_log_id ?? row.ops_error_log_id ?? row.latest_request_dump_name ?? ""}`
        }
        emptyIcon={Activity}
        emptyTitle={t("adminRequestEvidence.emptyTitle")}
        emptyBody={t("adminRequestEvidence.emptyBody")}
        minWidth={1120}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminRequestEvidence.searchPlaceholder")}
              className="sm:max-w-md"
            />
            <SearchInput
              value={requestID ?? ""}
              onChange={(value) => list.setFilter("request_id", value.trim() || undefined)}
              placeholder={t("adminRequestEvidence.requestIdPlaceholder")}
              className="sm:max-w-xs"
            />
            <FilterSelect
              value={kind}
              onChange={(value) => list.setFilter("kind", value)}
              allLabel={t("adminRequestEvidence.allKinds")}
              options={[
                { value: "success", label: t("adminRequestEvidence.success") },
                { value: "error", label: t("adminRequestEvidence.errorKind") },
                { value: "unknown", label: t("adminRequestEvidence.unknown") },
              ]}
            />
            <FilterSelect
              value={source}
              onChange={(value) => list.setFilter("source", value)}
              allLabel={t("adminRequestEvidence.allSources")}
              options={[
                { value: "usage", label: t("adminRequestEvidence.usage") },
                { value: "ops_error", label: t("adminRequestEvidence.opsError") },
                { value: "request_dump", label: t("adminRequestEvidence.dump") },
              ]}
            />
            <FilterSelect
              value={windowFilter}
              onChange={(value) => list.setFilter("window", value)}
              allLabel={t(LOG_WINDOW_ALL_LABEL_KEY)}
              options={LOG_WINDOW_PRESETS.map((preset) => ({
                value: preset.value,
                label: t(preset.labelKey),
              }))}
            />
            <div className="text-srapi-text-tertiary ml-auto text-xs">
              {t("adminRequestEvidence.total", { count: total })}
            </div>
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        isFiltered={isFiltered}
        noResultsTitle={t("adminRequestEvidence.noResultsTitle")}
        noResultsBody={t("adminRequestEvidence.noResultsBody")}
        onClearFilters={list.clearFilters}
      />
    </div>
  );
}

function EvidenceLinks({ row }: { row: RequestEvidenceRow }) {
  const { t } = useLanguage();
  const params = { request_id: row.request_id };
  const errorHref = adminErrorLogsHref(params);
  const systemHref = adminSystemLogsHref(params);
  const dumpHref = adminRequestDumpsHref(params);
  return (
    <div className="flex flex-wrap gap-1">
      {row.has_ops_error_log && errorHref ? (
        <EvidenceLink
          href={errorHref}
          icon={<Bug className="size-3" />}
          label={t("adminRequestEvidence.errorLog")}
        />
      ) : null}
      {systemHref ? (
        <EvidenceLink
          href={systemHref}
          icon={<ScrollText className="size-3" />}
          label={t("adminRequestEvidence.systemLog")}
        />
      ) : null}
      {row.has_request_dump && dumpHref ? (
        <EvidenceLink
          href={dumpHref}
          icon={<FileText className="size-3" />}
          label={t("adminRequestEvidence.dump")}
        />
      ) : null}
    </div>
  );
}

function EvidenceLink({
  href,
  icon,
  label,
}: {
  href: string;
  icon: React.ReactNode;
  label: string;
}) {
  return (
    <Link
      href={href}
      className="border-srapi-border-subtle text-2xs text-srapi-text-secondary hover:bg-srapi-bg-card-elevated hover:text-srapi-text-primary inline-flex items-center gap-1 rounded border px-1.5 py-0.5"
    >
      {icon}
      {label}
    </Link>
  );
}

function SourceChip({
  label,
  tone = "neutral",
}: {
  label: string;
  tone?: "neutral" | "error" | "info";
}) {
  const className =
    tone === "error"
      ? "border-red-500/30 bg-red-500/10 text-red-300"
      : tone === "info"
        ? "border-sky-500/30 bg-sky-500/10 text-sky-300"
        : "border-srapi-border-subtle bg-srapi-bg-card-elevated text-srapi-text-secondary";
  return <span className={`text-2xs rounded border px-1.5 py-0.5 ${className}`}>{label}</span>;
}

function kindTone(kind: RequestEvidenceRow["kind"]): QuietStatus {
  if (kind === "success") return "active";
  if (kind === "error") return "error";
  return "disabled";
}

function kindLabel(
  t: (key: string, values?: Record<string, string | number>) => string,
  kind: RequestEvidenceRow["kind"],
): string {
  if (kind === "success") return t("adminRequestEvidence.success");
  if (kind === "error") return t("adminRequestEvidence.errorKind");
  return t("adminRequestEvidence.unknown");
}

function statusClass(status: number | undefined): string {
  const base = "font-mono text-2xs tabular";
  if (status == null) return `${base} text-srapi-text-tertiary`;
  if (status >= 500) return `${base} text-srapi-error`;
  if (status >= 400) return `${base} text-amber-500`;
  return `${base} text-emerald-400`;
}
