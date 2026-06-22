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
import { DataPill } from "@/components/ui/data-pill";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  InlineDetailGrid,
  type InlineDetailRow,
  type InlineDetailSection,
} from "@/components/ui/inline-detail-grid";
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

type DetailTab = "request" | "response" | "diagnosis" | "related";

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
          <DialogTitle className="text-lg font-semibold tracking-tight">
            {t("adminErrorLogs.detailTitle")}
            {userEmail ? (
              <span className="text-srapi-text-tertiary"> · {userEmail}</span>
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
  const [tab, setTab] = useState<DetailTab>("request");
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

  const requestSections: InlineDetailSection[] = [
    {
      title: t("adminErrorLogs.requestId"),
      rows: [
        rowText(t("adminErrorLogs.requestId"), detail.request_id || "—", { mono: true }),
        rowText(t("adminErrorLogs.traceId"), detail.trace_id || "—", { mono: true }),
        rowText(t("adminErrorLogs.upstreamRequestId"), detail.upstream_request_id || "—", { mono: true }),
      ],
    },
    {
      title: t("adminErrorLogs.sourceEndpoint"),
      rows: [
        rowText(t("adminErrorLogs.sourceEndpoint"), detail.source_endpoint || "—", { mono: true }),
        rowText(t("adminErrorLogs.protocol"), protocol, { mono: true }),
        rowText(t("adminErrorLogs.model"), detail.model || "—"),
      ],
    },
    {
      title: t("adminErrorLogs.user"),
      rows: [
        rowText(t("adminErrorLogs.user"), userLookup.get(detail.user_id)),
        rowText(t("adminErrorLogs.apiKey"), apiKeyLookup.get(detail.api_key_id)),
        rowText(t("adminErrorLogs.apiKeyPrefix"), detail.api_key_prefix || "—", { mono: true }),
        rowText(t("adminErrorLogs.account"), accountLookup.get(detail.account_id)),
        rowText(t("adminErrorLogs.provider"), providerLookup.get(detail.provider_id)),
      ],
    },
  ];

  const responseSections: InlineDetailSection[] = [
    {
      title: t("adminErrorLogs.statusCode"),
      rows: [
        {
          label: t("adminErrorLogs.statusCode"),
          value: detail.status_code != null ? String(detail.status_code) : "—",
          mono: true,
          tone:
            detail.status_code != null && detail.status_code >= 500
              ? "error"
              : detail.status_code != null && detail.status_code >= 400
                ? "warning"
                : "default",
        },
        rowText(t("adminErrorLogs.latency"), formatLatency(detail.latency_ms ?? 0), { mono: true }),
        rowText(t("adminErrorLogs.attempt"), formatInteger(detail.attempt_no ?? 1), { mono: true }),
        rowText(t("adminErrorLogs.streamCompletionState"), detail.stream_completion_state || "—", { mono: true }),
      ],
    },
    {
      title: t("adminErrorLogs.errorClass"),
      rows: [
        rowText(t("adminErrorLogs.errorClass"), detail.error_class || "—", { mono: true }),
        rowText(t("adminErrorLogs.errorPhase"), detail.error_phase || "—", { mono: true }),
        rowText(t("adminErrorLogs.errorOwner"), detail.error_owner || "—", { mono: true }),
        rowText(t("adminErrorLogs.errorSource"), detail.error_source || "—", { mono: true }),
      ],
    },
    {
      title: t("adminErrorLogs.inputTokens"),
      rows: [
        rowText(t("adminErrorLogs.inputTokens"), formatInteger(detail.input_tokens ?? 0), { mono: true }),
        rowText(t("adminErrorLogs.outputTokens"), formatInteger(detail.output_tokens ?? 0), { mono: true }),
        rowText(
          t("adminErrorLogs.usageEstimated"),
          detail.usage_estimated ? t("adminErrorLogs.estimated") : t("adminErrorLogs.exact"),
          { tone: "muted" },
        ),
        rowText(t("adminErrorLogs.time"), formatDateTime(detail.occurred_at), { mono: true }),
        rowText(t("adminErrorLogs.updatedAt"), formatDateTime(detail.updated_at), { mono: true }),
      ],
    },
  ];

  return (
    <div className="space-y-4">
      {/* Hero: error class + resolution chip */}
      <div className="flex items-center justify-between gap-3 rounded-2xl border border-srapi-border bg-srapi-card-muted p-4">
        <div className="min-w-0">
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminErrorLogs.errorClass")}
          </p>
          <div className="mt-1 flex items-center gap-1.5">
            <p className="truncate text-sm font-medium text-srapi-text-primary">
              {detail.error_class || "—"}
            </p>
            {detail.error_class ? <CopyButton value={detail.error_class} size="inline" /> : null}
          </div>
          <div className="mt-2 flex flex-wrap gap-1.5">
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
        <QuietBadge
          status={resolutionTone(detail.resolution)}
          label={resolutionLabel(t, detail.resolution)}
        />
      </div>

      {/* Sticky tabs */}
      <Tabs value={tab} onValueChange={(v) => setTab(v as DetailTab)}>
        <div className="sticky top-0 z-10 -mx-1 bg-srapi-card/95 px-1 py-1 backdrop-blur supports-[backdrop-filter]:bg-srapi-card/80">
          <TabsList className="flex flex-wrap">
            <TabsTrigger value="request">{t("adminErrorLogs.requestTab") || "Request"}</TabsTrigger>
            <TabsTrigger value="response">{t("adminErrorLogs.responseTab") || "Response"}</TabsTrigger>
            <TabsTrigger value="diagnosis">{t("adminErrorLogs.diagnosisTab") || "Diagnosis"}</TabsTrigger>
            <TabsTrigger value="related">
              {t("adminErrorLogs.relatedTab") || "Related"}
              {events.length > 0 ? (
                <DataPill tone="neutral" size="sm" className="ml-2">
                  <span className="tabular">{events.length}</span>
                </DataPill>
              ) : null}
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="request">
          <div className="overflow-hidden rounded-2xl border border-srapi-border bg-srapi-card">
            <InlineDetailGrid sections={requestSections} />
          </div>
        </TabsContent>

        <TabsContent value="response">
          <div className="space-y-3">
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
            <div className="overflow-hidden rounded-2xl border border-srapi-border bg-srapi-card">
              <InlineDetailGrid sections={responseSections} />
            </div>
          </div>
        </TabsContent>

        <TabsContent value="diagnosis">
          <div className="space-y-3">
            {schedulerDiagnostic ? (
              <SchedulerDiagnosticSummary
                diagnostic={schedulerDiagnostic}
                requestID={detail.request_id}
              />
            ) : null}
            {upstreamDiagnostic ? (
              <UpstreamErrorDiagnosticSummary diagnostic={upstreamDiagnostic} />
            ) : null}
            <ErrorLogTriageSummary detail={detail} triage={triage} />
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
            {detail.resolved_at || detail.resolved_by_user_id ? (
              <div className="rounded-2xl border border-srapi-border bg-srapi-card p-4">
                <InlineDetailGrid
                  sections={[
                    {
                      title: t("adminErrorLogs.resolved"),
                      rows: [
                        ...(detail.resolved_at
                          ? [rowText(t("adminErrorLogs.resolvedAt"), formatDateTime(detail.resolved_at), { mono: true })]
                          : []),
                        ...(detail.resolved_by_user_id
                          ? [rowText(t("adminErrorLogs.resolvedBy"), userLookup.get(detail.resolved_by_user_id))]
                          : []),
                      ],
                    },
                  ]}
                />
              </div>
            ) : null}
          </div>
        </TabsContent>

        <TabsContent value="related">
          <div className="space-y-3">
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
            {events.length > 0 ? (
              <AttemptHistory
                events={events}
                firstAt={firstAt}
                attemptSummary={attemptSummary}
              />
            ) : null}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}

function rowText(
  label: string,
  value: string,
  opts?: { mono?: boolean; tone?: InlineDetailRow["tone"] },
): InlineDetailRow {
  return { label, value, mono: opts?.mono, tone: opts?.tone };
}

function AttemptHistory({
  events,
  firstAt,
  attemptSummary,
}: {
  events: UpstreamErrorEvent[];
  firstAt: number;
  attemptSummary: ReturnType<typeof summarizeUpstreamAttempts>;
}) {
  const { t } = useLanguage();
  return (
    <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
          {t("adminErrorLogs.attemptHistory")}
        </p>
        <DataPill tone="neutral" size="sm">
          <span className="tabular">{formatInteger(attemptSummary.attempts)}</span>{" "}
          {t("adminErrorLogs.attempt").toLowerCase()}
        </DataPill>
        {attemptSummary.statuses ? (
          <DataPill tone="warning" size="sm">
            <span className="tabular">{attemptSummary.statuses}</span>
          </DataPill>
        ) : null}
        {attemptSummary.targetCount > 0 ? (
          <DataPill tone="neutral" size="sm">
            <span className="tabular">{formatInteger(attemptSummary.targetCount)}</span>{" "}
            {t("adminErrorLogs.upstreamTargetCount").toLowerCase()}
          </DataPill>
        ) : null}
      </div>

      <ol className="mt-3 space-y-2">
        {events.map((ev, idx) => {
          const offsetMs =
            firstAt > 0 && (ev.at_unix_ms ?? 0) > 0 ? (ev.at_unix_ms ?? 0) - firstAt : 0;
          const eventDiagnostic = parseUpstreamErrorDiagnostic(ev.body_excerpt);
          const upstreamTarget = formatUpstreamTarget(ev.upstream_url);
          const sev: "error" | "warning" | "info" =
            ev.upstream_status_code != null && ev.upstream_status_code >= 500
              ? "error"
              : ev.upstream_status_code != null && ev.upstream_status_code >= 400
                ? "warning"
                : "info";
          return (
            <li
              key={`${ev.attempt_no ?? idx}-${idx}`}
              className="log-row rounded-md border border-srapi-border bg-srapi-card p-3"
              data-sev={sev}
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-[11px] font-semibold text-srapi-text-primary">
                  {t("adminErrorLogs.attemptN", { n: ev.attempt_no ?? idx + 1 })}
                </span>
                {ev.kind ? (
                  <DataPill tone="neutral" size="sm">{ev.kind}</DataPill>
                ) : null}
                {ev.upstream_status_code != null && ev.upstream_status_code > 0 ? (
                  <DataPill tone="error" size="sm">
                    <span className="tabular">{ev.upstream_status_code}</span>
                  </DataPill>
                ) : null}
                {offsetMs > 0 ? (
                  <span className="font-mono text-[11px] tabular text-srapi-text-tertiary">
                    +{offsetMs}ms
                  </span>
                ) : null}
              </div>
              <div className="mt-1 text-xs text-srapi-text-secondary">
                {ev.account_name || "—"}
                {ev.upstream_request_id ? (
                  <span className="ml-2 font-mono text-[11px] text-srapi-text-tertiary">
                    · {ev.upstream_request_id}
                  </span>
                ) : null}
              </div>
              {upstreamTarget ? (
                <div className="mt-1 flex items-start gap-1.5 text-xs text-srapi-text-secondary">
                  <span className="shrink-0 font-medium">
                    {t("adminErrorLogs.upstreamTarget")}:
                  </span>
                  <span className="min-w-0 break-all font-mono text-[11px] text-srapi-text-tertiary">
                    {upstreamTarget}
                  </span>
                  <CopyButton value={upstreamTarget} size="inline" />
                </div>
              ) : null}
              {ev.message ? (
                <p className="mt-1 break-words text-xs text-srapi-text-primary">{ev.message}</p>
              ) : null}
              {eventDiagnostic ? <UpstreamDiagnosticPills diagnostic={eventDiagnostic} /> : null}
              {ev.body_excerpt ? (
                <p className="mt-1 break-words font-mono text-[11px] text-srapi-text-tertiary">
                  {ev.body_excerpt}
                </p>
              ) : null}
            </li>
          );
        })}
      </ol>
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
    <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
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
              <span className="font-mono text-[11px] tabular text-srapi-text-tertiary">
                {index + 1}
              </span>
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
    <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
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
              className="log-row rounded-md border border-srapi-border bg-srapi-card px-3 py-2"
              data-sev={systemLogSeverity(log.level)}
            >
              <div className="flex flex-wrap items-center gap-2">
                <QuietBadge status={systemLogTone(log.level)} label={log.level} />
                <span className="font-mono text-[11px] tabular text-srapi-text-tertiary">
                  {formatDateTime(log.created_at)}
                </span>
                <span className="text-[11px] text-srapi-text-tertiary">{log.source || "—"}</span>
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
    <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminErrorLogs.requestDump")}
          </p>
          {first ? (
            <div className="mt-1 min-w-0">
              <p className="break-all font-mono text-[11px] text-srapi-text-primary">
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
        <div className="mt-3 rounded-md border border-srapi-border bg-srapi-card p-3">
          <div className="mb-2 flex items-center justify-between gap-3">
            <p className="min-w-0 break-all font-mono text-[11px] text-srapi-text-tertiary">
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
              <pre className="max-h-[60vh] overflow-auto rounded bg-srapi-card p-3 text-xs">
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
    <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted p-4">
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
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
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
                    : "border-srapi-border bg-srapi-card text-srapi-text-secondary hover:bg-srapi-card-muted")
                }
              >
                {option.label}
              </button>
            ))}
          </div>
        </div>
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminErrorLogs.previousResolution")}
          </p>
          <p className="mt-1 text-sm text-srapi-text-secondary">{resolutionLabel(t, current)}</p>
        </div>
      </div>

      <div className="mt-3">
        <label className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
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
    <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted p-4">
      <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{label}</p>
      <div className="mt-1 flex items-start gap-1.5">
        <p
          className={
            "min-w-0 whitespace-pre-wrap break-words text-srapi-text-primary" +
            (mono ? " font-mono text-[11px]" : " text-sm")
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

  const sections: InlineDetailSection[] = [
    {
      title: t("adminErrorLogs.schedulerDiagnostic"),
      rows: [
        rowText(
          t("adminErrorLogs.schedulerDecisionId"),
          diagnostic.decisionId != null ? formatInteger(diagnostic.decisionId) : "—",
          { mono: true },
        ),
        rowText(
          t("adminErrorLogs.schedulerCandidates"),
          diagnostic.candidateCount != null ? formatInteger(diagnostic.candidateCount) : "—",
          { mono: true },
        ),
        rowText(
          t("adminErrorLogs.schedulerRejected"),
          diagnostic.rejectedCount != null ? formatInteger(diagnostic.rejectedCount) : "—",
          { mono: true },
        ),
        rowText(
          t("adminErrorLogs.responseStatus"),
          diagnostic.responseStatus != null ? String(diagnostic.responseStatus) : "—",
          { mono: true },
        ),
      ],
    },
  ];

  return (
    <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminErrorLogs.schedulerDiagnostic")}
          </p>
          <p className="mt-1 text-sm text-srapi-text-primary">
            {diagnostic.primaryReason || "—"}
            {diagnostic.primaryCount != null ? (
              <span className="ml-2 font-mono text-[11px] tabular text-srapi-text-tertiary">
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

      <div className="mt-3 overflow-hidden rounded-xl border border-srapi-border bg-srapi-card">
        <InlineDetailGrid sections={sections} />
      </div>

      {topReasons.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-1.5">
          {topReasons.map((item) => (
            <DataPill key={item.reason} tone="neutral" size="sm">
              <span>{item.reason}</span>
              <span className="font-mono tabular text-srapi-text-tertiary">
                ({formatInteger(item.count)})
              </span>
            </DataPill>
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
    <div className="rounded-2xl border border-srapi-border bg-srapi-card-muted p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminErrorLogs.upstreamDiagnostic")}
          </p>
          {diagnostic.message ? (
            <p className="mt-1 break-words text-sm text-srapi-text-primary">{diagnostic.message}</p>
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
        <DataPill key={part} tone="neutral" size="sm">
          {part}
        </DataPill>
      ))}
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

function systemLogSeverity(level: OpsSystemLog["level"]): "error" | "warning" | "info" | "success" {
  switch (level) {
    case "error":
      return "error";
    case "warn":
      return "warning";
    case "info":
      return "info";
    default:
      return "info";
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
