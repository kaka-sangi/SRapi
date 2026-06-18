"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { Bug, FileText, ExternalLink } from "lucide-react";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { RequestDumpSummaryGrid } from "@/components/admin/request-log-dump-summary-panel";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime } from "@/lib/admin-format";
import { adminErrorLogsHref, adminSystemLogsHref } from "@/lib/admin-log-links";
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

  const formattedRows = useMemo(
    () =>
      items.map((item) => ({
        ...item,
        createdAtLabel: formatDateTime(item.created_at),
        sizeLabel: formatSize(item.size),
        summary: requestLogDescriptorSummary(item),
      })),
    [items],
  );

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("adminRequestLogFiles.title")}
        description={t("adminRequestLogFiles.subtitle")}
      />

      <div className="flex flex-wrap items-center gap-3 rounded-lg border border-srapi-border-subtle bg-srapi-bg-card p-3">
        <input
          value={prefix}
          onChange={(e) => setPrefix(e.target.value)}
          placeholder={t("adminRequestLogFiles.searchPlaceholder")}
          className="h-8 flex-1 min-w-[180px] rounded border border-srapi-border-subtle bg-srapi-bg-input px-2 text-sm"
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

      {formattedRows.length === 0 ? (
        <div className="rounded-lg border border-srapi-border-subtle bg-srapi-bg-card p-6 text-center text-sm text-srapi-text-tertiary">
          <p className="font-medium text-srapi-text-secondary">
            {t("adminRequestLogFiles.emptyTitle")}
          </p>
          <p>{t("adminRequestLogFiles.emptyBody")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-srapi-border-subtle bg-srapi-bg-card">
          <table className="w-full table-fixed text-left text-sm">
            <thead className="border-b border-srapi-border-subtle bg-srapi-bg-card-elevated">
              <tr>
                <th className="px-3 py-2 font-medium">{t("adminRequestLogFiles.name")}</th>
                <th className="w-44 px-3 py-2 font-medium">
                  {t("adminRequestLogFiles.requestId")}
                </th>
                <th className="w-40 px-3 py-2 font-medium">
                  {t("adminRequestLogFiles.createdAt")}
                </th>
                <th className="w-24 px-3 py-2 font-medium">{t("adminRequestLogFiles.size")}</th>
                <th className="w-56 px-3 py-2 font-medium">
                  {t("adminRequestLogFiles.diagnosticSummary")}
                </th>
                <th className="w-36 px-3 py-2 font-medium">
                  {t("adminRequestLogFiles.relatedEvidence")}
                </th>
                <th className="w-48 px-3 py-2 font-medium">&nbsp;</th>
              </tr>
            </thead>
            <tbody>
              {formattedRows.map((row) => (
                <tr key={row.name} className="border-t border-srapi-border-subtle">
                  <td className="px-3 py-2 font-mono text-xs">
                    <div className="flex min-w-0 items-center gap-2">
                      <button
                        type="button"
                        onClick={() => openPreview(row)}
                        title={row.name}
                        className="min-w-0 truncate underline-offset-2 hover:underline"
                      >
                        {row.name}
                      </button>
                      {row.is_error_only ? (
                        <span className="shrink-0 rounded bg-red-500/15 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-red-300">
                          error
                        </span>
                      ) : null}
                    </div>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-srapi-text-secondary">
                    <div className="truncate" title={row.request_id}>
                      {row.request_id}
                    </div>
                  </td>
                  <td className="whitespace-nowrap px-3 py-2 text-xs text-srapi-text-tertiary">
                    {row.createdAtLabel}
                  </td>
                  <td className="px-3 py-2 text-xs text-srapi-text-tertiary">
                    {row.sizeLabel}
                  </td>
                  <td className="px-3 py-2">
                    <RequestDumpDescriptorSummary file={row} />
                  </td>
                  <td className="px-3 py-2">
                    <RequestDumpEvidencePills file={row} />
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex gap-2">
                      <button
                        type="button"
                        onClick={() => openPreview(row)}
                        className="rounded border border-srapi-border-subtle px-2 py-1 text-xs hover:bg-srapi-bg-card-elevated"
                      >
                        {t("adminRequestLogFiles.preview")}
                      </button>
                      <button
                        type="button"
                        onClick={() => downloadFile(row)}
                        className="rounded border border-srapi-border-subtle px-2 py-1 text-xs hover:bg-srapi-bg-card-elevated"
                      >
                        {t("adminRequestLogFiles.download")}
                      </button>
                      <button
                        type="button"
                        onClick={() => setDeleteTarget(row)}
                        className="rounded border border-red-500/30 px-2 py-1 text-xs text-red-300 hover:bg-red-500/10"
                      >
                        {t("adminRequestLogFiles.delete")}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
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
            <p className="text-sm text-red-300">{t("adminRequestLogFiles.detailLoadFailed")}</p>
          ) : (
            <div className="space-y-3">
              {selected ? <RequestDumpEvidenceLinks file={selected} /> : null}
              {dumpSummary ? <RequestDumpSummaryGrid summary={dumpSummary} /> : null}
              <pre className="max-h-[60vh] overflow-auto rounded bg-srapi-bg-input p-3 text-xs">
                {downloadQuery.data ?? ""}
              </pre>
            </div>
          )}
          {selected ? (
            <div className="flex justify-end">
              <button
                type="button"
                onClick={() => selected && downloadFile(selected)}
                className="rounded border border-srapi-border-subtle px-3 py-1 text-sm hover:bg-srapi-bg-card-elevated"
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
      <span className="rounded bg-srapi-card-muted px-1.5 py-0.5 text-2xs text-srapi-text-tertiary">
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
      ? "bg-emerald-500/15 text-emerald-300"
      : summary.success === false
        ? "bg-red-500/15 text-red-300"
        : "bg-srapi-card-muted text-srapi-text-tertiary";

  return (
    <div className="space-y-1 text-xs">
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <span className={`rounded px-1.5 py-0.5 text-2xs font-medium ${outcomeClass}`}>
          {outcome}
        </span>
        {summary.statusCode !== undefined ? (
          <span className="font-mono text-srapi-text-secondary">{summary.statusCode}</span>
        ) : null}
        {summary.errorClass ? (
          <span className="min-w-0 truncate font-mono text-2xs text-red-300" title={summary.errorClass}>
            {summary.errorClass}
          </span>
        ) : null}
      </div>
      <div className="flex min-w-0 flex-wrap gap-x-2 gap-y-1 text-2xs text-srapi-text-tertiary">
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
        <div className="truncate font-mono text-2xs text-srapi-text-tertiary" title={summary.sourceEndpoint}>
          {summary.sourceEndpoint}
        </div>
      ) : null}
    </div>
  );
}

function RequestDumpEvidencePills({ file }: { file: RequestLogFileDescriptor }) {
  const { t } = useLanguage();
  const errorHref = adminErrorLogsHref({ request_id: file.request_id });
  const systemHref = adminSystemLogsHref({ request_id: file.request_id });
  if (!errorHref && !systemHref) {
    return <span className="font-mono text-2xs text-srapi-text-tertiary">—</span>;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {errorHref ? (
        <Link
          href={errorHref}
          className="rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
        >
          {t("adminRequestLogFiles.openErrorLogs")}
        </Link>
      ) : null}
      {systemHref ? (
        <Link
          href={systemHref}
          className="rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
        >
          {t("adminRequestLogFiles.openSystemLogs")}
        </Link>
      ) : null}
    </div>
  );
}

function RequestDumpEvidenceLinks({ file }: { file: RequestLogFileDescriptor }) {
  const { t } = useLanguage();
  const errorHref = adminErrorLogsHref({ request_id: file.request_id });
  const systemHref = adminSystemLogsHref({ request_id: file.request_id });
  if (!errorHref && !systemHref) return null;

  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-srapi-border-subtle bg-srapi-bg-card-elevated px-3 py-2">
      <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
        {t("adminRequestLogFiles.relatedEvidence")}
      </span>
      <div className="flex flex-wrap gap-2">
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
