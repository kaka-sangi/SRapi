"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useCleanupOpsSystemLogs } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { localDateTimeInputToIso } from "@/lib/datetime-local";
import type { OpsSystemLogCleanupResult, OpsSystemLogLevel } from "@/lib/sdk-types";

const LEVELS: OpsSystemLogLevel[] = ["debug", "info", "warn", "error"];
const ANY = "any";

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
  const [level, setLevel] = useState<string>(ANY);
  const [source, setSource] = useState("");
  const [q, setQ] = useState("");
  const [requestID, setRequestID] = useState("");
  const [traceID, setTraceID] = useState("");
  const [before, setBefore] = useState("");
  const [maxDelete, setMaxDelete] = useState("1000");
  const [dryRun, setDryRun] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<OpsSystemLogCleanupResult | null>(null);

  const hasFilter = level !== ANY || Boolean(source.trim() || q.trim() || requestID.trim() || traceID.trim() || before.trim());

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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="text-lg font-semibold tracking-tight">{t("adminOpsCleanup.title")}</DialogTitle>
            <DialogDescription>{t("adminOpsCleanup.subtitle")}</DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-4">
            <div>
              <Label htmlFor="cl-level">{t("adminOpsCleanup.level")}</Label>
              <Select value={level} onValueChange={setLevel}>
                <SelectTrigger id="cl-level">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ANY}>{t("adminOpsCleanup.anyLevel")}</SelectItem>
                  {LEVELS.map((l) => (
                    <SelectItem key={l} value={l}>
                      {l}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="cl-source">{t("adminOpsCleanup.source")}</Label>
              <Input
                id="cl-source"
                value={source}
                onChange={(e) => setSource(e.target.value)}
                placeholder="scheduler, gateway…"
              />
            </div>
            <div>
              <Label htmlFor="cl-q">{t("adminOpsCleanup.q")}</Label>
              <Input id="cl-q" value={q} onChange={(e) => setQ(e.target.value)} />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <Label htmlFor="cl-request-id">{t("adminOpsCleanup.requestId")}</Label>
                <Input
                  id="cl-request-id"
                  value={requestID}
                  onChange={(e) => setRequestID(e.target.value)}
                  className="text-xs"
                />
              </div>
              <div>
                <Label htmlFor="cl-trace-id">{t("adminOpsCleanup.traceId")}</Label>
                <Input
                  id="cl-trace-id"
                  value={traceID}
                  onChange={(e) => setTraceID(e.target.value)}
                  className="text-xs"
                />
              </div>
            </div>
            <div>
              <Label htmlFor="cl-before">{t("adminOpsCleanup.before")}</Label>
              <Input
                id="cl-before"
                type="datetime-local"
                value={before}
                onChange={(e) => setBefore(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="cl-max">{t("adminOpsCleanup.maxDelete")}</Label>
              <Input
                id="cl-max"
                type="number"
                min={1}
                inputMode="numeric"
                value={maxDelete}
                onChange={(e) => setMaxDelete(e.target.value)}
              />
            </div>
            <div className="flex items-center justify-between">
              <Label htmlFor="cl-dry" className="mb-0">
                {t("adminOpsCleanup.dryRun")}
              </Label>
              <Switch
                id="cl-dry"
                checked={dryRun}
                onCheckedChange={(v) => {
                  setDryRun(v);
                  setResult(null);
                }}
              />
            </div>
            <p className="text-[11px] text-srapi-text-tertiary">{t("adminOpsCleanup.hint")}</p>
            {result ? (
              <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-3 text-xs text-srapi-text-secondary">
                {result.dry_run
                  ? t("adminOpsCleanup.previewResult", { matched: String(result.matched) })
                  : t("adminOpsCleanup.deletedResult", {
                      deleted: String(result.deleted),
                      matched: String(result.matched),
                    })}
                {result.limited
                  ? ` · ${t("adminOpsCleanup.limited", { max: String(result.max_delete) })}`
                  : ""}
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
              {dryRun ? t("adminOpsCleanup.preview") : t("adminOpsCleanup.delete")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
