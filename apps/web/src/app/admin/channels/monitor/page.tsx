"use client";

import { useState } from "react";
import { Gauge } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { useAdminList } from "@/hooks/use-admin-list";
import { useClientPagedList } from "@/hooks/use-client-list";
import { useAccountsAvailability } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatPercent, formatDateTime } from "@/lib/admin-format";
import { cn } from "@/lib/cn";
import type { AccountAvailabilitySummary } from "@/lib/sdk-types";

const WINDOW_OPTIONS = [7, 14, 30, 90];

function monitorMatch(row: AccountAvailabilitySummary, term: string): boolean {
  if (!term) return true;
  return [row.account_name, row.status].filter(Boolean).join(" ").toLowerCase().includes(term);
}

// Worst uptime first — the accounts that need attention surface at the top.
const monitorCompare = (a: AccountAvailabilitySummary, b: AccountAvailabilitySummary) =>
  a.overall_uptime - b.overall_uptime || a.account_name.localeCompare(b.account_name);

export default function AdminMonitorPage() {
  return (
    <AdminShell>
      <MonitorContent />
    </AdminShell>
  );
}

function MonitorContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const [days, setDays] = useState(7);
  const all = useAccountsAvailability(days);
  const { query, total } = useClientPagedList(all, list, {
    match: monitorMatch,
    compare: monitorCompare,
  });
  const isFiltered = Boolean(list.search);

  const columns: Column<AccountAvailabilitySummary>[] = [
    {
      key: "account",
      header: t("adminMonitor.account"),
      render: (r) => <span className="text-srapi-text-primary">{r.account_name}</span>,
    },
    {
      key: "status",
      header: t("adminMonitor.status"),
      render: (r) => <QuietBadge status={quietStatusFor(r.status)} label={statusLabel(t, r.status)} />,
    },
    {
      key: "uptime",
      header: t("adminMonitor.uptime"),
      align: "right",
      render: (r) => (
        <span
          className={cn(
            "font-mono tabular",
            r.overall_uptime < 0.95 ? "text-srapi-error" : "text-srapi-text-secondary",
          )}
        >
          {formatPercent(r.overall_uptime)}
        </span>
      ),
    },
    {
      key: "checked",
      header: t("adminMonitor.lastChecked"),
      align: "right",
      hideOnMobile: true,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(r.last_checked_at)}
        </span>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminMonitor.title")}
        description={t("adminMonitor.subtitle")}
        actions={all.data ? <ListCount total={total} /> : undefined}
      />
      <AdminListView
        query={query}
        columns={columns}
        getRowId={(r) => String(r.account_id)}
        emptyIcon={Gauge}
        emptyTitle={t("adminMonitor.emptyTitle")}
        emptyBody={t("adminMonitor.emptyBody")}
        minWidth={640}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminMonitor.searchPlaceholder")}
            />
            <Select
              value={String(days)}
              onValueChange={(v) => {
                setDays(Number(v));
                list.setPage(1);
              }}
            >
              <SelectTrigger className="h-9 w-auto min-w-[7rem] gap-2 rounded-lg">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WINDOW_OPTIONS.map((d) => (
                  <SelectItem key={d} value={String(d)}>
                    {t("adminMonitor.windowDays", { days: d })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
      />
    </>
  );
}
