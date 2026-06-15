"use client";

import { useMemo, useState } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { Download, Inbox, SearchX } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useAvailableModels, useUsageLogs } from "@/hooks/queries";
import { useUsageTotals } from "@/hooks/use-usage-totals";
import { useLanguage } from "@/context/LanguageContext";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { StatCard, StatCardSkeleton } from "@/components/ui/stat-card";
import {
  Table,
  TableScroll,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import type { UsageLogSummary } from "@/lib/srapi-types";
import { formatMoney } from "@/lib/admin-format";
import { UsageExportDialog } from "@/components/usage-export-dialog";
import { UsageBreakdown } from "./usage-breakdown";

export default function UsagePage() {
  return (
    <AppShell allowedRole="user">
      <UsageContent />
    </AppShell>
  );
}

function UsageContent() {
  const { t } = useLanguage();
  const usage = useUsageLogs();
  const searchParams = useSearchParams();
  const router = useRouter();
  const [exportOpen, setExportOpen] = useState(false);
  const model = searchParams.get("model") ?? "all";
  const status = searchParams.get("status") ?? "all";
  function setFilter(key: string, value: string) {
    const params = new URLSearchParams(searchParams.toString());
    if (value === "all") params.delete(key);
    else params.set(key, value);
    const qs = params.toString();
    router.replace(`/usage${qs ? `?${qs}` : ""}`, { scroll: false });
  }
  const setModel = (v: string) => setFilter("model", v);
  const setStatus = (v: string) => setFilter("status", v);

  const hasLogs = (usage.data?.length ?? 0) > 0;

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionWorkspace")}
        title={t("usage.title")}
        description={t("usage.subtitle")}
        actions={
          <Button
            variant="outline"
            size="sm"
            disabled={!hasLogs}
            onClick={() => setExportOpen(true)}
          >
            <Download className="size-4" aria-hidden />
            {t("usage.export")}
          </Button>
        }
      />

      <PageQueryState query={usage} skeleton={<UsageSkeleton />}>
        {(logs) => <UsageBody logs={logs} model={model} status={status} setModel={setModel} setStatus={setStatus} />}
      </PageQueryState>

      <UsageExportDialog open={exportOpen} onOpenChange={setExportOpen} />
    </>
  );
}

function UsageBody({
  logs,
  model,
  status,
  setModel,
  setStatus,
}: {
  logs: UsageLogSummary[];
  model: string;
  status: string;
  setModel: (v: string) => void;
  setStatus: (v: string) => void;
}) {
  const { t } = useLanguage();
  const availableModels = useAvailableModels();

  const models = useMemo(
    () => (availableModels.data ?? []).map((m) => m.id).sort(),
    [availableModels.data],
  );

  const filtered = useMemo(
    () =>
      logs.filter(
        (l) =>
          (model === "all" || l.model === model) &&
          (status === "all" || (status === "ok" ? l.success : !l.success)),
      ),
    [logs, model, status],
  );

  const totals = useUsageTotals(filtered);

  return (
    <>
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label={t("usage.requests")} value={totals.requests} format={(n) => Math.round(n).toLocaleString()} />
        <StatCard label={t("usage.successRate")} value={totals.successRate} format={(n) => `${n.toFixed(1)}%`} />
        <StatCard label={t("usage.totalTokens")} value={totals.totalTokens} format={(n) => Math.round(n).toLocaleString()} />
        <StatCard label={t("usage.cost")} value={formatMoney(totals.totalCost, totals.currency)} />
      </div>

      <UsageBreakdown logs={filtered} />

      <Card>
        <div className="flex flex-wrap items-center gap-3 border-b border-srapi-border p-4">
          <Select value={model} onValueChange={setModel}>
            <SelectTrigger className="w-44">
              <SelectValue placeholder={t("usage.allModels")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("usage.allModels")}</SelectItem>
              {models.map((m) => (
                <SelectItem key={m} value={m}>
                  {m}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className="w-40">
              <SelectValue placeholder={t("usage.allStatuses")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("usage.allStatuses")}</SelectItem>
              <SelectItem value="ok">{t("usage.successful")}</SelectItem>
              <SelectItem value="failed">{t("usage.failed")}</SelectItem>
            </SelectContent>
          </Select>
          <span className="ml-auto font-mono text-2xs text-srapi-text-tertiary tabular">
            {t("usage.showing", { filtered: filtered.length, total: logs.length })}
          </span>
        </div>

        {filtered.length === 0 ? (
          logs.length === 0 ? (
            <EmptyState icon={Inbox} title={t("usage.emptyTitle")} description={t("usage.emptyBody")} />
          ) : (
            <EmptyState
              icon={SearchX}
              title={t("adminCommon.noResults")}
              description={t("adminCommon.noResultsBody")}
              action={
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setModel("all");
                    setStatus("all");
                  }}
                >
                  {t("adminCommon.clearFilters")}
                </Button>
              }
            />
          )
        ) : (
          <TableScroll minWidth={640}>
            <Table>
              <TableHeader>
                <tr>
                  <TableHead>{t("usage.time")}</TableHead>
                  <TableHead>{t("usage.model")}</TableHead>
                  <TableHead>{t("usage.endpoint")}</TableHead>
                  <TableHead>{t("usage.status")}</TableHead>
                  <TableHead className="text-right">{t("usage.tokens")}</TableHead>
                  <TableHead className="text-right">{t("usage.cost")}</TableHead>
                </tr>
              </TableHeader>
              <TableBody>
                {filtered.map((log) => (
                  <TableRow key={log.request_id}>
                    <TableCell className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
                      {log.created_at.replace("T", " ").slice(0, 16)}
                    </TableCell>
                    <TableCell className="max-w-[180px] truncate text-srapi-text-primary" title={log.model}>{log.model}</TableCell>
                    <TableCell className="max-w-[140px] truncate font-mono text-2xs text-srapi-text-tertiary" title={log.source_endpoint}>
                      {log.source_endpoint}
                    </TableCell>
                    <TableCell>
                      <QuietBadge
                        status={log.success ? "active" : "error"}
                        label={log.success ? t("usage.successful") : t("usage.failed")}
                      />
                    </TableCell>
                    <TableCell className="text-right font-mono text-srapi-text-secondary tabular">
                      {log.total_tokens.toLocaleString()}
                    </TableCell>
                    <TableCell className="text-right font-mono text-srapi-text-secondary tabular">
                      <div>{formatMoney(log.cost, log.currency)}</div>
                      <div className="mt-1 max-w-[200px] truncate text-2xs text-srapi-text-tertiary" title={usageCostBreakdown(log, t)}>
                        {usageCostBreakdown(log, t)}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableScroll>
        )}
      </Card>
    </>
  );
}

function usageCostBreakdown(
  log: UsageLogSummary,
  t: ReturnType<typeof useLanguage>["t"],
): string {
  return [
    `${t("usage.costIn")} ${formatMoney(log.input_cost, log.currency)}`,
    `${t("usage.costOut")} ${formatMoney(log.output_cost, log.currency)}`,
    `${t("usage.costCacheRead")} ${formatMoney(log.cache_read_cost, log.currency)}`,
    `${t("usage.costCacheWrite")} ${formatMoney(log.cache_write_cost, log.currency)}`,
  ].join(" / ");
}

function UsageSkeleton() {
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <StatCardSkeleton key={i} />
        ))}
      </div>
      <DialogListSkeleton rows={8} />
    </div>
  );
}
