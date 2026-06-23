"use client";

import type { UseQueryResult } from "@tanstack/react-query";
import { useState } from "react";
import Link from "next/link";
import { Bug, FileText, Fingerprint, Link2, ScrollText } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { ErrorLogDetailDialog } from "@/components/admin/error-log-detail-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { Input } from "@/components/ui/input";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { QuietBadge, type QuietStatus } from "@/components/ui/quiet-badge";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DensityToggle, type DensityValue } from "@/components/ui/density-toggle";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import {
  useAdminErrorLogFingerprints,
  useAdminErrorLogs,
  useAdminModels,
} from "@/hooks/admin-queries";
import { useAccountNameLookup } from "@/hooks/use-account-name-lookup";
import { useApiKeyNameLookup } from "@/hooks/use-api-key-name-lookup";
import { useProviderNameLookup } from "@/hooks/use-provider-name-lookup";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatLatency } from "@/lib/admin-format";
import {
  adminRequestDumpsHref,
  adminSystemLogsHref,
} from "@/lib/admin-log-links";
import { buildErrorLogTriage } from "@/lib/admin-error-log-triage";
import type { OpsErrorFingerprint, OpsErrorLog } from "@/lib/sdk-types";
import {
  LOG_WINDOW_PRESETS,
  LOG_WINDOW_ALL_LABEL_KEY,
  logWindowSince,
} from "@/lib/log-window-filter";
import { compactSchedulerDiagnostic } from "@/lib/scheduler-diagnostic";
import { compactUpstreamErrorDiagnostic } from "@/lib/upstream-error-diagnostic";

type ErrorLogFingerprintQuery = UseQueryResult<{
  data: OpsErrorFingerprint[];
  meta: {
    total: number;
    scanned: number;
    truncated: boolean;
    window_start?: string;
    window_end?: string;
  };
}>;

const DEFAULT_HIDDEN_COLUMNS = ["api_key_id", "provider_id", "source_endpoint", "error_owner"];
const STATUS_FILTER_OPTIONS = [
  { value: "4xx", min: 400, max: 499 },
  { value: "5xx", min: 500, max: 599 },
  { value: "400", min: 400, max: 400 },
  { value: "401", min: 401, max: 401 },
  { value: "403", min: 403, max: 403 },
  { value: "404", min: 404, max: 404 },
  { value: "429", min: 429, max: 429 },
  { value: "500", min: 500, max: 500 },
  { value: "502", min: 502, max: 502 },
  { value: "503", min: 503, max: 503 },
  { value: "504", min: 504, max: 504 },
] as const;
const ERROR_PHASE_OPTIONS = [
  "request",
  "auth",
  "routing",
  "upstream",
  "network",
  "internal",
] as const;
const ERROR_OWNER_OPTIONS = [
  "client",
  "provider",
  "scheduler",
  "reverse_proxy",
  "platform",
  "internal",
  "business",
] as const;

export function ErrorLogsPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-error-logs", DEFAULT_HIDDEN_COLUMNS);
  const accountLookup = useAccountNameLookup();
  const apiKeyLookup = useApiKeyNameLookup();
  const providerLookup = useProviderNameLookup();
  const [detail, setDetail] = useState<{ id: string; email?: string } | null>(null);
  const [density, setDensity] = useState<DensityValue>("regular");

  const modelFilter = list.filters.model || undefined;
  const userFilter = list.filters.user || undefined;
  const accountFilter = list.filters.account || undefined;
  const providerFilter = list.filters.provider || undefined;
  const sourceEndpointFilter = list.filters.source_endpoint || undefined;
  const errorClassFilter = list.filters.error_class || undefined;
  const errorPhaseFilter = list.filters.error_phase || undefined;
  const errorOwnerFilter = list.filters.error_owner || undefined;
  const resolutionFilter = list.filters.resolution || undefined;
  const statusFilter = statusFilterBounds(list.filters.status);
  const windowFilter = list.filters.window;
  const sinceFilter = logWindowSince(windowFilter)?.toISOString();

  const searchQuery = list.search || undefined;
  const errorLogs = useAdminErrorLogs({
    page: list.page,
    page_size: list.pageSize,
    model: modelFilter,
    user_id: userFilter,
    account_id: accountFilter,
    provider_id: providerFilter,
    source_endpoint: sourceEndpointFilter,
    resolution: resolutionFilter as "open" | "investigating" | "resolved" | "muted" | undefined,
    error_class: errorClassFilter,
    error_phase: errorPhaseFilter,
    error_owner: errorOwnerFilter,
    status_min: statusFilter?.min,
    status_max: statusFilter?.max,
    start: sinceFilter,
    q: searchQuery,
  });
  const fingerprintQueryParams = {
    model: modelFilter,
    user_id: userFilter,
    account_id: accountFilter,
    provider_id: providerFilter,
    source_endpoint: sourceEndpointFilter,
    resolution: resolutionFilter as "open" | "investigating" | "resolved" | "muted" | undefined,
    error_class: errorClassFilter,
    error_phase: errorPhaseFilter,
    error_owner: errorOwnerFilter,
    status_min: statusFilter?.min,
    status_max: statusFilter?.max,
    start: sinceFilter,
    q: searchQuery,
    limit: 6,
  };
  const fingerprints = useAdminErrorLogFingerprints(fingerprintQueryParams);
  const models = useAdminModels({ page: 1, page_size: 100 });
  const userLookup = useUserEmailLookup();
  const modelOptions = (models.data?.data ?? []).map((m) => ({
    value: m.canonical_name,
    label: m.canonical_name,
  }));
  // Both userOptions and the row email rendering come from the same shared
  // lookup. Bumps the dropdown from 100 to the hook's 200-row window for
  // free (more emails visible without changing the rendering contract).
  const userOptions = (userLookup.query.data?.data ?? []).map((u) => ({
    value: String(u.id),
    label: u.email,
  }));

  const isFiltered = Boolean(
    modelFilter ||
    userFilter ||
    accountFilter ||
    providerFilter ||
    sourceEndpointFilter ||
    errorClassFilter ||
    errorPhaseFilter ||
    errorOwnerFilter ||
    statusFilter ||
    resolutionFilter ||
    windowFilter ||
    searchQuery,
  );
  const accountOptions = (accountLookup.query.data?.data ?? []).map((a) => ({
    value: String(a.id),
    label: a.name,
  }));
  const providerOptions = (providerLookup.query.data?.data ?? []).map((p) => ({
    value: String(p.id),
    label: p.display_name || p.name,
  }));
  const total = errorLogs.data?.pagination?.total ?? errorLogs.data?.data.length ?? 0;

  const emailFor = (e: OpsErrorLog) => userLookup.get(e.user_id);
  const openDetail = (e: OpsErrorLog) => {
    if (!e.id) return;
    setDetail({ id: e.id, email: emailFor(e) });
  };

  const columns: Column<OpsErrorLog>[] = [
    {
      key: "time",
      header: t("adminErrorLogs.time"),
      pinned: true,
      render: (e) => (
        <button
          type="button"
          onClick={() => openDetail(e)}
          className="whitespace-nowrap text-[12px] tabular text-srapi-text-tertiary underline-offset-2 transition-colors hover:text-srapi-text-primary hover:underline"
        >
          {formatDateTime(e.occurred_at ?? e.created_at)}
        </button>
      ),
    },
    {
      key: "user",
      header: t("adminErrorLogs.user"),
      hideOnMobile: true,
      render: (e) => <span className="text-srapi-text-secondary truncate">{emailFor(e)}</span>,
    },
    {
      key: "account",
      header: t("adminErrorLogs.account"),
      hideOnMobile: true,
      render: (e) => (
        <span className="text-srapi-text-secondary">{accountLookup.get(e.account_id)}</span>
      ),
    },
    {
      key: "model",
      header: t("adminErrorLogs.model"),
      render: (e) => <span className="text-srapi-text-primary">{e.model || "—"}</span>,
    },
    {
      key: "status_code",
      header: t("adminErrorLogs.statusCode"),
      align: "right",
      render: (e) => {
        const code = e.status_code ?? null;
        const color =
          code != null && code >= 500
            ? "text-srapi-error"
            : code != null && code >= 400
              ? "text-srapi-warning"
              : "text-srapi-text-tertiary";
        // Status-code breakdown on hover: phase + owner + protocol so ops can
        // see «what kind of failure» without opening the dialog.
        return (
          <DataTooltip
            title={t("adminErrorLogs.statusCode")}
            primary={code ?? "—"}
            rows={[
              { label: t("adminErrorLogs.errorClass"), value: e.error_class || "—", tone: "error" },
              { label: t("adminErrorLogs.errorPhase"), value: e.error_phase || "—" },
              { label: t("adminErrorLogs.errorOwner"), value: e.error_owner || "—" },
              { label: t("adminErrorLogs.protocol"), value: `${e.source_protocol ?? e.platform ?? "—"}${e.target_protocol ? ` → ${e.target_protocol}` : ""}` },
              { label: t("adminErrorLogs.attempt"), value: e.attempt_no ?? 1, tone: "muted" },
            ]}
          >
            <span className={`text-[12px] font-medium tabular ${color}`}>{code ?? "—"}</span>
          </DataTooltip>
        );
      },
    },
    {
      key: "error_class",
      header: t("adminErrorLogs.errorClass"),
      render: (e) => (
        <span className="text-srapi-error font-mono text-xs">{e.error_class || "—"}</span>
      ),
    },
    {
      key: "phase",
      header: t("adminErrorLogs.errorPhase"),
      hideOnMobile: true,
      render: (e) => (
        <div className="flex flex-wrap gap-1">
          {e.error_phase ? (
            <span className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary">
              {e.error_phase}
            </span>
          ) : null}
          {e.error_owner ? (
            <span className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-tertiary">
              {e.error_owner}
            </span>
          ) : null}
          {e.stream_completion_state ? (
            <span className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-tertiary">
              {e.stream_completion_state}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      key: "resolution",
      header: t("adminErrorLogs.resolution"),
      render: (e) => (
        <QuietBadge
          status={resolutionTone(e.resolution)}
          label={e.resolution ? t(`adminErrorLogs.${e.resolution}`) : "—"}
        />
      ),
    },
    {
      key: "evidence",
      header: t("adminErrorLogs.relatedEvidence"),
      hideOnMobile: true,
      className: "w-44",
      render: (e) => <RelatedEvidencePills log={e} />,
    },
    {
      // Verbatim upstream provider message (sub2api parity:
      // ops_error_logs.upstream_error_message). Truncated visually but the
      // full text + body excerpt live in the detail dialog.
      key: "error_message",
      header: t("adminErrorLogs.upstreamMessage"),
      hideOnMobile: true,
      render: (e) => <ErrorMessageCell log={e} />,
    },
    {
      key: "latency",
      header: t("adminErrorLogs.latency"),
      align: "right",
      hideOnMobile: true,
      // Latency tooltip shows the full timing waterfall: queue + route + upstream
      // + stream so a 30s outlier can be triaged in one hover.
      render: (e) => {
        const total = e.latency_ms ?? 0;
        // Backend doesn't currently break out per-phase latency on
        // OpsErrorLog — show derived approximations and the real total. The
        // 4-row breakdown still answers «where was the time spent» at a glance
        // once those fields land.
        return (
          <DataTooltip
            title={t("adminErrorLogs.latency")}
            primary={formatLatency(total)}
            rows={[
              { label: "queue", value: "—", tone: "muted" },
              { label: "route", value: "—", tone: "muted" },
              { label: "upstream", value: formatLatency(total), tone: total >= 10000 ? "warning" : "default" },
              { label: "stream", value: e.stream_completion_state || "—", tone: e.stream_completion_state === "failed" ? "error" : "muted" },
              { label: t("adminErrorLogs.attempt"), value: e.attempt_no ?? 1, tone: "muted" },
            ]}
            footer={total >= 30000 ? "outlier — likely upstream stall" : undefined}
          >
            <span className="text-[12px] tabular text-srapi-text-tertiary">
              {formatLatency(total)}
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "protocol",
      header: t("adminErrorLogs.protocol"),
      hideOnMobile: true,
      render: (e) => (
        <span className="text-[12px] text-srapi-text-tertiary">
          {e.source_protocol ?? e.platform ?? "—"}
          {e.target_protocol ? ` → ${e.target_protocol}` : ""}
        </span>
      ),
    },
    {
      key: "attempt_no",
      header: t("adminErrorLogs.attempt"),
      align: "right",
      hideOnMobile: true,
      render: (e) => (
        <span className="text-[12px] tabular text-srapi-text-tertiary">
          {e.attempt_no ?? 1}
        </span>
      ),
    },
    {
      key: "source_endpoint",
      header: t("adminErrorLogs.sourceEndpoint"),
      hideOnMobile: true,
      render: (e) => (
        <span className="text-[12px] text-srapi-text-tertiary">
          {e.source_endpoint || "—"}
        </span>
      ),
    },
    {
      key: "error_owner",
      header: t("adminErrorLogs.errorOwner"),
      hideOnMobile: true,
      render: (e) => (
        <span className="text-[12px] text-srapi-text-tertiary">{e.error_owner || "—"}</span>
      ),
    },
    {
      key: "api_key_id",
      header: t("adminErrorLogs.apiKey"),
      hideOnMobile: true,
      render: (e) => (
        <span className="text-srapi-text-secondary">{apiKeyLookup.get(e.api_key_id)}</span>
      ),
    },
    {
      key: "provider_id",
      header: t("adminErrorLogs.provider"),
      hideOnMobile: true,
      render: (e) => (
        <span className="text-srapi-text-secondary">{providerLookup.get(e.provider_id)}</span>
      ),
    },
    {
      key: "request_id",
      header: t("adminErrorLogs.requestId"),
      hideOnMobile: true,
      render: (e) => (
        <button
          type="button"
          onClick={() => openDetail(e)}
          className="text-[12px] text-srapi-text-tertiary underline-offset-2 transition-colors hover:text-srapi-text-primary hover:underline"
        >
          {e.request_id}
        </button>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminErrorLogs.title")}
        description={t("adminErrorLogs.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {errorLogs.data ? <ListCount total={total} /> : null}
            <DensityToggle value={density} onChange={setDensity} />
            <ColumnToggle
              columns={columns
                .filter((c) => !c.pinned)
                .map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <AutoRefreshControl
              onRefresh={() => void errorLogs.refetch()}
              isRefreshing={errorLogs.isFetching}
              storageKey="srapi.autorefresh.admin-error-logs"
            />
          </div>
        }
      />
      <AdminListView
        query={errorLogs}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(e) => e.id ?? e.request_id ?? ""}
        emptyIcon={Bug}
        emptyTitle={t("adminErrorLogs.emptyTitle")}
        emptyBody={t("adminErrorLogs.emptyBody")}
        minWidth={900}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        density={density}
        enableKeyboardNav
        // Severity stripe at the row's leading edge — turns the table into a
        // glanceable feed: 5xx → red, 429/4xx → amber, transient → grey.
        rowSeverity={(e) => {
          const code = e.status_code ?? null;
          if (code !== null && code >= 500) return "critical";
          if (code !== null && code >= 400) return "error";
          if (e.error_phase === "network") return "warning";
          return "info";
        }}
        // Inline error-log detail — Request / Response / Routing / Cost
        // sections expose the full triage payload (request ID, full URL, status
        // chip, latency, error message, upstream excerpt) without the heavier
        // dialog. The dialog still owns the deep-link triage flow for
        // operators copy-pasting from incident channels.
        expandRow={(e) => (
          <InlineDetailGrid
            sections={[
              {
                title: "Request",
                rows: [
                  { label: t("adminErrorLogs.requestId"), value: e.request_id || "—", mono: true },
                  { label: t("adminErrorLogs.sourceEndpoint"), value: e.source_endpoint || "—", mono: true },
                  { label: t("adminErrorLogs.model"), value: e.model || "—", mono: true },
                  { label: t("adminErrorLogs.user"), value: emailFor(e) || "—" },
                ],
              },
              {
                title: "Response",
                rows: [
                  { label: t("adminErrorLogs.statusCode"), value: e.status_code ?? "—", mono: true, tone: (e.status_code ?? 0) >= 500 ? "error" : (e.status_code ?? 0) >= 400 ? "warning" : "default" },
                  { label: t("adminErrorLogs.errorClass"), value: e.error_class || "—", mono: true, tone: "error" },
                  { label: t("adminErrorLogs.upstreamMessage"), value: e.error_message || "—", tone: "muted" },
                  { label: t("adminErrorLogs.latency"), value: formatLatency(e.latency_ms ?? 0), mono: true },
                ],
              },
              {
                title: "Routing",
                rows: [
                  { label: t("adminErrorLogs.protocol"), value: `${e.source_protocol ?? e.platform ?? "—"}${e.target_protocol ? ` → ${e.target_protocol}` : ""}`, mono: true },
                  { label: t("adminErrorLogs.errorPhase"), value: e.error_phase || "—" },
                  { label: t("adminErrorLogs.errorOwner"), value: e.error_owner || "—" },
                  { label: t("adminErrorLogs.attempt"), value: e.attempt_no ?? 1, mono: true, tone: "muted" },
                  { label: t("adminErrorLogs.account"), value: accountLookup.get(e.account_id) || "—" },
                  { label: t("adminErrorLogs.provider"), value: providerLookup.get(e.provider_id) || "—" },
                ],
              },
            ]}
          />
        )}
        toolbar={
          <>
            <ErrorFingerprintStrip
              query={fingerprints}
              onSelect={(item) => {
                list.setFilter("source_endpoint", item.source_endpoint || undefined);
                list.setFilter("model", item.model || undefined);
                list.setFilter("error_class", item.error_class || undefined);
                list.setFilter("error_phase", item.error_phase || undefined);
                list.setFilter("error_owner", item.error_owner || undefined);
                if (item.status_class !== "unknown") {
                  list.setFilter("status", item.status_class);
                }
              }}
              onOpenExample={(item) => {
                if (item.example_error_log_id) {
                  setDetail({ id: item.example_error_log_id });
                }
              }}
            />
            {/* Severity quick-filter — one click jump to «only 5xx», «only 4xx»,
                or back to all. The narrow strip is faster to read than the
                «Status» dropdown 9 rows down the toolbar. */}
            <div className="flex items-center gap-3 border-b border-srapi-border/60 bg-srapi-card-muted/40 px-4 py-2">
              <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                Severity
              </span>
              <SegmentedControl
                value={list.filters.status === "5xx" ? "5xx" : list.filters.status === "4xx" ? "4xx" : "all"}
                onChange={(v) => list.setFilter("status", v === "all" ? undefined : v)}
                options={[
                  { value: "all", label: "All" },
                  { value: "5xx", label: "5xx critical" },
                  { value: "4xx", label: "4xx error" },
                ]}
                size="sm"
                ariaLabel="severity quick filter"
              />
            </div>
            <ListToolbar>
              <SearchInput
                value={list.searchInput}
                onChange={list.setSearchInput}
                placeholder={t("adminErrorLogs.searchPlaceholder")}
              />
              <FilterSelect
                value={list.filters.model}
                onChange={(v) => list.setFilter("model", v)}
                options={modelOptions}
                allLabel={t("adminErrorLogs.allModels")}
              />
              <FilterSelect
                value={list.filters.user}
                onChange={(v) => list.setFilter("user", v)}
                options={userOptions}
                allLabel={t("adminErrorLogs.allUsers")}
              />
              <FilterSelect
                value={list.filters.account}
                onChange={(v) => list.setFilter("account", v)}
                options={accountOptions}
                allLabel={t("adminAccounts.allAccounts")}
              />
              <FilterSelect
                value={list.filters.provider}
                onChange={(v) => list.setFilter("provider", v)}
                options={providerOptions}
                allLabel={t("adminAccounts.allProviders")}
              />
              <ErrorClassFilter
                value={list.filters.error_class ?? ""}
                onChange={(v) => list.setFilter("error_class", v || undefined)}
                ariaLabel={t("adminErrorLogs.errorClassFilter")}
                placeholder={t("adminErrorLogs.errorClassPlaceholder")}
              />
              <FilterSelect
                value={list.filters.status}
                onChange={(v) => list.setFilter("status", v)}
                options={STATUS_FILTER_OPTIONS.map((option) => ({
                  value: option.value,
                  label: option.value,
                }))}
                allLabel={t("adminErrorLogs.allStatuses")}
              />
              <FilterSelect
                value={list.filters.error_phase}
                onChange={(v) => list.setFilter("error_phase", v)}
                options={ERROR_PHASE_OPTIONS.map((phase) => ({
                  value: phase,
                  label: t(`adminErrorLogs.phase.${phase}`),
                }))}
                allLabel={t("adminErrorLogs.allPhases")}
              />
              <FilterSelect
                value={list.filters.error_owner}
                onChange={(v) => list.setFilter("error_owner", v)}
                options={ERROR_OWNER_OPTIONS.map((owner) => ({
                  value: owner,
                  label: t(`adminErrorLogs.owner.${owner}`),
                }))}
                allLabel={t("adminErrorLogs.allOwners")}
              />
              <FilterSelect
                value={list.filters.resolution}
                onChange={(v) => list.setFilter("resolution", v)}
                options={[
                  { value: "open", label: t("adminErrorLogs.open") },
                  { value: "investigating", label: t("adminErrorLogs.investigating") },
                  { value: "resolved", label: t("adminErrorLogs.resolved") },
                  { value: "muted", label: t("adminErrorLogs.muted") },
                ]}
                allLabel={t("adminErrorLogs.allResolutions")}
              />
              <FilterSelect
                value={list.filters.window}
                onChange={(v) => list.setFilter("window", v)}
                options={LOG_WINDOW_PRESETS.map((p) => ({ value: p.value, label: t(p.labelKey) }))}
                allLabel={t(LOG_WINDOW_ALL_LABEL_KEY)}
              />
            </ListToolbar>
          </>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(e) => (
          <RowActionsMenu
            actions={[{ label: t("adminErrorLogs.detailTitle"), onSelect: () => openDetail(e) }]}
          />
        )}
      />

      <ErrorLogDetailDialog
        errorLogId={detail?.id ?? null}
        userEmail={detail?.email}
        open={detail !== null}
        onOpenChange={(open) => {
          if (!open) setDetail(null);
        }}
      />
    </>
  );
}

function ErrorMessageCell({ log }: { log: OpsErrorLog }) {
  const diagnostic = compactSchedulerDiagnostic(log.error_body_excerpt);
  if (diagnostic) {
    return (
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap gap-1">
          <span className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-primary">
            {diagnostic.reason}
          </span>
          {diagnostic.count ? (
            <span className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-tertiary">
              ×{diagnostic.count}
            </span>
          ) : null}
          {diagnostic.action ? (
            <span className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-tertiary">
              {diagnostic.action}
            </span>
          ) : null}
        </div>
        <span className="text-srapi-text-secondary line-clamp-1 text-xs break-words">
          {log.error_message || "—"}
        </span>
      </div>
    );
  }
  const upstreamDiagnostic = compactUpstreamErrorDiagnostic(log.error_body_excerpt);
  if (upstreamDiagnostic) {
    return (
      <div className="min-w-0 space-y-1">
        <div className="flex flex-wrap gap-1">
          {upstreamDiagnostic.parts.slice(0, 4).map((part) => (
            <span
              key={part}
              className="rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-primary"
            >
              {part}
            </span>
          ))}
        </div>
        <span className="text-srapi-text-secondary line-clamp-1 text-xs break-words">
          {upstreamDiagnostic.message || log.error_message || "—"}
        </span>
      </div>
    );
  }
  return (
    <span className="text-srapi-text-secondary line-clamp-2 text-xs break-words">
      {log.error_message || "—"}
    </span>
  );
}

function ErrorFingerprintStrip({
  query,
  onSelect,
  onOpenExample,
}: {
  query: ErrorLogFingerprintQuery;
  onSelect: (item: OpsErrorFingerprint) => void;
  onOpenExample: (item: OpsErrorFingerprint) => void;
}) {
  const { t } = useLanguage();
  const items = query.data?.data ?? [];
  const meta = query.data?.meta;
  if (query.isLoading) {
    return (
      <div className="border-srapi-border border-b px-4 py-3">
        <div className="bg-srapi-card-muted h-16 animate-pulse rounded-md" />
      </div>
    );
  }
  if (query.isError) {
    return (
      <div className="border-srapi-border text-srapi-error border-b px-4 py-3 text-xs">
        {t("adminErrorLogs.fingerprintsFailed")}
      </div>
    );
  }
  if (items.length === 0) {
    return null;
  }

  return (
    <div className="border-srapi-border border-b px-4 py-3">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <div className="text-srapi-text-primary inline-flex items-center gap-2 text-xs font-semibold">
          <Fingerprint aria-hidden className="text-srapi-text-tertiary size-4" />
          <span>{t("adminErrorLogs.fingerprintsTitle")}</span>
          {meta ? (
            <span className="text-[12px] text-srapi-text-tertiary font-normal">
              {t("adminErrorLogs.fingerprintsMeta", {
                total: meta.total,
                scanned: meta.scanned,
              })}
            </span>
          ) : null}
        </div>
        {meta?.truncated ? (
          <span className="rounded-full bg-srapi-warning/12 px-2 py-0.5 text-[11px] font-medium text-srapi-warning">
            {t("adminErrorLogs.fingerprintsTruncated")}
          </span>
        ) : null}
      </div>
      <div className="grid gap-2 lg:grid-cols-3">
        {items.slice(0, 6).map((item) => (
          <div
            key={item.fingerprint}
            className="border-srapi-border bg-srapi-card/60 min-w-0 rounded-md border p-2"
          >
            <div className="flex min-w-0 items-start justify-between gap-2">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-srapi-text-primary font-mono text-xs font-semibold">
                    {item.error_class || "unknown"}
                  </span>
                  <span className="text-[12px] text-srapi-text-tertiary">
                    {item.status_code ?? item.status_class}
                  </span>
                  <span className="text-[12px] text-srapi-text-tertiary">
                    {item.error_owner || "—"}
                  </span>
                </div>
                <p className="mt-1 line-clamp-2 break-words text-[11px] text-srapi-text-secondary">
                  {item.message_pattern || item.example_error_message || "—"}
                </p>
              </div>
              <span className="tabular text-srapi-text-primary shrink-0 font-mono text-sm font-semibold">
                {formatCompactCount(item.count)}
              </span>
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              <QuietBadge
                status="error"
                label={t("adminErrorLogs.fingerprintOpen", { count: item.open_count })}
                className="px-1.5"
              />
              {item.investigating_count > 0 ? (
                <QuietBadge
                  status="limited"
                  label={t("adminErrorLogs.fingerprintInvestigating", {
                    count: item.investigating_count,
                  })}
                  className="px-1.5"
                />
              ) : null}
              <button
                type="button"
                onClick={() => onSelect(item)}
                className="rounded-full border border-srapi-border bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:text-srapi-text-primary"
              >
                {t("adminErrorLogs.focusFingerprint")}
              </button>
              {item.example_error_log_id ? (
                <button
                  type="button"
                  onClick={() => onOpenExample(item)}
                  className="rounded-full border border-srapi-border bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary transition-colors hover:text-srapi-text-primary"
                >
                  {t("adminErrorLogs.openExample")}
                </button>
              ) : null}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function ErrorClassFilter({
  value,
  onChange,
  ariaLabel,
  placeholder,
}: {
  value: string;
  onChange: (value: string) => void;
  ariaLabel: string;
  placeholder: string;
}) {
  return (
    <Input
      value={value}
      onChange={(e) => onChange(e.target.value.trim())}
      aria-label={ariaLabel}
      placeholder={placeholder}
      className="h-9 w-full font-mono text-xs sm:w-36"
    />
  );
}

function RelatedEvidencePills({ log }: { log: OpsErrorLog }) {
  const { t } = useLanguage();
  const triage = buildErrorLogTriage(log);
  const systemHref = adminSystemLogsHref(log);
  const requestDumpHref = adminRequestDumpsHref(log);
  const requestEvidenceHref = triage.links.find((link) => link.kind === "requestEvidence")?.href;
  if (!systemHref && !requestDumpHref && !requestEvidenceHref) {
    return <span className="text-[12px] text-srapi-text-tertiary">—</span>;
  }

  return (
    <div className="flex flex-wrap gap-1.5">
      {systemHref ? (
        <Link
          href={systemHref}
          className="inline-flex max-w-full items-center gap-1 rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
        >
          <ScrollText aria-hidden className="size-3 shrink-0" />
          <span className="truncate">{t("adminErrorLogs.openSystemLogs")}</span>
        </Link>
      ) : null}
      {requestDumpHref ? (
        <Link
          href={requestDumpHref}
          className="inline-flex max-w-full items-center gap-1 rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
        >
          <FileText aria-hidden className="size-3 shrink-0" />
          <span className="truncate">{t("adminErrorLogs.openRequestDumps")}</span>
        </Link>
      ) : null}
      {requestEvidenceHref ? (
        <Link
          href={requestEvidenceHref}
          className="inline-flex max-w-full items-center gap-1 rounded-full bg-srapi-card-muted px-2 py-0.5 text-[11px] font-medium text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
        >
          <Link2 aria-hidden className="size-3 shrink-0" />
          <span className="truncate">{t("adminErrorLogs.openRequestEvidence")}</span>
        </Link>
      ) : null}
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

function statusFilterBounds(value?: string): { min: number; max: number } | undefined {
  return STATUS_FILTER_OPTIONS.find((option) => option.value === value);
}

function formatCompactCount(value: number): string {
  if (value >= 1000) {
    return `${(value / 1000).toFixed(value >= 10000 ? 0 : 1)}k`;
  }
  return String(value);
}
