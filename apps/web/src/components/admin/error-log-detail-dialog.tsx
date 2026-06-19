"use client";

import { useCallback, useMemo, useState } from "react";
import Link from "next/link";
import { ExternalLink } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { CopyButton } from "@/components/ui/copy-button";
import { Textarea } from "@/components/ui/textarea";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import { RequestDumpSummaryGrid } from "@/components/admin/request-log-dump-summary-panel";
import {
  downloadAdminRequestLogFileText,
  useAdminErrorLog,
  useAdminRequestLogFileDownload,
  useAdminRequestLogFiles,
  useOpsSystemLogs,
  useUpdateErrorLogResolution,
} from "@/hooks/admin-queries";
import { useAccountNameLookup } from "@/hooks/use-account-name-lookup";
import { useApiKeyNameLookup } from "@/hooks/use-api-key-name-lookup";
import { useProviderNameLookup } from "@/hooks/use-provider-name-lookup";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatInteger, formatLatency } from "@/lib/admin-format";
import {
  buildErrorLogTriage,
  type ErrorLogTriageLinkKind,
} from "@/lib/admin-error-log-triage";
import {
  adminRequestDumpsHref,
  adminSchedulerDecisionsHref,
  adminSystemLogsHref,
} from "@/lib/admin-log-links";
import { parseRequestDumpSummary } from "@/lib/request-log-dump-summary";
import {
  parseSchedulerDiagnostic,
  type SchedulerDiagnostic,
} from "@/lib/scheduler-diagnostic";
import {
  diagnosticParts,
  parseUpstreamErrorDiagnostic,
  resolveUpstreamErrorDiagnostic,
  type UpstreamErrorDiagnostic,
} from "@/lib/upstream-error-diagnostic";
import type { OpsErrorLog, OpsSystemLog, RequestLogFileDescriptor } from "@/lib/sdk-types";

const RESOLUTION_OPTIONS = ["open", "investigating", "resolved", "muted"] as const;
type ResolutionValue = (typeof RESOLUTION_OPTIONS)[number];
type UpstreamErrorEvent = NonNullable<OpsErrorLog["upstream_errors"]>[number];

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
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>
            {t("adminErrorLogs.detailTitle")}
            {userEmail ? (
              <span className="font-mono text-srapi-text-tertiary"> · {userEmail}</span>
            ) : null}
          </DialogTitle>
          <DialogDescription className="sr-only">{t("adminErrorLogs.subtitle")}</DialogDescription>
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
  const requestLogQuery = useAdminRequestLogFiles(
    { request_id: detail.request_id || undefined, limit: 3 },
    Boolean(detail.request_id),
  );
  const systemLogLookup = detail.request_id
    ? { request_id: detail.request_id, page: 1, page_size: 5 }
    : { trace_id: detail.trace_id || undefined, page: 1, page_size: 5 };
  const systemLogQuery = useOpsSystemLogs(systemLogLookup, Boolean(detail.request_id || detail.trace_id));
  const protocol = detail.target_protocol
    ? `${detail.source_protocol ?? detail.platform ?? "—"} → ${detail.target_protocol}`
    : detail.source_protocol ?? detail.platform ?? "—";
  const events = detail.upstream_errors ?? [];
  const firstAt = events.length > 0 ? events[0]?.at_unix_ms ?? 0 : 0;
  const attemptSummary = summarizeUpstreamAttempts(events);
  const resolutionMutation = useUpdateErrorLogResolution();
  const schedulerDiagnostic = parseSchedulerDiagnostic(detail.error_body_excerpt);
  const upstreamDiagnostic = schedulerDiagnostic
    ? null
    : resolveUpstreamErrorDiagnostic({
        errorBodyExcerpt: detail.error_body_excerpt,
        upstreamErrors: events,
      });
  const triage = useMemo(() => buildErrorLogTriage(detail), [detail]);

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

      {schedulerDiagnostic ? (
        <SchedulerDiagnosticSummary diagnostic={schedulerDiagnostic} requestID={detail.request_id} />
      ) : null}

      {upstreamDiagnostic ? <UpstreamErrorDiagnosticSummary diagnostic={upstreamDiagnostic} /> : null}

      <ErrorLogTriageSummary detail={detail} triage={triage} />

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

      <SystemLogEvidence
        logs={systemLogQuery.data?.data ?? []}
        loading={systemLogQuery.isFetching}
        requestID={detail.request_id}
        traceID={detail.trace_id}
        total={systemLogQuery.data?.pagination?.total}
        requestEvidenceHref={triage.links.find((link) => link.kind === "requestEvidence")?.href}
      />

      <RequestLogEvidence
        files={requestLogQuery.data?.data ?? []}
        loading={requestLogQuery.isFetching}
        requestID={detail.request_id}
        total={requestLogQuery.data?.pagination?.total}
      />

      <ResolutionEditorShell
        key={`${detail.id ?? ""}:${detail.resolution ?? "open"}:${detail.resolution_note ?? ""}`}
        current={detail.resolution ?? "open"}
        note={detail.resolution_note ?? ""}
        pending={resolutionMutation.isPending || !detail.id}
        onSubmit={(resolution, note) => {
          if (!detail.id) return;
          resolutionMutation.mutate({
            id: detail.id,
            resolution,
            note: note.trim() || undefined,
          });
        }}
      />

      {events.length > 0 ? (
        <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.attemptHistory")}
          </p>
          <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <Field
              label={t("adminErrorLogs.attempt")}
              value={formatInteger(attemptSummary.attempts)}
              mono
            />
            <Field
              label={t("adminErrorLogs.statusCode")}
              value={attemptSummary.statuses || "—"}
              mono
            />
            <Field
              label={t("adminErrorLogs.account")}
              value={attemptSummary.accounts || "—"}
            />
            <Field
              label={t("adminErrorLogs.upstreamTargetCount")}
              value={
                attemptSummary.targetCount > 0
                  ? formatInteger(attemptSummary.targetCount)
                  : "—"
              }
              mono
            />
          </div>
          <ol className="mt-2 space-y-2">
            {events.map((ev, idx) => {
              const offsetMs =
                firstAt > 0 && (ev.at_unix_ms ?? 0) > 0 ? (ev.at_unix_ms ?? 0) - firstAt : 0;
              const eventDiagnostic = parseUpstreamErrorDiagnostic(ev.body_excerpt);
              const upstreamTarget = formatUpstreamTarget(ev.upstream_url);
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
                  {upstreamTarget ? (
                    <div className="mt-1 flex items-start gap-1.5 text-xs text-srapi-text-secondary">
                      <span className="shrink-0 font-medium">
                        {t("adminErrorLogs.upstreamTarget")}:
                      </span>
                      <span className="min-w-0 break-all font-mono text-2xs text-srapi-text-tertiary">
                        {upstreamTarget}
                      </span>
                      <CopyButton value={upstreamTarget} size="inline" />
                    </div>
                  ) : null}
                  {ev.message ? (
                    <p className="mt-1 break-words text-xs text-srapi-text-primary">
                      {ev.message}
                    </p>
                  ) : null}
                  {eventDiagnostic ? <UpstreamDiagnosticPills diagnostic={eventDiagnostic} /> : null}
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

function ErrorLogTriageSummary({
  detail,
  triage,
}: {
  detail: OpsErrorLog;
  triage: ReturnType<typeof buildErrorLogTriage>;
}) {
  const { t } = useLanguage();
  if (triage.steps.length === 0 && triage.links.length === 0) return null;

  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminOps.runbook.title")}
          </p>
          <div className="mt-1 flex flex-wrap gap-1.5">
            {detail.error_owner ? (
              <QuietBadge status={ownerTone(detail.error_owner)} label={detail.error_owner} />
            ) : null}
            {detail.error_phase ? (
              <QuietBadge status="disabled" label={detail.error_phase} />
            ) : null}
            {detail.status_code != null ? (
              <QuietBadge status={statusTone(detail.status_code)} label={String(detail.status_code)} />
            ) : null}
          </div>
        </div>
        {triage.links.length > 0 ? (
          <div className="flex shrink-0 flex-wrap justify-end gap-2">
            {triage.links.slice(0, 4).map((link) => (
              <Button key={`${link.kind}:${link.href}`} asChild variant="ghost" size="sm">
                <Link href={link.href}>
                  <ExternalLink aria-hidden />
                  {triageLinkLabel(t, link.kind)}
                </Link>
              </Button>
            ))}
          </div>
        ) : null}
      </div>

      {triage.steps.length > 0 ? (
        <ol className="mt-3 grid gap-1.5">
          {triage.steps.slice(0, 4).map((step, index) => (
            <li key={step} className="flex gap-2 text-xs text-srapi-text-secondary">
              <span className="font-mono text-2xs text-srapi-text-tertiary">{index + 1}</span>
              <span>{t(`adminOps.runbook.steps.${step}`)}</span>
            </li>
          ))}
        </ol>
      ) : null}
    </div>
  );
}

function SystemLogEvidence({
  logs,
  loading,
  requestID,
  traceID,
  total,
  requestEvidenceHref,
}: {
  logs: OpsSystemLog[];
  loading: boolean;
  requestID?: string | null;
  traceID?: string | null;
  total?: number;
  requestEvidenceHref?: string | null;
}) {
  const { t } = useLanguage();
  const systemLogHref = adminSystemLogsHref(
    requestID ? { request_id: requestID } : { trace_id: traceID },
  );
  const relatedTotal = Math.max(total ?? logs.length, logs.length);
  const remaining = logs.length > 0 ? Math.max(relatedTotal - logs.length, 0) : 0;

  if (!systemLogHref && !requestEvidenceHref) return null;

  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.systemLogs")}
          </p>
          <p className="mt-1 text-xs text-srapi-text-tertiary">
            {loading && logs.length === 0
              ? t("adminErrorLogs.systemLogLoading")
              : logs.length === 0
                ? t("adminErrorLogs.systemLogMissing")
                : t("adminErrorLogs.systemLogCount", { count: relatedTotal })}
          </p>
        </div>
        <div className="flex shrink-0 flex-wrap justify-end gap-2">
          {systemLogHref ? (
            <Button asChild variant="outline" size="sm">
              <Link href={systemLogHref}>
                <ExternalLink aria-hidden />
                {t("adminErrorLogs.openSystemLogs")}
              </Link>
            </Button>
          ) : null}
          {requestEvidenceHref ? (
            <Button asChild variant="ghost" size="sm">
              <Link href={requestEvidenceHref}>
                <ExternalLink aria-hidden />
                {t("adminErrorLogs.openRequestEvidence")}
              </Link>
            </Button>
          ) : null}
        </div>
      </div>

      {logs.length > 0 ? (
        <ol className="mt-3 space-y-2">
          {logs.map((log) => (
            <li
              key={log.id}
              className="rounded-md border border-srapi-border bg-srapi-surface px-3 py-2"
            >
              <div className="flex flex-wrap items-center gap-2">
                <QuietBadge status={systemLogTone(log.level)} label={log.level} />
                <span className="font-mono text-2xs text-srapi-text-tertiary">
                  {formatDateTime(log.created_at)}
                </span>
                <span className="font-mono text-2xs text-srapi-text-tertiary">
                  {log.source || "—"}
                </span>
              </div>
              <p className="mt-1 break-words text-xs text-srapi-text-primary">{log.message}</p>
            </li>
          ))}
        </ol>
      ) : null}

      {remaining > 0 ? (
        <p className="mt-2 text-xs text-srapi-text-tertiary">
          {t("adminErrorLogs.systemLogMore", { count: remaining })}
        </p>
      ) : null}
    </div>
  );
}

function ResolutionEditorShell({
  current,
  note,
  pending,
  onSubmit,
}: {
  current: ResolutionValue;
  note: string;
  pending: boolean;
  onSubmit: (resolution: ResolutionValue, note: string) => void;
}) {
  const [resolution, setResolution] = useState<ResolutionValue>(current);
  const [draftNote, setDraftNote] = useState(note);
  const dirty = resolution !== current || draftNote.trim() !== note.trim();

  return (
    <ResolutionEditor
      current={current}
      value={resolution}
      note={draftNote}
      pending={pending}
      dirty={dirty}
      onResolutionChange={setResolution}
      onNoteChange={setDraftNote}
      onSubmit={() => onSubmit(resolution, draftNote)}
      onReset={() => {
        setResolution(current);
        setDraftNote(note);
      }}
    />
  );
}

function RequestLogEvidence({
  files,
  loading,
  requestID,
  total,
}: {
  files: RequestLogFileDescriptor[];
  loading: boolean;
  requestID?: string | null;
  total?: number;
}) {
  const { t } = useLanguage();
  const [selected, setSelected] = useState<RequestLogFileDescriptor | null>(null);
  const downloadQuery = useAdminRequestLogFileDownload(selected?.name ?? null, selected !== null);
  const dumpSummary = useMemo(
    () => (downloadQuery.data ? parseRequestDumpSummary(downloadQuery.data) : null),
    [downloadQuery.data],
  );
  const first = files[0];
  const relatedTotal = Math.max(total ?? files.length, files.length);
  const remaining = first ? Math.max(relatedTotal - 1, 0) : 0;
  const requestDumpsHref = adminRequestDumpsHref({ request_id: requestID });

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
          <div className="flex shrink-0 flex-wrap justify-end gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setSelected(first)}>
              {t("adminRequestLogFiles.preview")}
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => void downloadFile(first)}>
              {t("adminRequestLogFiles.download")}
            </Button>
            {requestDumpsHref ? (
              <Button asChild variant="ghost" size="sm">
                <Link href={requestDumpsHref}>
                  <ExternalLink aria-hidden />
                  {t("adminErrorLogs.openRequestDumps")}
                </Link>
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      {remaining > 0 ? (
        <p className="mt-2 text-xs text-srapi-text-tertiary">
          {t("adminErrorLogs.requestDumpMore", { count: remaining })}
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
            <div className="space-y-3">
              {dumpSummary ? <RequestDumpSummaryGrid summary={dumpSummary} /> : null}
              <pre className="max-h-[60vh] overflow-auto rounded bg-srapi-bg-input p-3 text-xs">
                {downloadQuery.data ?? ""}
              </pre>
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
}

function ResolutionEditor({
  current,
  value,
  note,
  pending,
  dirty,
  onResolutionChange,
  onNoteChange,
  onSubmit,
  onReset,
}: {
  current: ResolutionValue;
  value: ResolutionValue;
  note: string;
  pending: boolean;
  dirty: boolean;
  onResolutionChange: (value: ResolutionValue) => void;
  onNoteChange: (value: string) => void;
  onSubmit: () => void;
  onReset: () => void;
}) {
  const { t } = useLanguage();
  const options = useMemo(
    () =>
      RESOLUTION_OPTIONS.map((item) => ({
        value: item,
        label: t(`adminErrorLogs.${item}`),
      })),
    [t],
  );

  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <QuietBadge status={resolutionTone(value)} label={resolutionLabel(t, value)} />
        <div className="flex flex-wrap items-center gap-2">
          <Button type="button" variant="ghost" size="sm" onClick={onReset} disabled={pending || !dirty}>
            {t("common.reset")}
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={onSubmit} disabled={pending || !dirty}>
            {t("adminErrorLogs.saveResolution")}
          </Button>
        </div>
      </div>

      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <div>
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.currentResolution")}
          </p>
          <div className="mt-1 flex flex-wrap gap-2">
            {options.map((option) => (
              <button
                key={option.value}
                type="button"
                onClick={() => onResolutionChange(option.value)}
                className={
                  "rounded-md border px-2.5 py-1 text-xs font-medium transition-colors " +
                  (value === option.value
                    ? "border-srapi-accent bg-srapi-accent/10 text-srapi-text-primary"
                    : "border-srapi-border bg-srapi-surface text-srapi-text-secondary hover:bg-srapi-card-muted")
                }
              >
                {option.label}
              </button>
            ))}
          </div>
        </div>
        <div>
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.previousResolution")}
          </p>
          <p className="mt-1 text-sm text-srapi-text-secondary">{resolutionLabel(t, current)}</p>
        </div>
      </div>

      <div className="mt-3">
        <label className="font-mono text-2xs uppercase text-srapi-text-tertiary">
          {t("adminErrorLogs.resolutionNote")}
        </label>
        <Textarea
          value={note}
          onChange={(e) => onNoteChange(e.target.value)}
          placeholder={t("adminErrorLogs.resolutionNotePlaceholder")}
          className="mt-1 min-h-20"
        />
      </div>
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

function SchedulerDiagnosticSummary({
  diagnostic,
  requestID,
}: {
  diagnostic: SchedulerDiagnostic;
  requestID?: string | null;
}) {
  const { t } = useLanguage();
  const topReasons = diagnostic.reasonCounts.slice(0, 4);
  const schedulerHref = adminSchedulerDecisionsHref({ request_id: requestID });
  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.schedulerDiagnostic")}
          </p>
          <p className="mt-1 text-sm text-srapi-text-primary">
            {diagnostic.primaryReason || "—"}
            {diagnostic.primaryCount != null ? (
              <span className="ml-2 font-mono text-2xs text-srapi-text-tertiary">
                ×{formatInteger(diagnostic.primaryCount)}
              </span>
            ) : null}
          </p>
        </div>
        <div className="flex shrink-0 flex-wrap justify-end gap-2">
          {diagnostic.operatorAction ? (
            <QuietBadge status="limited" label={diagnostic.operatorAction} />
          ) : null}
          {schedulerHref ? (
            <Button asChild variant="ghost" size="sm">
              <Link href={schedulerHref}>
                <ExternalLink aria-hidden />
                {t("adminErrorLogs.openSchedulerDecision")}
              </Link>
            </Button>
          ) : null}
        </div>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Field
          label={t("adminErrorLogs.schedulerDecisionId")}
          value={diagnostic.decisionId != null ? formatInteger(diagnostic.decisionId) : "—"}
          mono
          copyable={diagnostic.decisionId != null}
        />
        <Field
          label={t("adminErrorLogs.schedulerCandidates")}
          value={diagnostic.candidateCount != null ? formatInteger(diagnostic.candidateCount) : "—"}
          mono
        />
        <Field
          label={t("adminErrorLogs.schedulerRejected")}
          value={diagnostic.rejectedCount != null ? formatInteger(diagnostic.rejectedCount) : "—"}
          mono
        />
        <Field
          label={t("adminErrorLogs.responseStatus")}
          value={diagnostic.responseStatus != null ? String(diagnostic.responseStatus) : "—"}
          mono
        />
      </div>

      {topReasons.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {topReasons.map((item) => (
            <span
              key={item.reason}
              className="rounded bg-srapi-surface px-2 py-1 font-mono text-2xs text-srapi-text-secondary"
            >
              {item.reason}({formatInteger(item.count)})
            </span>
          ))}
        </div>
      ) : null}

      {diagnostic.selectionRationale ? (
        <p className="mt-3 break-words text-xs text-srapi-text-tertiary">
          {diagnostic.selectionRationale}
        </p>
      ) : null}
    </div>
  );
}

function UpstreamErrorDiagnosticSummary({ diagnostic }: { diagnostic: UpstreamErrorDiagnostic }) {
  const { t } = useLanguage();
  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.upstreamDiagnostic")}
          </p>
          {diagnostic.message ? (
            <p className="mt-1 break-words text-sm text-srapi-text-primary">
              {diagnostic.message}
            </p>
          ) : null}
        </div>
        {diagnostic.source === "attempt" ? (
          <QuietBadge status="limited" label={t("adminErrorLogs.attemptEvidence")} />
        ) : null}
      </div>
      <UpstreamDiagnosticPills diagnostic={diagnostic} />
    </div>
  );
}

function UpstreamDiagnosticPills({ diagnostic }: { diagnostic: UpstreamErrorDiagnostic }) {
  const parts = diagnosticParts(diagnostic);
  if (parts.length === 0) return null;
  return (
    <div className="mt-2 flex flex-wrap gap-1.5">
      {parts.map((part) => (
        <span
          key={part}
          className="rounded bg-srapi-surface px-2 py-1 font-mono text-2xs text-srapi-text-secondary"
        >
          {part}
        </span>
      ))}
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

function systemLogTone(level: OpsSystemLog["level"]): QuietStatus {
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

function ownerTone(owner: string): QuietStatus {
  switch (owner) {
    case "provider":
    case "scheduler":
    case "reverse_proxy":
      return "limited";
    case "platform":
    case "internal":
      return "error";
    case "client":
    case "business":
      return "disabled";
    default:
      return "disabled";
  }
}

function statusTone(statusCode: number): QuietStatus {
  if (statusCode >= 500) return "error";
  if (statusCode === 429 || statusCode === 408) return "limited";
  if (statusCode >= 400) return "disabled";
  return "active";
}

function triageLinkLabel(
  t: (key: string, vars?: Record<string, string | number>) => string,
  kind: ErrorLogTriageLinkKind,
): string {
  switch (kind) {
    case "systemLogs":
      return t("adminErrorLogs.openSystemLogs");
    case "requestEvidence":
      return t("adminErrorLogs.openRequestEvidence");
    case "requestDumps":
      return t("adminErrorLogs.openRequestDumps");
    case "schedulerDecision":
      return t("adminErrorLogs.openSchedulerDecision");
    case "accountHealth":
      return t("adminOps.evidence.accountHealth");
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

function summarizeUpstreamAttempts(events: UpstreamErrorEvent[]): {
  attempts: number;
  statuses: string;
  accounts: string;
  targetCount: number;
} {
  return {
    attempts: events.length,
    statuses: countLabels(
      events
        .map((event) => event.upstream_status_code)
        .filter((status) => status != null && status > 0)
        .map((status) => String(status)),
    ),
    accounts: countLabels(
      events
        .map((event) => event.account_name || event.account_id || "")
        .filter((account) => account.trim() !== ""),
    ),
    targetCount: new Set(
      events
        .map((event) => formatUpstreamTarget(event.upstream_url))
        .filter((target) => target !== ""),
    ).size,
  };
}

function countLabels(values: string[]): string {
  const counts = new Map<string, number>();
  for (const value of values) {
    counts.set(value, (counts.get(value) ?? 0) + 1);
  }

  return Array.from(counts.entries())
    .slice(0, 4)
    .map(([value, count]) => (count > 1 ? `${value}×${formatInteger(count)}` : value))
    .join(", ");
}

function formatUpstreamTarget(raw?: string | null): string {
  const value = raw?.trim();
  if (!value) return "";

  try {
    const parsed = new URL(value);
    return `${parsed.host}${parsed.pathname || "/"}`;
  } catch {
    return value
      .split("#", 1)[0]
      .split("?", 1)[0]
      .replace(/^[a-z][a-z0-9+.-]*:\/\//i, "");
  }
}
