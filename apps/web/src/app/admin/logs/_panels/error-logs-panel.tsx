"use client";

import { useState } from "react";
import { Bug } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { ErrorLogDetailDialog } from "@/components/admin/error-log-detail-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { useAdminErrorLogs, useAdminModels, useResolveErrorLog } from "@/hooks/admin-queries";
import { useAccountNameLookup } from "@/hooks/use-account-name-lookup";
import { useApiKeyNameLookup } from "@/hooks/use-api-key-name-lookup";
import { useProviderNameLookup } from "@/hooks/use-provider-name-lookup";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatLatency } from "@/lib/admin-format";
import type { OpsErrorLog } from "@/lib/sdk-types";
import {
  LOG_WINDOW_PRESETS,
  LOG_WINDOW_ALL_LABEL_KEY,
  logWindowSince,
} from "@/lib/log-window-filter";

const DEFAULT_HIDDEN_COLUMNS = ["api_key_id", "provider_id", "source_endpoint", "error_owner"];

export function ErrorLogsPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-error-logs", DEFAULT_HIDDEN_COLUMNS);
  const accountLookup = useAccountNameLookup();
  const apiKeyLookup = useApiKeyNameLookup();
  const providerLookup = useProviderNameLookup();
  const [detail, setDetail] = useState<{ id: string; email?: string } | null>(null);

  const modelFilter = list.filters.model || undefined;
  const userFilter = list.filters.user || undefined;
  const accountFilter = list.filters.account || undefined;
  const resolutionFilter = list.filters.resolution || undefined;
  const windowFilter = list.filters.window;
  const sinceFilter = logWindowSince(windowFilter)?.toISOString();

  const searchQuery = list.search || undefined;
  const errorLogs = useAdminErrorLogs({
    page: list.page,
    page_size: list.pageSize,
    model: modelFilter,
    user_id: userFilter,
    account_id: accountFilter,
    resolution: resolutionFilter as "open" | "investigating" | "resolved" | "muted" | undefined,
    start: sinceFilter,
    q: searchQuery,
  });
  const resolveMutation = useResolveErrorLog();

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

  const isFiltered = Boolean(modelFilter || userFilter || accountFilter || resolutionFilter || windowFilter || searchQuery);
  const accountOptions = (accountLookup.query.data?.data ?? []).map((a) => ({
    value: String(a.id),
    label: a.name,
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
          className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular underline-offset-2 transition-colors hover:text-srapi-text-primary hover:underline"
        >
          {formatDateTime(e.occurred_at ?? e.created_at)}
        </button>
      ),
    },
    {
      key: "user",
      header: t("adminErrorLogs.user"),
      hideOnMobile: true,
      render: (e) => (
        <span className="truncate text-srapi-text-secondary">{emailFor(e)}</span>
      ),
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
              ? "text-amber-500"
              : "text-srapi-text-tertiary";
        return (
          <span className={`font-mono text-2xs tabular ${color}`}>{code ?? "—"}</span>
        );
      },
    },
    {
      key: "error_class",
      header: t("adminErrorLogs.errorClass"),
      render: (e) => (
        <span className="font-mono text-xs text-srapi-error">{e.error_class || "—"}</span>
      ),
    },
    {
      key: "phase",
      header: t("adminErrorLogs.errorPhase"),
      hideOnMobile: true,
      render: (e) => (
        <div className="flex flex-wrap gap-1">
          {e.error_phase ? (
            <span className="rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-secondary">
              {e.error_phase}
            </span>
          ) : null}
          {e.error_owner ? (
            <span className="rounded bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary">
              {e.error_owner}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      key: "resolution",
      header: t("adminErrorLogs.resolution"),
      render: (e) => (
        <input
          type="checkbox"
          checked={e.resolution === "resolved"}
          disabled={resolveMutation.isPending || !e.id}
          onChange={(ev) => {
            ev.stopPropagation();
            if (e.id) resolveMutation.mutate({ id: e.id, resolved: ev.target.checked });
          }}
          onClick={(ev) => ev.stopPropagation()}
          aria-label={
            e.resolution === "resolved" ? t("adminErrorLogs.markUnresolved") : t("adminErrorLogs.markResolved")
          }
          className="h-4 w-4 cursor-pointer accent-srapi-accent"
        />
      ),
    },
    {
      // Verbatim upstream provider message (sub2api parity:
      // ops_error_logs.upstream_error_message). Truncated visually but the
      // full text + body excerpt live in the detail dialog.
      key: "error_message",
      header: t("adminErrorLogs.upstreamMessage"),
      hideOnMobile: true,
      render: (e) => (
        <span className="line-clamp-2 break-words text-xs text-srapi-text-secondary">
          {e.error_message || "—"}
        </span>
      ),
    },
    {
      key: "latency",
      header: t("adminErrorLogs.latency"),
      align: "right",
      hideOnMobile: true,
      render: (e) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatLatency(e.latency_ms ?? 0)}
        </span>
      ),
    },
    {
      key: "protocol",
      header: t("adminErrorLogs.protocol"),
      hideOnMobile: true,
      render: (e) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">
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
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {e.attempt_no ?? 1}
        </span>
      ),
    },
    {
      key: "source_endpoint",
      header: t("adminErrorLogs.sourceEndpoint"),
      hideOnMobile: true,
      render: (e) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">
          {e.source_endpoint || "—"}
        </span>
      ),
    },
    {
      key: "error_owner",
      header: t("adminErrorLogs.errorOwner"),
      hideOnMobile: true,
      render: (e) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">
          {e.error_owner || "—"}
        </span>
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
          className="font-mono text-2xs text-srapi-text-tertiary underline-offset-2 transition-colors hover:text-srapi-text-primary hover:underline"
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
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
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
        toolbar={
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
