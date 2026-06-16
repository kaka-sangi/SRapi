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
import { useAdminErrorLog } from "@/hooks/admin-queries";
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
  const protocol = detail.target_protocol
    ? `${detail.source_protocol} → ${detail.target_protocol}`
    : detail.source_protocol;

  return (
    <div className="space-y-4">
      {/* Error class banner */}
      <div className="flex items-center justify-between gap-3 rounded-xl border border-srapi-border bg-srapi-card-muted p-4">
        <div className="min-w-0">
          <p className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("adminErrorLogs.errorClass")}
          </p>
          <p className="mt-1 truncate text-sm font-medium text-srapi-text-primary">
            {detail.error_class || "—"}
          </p>
        </div>
        <QuietBadge
          status={detail.usage_estimated ? "limited" : "active"}
          label={detail.usage_estimated ? t("adminErrorLogs.estimated") : t("adminErrorLogs.exact")}
        />
      </div>

      {/* Metadata grid */}
      <div className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
        <Field label={t("adminErrorLogs.requestId")} value={detail.request_id} mono copyable />
        <Field label={t("adminErrorLogs.model")} value={detail.model || "—"} />
        <Field label={t("adminErrorLogs.sourceEndpoint")} value={detail.source_endpoint || "—"} mono copyable />
        <Field label={t("adminErrorLogs.protocol")} value={protocol} mono />
        <Field label={t("adminErrorLogs.latency")} value={formatLatency(detail.latency_ms)} mono />
        <Field label={t("adminErrorLogs.attempt")} value={formatInteger(detail.attempt_no)} mono />
        <Field label={t("adminErrorLogs.inputTokens")} value={formatInteger(detail.input_tokens)} mono />
        <Field label={t("adminErrorLogs.outputTokens")} value={formatInteger(detail.output_tokens)} mono />
        <Field label={t("adminErrorLogs.apiKey")} value={apiKeyLookup.get(detail.api_key_id)} />
        <Field label={t("adminErrorLogs.account")} value={accountLookup.get(detail.account_id)} />
        <Field label={t("adminErrorLogs.provider")} value={providerLookup.get(detail.provider_id)} />
        <Field label={t("adminErrorLogs.time")} value={formatDateTime(detail.created_at)} mono />
      </div>
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
