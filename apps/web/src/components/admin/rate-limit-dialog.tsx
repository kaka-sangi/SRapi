"use client";

import { useState } from "react";
import { Gauge, Activity, Zap, Layers, Info } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { FloatingInput } from "@/components/ui/floating-input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DataPill } from "@/components/ui/data-pill";
import { IconBubble } from "@/components/ui/icon-bubble";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";

export interface RateLimitValues {
  rpm_limit: number;
  tpm_limit: number;
  max_concurrency: number;
  enabled: boolean;
}

/**
 * Edit a per-model or per-account-group rate limit (RPM / TPM / max concurrency
 * + enabled). 0 / blank = unlimited for that dimension. When a limit already
 * exists, a "clear" action removes it entirely. The parent owns the id and the
 * upsert/delete mutations; this dialog only collects the four values.
 */
export function RateLimitDialog({
  open,
  onOpenChange,
  title,
  current,
  onSubmit,
  onClear,
  isPending,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  current?: RateLimitValues;
  onSubmit: (values: RateLimitValues) => Promise<unknown>;
  onClear?: () => Promise<unknown>;
  isPending?: boolean;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const [rpm, setRpm] = useState(current ? String(current.rpm_limit) : "");
  const [tpm, setTpm] = useState(current ? String(current.tpm_limit) : "");
  const [concurrency, setConcurrency] = useState(current ? String(current.max_concurrency) : "");
  const [enabled, setEnabled] = useState(current ? current.enabled : true);
  const [error, setError] = useState<string | null>(null);

  function toCount(value: string): number {
    const parsed = Number(value.trim() || "0");
    return Number.isFinite(parsed) && parsed >= 0 ? Math.floor(parsed) : 0;
  }

  const rpmCount = toCount(rpm);
  const tpmCount = toCount(tpm);
  const concurrencyCount = toCount(concurrency);
  // How many dimensions are unlimited (= 0)? Drives the summary pill so the
  // operator sees at a glance whether they've effectively turned the limit off.
  const unlimitedDims = [rpmCount, tpmCount, concurrencyCount].filter((n) => n === 0).length;

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    try {
      await onSubmit({
        rpm_limit: rpmCount,
        tpm_limit: tpmCount,
        max_concurrency: concurrencyCount,
        enabled,
      });
      toast({ title: t("feedback.saved"), tone: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  async function clear() {
    if (!onClear) return;
    setError(null);
    try {
      await onClear();
      toast({ title: t("feedback.saved"), tone: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2.5">
              <IconBubble tone="accent" size="md">
                <Gauge />
              </IconBubble>
              <span>{title}</span>
            </DialogTitle>
            <DialogDescription>{t("adminRateLimit.subtitle")}</DialogDescription>
          </DialogHeader>

          {/* Live summary — three at-a-glance pills with DataTooltip context */}
          <div className="mt-4 flex flex-wrap items-center gap-2">
            <DataTooltip
              title={t("adminRateLimit.rpm")}
              primary={
                <span className="tabular">
                  {rpmCount > 0 ? rpmCount.toLocaleString() : t("adminRateLimit.none")}
                </span>
              }
              rows={[
                {
                  label: "per second",
                  value: rpmCount > 0 ? (rpmCount / 60).toFixed(2) : "—",
                  tone: "muted",
                },
              ]}
              footer={rpmCount === 0 ? t("adminRateLimit.zeroHint") : undefined}
            >
              <DataPill
                tone={rpmCount > 0 ? "accent" : "neutral"}
                size="sm"
                className="cursor-help"
              >
                <Activity className="size-3" /> RPM {rpmCount > 0 ? rpmCount : "∞"}
              </DataPill>
            </DataTooltip>
            <DataTooltip
              title={t("adminRateLimit.tpm")}
              primary={
                <span className="tabular">
                  {tpmCount > 0 ? tpmCount.toLocaleString() : t("adminRateLimit.none")}
                </span>
              }
              rows={[
                {
                  label: "tokens / hour",
                  value: tpmCount > 0 ? (tpmCount * 60).toLocaleString() : "—",
                  tone: "muted",
                },
              ]}
              footer={tpmCount === 0 ? t("adminRateLimit.zeroHint") : undefined}
            >
              <DataPill
                tone={tpmCount > 0 ? "accent" : "neutral"}
                size="sm"
                className="cursor-help"
              >
                <Zap className="size-3" /> TPM {tpmCount > 0 ? tpmCount : "∞"}
              </DataPill>
            </DataTooltip>
            <DataTooltip
              title={t("adminRateLimit.concurrency")}
              primary={
                <span className="tabular">
                  {concurrencyCount > 0
                    ? concurrencyCount.toLocaleString()
                    : t("adminRateLimit.none")}
                </span>
              }
              footer={concurrencyCount === 0 ? t("adminRateLimit.zeroHint") : undefined}
            >
              <DataPill
                tone={concurrencyCount > 0 ? "accent" : "neutral"}
                size="sm"
                className="cursor-help"
              >
                <Layers className="size-3" /> ‖ {concurrencyCount > 0 ? concurrencyCount : "∞"}
              </DataPill>
            </DataTooltip>
            {!enabled ? (
              <DataPill tone="warning" size="sm">
                {t("adminRateLimit.off")}
              </DataPill>
            ) : null}
          </div>

          <div className="mt-5 grid gap-4 sm:grid-cols-3">
            <FloatingInput
              label={t("adminRateLimit.rpm")}
              value={rpm}
              onChange={setRpm}
              type="number"
              disabled={isPending}
              placeholder="0"
            />
            <FloatingInput
              label={t("adminRateLimit.tpm")}
              value={tpm}
              onChange={setTpm}
              type="number"
              disabled={isPending}
              placeholder="0"
            />
            <FloatingInput
              label={t("adminRateLimit.concurrency")}
              value={concurrency}
              onChange={setConcurrency}
              type="number"
              disabled={isPending}
              placeholder="0"
            />
          </div>

          <div className="mt-4 space-y-4">
            <p className="flex items-center gap-1.5 text-xs text-srapi-text-tertiary">
              <Info className="size-3.5" aria-hidden />
              {t("adminRateLimit.zeroHint")}
              {unlimitedDims === 3 ? (
                <span className="ml-1 text-srapi-warning">
                  ({t("adminRateLimit.none")})
                </span>
              ) : null}
            </p>
            <div className="flex items-center justify-between rounded-xl border border-srapi-border bg-srapi-card-muted/40 px-4 py-3">
              <Label htmlFor="rl-enabled" className="mb-0 flex flex-col gap-0.5">
                <span className="text-sm font-medium text-srapi-text-primary">
                  {t("adminRateLimit.enabled")}
                </span>
                <span className="text-[11px] text-srapi-text-tertiary">
                  {enabled
                    ? t("adminRateLimit.subtitle")
                    : t("adminRateLimit.off")}
                </span>
              </Label>
              <Switch
                id="rl-enabled"
                checked={enabled}
                disabled={isPending}
                onCheckedChange={setEnabled}
              />
            </div>
            {error ? (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-6">
            {onClear && current ? (
              <Button
                type="button"
                variant="ghost"
                disabled={isPending}
                onClick={clear}
                className="mr-auto text-srapi-error hover:text-srapi-error"
              >
                {t("adminRateLimit.clear")}
              </Button>
            ) : null}
            <Button type="button" variant="ghost" disabled={isPending} onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={isPending}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
