"use client";

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { CopyButton } from "@/components/ui/copy-button";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useAdminErrorLog, useResolveErrorLog } from "@/hooks/admin-queries";
import { useAccountNameLookup } from "@/hooks/use-account-name-lookup";
import { useApiKeyNameLookup } from "@/hooks/use-api-key-name-lookup";
import { useProviderNameLookup } from "@/hooks/use-provider-name-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatInteger, formatLatency } from "@/lib/admin-format";
import type { ErrorLog } from "@/lib/sdk-types";

export interface ErrorLogDetailDialogProps {
  /** The clicked row's id; `null` keeps the detail query disabled. */
  errorLogId: string | null;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  /** Email for the row's user, resolved by the caller's user map. */
  userEmail?: string;
}

/**
 * Admin error-log detail dialog.
 *
 * Opened by a row click on the error-logs page; fetches the full failed-request
 * record via `getAdminErrorLog` (lazy — only while `open` with an id). Layout
 * follows sub2api's `UserErrorDetailModal`: a labelled metadata grid (request id,
 * model, source/target protocol, error class, latency, tokens, attempt,
 * timestamps) over the raw record.
 */
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

function ErrorLogDetailBody({ detail }: { detail: ErrorLog }) {
  const { t } = useLanguage();
  const accountLookup = useAccountNameLookup();
  const apiKeyLookup = useApiKeyNameLookup();
  const providerLookup = useProviderNameLookup();
  const resolveMutation = useResolveErrorLog();
  const protocol = detail.target_protocol
    ? `${detail.source_protocol} → ${detail.target_protocol}`
    : detail.source_protocol;
  const events = detail.upstream_errors ?? [];
  const firstAt = events.length > 0 ? events[0]?.at_unix_ms ?? 0 : 0;

  return (
    <div className="space-y-4">
      {/* Error class banner */}
      <div className="flex items-center justify-between gap-3 rounded-xl border border-srapi-border bg-srapi-card-muted p-4">
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
          status={detail.usage_estimated ? "limited" : "active"}
          label={detail.usage_estimated ? t("adminErrorLogs.estimated") : t("adminErrorLogs.exact")}
        />
      </div>

      {/* Upstream verbatim message — sub2api parity (ops_error_logs.upstream_error_message).
          Surfaced verbatim so operators can see what the provider actually returned
          instead of srapi's generic class-level substitution. */}
      {detail.error_message ? (
        <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-4">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.upstreamMessage")}
          </p>
          <div className="mt-1 flex items-start gap-1.5">
            <p className="min-w-0 whitespace-pre-wrap break-words text-sm text-srapi-text-primary">
              {detail.error_message}
            </p>
            <CopyButton value={detail.error_message} size="inline" />
          </div>
        </div>
      ) : null}

      {/* Upstream body excerpt (compacted envelope) — sub2api parity for
          upstream_error_detail. */}
      {detail.error_body_excerpt ? (
        <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-4">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.upstreamBodyExcerpt")}
          </p>
          <div className="mt-1 flex items-start gap-1.5">
            <p className="min-w-0 whitespace-pre-wrap break-words font-mono text-2xs text-srapi-text-primary">
              {detail.error_body_excerpt}
            </p>
            <CopyButton value={detail.error_body_excerpt} size="inline" />
          </div>
        </div>
      ) : null}

      {/* Metadata grid */}
      <div className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
        <Field label={t("adminErrorLogs.requestId")} value={detail.request_id} mono copyable />
        <Field label={t("adminErrorLogs.model")} value={detail.model || "—"} />
        <Field label={t("adminErrorLogs.sourceEndpoint")} value={detail.source_endpoint || "—"} mono copyable />
        <Field label={t("adminErrorLogs.protocol")} value={protocol} mono />
        <Field label={t("adminErrorLogs.latency")} value={formatLatency(detail.latency_ms)} mono />
        <Field label={t("adminErrorLogs.attempt")} value={formatInteger(detail.attempt_no)} mono />
        <Field
          label={t("adminErrorLogs.statusCode")}
          value={detail.status_code != null ? String(detail.status_code) : "—"}
          mono
        />
        <Field
          label={t("adminErrorLogs.upstreamRequestId")}
          value={detail.upstream_request_id || "—"}
          mono
          copyable
        />
        <Field
          label={t("adminErrorLogs.errorPhase")}
          value={detail.error_phase || "—"}
          mono
        />
        <Field
          label={t("adminErrorLogs.errorOwner")}
          value={detail.error_owner || "—"}
          mono
        />
        <Field
          label={t("adminErrorLogs.errorSource")}
          value={detail.error_source || "—"}
          mono
        />
        <Field label={t("adminErrorLogs.inputTokens")} value={formatInteger(detail.input_tokens)} mono />
        <Field label={t("adminErrorLogs.outputTokens")} value={formatInteger(detail.output_tokens)} mono />
        <Field label={t("adminErrorLogs.apiKey")} value={apiKeyLookup.get(detail.api_key_id)} />
        <Field label={t("adminErrorLogs.account")} value={accountLookup.get(detail.account_id)} />
        <Field label={t("adminErrorLogs.provider")} value={providerLookup.get(detail.provider_id)} />
        <Field label={t("adminErrorLogs.time")} value={formatDateTime(detail.created_at)} mono />
        {detail.resolved_at ? (
          <Field
            label={t("adminErrorLogs.resolvedAt")}
            value={formatDateTime(detail.resolved_at)}
            mono
          />
        ) : null}
        {detail.resolved_by ? (
          <Field label={t("adminErrorLogs.resolvedBy")} value={detail.resolved_by} mono />
        ) : null}
      </div>

      {/* Resolve toggle */}
      <div className="flex items-center justify-between gap-3 rounded-xl border border-srapi-border bg-srapi-card-muted p-4">
        <div className="min-w-0">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {detail.resolved ? t("adminErrorLogs.resolved") : t("adminErrorLogs.unresolved")}
          </p>
        </div>
        <button
          type="button"
          disabled={resolveMutation.isPending}
          onClick={() =>
            resolveMutation.mutate({ id: detail.id, resolved: !detail.resolved })
          }
          className="rounded-md border border-srapi-border bg-srapi-surface px-3 py-1.5 text-xs font-medium text-srapi-text-primary transition-colors hover:bg-srapi-card-muted disabled:opacity-50"
        >
          {detail.resolved
            ? t("adminErrorLogs.markUnresolved")
            : t("adminErrorLogs.markResolved")}
        </button>
      </div>

      {/* Per-attempt timeline */}
      {events.length > 0 ? (
        <div className="rounded-xl border border-srapi-border bg-srapi-card-muted p-4">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.attemptHistory")}
          </p>
          <ol className="mt-2 space-y-2">
            {events.map((ev, idx) => {
              const offsetMs =
                firstAt > 0 && ev.at_unix_ms > 0 ? ev.at_unix_ms - firstAt : 0;
              return (
                <li
                  key={`${ev.attempt_no}-${idx}`}
                  className="rounded-md border border-srapi-border bg-srapi-surface p-3"
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-2xs font-semibold text-srapi-text-primary">
                      {t("adminErrorLogs.attemptN", { n: ev.attempt_no })}
                    </span>
                    {ev.kind ? (
                      <span className="rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary">
                        {ev.kind}
                      </span>
                    ) : null}
                    {ev.upstream_status_code > 0 ? (
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

/** Labelled metadata cell: caption above value, optional mono for ids/codes. */
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
