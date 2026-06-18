"use client";

import { useCallback, useState } from "react";
import Link from "next/link";
import { ExternalLink } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { CopyButton } from "@/components/ui/copy-button";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import {
  downloadAdminRequestLogFileText,
  useAdminErrorLog,
  useAdminRequestLogFileDownload,
  useAdminRequestLogFiles,
  useResolveErrorLog,
} from "@/hooks/admin-queries";
import { useAccountNameLookup } from "@/hooks/use-account-name-lookup";
import { useApiKeyNameLookup } from "@/hooks/use-api-key-name-lookup";
import { useProviderNameLookup } from "@/hooks/use-provider-name-lookup";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatInteger, formatLatency } from "@/lib/admin-format";
import { adminSystemLogsHref } from "@/lib/admin-log-links";
import type { OpsErrorLog, RequestLogFileDescriptor } from "@/lib/sdk-types";

export interface ErrorLogDetailDialogProps {
  errorLogId: string | null;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  userEmail?: string;
}

export function ErrorLogDetailDialog({
  errorLogId,
  open,
  onOpenChange,
  userEmail,
}: ErrorLogDetailDialogProps) {
  const { t } = useLanguage();
  const query = useAdminErrorLog(errorLogId, open);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t("adminErrorLogs.detailTitle")}
            {userEmail ? (
              <span className="font-mono text-srapi-text-tertiary"> · {userEmail}</span>
            ) : null}
          </DialogTitle>
        </DialogHeader>

        <PageQueryState query={query} skeleton={<DialogListSkeleton rows={6} />}>
          {(detail) => <ErrorLogDetailBody detail={detail} />}
        </PageQueryState>
      </DialogContent>
    </Dialog>
  );
}

function ErrorLogDetailBody({ detail }: { detail: OpsErrorLog }) {
  const { t } = useLanguage();
  const accountLookup = useAccountNameLookup();
  const apiKeyLookup = useApiKeyNameLookup();
  const providerLookup = useProviderNameLookup();
  const userLookup = useUserEmailLookup();
  const resolveMutation = useResolveErrorLog();
  const isResolved = detail.resolution === "resolved";
  const requestLogQuery = useAdminRequestLogFiles(
    { request_id: detail.request_id || undefined, limit: 3 },
    Boolean(detail.request_id),
  );
  const protocol = detail.target_protocol
    ? `${detail.source_protocol ?? detail.platform ?? "—"} → ${detail.target_protocol}`
    : detail.source_protocol ?? detail.platform ?? "—";
  const events = detail.upstream_errors ?? [];
  const firstAt = events.length > 0 ? events[0]?.at_unix_ms ?? 0 : 0;
  const systemLogHref = adminSystemLogsHref(detail);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3 rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
        <div className="min-w-0">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.errorClass")}
          </p>
          <div className="mt-1 flex items-center gap-1.5">
            <p className="truncate text-sm font-medium text-srapi-text-primary">
              {detail.error_class || "—"}
            </p>
            {detail.error_class ? <CopyButton value={detail.error_class} size="inline" /> : null}
          </div>
        </div>
        <QuietBadge
          status={resolutionTone(detail.resolution)}
          label={resolutionLabel(t, detail.resolution)}
        />
      </div>

      {detail.error_message ? (
        <EvidenceBlock
          label={t("adminErrorLogs.upstreamMessage")}
          value={detail.error_message}
        />
      ) : null}

      {detail.error_body_excerpt ? (
        <EvidenceBlock
          label={t("adminErrorLogs.upstreamBodyExcerpt")}
          value={detail.error_body_excerpt}
          mono
        />
      ) : null}

      <div className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
        <Field label={t("adminErrorLogs.requestId")} value={detail.request_id || "—"} mono copyable />
        <Field label={t("adminErrorLogs.traceId")} value={detail.trace_id || "—"} mono copyable />
        <Field label={t("adminErrorLogs.model")} value={detail.model || "—"} />
        <Field label={t("adminErrorLogs.sourceEndpoint")} value={detail.source_endpoint || "—"} mono copyable />
        <Field label={t("adminErrorLogs.protocol")} value={protocol} mono />
        <Field label={t("adminErrorLogs.latency")} value={formatLatency(detail.latency_ms ?? 0)} mono />
        <Field label={t("adminErrorLogs.attempt")} value={formatInteger(detail.attempt_no ?? 1)} mono />
        <Field
          label={t("adminErrorLogs.statusCode")}
          value={detail.status_code != null ? String(detail.status_code) : "—"}
          mono
        />
        <Field label={t("adminErrorLogs.upstreamRequestId")} value={detail.upstream_request_id || "—"} mono copyable />
        <Field label={t("adminErrorLogs.errorPhase")} value={detail.error_phase || "—"} mono />
        <Field label={t("adminErrorLogs.errorOwner")} value={detail.error_owner || "—"} mono />
        <Field label={t("adminErrorLogs.errorSource")} value={detail.error_source || "—"} mono />
        <Field label={t("adminErrorLogs.inputTokens")} value={formatInteger(detail.input_tokens ?? 0)} mono />
        <Field label={t("adminErrorLogs.outputTokens")} value={formatInteger(detail.output_tokens ?? 0)} mono />
        <Field
          label={t("adminErrorLogs.usageEstimated")}
          value={detail.usage_estimated ? t("adminErrorLogs.estimated") : t("adminErrorLogs.exact")}
        />
        <Field label={t("adminErrorLogs.user")} value={userLookup.get(detail.user_id)} />
        <Field label={t("adminErrorLogs.apiKey")} value={apiKeyLookup.get(detail.api_key_id)} />
        <Field label={t("adminErrorLogs.apiKeyPrefix")} value={detail.api_key_prefix || "—"} mono copyable />
        <Field label={t("adminErrorLogs.account")} value={accountLookup.get(detail.account_id)} />
        <Field label={t("adminErrorLogs.provider")} value={providerLookup.get(detail.provider_id)} />
        <Field label={t("adminErrorLogs.time")} value={formatDateTime(detail.occurred_at)} mono />
        <Field label={t("adminErrorLogs.updatedAt")} value={formatDateTime(detail.updated_at)} mono />
        {detail.resolved_at ? (
          <Field label={t("adminErrorLogs.resolvedAt")} value={formatDateTime(detail.resolved_at)} mono />
        ) : null}
        {detail.resolved_by_user_id ? (
          <Field label={t("adminErrorLogs.resolvedBy")} value={userLookup.get(detail.resolved_by_user_id)} />
        ) : null}
      </div>

      {systemLogHref ? (
        <div className="flex justify-end">
          <Button asChild variant="outline" size="sm">
            <Link href={systemLogHref}>
              <ExternalLink aria-hidden />
              {t("adminErrorLogs.openSystemLogs")}
            </Link>
          </Button>
        </div>
      ) : null}

      <RequestLogEvidence files={requestLogQuery.data?.data ?? []} loading={requestLogQuery.isFetching} />

      <div className="flex items-center justify-between gap-3 rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
        <QuietBadge
          status={resolutionTone(detail.resolution)}
          label={resolutionLabel(t, detail.resolution)}
        />
        <button
          type="button"
          disabled={resolveMutation.isPending || !detail.id}
          onClick={() => {
            if (detail.id) resolveMutation.mutate({ id: detail.id, resolved: !isResolved });
          }}
          className="rounded-md border border-srapi-border bg-srapi-surface px-3 py-1.5 text-xs font-medium text-srapi-text-primary transition-colors hover:bg-srapi-card-muted disabled:opacity-50"
        >
          {isResolved ? t("adminErrorLogs.markUnresolved") : t("adminErrorLogs.markResolved")}
        </button>
      </div>

      {events.length > 0 ? (
        <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.attemptHistory")}
          </p>
          <ol className="mt-2 space-y-2">
            {events.map((ev, idx) => {
              const offsetMs =
                firstAt > 0 && (ev.at_unix_ms ?? 0) > 0 ? (ev.at_unix_ms ?? 0) - firstAt : 0;
              return (
                <li
                  key={`${ev.attempt_no ?? idx}-${idx}`}
                  className="rounded-md border border-srapi-border bg-srapi-surface p-3"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-2xs font-semibold text-srapi-text-primary">
                      {t("adminErrorLogs.attemptN", { n: ev.attempt_no ?? idx + 1 })}
                    </span>
                    {ev.kind ? (
                      <span className="rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary">
                        {ev.kind}
                      </span>
                    ) : null}
                    {ev.upstream_status_code != null && ev.upstream_status_code > 0 ? (
                      <span className="font-mono text-2xs text-srapi-error">
                        {ev.upstream_status_code}
                      </span>
                    ) : null}
                    {offsetMs > 0 ? (
                      <span className="font-mono text-2xs text-srapi-text-tertiary">
                        +{offsetMs}ms
                      </span>
                    ) : null}
                  </div>
                  <div className="mt-1 text-xs text-srapi-text-secondary">
                    {ev.account_name || "—"}
                    {ev.upstream_request_id ? (
                      <span className="ml-2 font-mono text-2xs text-srapi-text-tertiary">
                        · {ev.upstream_request_id}
                      </span>
                    ) : null}
                  </div>
                  {ev.message ? (
                    <p className="mt-1 break-words text-xs text-srapi-text-primary">
                      {ev.message}
                    </p>
                  ) : null}
                  {ev.body_excerpt ? (
                    <p className="mt-1 break-words font-mono text-2xs text-srapi-text-tertiary">
                      {ev.body_excerpt}
                    </p>
                  ) : null}
                </li>
              );
            })}
          </ol>
        </div>
      ) : null}
    </div>
  );
}

function RequestLogEvidence({
  files,
  loading,
}: {
  files: RequestLogFileDescriptor[];
  loading: boolean;
}) {
  const { t } = useLanguage();
  const [selected, setSelected] = useState<RequestLogFileDescriptor | null>(null);
  const downloadQuery = useAdminRequestLogFileDownload(selected?.name ?? null, selected !== null);
  const first = files[0];

  const downloadFile = useCallback(async (file: RequestLogFileDescriptor) => {
    try {
      const text = await downloadAdminRequestLogFileText(file.name);
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
      setSelected(file);
    }
  }, []);

  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.requestDump")}
          </p>
          {first ? (
            <div className="mt-1 min-w-0">
              <p className="break-all font-mono text-2xs text-srapi-text-primary">
                {first.name}
              </p>
              <p className="mt-1 text-xs text-srapi-text-tertiary">
                {formatDateTime(first.created_at)} · {formatSize(first.size)}
                {first.is_error_only ? ` · ${t("adminRequestLogFiles.errorOnly")}` : ""}
              </p>
            </div>
          ) : (
            <p className="mt-1 text-xs text-srapi-text-tertiary">
              {loading ? t("adminErrorLogs.requestDumpLoading") : t("adminErrorLogs.requestDumpMissing")}
            </p>
          )}
        </div>
        {first ? (
          <div className="flex shrink-0 gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setSelected(first)}>
              {t("adminRequestLogFiles.preview")}
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => void downloadFile(first)}>
              {t("adminRequestLogFiles.download")}
            </Button>
          </div>
        ) : null}
      </div>
      {files.length > 1 ? (
        <p className="mt-2 text-xs text-srapi-text-tertiary">
          {t("adminErrorLogs.requestDumpMore", { count: files.length - 1 })}
        </p>
      ) : null}

      {selected ? (
        <div className="mt-3 rounded-md border border-srapi-border bg-srapi-surface p-3">
          <div className="mb-2 flex items-center justify-between gap-3">
            <p className="min-w-0 break-all font-mono text-2xs text-srapi-text-tertiary">
              {selected.name}
            </p>
            <Button type="button" variant="ghost" size="sm" onClick={() => setSelected(null)}>
              {t("common.close")}
            </Button>
          </div>
          {downloadQuery.isError ? (
            <p className="text-sm text-srapi-error">
              {t("adminRequestLogFiles.detailLoadFailed")}
            </p>
          ) : (
            <pre className="max-h-[60vh] overflow-auto rounded bg-srapi-bg-input p-3 text-xs">
              {downloadQuery.data ?? ""}
            </pre>
          )}
        </div>
      ) : null}
    </div>
  );
}

function EvidenceBlock({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
      <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">{label}</p>
      <div className="mt-1 flex items-start gap-1.5">
        <p
          className={
            "min-w-0 whitespace-pre-wrap break-words text-srapi-text-primary" +
            (mono ? " font-mono text-2xs" : " text-sm")
          }
        >
          {value}
        </p>
        <CopyButton value={value} size="inline" />
      </div>
    </div>
  );
}

function Field({
  label,
  value,
  mono,
  copyable,
}: {
  label: string;
  value: string;
  mono?: boolean;
  copyable?: boolean;
}) {
  return (
    <div className="min-w-0">
      <span className="font-medium text-srapi-text-tertiary">{label}</span>
      <div className="mt-0.5 flex items-start gap-1.5">
        <p
          className={
            "min-w-0 break-all text-srapi-text-primary" +
            (mono ? " font-mono text-2xs tabular" : " text-sm")
          }
        >
          {value}
        </p>
        {copyable && value && value !== "—" ? <CopyButton value={value} size="inline" /> : null}
      </div>
    </div>
  );
}

function resolutionTone(resolution: OpsErrorLog["resolution"]): QuietStatus {
  switch (resolution) {
    case "resolved":
      return "active";
    case "investigating":
      return "limited";
    case "muted":
      return "disabled";
    default:
      return "error";
  }
}

function resolutionLabel(
  t: (key: string, vars?: Record<string, string | number>) => string,
  resolution: OpsErrorLog["resolution"],
): string {
  switch (resolution) {
    case "resolved":
      return t("adminErrorLogs.resolved");
    case "investigating":
      return t("adminErrorLogs.investigating");
    case "muted":
      return t("adminErrorLogs.muted");
    default:
      return t("adminErrorLogs.open");
  }
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}
