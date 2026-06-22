"use client";

import { useEffect } from "react";
import { Download } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { IconBubble } from "@/components/ui/icon-bubble";
import { Kbd } from "@/components/ui/kbd";
import { useToast } from "@/context/ToastContext";
import { useLanguage } from "@/context/LanguageContext";
import { useUsageExport } from "@/hooks/use-usage-export";

/**
 * Streams the current user's usage logs into a CSV, paging through
 * /me/usage one page at a time. Surfaces a live progress bar (current /
 * total / percent), an aria-live status line for screen readers, and a
 * cancel button backed by an AbortController in {@link useUsageExport}.
 */
export function UsageExportDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const { progress, isExporting, start, cancel, reset } = useUsageExport();

  // Kick the export off when the dialog opens; tear it down when it closes.
  useEffect(() => {
    if (!open) {
      reset();
      return;
    }
    let active = true;
    void start().then((rows) => {
      if (!active) return;
      if (rows > 0) {
        toast({ title: t("usage.exportDone", { count: rows }), tone: "success" });
      }
    });
    return () => {
      active = false;
    };
    // start/reset are stable (useCallback); we intentionally run this only on open changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  // Surface a one-time error toast when the run fails.
  useEffect(() => {
    if (progress.phase === "error") {
      toast({
        title: t("usage.exportFailed"),
        description: progress.error,
        tone: "error",
      });
    }
  }, [progress.phase, progress.error, toast, t]);

  // Block closing via the backdrop while a fetch is in flight so the user
  // makes a deliberate choice (cancel) rather than losing progress silently.
  function handleOpenChange(next: boolean) {
    if (!next && isExporting) {
      cancel();
    }
    onOpenChange(next);
  }

  const percent = progress.percent;
  const totalKnown = progress.total > 0;

  const statusLabel =
    progress.phase === "done"
      ? t("usage.exportDone", { count: progress.current })
      : progress.phase === "cancelled"
        ? t("usage.exportCancelled")
        : progress.phase === "error"
          ? t("usage.exportFailed")
          : totalKnown
            ? t("usage.exportProgressCount", { current: progress.current, total: progress.total })
            : t("usage.exportPreparing");

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2.5 text-lg font-semibold tracking-tight">
            <IconBubble
              tone={
                progress.phase === "error"
                  ? "error"
                  : progress.phase === "done"
                    ? "success"
                    : "accent"
              }
              size="sm"
            >
              <Download aria-hidden />
            </IconBubble>
            {t("usage.exportTitle")}
          </DialogTitle>
          <DialogDescription>{t("usage.exportSubtitle")}</DialogDescription>
        </DialogHeader>

        <div className="mt-1 space-y-3">
          <div className="flex items-center justify-between text-sm text-srapi-text-secondary">
            <span className="flex items-center gap-2">
              {isExporting ? <Spinner /> : null}
              <span className="tabular">
                {totalKnown
                  ? t("usage.exportProgressCount", {
                      current: progress.current,
                      total: progress.total,
                    })
                  : t("usage.exportPreparing")}
              </span>
            </span>
            <span className="metric-primary tabular tracking-tight">{percent}%</span>
          </div>

          <div className="h-2 w-full overflow-hidden rounded-full bg-srapi-card-muted">
            <div
              role="progressbar"
              aria-valuenow={percent}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-label={t("usage.exportTitle")}
              className="h-full rounded-full bg-srapi-primary transition-[width] duration-200 ease-out"
              style={{ width: `${percent}%` }}
            />
          </div>

          {/* Screen-reader live region — announces phase / count changes. */}
          <p className="sr-only" aria-live="polite" aria-atomic="true" role="status">
            {statusLabel}
          </p>

          {progress.phase === "error" && progress.error ? (
            <p className="text-xs text-srapi-error" role="alert">
              {progress.error}
            </p>
          ) : null}
        </div>

        <DialogFooter>
          {isExporting ? (
            <div className="flex items-center gap-2">
              <span className="hidden items-center gap-1.5 text-[11px] text-srapi-text-tertiary sm:inline-flex">
                <Kbd>Esc</Kbd>
                <span>{t("usage.exportCancel")}</span>
              </span>
              <Button variant="outline" size="sm" onClick={cancel}>
                {t("usage.exportCancel")}
              </Button>
            </div>
          ) : (
            <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
              {t("common.close")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
