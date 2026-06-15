"use client";

import { useState } from "react";
import { Bug } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { ErrorLogDetailDialog } from "@/components/admin/error-log-detail-dialog";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { useAdminErrorLogs, useAdminModels, useAdminUsers } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime, formatLatency } from "@/lib/admin-format";
import type { ErrorLog } from "@/lib/sdk-types";

// Verbose / low-frequency ErrorLog fields start hidden; the always-useful
// columns (time, user, account, model, error, latency, request_id) stay visible.
const DEFAULT_HIDDEN_COLUMNS = ["api_key_id", "provider_id", "protocol", "attempt_no"];

export default function AdminErrorLogsPage() {
  return (
    <AdminShell>
      <ErrorLogsContent />
    </AdminShell>
  );
}

function ErrorLogsContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-error-logs", DEFAULT_HIDDEN_COLUMNS);
  // The clicked row opens the detail modal (which lazily fetches the full record).
  const [detail, setDetail] = useState<{ id: string; email?: string } | null>(null);

  const modelFilter = list.filters.model || undefined;
  const userFilter = list.filters.user || undefined;
  // `search` is mapped to the backend `error_class` free-text filter.
  const errorClassFilter = list.search || undefined;

  // Server-side: page/filters drive the query (the log can grow unbounded).
  const errorLogs = useAdminErrorLogs({
    page: list.page,
    page_size: list.pageSize,
    model: modelFilter,
    user_id: userFilter,
    error_class: errorClassFilter,
  });

  // Filter option sources: catalog models + users (by email).
  const models = useAdminModels({ page: 1, page_size: 100 });
  const usersList = useAdminUsers({ page: 1, page_size: 100 });
  const modelOptions = (models.data?.data ?? []).map((m) => ({
    value: m.canonical_name,
    label: m.canonical_name,
  }));
  const userOptions = (usersList.data?.data ?? []).map((u) => ({
    value: String(u.id),
    label: u.email,
  }));
  const userEmailById = new Map(
    (usersList.data?.data ?? []).map((u) => [String(u.id), u.email] as const),
  );

  const isFiltered = Boolean(modelFilter || userFilter || errorClassFilter);
  const total = errorLogs.data?.pagination?.total ?? errorLogs.data?.data.length ?? 0;

  const emailFor = (e: ErrorLog) => userEmailById.get(String(e.user_id)) || String(e.user_id);
  const openDetail = (e: ErrorLog) => setDetail({ id: e.id, email: emailFor(e) });

  const columns: Column<ErrorLog>[] = [
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
          {formatDateTime(e.created_at)}
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
        <span className="font-mono text-2xs text-srapi-text-tertiary">{e.account_id || "—"}</span>
      ),
    },
    {
      key: "model",
      header: t("adminErrorLogs.model"),
      render: (e) => <span className="text-srapi-text-primary">{e.model || "—"}</span>,
    },
    {
      key: "error_class",
      header: t("adminErrorLogs.errorClass"),
      render: (e) => (
        <span className="font-mono text-xs text-srapi-error">{e.error_class || "—"}</span>
      ),
    },
    {
      key: "latency",
      header: t("adminErrorLogs.latency"),
      align: "right",
      hideOnMobile: true,
      render: (e) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatLatency(e.latency_ms)}
        </span>
      ),
    },
    {
      key: "protocol",
      header: t("adminErrorLogs.protocol"),
      hideOnMobile: true,
      render: (e) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">
          {e.source_protocol}
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
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">{e.attempt_no}</span>
      ),
    },
    {
      key: "api_key_id",
      header: t("adminErrorLogs.apiKey"),
      hideOnMobile: true,
      render: (e) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{e.api_key_id || "—"}</span>
      ),
    },
    {
      key: "provider_id",
      header: t("adminErrorLogs.provider"),
      hideOnMobile: true,
      render: (e) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary">{e.provider_id || "—"}</span>
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
              placeholder={t("adminErrorLogs.errorClassPlaceholder")}
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
