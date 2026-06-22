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
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DensityToggle, type DensityValue } from "@/components/ui/density-toggle";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
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

// Severity for the row stripe — driven by HTTP code, with stream_completion_state
// overriding to «warning» when the request completed but bailed mid-stream.
function evidenceSeverity(row: RequestEvidenceRow): "info" | "success" | "warning" | "error" | "critical" {
  const code = row.status_code ?? 0;
  if (code >= 500) return "critical";
  if (code >= 400) return "error";
  if (row.stream_completion_state === "failed" || row.stream_completion_state === "idle_timeout") return "error";
  if (row.stream_completion_state === "interrupted") return "warning";
  if (row.kind === "success") return "success";
  if (row.kind === "error") return "error";
  return "info";
}

export function RequestEvidencePanel() {
  const { t } = useLanguage();
  const list = useAdminList({ pageSize: 50 });
  const [detailRequestID, setDetailRequestID] = useState<string | undefined>();
  const [mountedAtMS] = useState(() => Date.now());
  const [density, setDensity] = useState<DensityValue>("regular");
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
        <span className="whitespace-nowrap text-[12px] tabular text-srapi-text-tertiary">
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
            className="truncate text-[12px] text-srapi-text-tertiary"
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
            className="truncate text-[12px] text-srapi-text-secondary"
            title={row.source_endpoint ?? ""}
          >
            {row.source_endpoint || "—"}
          </div>
          <div className="truncate text-[12px] text-srapi-text-tertiary">
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
        <div className="space-y-0.5 text-[12px] text-srapi-text-tertiary">
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
      // Status code + attempt with hover breakdown (kind / error class / stream
      // state) so a row at-a-glance answers «is this fatal or transient?».
      render: (row) => (
        <DataTooltip
          title={t("adminRequestEvidence.status")}
          primary={row.status_code ?? "—"}
          rows={[
            { label: t("adminRequestEvidence.result"), value: kindLabel(t, row.kind), tone: row.kind === "success" ? "success" : row.kind === "error" ? "error" : "muted" },
            { label: t("adminRequestEvidence.errorKind"), value: row.error_class || "—", tone: "error" },
            { label: t("adminRequestEvidence.stream"), value: row.stream_completion_state || "—", tone: row.stream_completion_state === "failed" ? "error" : row.stream_completion_state === "interrupted" ? "warning" : "muted" },
            { label: "attempt", value: row.attempt_no ? `#${row.attempt_no}` : "—", tone: "muted" },
          ]}
        >
          <div className="space-y-0.5 text-right">
            <div className={statusClass(row.status_code)}>{row.status_code ?? "—"}</div>
            <div className="text-[12px] text-srapi-text-tertiary">
              {row.attempt_no ? `#${row.attempt_no}` : "—"}
            </div>
          </div>
        </DataTooltip>
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
          <div className="line-clamp-2 break-words text-[12px] text-srapi-text-tertiary">
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
      // Latency tooltip: queue + route + upstream + stream waterfall. Per-phase
      // numbers aren't on RequestEvidenceRow yet — once the API exposes them
      // the breakdown lights up; today it surfaces stream-completion + outlier
      // hint so triage isn't blind.
      render: (row) => {
        const total = typeof row.latency_ms === "number" ? row.latency_ms : null;
        return (
          <DataTooltip
            title={t("adminRequestEvidence.latency")}
            primary={total !== null ? formatLatency(total) : "—"}
            rows={[
              { label: "queue", value: "—", tone: "muted" },
              { label: "route", value: "—", tone: "muted" },
              { label: "upstream", value: total !== null ? formatLatency(total) : "—", tone: total !== null && total >= 10000 ? "warning" : "default" },
              { label: "stream", value: row.stream_completion_state || "—", tone: row.stream_completion_state === "failed" ? "error" : row.stream_completion_state === "interrupted" ? "warning" : "muted" },
            ]}
            footer={total !== null && total >= 30000 ? "outlier — likely upstream stall" : undefined}
          >
            <div className="space-y-0.5 text-right">
              <div className="text-[12px] tabular text-srapi-text-tertiary">
                {total !== null ? formatLatency(total) : "—"}
              </div>
              <StreamCompletionBadge state={row.stream_completion_state} />
            </div>
          </DataTooltip>
        );
      },
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
          <div className="flex items-center gap-3">
            <DensityToggle value={density} onChange={setDensity} />
            <AutoRefreshControl
              onRefresh={async () => {
                await query.refetch();
              }}
              isRefreshing={query.isFetching}
              storageKey="admin-request-evidence-refresh"
              defaultSec={0}
            />
          </div>
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
        density={density}
        enableKeyboardNav
        rowSeverity={evidenceSeverity}
        // Inline detail — Request / Response / Routing / Cost panels covering the
        // ~12 fields a triage operator wants without opening the modal. The
        // request_id is still clickable in the row to deep-link into the modal
        // for the full attempt timeline + system logs.
        expandRow={(row) => (
          <InlineDetailGrid
            sections={[
              {
                title: "Request",
                rows: [
                  { label: "request_id", value: row.request_id, mono: true },
                  { label: t("adminRequestEvidence.userShort"), value: row.user_id ?? "—", mono: true, tone: "muted" },
                  { label: t("adminRequestEvidence.keyShort"), value: row.api_key_id ?? "—", mono: true, tone: "muted" },
                  { label: t("adminRequestEvidence.accountShort"), value: row.account_id ?? "—", mono: true, tone: "muted" },
                  { label: "endpoint", value: row.source_endpoint || "—", mono: true },
                ],
              },
              {
                title: "Response",
                rows: [
                  { label: "status", value: row.status_code ?? "—", mono: true, tone: (row.status_code ?? 0) >= 500 ? "error" : (row.status_code ?? 0) >= 400 ? "warning" : "default" },
                  { label: "kind", value: kindLabel(t, row.kind), tone: row.kind === "success" ? "success" : row.kind === "error" ? "error" : "muted" },
                  { label: "error_class", value: row.error_class || "—", mono: true, tone: "error" },
                  { label: "error_message", value: row.error_message || "—", tone: "muted" },
                  { label: "stream", value: row.stream_completion_state || "—" },
                ],
              },
              {
                title: "Routing",
                rows: [
                  { label: "protocol", value: `${row.source_protocol || "—"}${row.target_protocol ? ` → ${row.target_protocol}` : ""}`, mono: true },
                  { label: "attempt", value: row.attempt_no ? `#${row.attempt_no}` : "—", mono: true, tone: "muted" },
                  { label: "model", value: row.model || "—", mono: true },
                  { label: "scheduler", value: row.scheduler_strategy || "—", mono: true, tone: "muted" },
                ],
              },
              {
                title: "Cost",
                rows: [
                  { label: t("adminRequestEvidence.latency"), value: typeof row.latency_ms === "number" ? formatLatency(row.latency_ms) : "—", mono: true },
                  { label: t("adminRequestEvidence.tokens"), value: typeof row.total_tokens === "number" ? formatInteger(row.total_tokens) : "—", mono: true },
                  { label: "estimated", value: row.usage_estimated === true ? "yes" : row.usage_estimated === false ? "no" : "—", tone: row.usage_estimated === true ? "warning" : "muted" },
                ],
              },
            ]}
          />
        )}
        toolbar={
          <>
            {/* Severity chip strip — collapses the kind filter into a single-
                click triage pivot. The granular «source» FilterSelect still
                lives in the toolbar below for ops/dump/system splits. */}
            <div className="flex items-center gap-3 border-b border-srapi-border/60 bg-srapi-card-muted/40 px-4 py-2">
              <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                Severity
              </span>
              <SegmentedControl
                value={kind === "error" ? "error" : kind === "success" ? "success" : kind === "unknown" ? "unknown" : "all"}
                onChange={(v) => list.setFilter("kind", v === "all" ? undefined : v)}
                options={[
                  { value: "all", label: "All" },
                  { value: "error", label: "Errors" },
                  { value: "success", label: "OK" },
                  { value: "unknown", label: "Unknown" },
                ]}
                size="sm"
                ariaLabel="evidence severity filter"
              />
            </div>
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
          </>
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
      <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
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
      className="inline-flex max-w-full items-center gap-1 rounded-full bg-srapi-card-muted px-2.5 py-0.5 text-[11px] font-medium text-srapi-text-secondary sm:max-w-80"
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
          <DialogTitle className="text-lg font-semibold tracking-tight">
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
        <div className="border-srapi-border overflow-hidden rounded border">
          <div className="border-srapi-border bg-srapi-card-muted border-b px-3 py-2 text-xs font-semibold">
            {t("adminRequestEvidence.detailAttempts")}
          </div>
          <div className="divide-srapi-border divide-y">
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
          <div className="border-srapi-border rounded border p-3">
            <div className="text-srapi-text-tertiary mb-2 text-xs font-semibold">
              {t("adminRequestEvidence.detailLinks")}
            </div>
            <EvidenceLinks row={linksRow} />
          </div>
          <div className="border-srapi-border rounded border p-3">
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
          <div className="border-srapi-border rounded border p-3">
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
        ? "text-srapi-success"
        : tone === "muted"
          ? "text-srapi-text-tertiary"
          : "text-srapi-text-primary";
  return (
    <div className="rounded-xl border border-srapi-border p-3">
      <div className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">{label}</div>
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
  const tone = estimated === true ? "text-srapi-warning" : "text-srapi-success";
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
        <span className={`text-[11px] font-medium ${tone}`}>{label}</span>
      ) : null}
    </span>
  );
}

function StreamCompletionBadge({ state }: { state?: RequestEvidenceRow["stream_completion_state"] }) {
  const { t } = useLanguage();
  if (!state) return <span className="text-srapi-text-tertiary">—</span>;
  const className =
    state === "completed"
      ? "border-srapi-success/30 bg-srapi-success/10 text-srapi-success"
      : state === "idle_timeout" || state === "failed"
        ? "border-srapi-error/30 bg-srapi-error/10 text-srapi-error"
        : state === "interrupted"
          ? "border-srapi-warning/30 bg-srapi-warning/12 text-srapi-warning"
          : "border-srapi-border bg-srapi-card-muted text-srapi-text-tertiary";
  return (
    <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] font-medium ${className}`}>
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
      className="inline-flex items-center gap-1 rounded-full border border-srapi-border bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary hover:text-srapi-text-primary"
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
      ? "bg-srapi-error/12 text-srapi-error"
      : tone === "info"
        ? "bg-srapi-accent-soft text-srapi-primary"
        : "bg-srapi-card-muted text-srapi-text-secondary";
  return <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium ${className}`}>{label}</span>;
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
  const base = "text-[12px] font-medium tabular";
  if (status == null) return `${base} text-srapi-text-tertiary`;
  if (status >= 500) return `${base} text-srapi-error`;
  if (status >= 400) return `${base} text-srapi-warning`;
  return `${base} text-srapi-success`;
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
  const base = "text-[11px] font-semibold uppercase tracking-[0.12em]";
  if (level === "error") return `${base} text-srapi-error`;
  if (level === "warn") return `${base} text-srapi-warning`;
  if (level === "info") return `${base} text-srapi-success`;
  return `${base} text-srapi-text-tertiary`;
}
