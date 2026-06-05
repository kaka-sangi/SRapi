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
import { useCleanupAdminUsage } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { adminErrorMessage } from "@/lib/admin-api";
import type { UsageCleanupResult } from "@/lib/sdk-types";

const ANY = "any";

/**
 * Operator on-demand deletion of historical usage records — the counterpart to
 * the background retention worker, which only purges by age. The backend
 * requires at least one bounded filter (model/start/end) and caps deletion at
 * max_delete; dry_run previews the match count without deleting. The dialog
 * stays open after a run so an operator can preview, then re-run with dry_run
 * off to actually delete the oldest matching batch.
 */
export function UsageCleanupDialog({
  open,
  onOpenChange,
  modelOptions,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  modelOptions: { value: string; label: string }[];
}) {
  const { t } = useLanguage();
  const cleanup = useCleanupAdminUsage();
  const [model, setModel] = useState<string>(ANY);
  const [start, setStart] = useState("");
  const [before, setBefore] = useState("");
  const [maxDelete, setMaxDelete] = useState("1000");
  const [dryRun, setDryRun] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<UsageCleanupResult | null>(null);

  const hasFilter = model !== ANY || Boolean(start.trim() || before.trim());

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (!hasFilter) {
      setError(t("adminUsageCleanup.needFilter"));
      return;
    }
    try {
      const res = await cleanup.mutateAsync({
        model: model !== ANY ? model : undefined,
        start: start.trim() ? new Date(start).toISOString() : undefined,
        end: before.trim() ? new Date(before).toISOString() : undefined,
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
            <DialogTitle>{t("adminUsageCleanup.title")}</DialogTitle>
            <DialogDescription>{t("adminUsageCleanup.subtitle")}</DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-4">
            <div>
              <Label htmlFor="ucl-model">{t("adminUsageCleanup.model")}</Label>
              <Select value={model} onValueChange={setModel}>
                <SelectTrigger id="ucl-model">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ANY}>{t("adminUsageCleanup.anyModel")}</SelectItem>
                  {modelOptions.map((m) => (
                    <SelectItem key={m.value} value={m.value}>
                      {m.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label htmlFor="ucl-start">{t("adminUsageCleanup.start")}</Label>
              <Input
                id="ucl-start"
                type="datetime-local"
                value={start}
                onChange={(e) => setStart(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="ucl-before">{t("adminUsageCleanup.before")}</Label>
              <Input
                id="ucl-before"
                type="datetime-local"
                value={before}
                onChange={(e) => setBefore(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="ucl-max">{t("adminUsageCleanup.maxDelete")}</Label>
              <Input
                id="ucl-max"
                type="number"
                min={1}
                inputMode="numeric"
                value={maxDelete}
                onChange={(e) => setMaxDelete(e.target.value)}
              />
            </div>
            <div className="flex items-center justify-between">
              <Label htmlFor="ucl-dry" className="mb-0">
                {t("adminUsageCleanup.dryRun")}
              </Label>
              <Switch
                id="ucl-dry"
                checked={dryRun}
                onCheckedChange={(v) => {
                  setDryRun(v);
                  setResult(null);
                }}
              />
            </div>
            <p className="text-2xs text-srapi-text-tertiary">{t("adminUsageCleanup.hint")}</p>
            {result ? (
              <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-3 text-xs text-srapi-text-secondary">
                {result.dry_run
                  ? t("adminUsageCleanup.previewResult", { matched: String(result.matched) })
                  : t("adminUsageCleanup.deletedResult", {
                      deleted: String(result.deleted),
                      matched: String(result.matched),
                    })}
                {result.limited
                  ? ` · ${t("adminUsageCleanup.limited", { max: String(result.max_delete) })}`
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
              {dryRun ? t("adminUsageCleanup.preview") : t("adminUsageCleanup.delete")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
