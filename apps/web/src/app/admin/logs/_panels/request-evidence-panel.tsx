"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import { Activity, Bug, FileText, Route, ScrollText, X } from "lucide-react";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { FilterSelect, ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { PageHeader } from "@/components/layout/page-header";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { Input } from "@/components/ui/input";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useOpsRequestEvidence, useOpsRequestEvidenceDetail } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatInteger, formatLatency } from "@/lib/admin-format";
import {
  adminErrorLogsHref,
  adminRequestDumpsHref,
  adminSchedulerDecisionsHref,
  adminSystemLogsHref,
} from "@/lib/admin-log-links";
import {
  LOG_WINDOW_PRESETS,
  LOG_WINDOW_ALL_LABEL_KEY,
  logWindowSinceAt,
} from "@/lib/log-window-filter";
import type { RequestEvidenceDetailResponse, RequestEvidenceRow } from "@/lib/sdk-types";

export function RequestEvidencePanel() {
  const { t } = useLanguage();
  const list = useAdminList({ pageSize: 50 });
  const [detailRequestID, setDetailRequestID] = useState<string | undefined>();
  const [mountedAtMS] = useState(() => Date.now());
  const kind = list.filters.kind || undefined;
  const source = list.filters.source || undefined;
  const windowFilter = list.filters.window || "1h";
  const requestID = list.filters.request_id || undefined;
  const accountID = list.filters.account_id || undefined;
  const providerID = list.filters.provider_id || undefined;
  const model = list.filters.model || undefined;
  const sourceEndpoint = list.filters.source_endpoint || undefined;
  const errorClass = list.filters.error_class || undefined;
  const sort = requestEvidenceSortValue(list.filters.sort);
  const minLatencyMS = latencyFilterValue(list.filters.min_latency_ms);
  const maxLatencyMS = latencyFilterValue(list.filters.max_latency_ms);
  const exactStart = list.filters.start || undefined;
  const exactEnd = list.filters.end || undefined;
  const start = exactStart || logWindowSinceAt(windowFilter, mountedAtMS)?.toISOString();
  const query = useOpsRequestEvidence({
    page: list.page,
    page_size: list.pageSize,
    request_id: requestID,
    account_id: accountID,
    provider_id: providerID,
    model,
    source_endpoint: sourceEndpoint,
    error_class: errorClass,
    min_latency_ms: minLatencyMS,
    max_latency_ms: maxLatencyMS,
    kind: kind as "success" | "error" | "unknown" | undefined,
    evidence_source: source as
      | "usage"
      | "ops_error"
      | "request_dump"
      | "system_log"
      | "scheduler_decision"
      | undefined,
    start,
    end: exactEnd,
    sort,
    q: list.search || undefined,
  });
  const total = query.data?.pagination?.total ?? query.data?.data.length ?? 0;
  const isFiltered = Boolean(
    kind ||
      source ||
      requestID ||
      accountID ||
      providerID ||
      model ||
      sourceEndpoint ||
      errorClass ||
      minLatencyMS !== undefined ||
      maxLatencyMS !== undefined ||
      sort !== "created_at_desc" ||
      exactStart ||
      exactEnd ||
      list.search ||
      list.filters.window,
  );

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
            className="truncate"
            title={row.request_id}
          >
            <button
              type="button"
              className="text-srapi-text-primary hover:text-srapi-accent font-mono text-xs underline-offset-2 hover:underline"
              onClick={() => setDetailRequestID(row.request_id)}
            >
              {row.request_id}
            </button>
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
        <div className="space-y-0.5 text-right">
          <div className="text-2xs text-srapi-text-tertiary tabular font-mono">
            {typeof row.latency_ms === "number" ? formatLatency(row.latency_ms) : "—"}
          </div>
          <StreamCompletionBadge state={row.stream_completion_state} />
        </div>
      ),
    },
    {
      key: "tokens",
      header: t("adminRequestEvidence.tokens"),
      align: "right",
      hideOnMobile: true,
      render: (row) => (
        <TokenEvidenceValue total={row.total_tokens} estimated={row.usage_estimated} compact />
      ),
    },
    {
      key: "source",
      header: t("adminRequestEvidence.source"),
      hideOnMobile: true,
      render: (row) => (
        <div className="flex flex-wrap gap-1">
          {row.has_usage_log ? <SourceChip label={t("adminRequestEvidence.usage")} /> : null}
          {row.has_scheduler_decision ? (
            <SourceChip
              label={`${t("adminRequestEvidence.scheduler")} ${row.scheduler_decision_count}`}
              tone="info"
            />
          ) : null}
          {row.has_ops_error_log ? (
            <SourceChip label={t("adminRequestEvidence.opsError")} tone="error" />
          ) : null}
          {row.has_request_dump ? (
            <SourceChip
              label={`${t("adminRequestEvidence.dump")} ${row.request_dump_count}`}
              tone="info"
            />
          ) : null}
          {row.has_system_log ? (
            <SourceChip
              label={`${t("adminRequestEvidence.systemLog")} ${row.system_log_count}`}
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
            <RequestEvidenceScopeFilters
              requestID={requestID}
              accountID={accountID}
              providerID={providerID}
              model={model}
              sourceEndpoint={sourceEndpoint}
              errorClass={errorClass}
              exactStart={exactStart}
              exactEnd={exactEnd}
              onClear={(key) => list.setFilter(key, undefined)}
              onClearExactWindow={() => {
                list.setFilter("start", undefined);
                list.setFilter("end", undefined);
              }}
            />
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
                { value: "scheduler_decision", label: t("adminRequestEvidence.scheduler") },
                { value: "ops_error", label: t("adminRequestEvidence.opsError") },
                { value: "request_dump", label: t("adminRequestEvidence.dump") },
                { value: "system_log", label: t("adminRequestEvidence.systemLog") },
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
            <FilterSelect
              value={sort === "created_at_desc" ? undefined : sort}
              onChange={(value) => list.setFilter("sort", value)}
              allLabel={t("adminRequestEvidence.sortNewest")}
              options={[
                { value: "latency_desc", label: t("adminRequestEvidence.sortSlowest") },
              ]}
              className="min-w-[9.5rem]"
            />
            <LatencyFilterInput
              value={list.filters.min_latency_ms ?? ""}
              onChange={(value) => list.setFilter("min_latency_ms", value)}
              placeholder={t("adminRequestEvidence.minLatencyPlaceholder")}
            />
            <LatencyFilterInput
              value={list.filters.max_latency_ms ?? ""}
              onChange={(value) => list.setFilter("max_latency_ms", value)}
              placeholder={t("adminRequestEvidence.maxLatencyPlaceholder")}
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
      <RequestEvidenceDetailDialog
        requestID={detailRequestID}
        onClose={() => setDetailRequestID(undefined)}
      />
    </div>
  );
}

interface RequestEvidenceScopeFiltersProps {
  requestID?: string;
  accountID?: string;
  providerID?: string;
  model?: string;
  sourceEndpoint?: string;
  errorClass?: string;
  exactStart?: string;
  exactEnd?: string;
  onClear: (key: string) => void;
  onClearExactWindow: () => void;
}

function RequestEvidenceScopeFilters({
  requestID,
  accountID,
  providerID,
  model,
  sourceEndpoint,
  errorClass,
  exactStart,
  exactEnd,
  onClear,
  onClearExactWindow,
}: RequestEvidenceScopeFiltersProps) {
  const { t } = useLanguage();
  const chips = [
    requestID
      ? { key: "request_id", label: t("adminRequestEvidence.scopeRequest"), value: requestID }
      : null,
    accountID
      ? { key: "account_id", label: t("adminRequestEvidence.scopeAccount"), value: accountID }
      : null,
    providerID
      ? { key: "provider_id", label: t("adminRequestEvidence.scopeProvider"), value: providerID }
      : null,
    model ? { key: "model", label: t("adminRequestEvidence.scopeModel"), value: model } : null,
    sourceEndpoint
      ? { key: "source_endpoint", label: t("adminRequestEvidence.scopeEndpoint"), value: sourceEndpoint }
      : null,
    errorClass
      ? { key: "error_class", label: t("adminRequestEvidence.scopeErrorClass"), value: errorClass }
      : null,
  ].filter((chip): chip is { key: string; label: string; value: string } => Boolean(chip));
  const hasExactWindow = Boolean(exactStart || exactEnd);
  if (chips.length === 0 && !hasExactWindow) return null;

  return (
    <div className="flex w-full flex-wrap items-center gap-1.5">
      <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
        {t("adminRequestEvidence.scope")}
      </span>
      {chips.map((chip) => (
        <ScopeChip
          key={`${chip.key}:${chip.value}`}
          label={chip.label}
          value={chip.value}
          clearLabel={t("adminRequestEvidence.scopeClear", { label: chip.label })}
          onClear={() => onClear(chip.key)}
        />
      ))}
      {hasExactWindow ? (
        <ScopeChip
          label={t("adminRequestEvidence.scopeWindow")}
          value={`${exactStart || "…"} → ${exactEnd || "…"}`}
          clearLabel={t("adminRequestEvidence.scopeClear", {
            label: t("adminRequestEvidence.scopeWindow"),
          })}
          onClear={onClearExactWindow}
        />
      ) : null}
    </div>
  );
}

function ScopeChip({
  label,
  value,
  clearLabel,
  onClear,
}: {
  label: string;
  value: string;
  clearLabel: string;
  onClear: () => void;
}) {
  return (
    <span
      className="border-srapi-border-subtle bg-srapi-card-muted inline-flex max-w-full items-center gap-1 rounded border px-1.5 py-0.5 font-mono text-2xs text-srapi-text-secondary sm:max-w-80"
      title={`${label}:${value}`}
    >
      <span className="text-srapi-text-tertiary">{label}</span>
      <span className="max-w-48 truncate text-srapi-text-primary sm:max-w-56">{value}</span>
      <button
        type="button"
        className="text-srapi-text-tertiary hover:text-srapi-text-primary"
        aria-label={clearLabel}
        onClick={onClear}
      >
        <X className="size-3" aria-hidden="true" />
      </button>
    </span>
  );
}

function LatencyFilterInput({
  value,
  onChange,
  placeholder,
}: {
  value: string;
  onChange: (value: string | undefined) => void;
  placeholder: string;
}) {
  return (
    <Input
      type="number"
      inputMode="numeric"
      min={0}
      step={1}
      value={value}
      onChange={(event) => onChange(normalizeLatencyFilterInput(event.target.value))}
      placeholder={placeholder}
      className="h-9 w-full sm:w-28"
    />
  );
}

function RequestEvidenceDetailDialog({
  requestID,
  onClose,
}: {
  requestID?: string;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const query = useOpsRequestEvidenceDetail(requestID);
  const detail = query.data;
  return (
    <Dialog open={Boolean(requestID)} onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent className="max-w-5xl gap-5">
        <DialogHeader>
          <DialogTitle className="font-sans text-base">
            {t("adminRequestEvidence.detailTitle")}
          </DialogTitle>
          <DialogDescription className="font-mono text-xs">
            {requestID || "—"}
          </DialogDescription>
        </DialogHeader>
        {query.isLoading ? (
          <div className="text-srapi-text-tertiary py-10 text-sm">
            {t("adminRequestEvidence.detailLoading")}
          </div>
        ) : query.isError ? (
          <div className="border-srapi-error/30 bg-srapi-error/10 text-srapi-error rounded border p-3 text-sm">
            {t("adminRequestEvidence.detailFailed")}
          </div>
        ) : detail ? (
          <RequestEvidenceDetailContent detail={detail} />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function RequestEvidenceDetailContent({ detail }: { detail: RequestEvidenceDetailResponse }) {
  const { t } = useLanguage();
  const summary = detail.summary;
  const linksRow = detail.attempts[0] || ({
    request_id: detail.evidence_request_id,
    has_ops_error_log: summary.has_ops_error_log,
    has_request_dump: summary.has_request_dump,
    has_system_log: detail.system_log_summary.total_count > 0,
    has_usage_log: summary.has_usage_log,
    has_scheduler_decision: summary.has_scheduler_decision,
    request_dump_count: summary.request_dump_count,
    request_dump_error_count: summary.request_dump_error_count,
    system_log_count: detail.system_log_summary.total_count,
    scheduler_decision_count: summary.scheduler_decision_count,
    scheduler_decision_id: summary.scheduler_decision_id,
    scheduler_candidate_count: summary.scheduler_candidate_count,
    scheduler_rejected_count: summary.scheduler_rejected_count,
    scheduler_strategy: summary.scheduler_strategy,
    scheduler_selection_rationale: summary.scheduler_selection_rationale,
  } as RequestEvidenceRow);
  return (
    <div className="space-y-5">
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
        <DetailMetric
          label={t("adminRequestEvidence.result")}
          value={kindLabel(t, summary.kind)}
          tone={summary.kind === "error" ? "error" : summary.kind === "success" ? "success" : "muted"}
        />
        <DetailMetric
          label={t("adminRequestEvidence.latency")}
          value={typeof summary.latency_ms === "number" ? formatLatency(summary.latency_ms) : "—"}
        />
        <DetailMetric
          label={t("adminRequestEvidence.tokens")}
          value={
            <TokenEvidenceValue
              total={summary.total_tokens}
              estimated={summaryUsageEstimated(detail.attempts)}
            />
          }
        />
        <DetailMetric
          label={t("adminRequestEvidence.stream")}
          value={<StreamCompletionBadge state={summary.stream_completion_state} />}
        />
        <DetailMetric
          label={t("adminRequestEvidence.evidence")}
          value={t("adminRequestEvidence.detailEvidenceCounts", {
            usage: summary.usage_log_count,
            scheduler: summary.scheduler_decision_count,
            errors: summary.ops_error_log_count,
            dumps: summary.request_dump_count,
          })}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-[1fr_18rem]">
        <div className="border-srapi-border-subtle overflow-hidden rounded border">
          <div className="border-srapi-border-subtle bg-srapi-bg-card-elevated border-b px-3 py-2 text-xs font-semibold">
            {t("adminRequestEvidence.detailAttempts")}
          </div>
          <div className="divide-srapi-border-subtle divide-y">
            {detail.attempts.map((row, index) => (
              <div key={`${row.request_id}-${row.attempt_no ?? index}`} className="grid gap-3 px-3 py-2 text-xs md:grid-cols-[4rem_1fr_6rem_6rem]">
                <div className="font-mono text-srapi-text-tertiary">#{row.attempt_no ?? index + 1}</div>
                <div className="min-w-0">
                  <div className="text-srapi-text-primary truncate font-mono">
                    {row.source_endpoint || row.source_protocol || "—"}
                  </div>
                  <div className="text-srapi-text-tertiary truncate">
                    {row.model || "—"}
                    {row.scheduler_strategy ? ` / ${row.scheduler_strategy}` : ""}
                    {row.error_class ? ` / ${row.error_class}` : ""}
                  </div>
                  <div className="mt-1">
                    <StreamCompletionBadge state={row.stream_completion_state} />
                  </div>
                </div>
                <div className={statusClass(row.status_code)}>{row.status_code ?? "—"}</div>
                <div className="space-y-1 text-right">
                  <div className="text-srapi-text-tertiary font-mono">
                    {typeof row.latency_ms === "number" ? formatLatency(row.latency_ms) : "—"}
                  </div>
                  <TokenEvidenceValue total={row.total_tokens} estimated={row.usage_estimated} compact />
                </div>
                {row.error_message ? (
                  <div className="text-srapi-error md:col-span-4 line-clamp-2">{row.error_message}</div>
                ) : null}
                {row.scheduler_selection_rationale ? (
                  <div className="text-srapi-text-tertiary md:col-span-4 line-clamp-2">
                    {row.scheduler_selection_rationale}
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        </div>

        <div className="space-y-3">
          <div className="border-srapi-border-subtle rounded border p-3">
            <div className="text-srapi-text-tertiary mb-2 text-xs font-semibold">
              {t("adminRequestEvidence.detailLinks")}
            </div>
            <EvidenceLinks row={linksRow} />
          </div>
          <div className="border-srapi-border-subtle rounded border p-3">
            <div className="text-srapi-text-tertiary mb-2 text-xs font-semibold">
              {t("adminRequestEvidence.detailSystemLogs")}
            </div>
            {detail.system_log_summary.total_count > 0 ? (
              <div className="space-y-2">
                <div className="text-srapi-text-tertiary text-xs">
                  {t("adminRequestEvidence.detailSystemLogCounts", {
                    total: detail.system_log_summary.total_count,
                    warn: detail.system_log_summary.level_counts.warn ?? 0,
                    error: detail.system_log_summary.level_counts.error ?? 0,
                  })}
                </div>
                {detail.system_logs.map((log) => (
                  <div key={log.id} className="min-w-0 text-xs">
                    <div className="flex items-center justify-between gap-2">
                      <span className={systemLogLevelClass(log.level)}>{log.level}</span>
                      <span className="text-srapi-text-tertiary font-mono">
                        {formatDateTime(log.created_at)}
                      </span>
                    </div>
                    <div className="text-srapi-text-tertiary truncate font-mono" title={log.source}>
                      {log.source}
                    </div>
                    <div className="text-srapi-text-primary line-clamp-2">{log.message}</div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-srapi-text-tertiary text-xs">
                {t("adminRequestEvidence.detailNoSystemLogs")}
              </div>
            )}
          </div>
          <div className="border-srapi-border-subtle rounded border p-3">
            <div className="text-srapi-text-tertiary mb-2 text-xs font-semibold">
              {t("adminRequestEvidence.detailDumps")}
            </div>
            {detail.request_dumps.length > 0 ? (
              <div className="space-y-2">
                {detail.request_dumps.map((dump) => (
                  <div key={dump.name} className="min-w-0 text-xs">
                    <div className="text-srapi-text-primary truncate font-mono" title={dump.name}>
                      {dump.name}
                    </div>
                    <div className="text-srapi-text-tertiary">
                      {formatDateTime(dump.created_at)} · {dump.response_count}/{dump.attempt_count}
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="text-srapi-text-tertiary text-xs">
                {t("adminRequestEvidence.detailNoDumps")}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function DetailMetric({
  label,
  value,
  tone = "neutral",
}: {
  label: string;
  value: ReactNode;
  tone?: "neutral" | "success" | "error" | "muted";
}) {
  const valueClass =
    tone === "error"
      ? "text-srapi-error"
      : tone === "success"
        ? "text-emerald-400"
        : tone === "muted"
          ? "text-srapi-text-tertiary"
          : "text-srapi-text-primary";
  return (
    <div className="border-srapi-border-subtle rounded border p-3">
      <div className="text-srapi-text-tertiary text-2xs uppercase">{label}</div>
      <div className={`mt-1 text-sm font-semibold ${valueClass}`}>{value}</div>
    </div>
  );
}

function summaryUsageEstimated(rows: RequestEvidenceRow[]): boolean | undefined {
  const tokenRows = rows.filter((row) => typeof row.total_tokens === "number");
  if (tokenRows.length === 0) return undefined;
  if (tokenRows.some((row) => row.usage_estimated === true)) return true;
  if (tokenRows.every((row) => row.usage_estimated === false)) return false;
  return undefined;
}

function TokenEvidenceValue({
  total,
  estimated,
  compact = false,
}: {
  total: number | undefined;
  estimated: boolean | undefined;
  compact?: boolean;
}) {
  const { t } = useLanguage();
  if (typeof total !== "number" || !Number.isFinite(total)) {
    return <span className="text-srapi-text-tertiary">—</span>;
  }
  const tone = estimated === true ? "text-amber-400" : "text-emerald-400";
  const label =
    estimated === undefined
      ? null
      : estimated
        ? t("adminRequestEvidence.estimated")
        : t("adminRequestEvidence.exact");
  return (
    <span className="inline-flex items-baseline justify-end gap-1 font-mono tabular">
      <span className="text-srapi-text-primary">{formatInteger(total)}</span>
      {label ? (
        <span className={`${compact ? "text-[10px]" : "text-2xs"} ${tone}`}>{label}</span>
      ) : null}
    </span>
  );
}

function StreamCompletionBadge({ state }: { state?: RequestEvidenceRow["stream_completion_state"] }) {
  const { t } = useLanguage();
  if (!state) return <span className="text-srapi-text-tertiary">—</span>;
  const className =
    state === "completed"
      ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
      : state === "idle_timeout" || state === "failed"
        ? "border-red-500/30 bg-red-500/10 text-red-300"
        : state === "interrupted"
          ? "border-amber-500/30 bg-amber-500/10 text-amber-300"
          : "border-srapi-border-subtle bg-srapi-bg-card-elevated text-srapi-text-tertiary";
  return (
    <span className={`inline-flex rounded border px-1.5 py-0.5 font-mono text-2xs ${className}`}>
      {t(`adminRequestEvidence.streamState.${state}`)}
    </span>
  );
}

function EvidenceLinks({ row }: { row: RequestEvidenceRow }) {
  const { t } = useLanguage();
  const params = { request_id: row.request_id };
  const errorHref = adminErrorLogsHref(params);
  const systemHref = adminSystemLogsHref(params);
  const dumpHref = adminRequestDumpsHref(params);
  const schedulerHref = adminSchedulerDecisionsHref(params);
  return (
    <div className="flex flex-wrap gap-1">
      {row.has_scheduler_decision && schedulerHref ? (
        <EvidenceLink
          href={schedulerHref}
          icon={<Route className="size-3" />}
          label={t("adminRequestEvidence.scheduler")}
        />
      ) : null}
      {row.has_ops_error_log && errorHref ? (
        <EvidenceLink
          href={errorHref}
          icon={<Bug className="size-3" />}
          label={t("adminRequestEvidence.errorLog")}
        />
      ) : null}
      {row.has_system_log && systemHref ? (
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

function requestEvidenceSortValue(raw?: string): "created_at_desc" | "latency_desc" {
  return raw === "latency_desc" ? "latency_desc" : "created_at_desc";
}

function latencyFilterValue(raw?: string): number | undefined {
  if (!raw) return undefined;
  if (!/^\d+$/.test(raw.trim())) return undefined;
  const value = Number.parseInt(raw, 10);
  if (!Number.isFinite(value) || value < 0) return undefined;
  return value;
}

function normalizeLatencyFilterInput(raw: string): string | undefined {
  const trimmed = raw.trim();
  if (trimmed === "") return undefined;
  const value = latencyFilterValue(trimmed);
  return value === undefined ? undefined : String(value);
}

function systemLogLevelClass(level: string): string {
  const base = "font-mono text-2xs uppercase";
  if (level === "error") return `${base} text-srapi-error`;
  if (level === "warn") return `${base} text-amber-500`;
  if (level === "info") return `${base} text-sky-400`;
  return `${base} text-srapi-text-tertiary`;
}
