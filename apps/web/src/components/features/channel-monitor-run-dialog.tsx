"use client";

import { CheckCircle2, XCircle, Play, History } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { IconBubble } from "@/components/ui/icon-bubble";
import { DataPill } from "@/components/ui/data-pill";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { useRunChannelMonitor, useChannelMonitorRuns } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { formatDateTime } from "@/lib/admin-format";
import { cn } from "@/lib/cn";
import type { ChannelMonitorCheckResult } from "@/lib/sdk-types";

function formatLatency(ms: number | undefined): string {
  if (ms == null) return "—";
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${Math.round(ms)}ms`;
}

/**
 * ChannelMonitorRunDialog — runs a synthetic-probe monitor on demand and shows
 * the per-account / per-model CheckResult breakdown, plus recent run history.
 * The run is triggered by an explicit button (no fetch-in-effect); history is a
 * normal query gated by the open dialog.
 */
export function ChannelMonitorRunDialog({
  open,
  onOpenChange,
  monitorId,
  monitorName,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  monitorId: string;
  monitorName: string;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const runMut = useRunChannelMonitor();
  const history = useChannelMonitorRuns(open ? monitorId : null);

  const results = runMut.data?.results ?? [];

  async function handleRun() {
    try {
      await runMut.mutateAsync(monitorId);
    } catch (err) {
      toast({ title: adminErrorMessage(err), tone: "error" });
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2.5 text-lg font-semibold tracking-tight">
            <IconBubble
              tone={
                runMut.data
                  ? runMut.data.ok_count === runMut.data.checked_count
                    ? "success"
                    : runMut.data.ok_count === 0
                      ? "error"
                      : "warning"
                  : "accent"
              }
              size="sm"
            >
              <Play aria-hidden />
            </IconBubble>
            {t("adminMonitor.runTitle")}
          </DialogTitle>
          <DialogDescription className="truncate font-mono text-xs">
            {monitorName}
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-center justify-between">
          {runMut.data ? (
            <DataTooltip
              title={t("adminMonitor.runTitle")}
              primary={t("adminMonitor.runSummary", {
                ok: runMut.data.ok_count,
                total: runMut.data.checked_count,
              })}
              rows={[
                { label: t("apiKeys.usageOk"), value: runMut.data.ok_count, tone: "success" },
                {
                  label: t("apiKeys.usageErrors"),
                  value: runMut.data.checked_count - runMut.data.ok_count,
                  tone: runMut.data.ok_count === runMut.data.checked_count ? "muted" : "error",
                },
              ]}
            >
              <DataPill
                tone={
                  runMut.data.ok_count === runMut.data.checked_count
                    ? "success"
                    : runMut.data.ok_count === 0
                      ? "error"
                      : "warning"
                }
              >
                {t("adminMonitor.runSummary", {
                  ok: runMut.data.ok_count,
                  total: runMut.data.checked_count,
                })}
              </DataPill>
            </DataTooltip>
          ) : (
            <span className="text-xs text-srapi-text-tertiary">{t("adminMonitor.runHint")}</span>
          )}
          <Button variant="outline" size="sm" onClick={handleRun} loading={runMut.isPending}>
            <Play className="size-3.5" />
            {runMut.data ? t("adminMonitor.runAgain") : t("adminMonitor.runNow")}
          </Button>
        </div>

        {results.length > 0 ? (
          <ul className="overflow-hidden rounded-xl border border-srapi-border bg-srapi-card-muted">
            {results.map((result) => (
              <CheckRow key={`${result.account_id}-${result.model}`} result={result} />
            ))}
          </ul>
        ) : runMut.data ? (
          <IllustratedEmptyState
            illust="search"
            title={t("adminMonitor.runNoTargets")}
            className="py-6"
          />
        ) : null}

        <div className="border-t border-srapi-border/70 pt-3">
          <div className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            <History className="size-3.5" />
            {t("adminMonitor.runHistory")}
          </div>
          {history.data && history.data.data.length > 0 ? (
            <ul className="divide-y divide-srapi-border/70">
              {history.data.data.map((run) => {
                const allOk = run.ok_count === run.checked_count;
                const sev = run.ok ? (allOk ? "success" : "warning") : "error";
                return (
                  <li
                    key={run.id}
                    className="log-row flex items-center justify-between gap-3 py-2 pl-3 pr-2 text-xs text-srapi-text-secondary"
                    data-sev={sev}
                  >
                    <span className="flex items-center gap-1.5">
                      {run.ok ? (
                        <CheckCircle2 className="size-3 text-srapi-success" />
                      ) : (
                        <XCircle className="size-3 text-srapi-error" />
                      )}
                      {t("adminMonitor.runSummary", { ok: run.ok_count, total: run.checked_count })}
                    </span>
                    <span className="text-[12px] tabular text-srapi-text-tertiary">
                      {formatDateTime(run.created_at)}
                    </span>
                  </li>
                );
              })}
            </ul>
          ) : (
            <p className="text-xs text-srapi-text-tertiary">{t("adminMonitor.runNoHistory")}</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function CheckRow({ result }: { result: ChannelMonitorCheckResult }) {
  const sev = result.ok ? "success" : "error";
  const rows = [
    { label: "account", value: result.account_name },
    ...(result.model ? [{ label: "model", value: result.model }] : []),
    {
      label: "status",
      value: String(result.status_code || "—"),
      tone:
        result.status_code >= 200 && result.status_code < 300
          ? ("success" as const)
          : ("error" as const),
    },
    { label: "latency", value: formatLatency(result.latency_ms) },
    ...(result.error_class ? [{ label: "error", value: result.error_class, tone: "error" as const }] : []),
  ];
  return (
    <li
      className="log-row flex items-center justify-between gap-3 border-b border-srapi-border/70 py-1.5 pl-3 pr-2.5 text-xs last:border-b-0"
      data-sev={sev}
    >
      <span className="flex min-w-0 items-center gap-1.5 truncate">
        {result.ok ? (
          <CheckCircle2 className="size-3 shrink-0 text-srapi-success" />
        ) : (
          <XCircle className="size-3 shrink-0 text-srapi-error" />
        )}
        <span className="truncate font-medium text-srapi-text-primary">{result.account_name}</span>
        {result.model ? (
          <span className="truncate font-mono text-[11px] text-srapi-text-tertiary">{result.model}</span>
        ) : null}
      </span>
      <DataTooltip title={result.account_name} rows={rows}>
        <span className="flex shrink-0 items-center gap-2">
          {result.error_class ? (
            <span className="max-w-[10rem] truncate text-srapi-error">{result.error_class}</span>
          ) : null}
          <span
            className={cn(
              "tabular",
              result.status_code >= 200 && result.status_code < 300
                ? "text-srapi-text-secondary"
                : "text-srapi-text-tertiary",
            )}
          >
            {result.status_code || "—"}
          </span>
          <span className="tabular text-srapi-text-tertiary">{formatLatency(result.latency_ms)}</span>
        </span>
      </DataTooltip>
    </li>
  );
}
