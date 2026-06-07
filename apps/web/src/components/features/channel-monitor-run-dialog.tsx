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
          <DialogTitle className="flex items-center gap-2">
            <Play className="size-4 text-srapi-text-tertiary" />
            {t("adminMonitor.runTitle")}
          </DialogTitle>
          <DialogDescription className="truncate font-mono text-2xs">
            {monitorName}
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-center justify-between">
          <span className="text-2xs text-srapi-text-tertiary">
            {runMut.data
              ? t("adminMonitor.runSummary", {
                  ok: runMut.data.ok_count,
                  total: runMut.data.checked_count,
                })
              : t("adminMonitor.runHint")}
          </span>
          <Button variant="outline" size="sm" onClick={handleRun} loading={runMut.isPending}>
            <Play className="size-3.5" />
            {runMut.data ? t("adminMonitor.runAgain") : t("adminMonitor.runNow")}
          </Button>
        </div>

        {results.length > 0 ? (
          <div className="space-y-1 rounded-lg border border-srapi-border bg-srapi-card-muted p-2.5">
            {results.map((result) => (
              <CheckRow key={`${result.account_id}-${result.model}`} result={result} />
            ))}
          </div>
        ) : runMut.data ? (
          <p className="text-2xs text-srapi-text-tertiary">{t("adminMonitor.runNoTargets")}</p>
        ) : null}

        <div className="border-t border-srapi-border pt-3">
          <div className="mb-1.5 flex items-center gap-1.5 text-2xs font-medium text-srapi-text-tertiary">
            <History className="size-3.5" />
            {t("adminMonitor.runHistory")}
          </div>
          {history.data && history.data.data.length > 0 ? (
            <ul className="space-y-1">
              {history.data.data.map((run) => (
                <li
                  key={run.id}
                  className="flex items-center justify-between gap-3 font-mono text-2xs text-srapi-text-secondary"
                >
                  <span className="flex items-center gap-1.5">
                    {run.ok ? (
                      <CheckCircle2 className="size-3 text-srapi-success" />
                    ) : (
                      <XCircle className="size-3 text-srapi-error" />
                    )}
                    {t("adminMonitor.runSummary", { ok: run.ok_count, total: run.checked_count })}
                  </span>
                  <span className="tabular text-srapi-text-tertiary">
                    {formatDateTime(run.created_at)}
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-2xs text-srapi-text-tertiary">{t("adminMonitor.runNoHistory")}</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function CheckRow({ result }: { result: ChannelMonitorCheckResult }) {
  return (
    <div className="flex items-center justify-between gap-3 font-mono text-2xs">
      <span className="flex items-center gap-1.5 truncate">
        {result.ok ? (
          <CheckCircle2 className="size-3 shrink-0 text-srapi-success" />
        ) : (
          <XCircle className="size-3 shrink-0 text-srapi-error" />
        )}
        <span className="truncate text-srapi-text-primary">{result.account_name}</span>
        {result.model ? (
          <span className="truncate text-srapi-text-tertiary">{result.model}</span>
        ) : null}
      </span>
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
    </div>
  );
}
