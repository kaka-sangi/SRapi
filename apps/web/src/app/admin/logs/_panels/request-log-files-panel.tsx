"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { Bug, ChevronDown, ChevronRight, FileSearch, FileText, ExternalLink } from "lucide-react";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { RequestDumpSummaryGrid } from "@/components/admin/request-log-dump-summary-panel";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DensityToggle, type DensityValue } from "@/components/ui/density-toggle";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import { ExpandableRow } from "@/components/ui/expandable-row";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime } from "@/lib/admin-format";
import {
  adminErrorLogsHref,
  adminRequestEvidenceHref,
  adminSystemLogsHref,
} from "@/lib/admin-log-links";
import {
  parseRequestDumpSummary,
  requestLogDescriptorSummary,
} from "@/lib/request-log-dump-summary";
import {
  downloadAdminRequestLogFileText,
  requestLogFileDownloadQueryKey,
  useAdminRequestLogFileDownload,
  useAdminRequestLogFiles,
  useDeleteAdminRequestLogFile,
} from "@/hooks/admin-queries";
import type { RequestLogFileDescriptor } from "@/lib/admin-api/request-log-files";

// RequestLogFilesPanel renders the per-request HTTP envelope dumps written
// by the gateway when SRAPI_REQUEST_LOG_ENABLED=true. The dataset is small
// (capped by retention + count) and changes infrequently, so we use a
// small React Query wrapper with optional 30s auto-refresh rather than the
// AdminListView machinery used by the other tabs.
export function RequestLogFilesPanel() {
  const { t } = useLanguage();
  const queryClient = useQueryClient();
  const [errorOnly, setErrorOnly] = useState(() => readInitialErrorOnly());
  const [prefix, setPrefix] = useState(() => readInitialRequestIDPrefix());
  const [autoRefresh, setAutoRefresh] = useState(false);
  const [selected, setSelected] = useState<RequestLogFileDescriptor | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<RequestLogFileDescriptor | null>(null);
  const [severityFilter, setSeverityFilter] = useState<"all" | "error" | "ok">("all");
  const [density, setDensity] = useState<DensityValue>("regular");
  const [expandedName, setExpandedName] = useState<string | null>(null);
  const queryParams = useMemo(
    () => ({
      request_id: prefix.trim() || undefined,
      error_only: errorOnly,
      limit: 200,
    }),
    [errorOnly, prefix],
  );
  const filesQuery = useAdminRequestLogFiles(queryParams, true, autoRefresh ? 30000 : false);
  const downloadQuery = useAdminRequestLogFileDownload(selected?.name ?? null, selected !== null);
  const deleteMutation = useDeleteAdminRequestLogFile();
  const items = useMemo(() => filesQuery.data?.data ?? [], [filesQuery.data?.data]);
  const dumpSummary = useMemo(
    () => (downloadQuery.data ? parseRequestDumpSummary(downloadQuery.data) : null),
    [downloadQuery.data],
  );

  useEffect(() => {
    if (typeof window === "undefined") return;
    const sp = new URLSearchParams(window.location.search);
    sp.delete("f_request_id");
    sp.delete("f_error_only");
    const trimmed = prefix.trim();
    if (trimmed) sp.set("f_request_id", trimmed);
    if (errorOnly) sp.set("f_error_only", "true");
    const qs = sp.toString();
    const next = qs ? `${window.location.pathname}?${qs}` : window.location.pathname;
    window.history.replaceState(window.history.state, "", next);
  }, [errorOnly, prefix]);

  const openPreview = useCallback((file: RequestLogFileDescriptor) => {
    setSelected(file);
  }, []);

  const downloadFile = useCallback((file: RequestLogFileDescriptor) => {
    void (async () => {
      try {
        const text = await queryClient.fetchQuery({
          queryKey: requestLogFileDownloadQueryKey(file.name),
          queryFn: () => downloadAdminRequestLogFileText(file.name),
        });
        const blob = new Blob([text], { type: "text/plain;charset=utf-8" });
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = file.name;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
      } catch {
        /* ignore — operator can click again */
      }
    })();
  }, [queryClient]);

  const formattedRows = useMemo(() => {
    const rows = items.map((item) => {
      const summary = requestLogDescriptorSummary(item);
      return {
        ...item,
        createdAtLabel: formatDateTime(item.created_at),
        sizeLabel: formatSize(item.size),
        summary,
        // Severity drives the .log-row stripe — error dumps stand out, summary-
        // missing rows read as neutral info, successful captures show no stripe.
        severity:
          item.is_error_only || summary.success === false
            ? ("error" as const)
            : summary.success === true
              ? ("success" as const)
              : ("info" as const),
      };
    });
    if (severityFilter === "all") return rows;
    if (severityFilter === "error") return rows.filter((r) => r.severity === "error");
    return rows.filter((r) => r.severity !== "error");
  }, [items, severityFilter]);
  const rowPad = density === "compact" ? "py-1.5 px-3" : "py-3 px-4";

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("adminRequestLogFiles.title")}
        description={t("adminRequestLogFiles.subtitle")}
      />

      <div className="space-y-2 rounded-xl border border-srapi-border bg-srapi-card p-3">
        {/* Severity chip strip — picks between "all", "error only" or
            "successful" without a checkbox. The legacy errorOnly checkbox is
            still honored server-side; this strip narrows the visible feed. */}
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            Severity
          </span>
          <SegmentedControl
            value={severityFilter}
            onChange={(v) => setSeverityFilter(v)}
            options={[
              { value: "all", label: "All" },
              { value: "error", label: "Errors" },
              { value: "ok", label: "Successful" },
            ]}
            size="sm"
            ariaLabel="dump severity filter"
          />
          <div className="flex-1" />
          <DensityToggle value={density} onChange={setDensity} />
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <input
            value={prefix}
            onChange={(e) => setPrefix(e.target.value)}
            placeholder={t("adminRequestLogFiles.searchPlaceholder")}
            className="h-9 flex-1 min-w-[180px] rounded-lg border border-srapi-border bg-srapi-card px-2.5 text-sm"
          />
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={errorOnly}
              onChange={(e) => setErrorOnly(e.target.checked)}
            />
            {t("adminRequestLogFiles.errorOnly")}
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={autoRefresh}
              onChange={(e) => setAutoRefresh(e.target.checked)}
            />
            {t("adminRequestLogFiles.autoRefresh")}
          </label>
        </div>
      </div>

      {formattedRows.length === 0 ? (
        <IllustratedEmptyState
          illust="logs"
          title={t("adminRequestLogFiles.emptyTitle")}
          description={t("adminRequestLogFiles.emptyBody")}
        />
      ) : (
        <div className="overflow-x-auto rounded-xl border border-srapi-border bg-srapi-card">
          <table className="w-full table-fixed text-left text-sm">
            <thead className="border-b border-srapi-border bg-srapi-card-muted">
              <tr>
                <th className="px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminRequestLogFiles.name")}</th>
                <th className="w-44 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("adminRequestLogFiles.requestId")}
                </th>
                <th className="w-40 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("adminRequestLogFiles.createdAt")}
                </th>
                <th className="w-24 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{t("adminRequestLogFiles.size")}</th>
                <th className="w-56 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("adminRequestLogFiles.diagnosticSummary")}
                </th>
                <th className="w-36 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  {t("adminRequestLogFiles.relatedEvidence")}
                </th>
                <th className="w-48 px-4 py-3 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">&nbsp;</th>
              </tr>
            </thead>
            <tbody>
              {formattedRows.map((row) => {
                const open = expandedName === row.name;
                const sev =
                  row.severity === "error" ? "error" : row.severity === "success" ? "info" : "info";
                return (
                  <React.Fragment key={row.name}>
                    <tr
                      data-sev={sev}
                      onClick={(e) => {
                        const target = e.target as HTMLElement | null;
                        if (target?.closest('a,button,input')) return;
                        setExpandedName((prev) => (prev === row.name ? null : row.name));
                      }}
                      className={`log-row cursor-pointer border-t border-srapi-border/70 transition-colors hover:bg-srapi-card-muted/50`}
                    >
                      <td className={`${rowPad} text-xs`}>
                        <div className="flex min-w-0 items-center gap-2">
                          {open ? (
                            <ChevronDown aria-hidden className="size-3 shrink-0" />
                          ) : (
                            <ChevronRight aria-hidden className="size-3 shrink-0" />
                          )}
                          <button
                            type="button"
                            onClick={() => openPreview(row)}
                            title={row.name}
                            className="min-w-0 truncate underline-offset-2 hover:underline"
                          >
                            {row.name}
                          </button>
                          {row.is_error_only ? (
                            <span className="shrink-0 rounded-full bg-srapi-error/12 px-2 py-0.5 text-[11px] font-medium text-srapi-error">
                              error
                            </span>
                          ) : null}
                        </div>
                      </td>
                      <td className={`${rowPad} text-xs text-srapi-text-secondary`}>
                        <div className="truncate" title={row.request_id}>
                          {row.request_id}
                        </div>
                      </td>
                      <td className={`whitespace-nowrap ${rowPad} text-[12px] tabular text-srapi-text-tertiary`}>
                        {row.createdAtLabel}
                      </td>
                      <td className={`${rowPad} text-[12px] text-srapi-text-tertiary`}>
                        {/* File size with breakdown so ops can spot an
                            unusually-large dump before opening it. */}
                        <DataTooltip
                          title={t("adminRequestLogFiles.size")}
                          primary={row.sizeLabel}
                          rows={[
                            { label: "bytes", value: row.size.toLocaleString(), mono: true } as never,
                            { label: "attempts", value: row.summary.attemptCount, tone: "muted" },
                            { label: "responses", value: row.summary.responseCount, tone: "muted" },
                          ]}
                        >
                          <span>{row.sizeLabel}</span>
                        </DataTooltip>
                      </td>
                      <td className={rowPad}>
                        <RequestDumpDescriptorSummary file={row} />
                      </td>
                      <td className={rowPad}>
                        <RequestDumpEvidencePills file={row} />
                      </td>
                      <td className={rowPad}>
                        <div className="flex gap-2">
                          <button
                            type="button"
                            onClick={() => openPreview(row)}
                            className="rounded-lg border border-srapi-border px-2.5 py-1 text-xs font-medium text-srapi-text-secondary hover:bg-srapi-card-muted hover:text-srapi-text-primary"
                          >
                            {t("adminRequestLogFiles.preview")}
                          </button>
                          <button
                            type="button"
                            onClick={() => downloadFile(row)}
                            className="rounded-lg border border-srapi-border px-2.5 py-1 text-xs font-medium text-srapi-text-secondary hover:bg-srapi-card-muted hover:text-srapi-text-primary"
                          >
                            {t("adminRequestLogFiles.download")}
                          </button>
                          <button
                            type="button"
                            onClick={() => setDeleteTarget(row)}
                            className="rounded-lg border border-srapi-error/30 px-2.5 py-1 text-xs font-medium text-srapi-error hover:bg-srapi-error/10"
                          >
                            {t("adminRequestLogFiles.delete")}
                          </button>
                        </div>
                      </td>
                    </tr>
                    {open ? (
                      <tr data-expand-for={row.name}>
                        <td colSpan={7} className="p-0">
                          <ExpandableRow expanded>
                            <InlineDetailGrid
                              sections={[
                                {
                                  title: "Request",
                                  rows: [
                                    { label: "name", value: row.name, mono: true },
                                    { label: "request_id", value: row.request_id, mono: true },
                                    { label: "created_at", value: row.createdAtLabel, mono: true, tone: "muted" },
                                    { label: "endpoint", value: row.summary.sourceEndpoint || "—", mono: true },
                                  ],
                                },
                                {
                                  title: "Response",
                                  rows: [
                                    { label: "outcome", value: row.summary.success === true ? "success" : row.summary.success === false ? "error" : "—", tone: row.summary.success === true ? "success" : row.summary.success === false ? "error" : "muted" },
                                    { label: "status", value: row.summary.statusCode ?? "—", mono: true, tone: (row.summary.statusCode ?? 0) >= 500 ? "error" : (row.summary.statusCode ?? 0) >= 400 ? "warning" : "default" },
                                    { label: "error_class", value: row.summary.errorClass || "—", mono: true, tone: "error" },
                                  ],
                                },
                                {
                                  title: "Cost",
                                  rows: [
                                    { label: "size", value: row.sizeLabel, mono: true },
                                    { label: "latency_ms", value: row.summary.latencyMS ?? "—", mono: true, tone: typeof row.summary.latencyMS === "number" && row.summary.latencyMS >= 10000 ? "warning" : "default" },
                                    { label: "attempts", value: row.summary.attemptCount, mono: true, tone: "muted" },
                                    { label: "responses", value: row.summary.responseCount, mono: true, tone: "muted" },
                                  ],
                                },
                              ]}
                            />
                          </ExpandableRow>
                        </td>
                      </tr>
                    ) : null}
                  </React.Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {filesQuery.isFetching ? (
        <p className="text-xs text-srapi-text-tertiary">…</p>
      ) : null}

      <Dialog open={selected !== null} onOpenChange={(open) => !open && setSelected(null)}>
        <DialogContent className="max-w-4xl">
          <DialogHeader>
            <DialogTitle>{t("adminRequestLogFiles.detailTitle")}</DialogTitle>
            <DialogDescription>
              {selected ? (
                <span className="font-mono text-xs">{selected.name}</span>
              ) : null}
            </DialogDescription>
          </DialogHeader>
          {downloadQuery.isError ? (
            <p className="text-sm text-srapi-error">{t("adminRequestLogFiles.detailLoadFailed")}</p>
          ) : (
            <div className="space-y-3">
              {selected ? <RequestDumpEvidenceLinks file={selected} /> : null}
              {dumpSummary ? <RequestDumpSummaryGrid summary={dumpSummary} /> : null}
              <pre className="max-h-[60vh] overflow-auto rounded bg-srapi-card p-3 text-xs">
                {downloadQuery.data ?? ""}
              </pre>
            </div>
          )}
          {selected ? (
            <div className="flex justify-end">
              <button
                type="button"
                onClick={() => selected && downloadFile(selected)}
                className="rounded border border-srapi-border px-3 py-1 text-sm hover:bg-srapi-card-muted"
              >
                {t("adminRequestLogFiles.download")}
              </button>
            </div>
          ) : null}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("adminRequestLogFiles.deleteTitle")}
        body={deleteTarget ? deleteTarget.name : undefined}
        confirmLabel={t("adminRequestLogFiles.delete")}
        onConfirm={async () => {
          if (!deleteTarget) return;
          await deleteMutation.mutateAsync(deleteTarget.name);
          setDeleteTarget(null);
        }}
        isPending={deleteMutation.isPending}
      />
    </div>
  );
}

function RequestDumpDescriptorSummary({
  file,
}: {
  file: RequestLogFileDescriptor & { summary?: ReturnType<typeof requestLogDescriptorSummary> };
}) {
  const { t } = useLanguage();
  const summary = file.summary ?? requestLogDescriptorSummary(file);
  if (!summary.hasSummary) {
    return (
      <span className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-tertiary">
        {t("adminRequestLogFiles.summaryMissing")}
      </span>
    );
  }
  const outcome =
    summary.success === true
      ? t("adminRequestLogFiles.outcomeSuccess")
      : summary.success === false
        ? t("adminRequestLogFiles.outcomeError")
        : t("adminRequestLogFiles.summaryMissing");
  const outcomeClass =
    summary.success === true
      ? "bg-srapi-success/12 text-srapi-success"
      : summary.success === false
        ? "bg-srapi-error/12 text-srapi-error"
        : "bg-srapi-card-muted text-srapi-text-tertiary";

  return (
    <div className="space-y-1 text-xs">
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${outcomeClass}`}>
          {outcome}
        </span>
        {summary.statusCode !== undefined ? (
          <span className="text-srapi-text-secondary">{summary.statusCode}</span>
        ) : null}
        {summary.errorClass ? (
          <span className="min-w-0 truncate text-[11px] text-srapi-error" title={summary.errorClass}>
            {summary.errorClass}
          </span>
        ) : null}
      </div>
      <div className="flex min-w-0 flex-wrap gap-x-2 gap-y-1 text-[11px] text-srapi-text-tertiary">
        {summary.latencyMS !== undefined ? (
          <span>{t("adminRequestLogFiles.latencyMs", { value: summary.latencyMS })}</span>
        ) : null}
        <span>
          {t("adminRequestLogFiles.attemptsValue", {
            requests: summary.attemptCount,
            responses: summary.responseCount,
          })}
        </span>
      </div>
      {summary.sourceEndpoint ? (
        <div className="truncate text-[11px] text-srapi-text-tertiary" title={summary.sourceEndpoint}>
          {summary.sourceEndpoint}
        </div>
      ) : null}
    </div>
  );
}

function RequestDumpEvidencePills({ file }: { file: RequestLogFileDescriptor }) {
  const { t } = useLanguage();
  const requestEvidenceHref = adminRequestEvidenceHref({ request_id: file.request_id });
  const errorHref = adminErrorLogsHref({ request_id: file.request_id });
  const systemHref = adminSystemLogsHref({ request_id: file.request_id });
  if (!requestEvidenceHref && !errorHref && !systemHref) {
    return <span className="text-[11px] text-srapi-text-tertiary">—</span>;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {requestEvidenceHref ? (
        <Link
          href={requestEvidenceHref}
          className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
        >
          {t("adminRequestLogFiles.openRequestEvidence")}
        </Link>
      ) : null}
      {errorHref ? (
        <Link
          href={errorHref}
          className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
        >
          {t("adminRequestLogFiles.openErrorLogs")}
        </Link>
      ) : null}
      {systemHref ? (
        <Link
          href={systemHref}
          className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
        >
          {t("adminRequestLogFiles.openSystemLogs")}
        </Link>
      ) : null}
    </div>
  );
}

function RequestDumpEvidenceLinks({ file }: { file: RequestLogFileDescriptor }) {
  const { t } = useLanguage();
  const requestEvidenceHref = adminRequestEvidenceHref({ request_id: file.request_id });
  const errorHref = adminErrorLogsHref({ request_id: file.request_id });
  const systemHref = adminSystemLogsHref({ request_id: file.request_id });
  if (!requestEvidenceHref && !errorHref && !systemHref) return null;

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-srapi-border bg-srapi-card-muted px-3 py-2">
      <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
        {t("adminRequestLogFiles.relatedEvidence")}
      </span>
      <div className="flex flex-wrap gap-2">
        {requestEvidenceHref ? (
          <Button asChild variant="outline" size="sm">
            <Link href={requestEvidenceHref}>
              <FileSearch aria-hidden />
              {t("adminRequestLogFiles.openRequestEvidence")}
              <ExternalLink aria-hidden />
            </Link>
          </Button>
        ) : null}
        {errorHref ? (
          <Button asChild variant="outline" size="sm">
            <Link href={errorHref}>
              <Bug aria-hidden />
              {t("adminRequestLogFiles.openErrorLogs")}
              <ExternalLink aria-hidden />
            </Link>
          </Button>
        ) : null}
        {systemHref ? (
          <Button asChild variant="outline" size="sm">
            <Link href={systemHref}>
              <FileText aria-hidden />
              {t("adminRequestLogFiles.openSystemLogs")}
              <ExternalLink aria-hidden />
            </Link>
          </Button>
        ) : null}
      </div>
    </div>
  );
}

function readInitialRequestIDPrefix(): string {
  if (typeof window === "undefined") return "";
  const sp = new URLSearchParams(window.location.search);
  return sp.get("f_request_id") ?? "";
}

function readInitialErrorOnly(): boolean {
  if (typeof window === "undefined") return false;
  const sp = new URLSearchParams(window.location.search);
  return sp.get("f_error_only") === "true";
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}
