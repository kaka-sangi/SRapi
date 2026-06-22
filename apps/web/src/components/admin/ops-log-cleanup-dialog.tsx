"use client";

import { useState } from "react";
import { Eraser, Eye, Trash2 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { FloatingInput } from "@/components/ui/floating-input";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { DataPill } from "@/components/ui/data-pill";
import { IconBubble } from "@/components/ui/icon-bubble";
import { SectionTitle } from "@/components/ui/section-title";
import { useCleanupOpsSystemLogs } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { formatInteger } from "@/lib/admin-format";
import { localDateTimeInputToIso } from "@/lib/datetime-local";
import type { OpsSystemLogCleanupResult, OpsSystemLogLevel } from "@/lib/sdk-types";

const LEVELS: OpsSystemLogLevel[] = ["debug", "info", "warn", "error"];
const ANY = "any";
type LevelChoice = typeof ANY | OpsSystemLogLevel;

/**
 * Bounded cleanup of sanitized system logs. The backend requires at least one
 * filter and caps deletion at max_delete; dry_run previews the match count
 * without deleting. The dialog stays open after a run so an admin can preview,
 * then re-run with dry_run off to actually delete.
 */
export function OpsLogCleanupDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLanguage();
  const cleanup = useCleanupOpsSystemLogs();
  const [level, setLevel] = useState<LevelChoice>(ANY);
  const [source, setSource] = useState("");
  const [q, setQ] = useState("");
  const [requestID, setRequestID] = useState("");
  const [traceID, setTraceID] = useState("");
  const [before, setBefore] = useState("");
  const [maxDelete, setMaxDelete] = useState("1000");
  const [dryRun, setDryRun] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<OpsSystemLogCleanupResult | null>(null);

  const hasFilter =
    level !== ANY ||
    Boolean(source.trim() || q.trim() || requestID.trim() || traceID.trim() || before.trim());

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (!hasFilter) {
      setError(t("adminOpsCleanup.needFilter"));
      return;
    }
    const end = localDateTimeInputToIso(before);
    if (before.trim() && !end) {
      setError(t("adminOpsCleanup.invalidBefore"));
      return;
    }
    try {
      const res = await cleanup.mutateAsync({
        level: level !== ANY ? (level as OpsSystemLogLevel) : undefined,
        source: source.trim() || undefined,
        q: q.trim() || undefined,
        request_id: requestID.trim() || undefined,
        trace_id: traceID.trim() || undefined,
        end: end ?? undefined,
        dry_run: dryRun,
        max_delete: Number(maxDelete.trim()) || undefined,
      });
      setResult(res);
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  const bubbleTone = dryRun ? "accent" : "error";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <form onSubmit={submit}>
          <DialogHeader>
            <div className="flex items-start gap-3">
              <IconBubble tone={bubbleTone} size="md">
                {dryRun ? <Eye /> : <Trash2 />}
              </IconBubble>
              <div className="min-w-0 flex-1">
                <DialogTitle className="text-lg font-semibold tracking-tight">
                  {t("adminOpsCleanup.title")}
                </DialogTitle>
                <DialogDescription className="mt-0.5">
                  {t("adminOpsCleanup.subtitle")}
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>

          <div className="mt-5 space-y-5">
            {/* Level filter (SegmentedControl) */}
            <div>
              <Label className="mb-1.5 block">{t("adminOpsCleanup.level")}</Label>
              <SegmentedControl<LevelChoice>
                value={level}
                onChange={setLevel}
                options={[
                  { value: ANY, label: t("adminOpsCleanup.anyLevel") },
                  ...LEVELS.map((l) => ({ value: l as LevelChoice, label: l })),
                ]}
                ariaLabel={t("adminOpsCleanup.level")}
                size="sm"
              />
            </div>

            {/* Source + search */}
            <div className="grid gap-3 sm:grid-cols-2">
              <FloatingInput
                label={t("adminOpsCleanup.source")}
                value={source}
                onChange={setSource}
                placeholder="scheduler, gateway…"
              />
              <FloatingInput
                label={t("adminOpsCleanup.q")}
                value={q}
                onChange={setQ}
              />
            </div>

            {/* Correlation IDs */}
            <div className="space-y-3 rounded-xl border border-srapi-border/70 bg-srapi-card-muted/30 p-4">
              <SectionTitle label={t("adminOpsCleanup.requestId")} />
              <div className="grid gap-3 sm:grid-cols-2">
                <div>
                  <Label htmlFor="cl-request-id" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                    {t("adminOpsCleanup.requestId")}
                  </Label>
                  <Input
                    id="cl-request-id"
                    value={requestID}
                    onChange={(e) => setRequestID(e.target.value)}
                    className="font-mono text-xs"
                  />
                </div>
                <div>
                  <Label htmlFor="cl-trace-id" className="mb-1.5 block text-[11px] text-srapi-text-tertiary">
                    {t("adminOpsCleanup.traceId")}
                  </Label>
                  <Input
                    id="cl-trace-id"
                    value={traceID}
                    onChange={(e) => setTraceID(e.target.value)}
                    className="font-mono text-xs"
                  />
                </div>
              </div>
            </div>

            {/* Time bound + max delete */}
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <Label htmlFor="cl-before" className="mb-1.5 block">
                  {t("adminOpsCleanup.before")}
                </Label>
                <Input
                  id="cl-before"
                  type="datetime-local"
                  value={before}
                  onChange={(e) => setBefore(e.target.value)}
                />
              </div>
              <div>
                <Label htmlFor="cl-max" className="mb-1.5 block">
                  {t("adminOpsCleanup.maxDelete")}
                </Label>
                <Input
                  id="cl-max"
                  type="number"
                  min={1}
                  inputMode="numeric"
                  value={maxDelete}
                  onChange={(e) => setMaxDelete(e.target.value)}
                />
              </div>
            </div>

            {/* Dry run toggle */}
            <div className="flex items-center justify-between gap-4 rounded-xl border border-srapi-border bg-srapi-card px-4 py-3">
              <div className="min-w-0">
                <Label htmlFor="cl-dry" className="mb-0">
                  {t("adminOpsCleanup.dryRun")}
                </Label>
                <p className="mt-0.5 text-[11px] text-srapi-text-tertiary">
                  {t("adminOpsCleanup.hint")}
                </p>
              </div>
              <Switch
                id="cl-dry"
                checked={dryRun}
                onCheckedChange={(v) => {
                  setDryRun(v);
                  setResult(null);
                }}
              />
            </div>

            {/* Result panel */}
            {result ? (
              <div
                className="log-row rounded-xl border border-srapi-border bg-srapi-card p-4"
                data-sev={result.dry_run ? "info" : "warning"}
              >
                <div className="flex flex-wrap items-baseline gap-x-4 gap-y-2">
                  <div>
                    <p className="text-[11px] uppercase tracking-[0.12em] text-srapi-text-tertiary">
                      {result.dry_run
                        ? t("adminOpsCleanup.dryRun")
                        : t("adminOpsCleanup.delete")}
                    </p>
                    <p className="metric-primary tabular text-srapi-text-primary">
                      {formatInteger(result.dry_run ? result.matched : result.deleted)}
                    </p>
                  </div>
                  {!result.dry_run ? (
                    <div>
                      <p className="text-[11px] uppercase tracking-[0.12em] text-srapi-text-tertiary">
                        matched
                      </p>
                      <p className="metric-secondary tabular text-srapi-text-secondary">
                        {formatInteger(result.matched)}
                      </p>
                    </div>
                  ) : null}
                  {result.limited ? (
                    <DataPill tone="warning" size="sm">
                      {t("adminOpsCleanup.limited", { max: String(result.max_delete) })}
                    </DataPill>
                  ) : null}
                </div>
              </div>
            ) : null}

            {error ? (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>

          <DialogFooter className="mt-6">
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
              disabled={cleanup.isPending}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              variant={dryRun ? "primary" : "danger"}
              loading={cleanup.isPending}
              disabled={!hasFilter}
            >
              {dryRun ? <Eye /> : <Eraser />}
              {dryRun ? t("adminOpsCleanup.preview") : t("adminOpsCleanup.delete")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
