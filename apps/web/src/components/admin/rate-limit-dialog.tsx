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
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
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

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    try {
      await onSubmit({
        rpm_limit: toCount(rpm),
        tpm_limit: toCount(tpm),
        max_concurrency: toCount(concurrency),
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
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{t("adminRateLimit.subtitle")}</DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-4">
            <div>
              <Label htmlFor="rl-rpm">{t("adminRateLimit.rpm")}</Label>
              <Input
                id="rl-rpm"
                type="number"
                min={0}
                inputMode="numeric"
                placeholder="0"
                value={rpm}
                disabled={isPending}
                onChange={(e) => setRpm(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="rl-tpm">{t("adminRateLimit.tpm")}</Label>
              <Input
                id="rl-tpm"
                type="number"
                min={0}
                inputMode="numeric"
                placeholder="0"
                value={tpm}
                disabled={isPending}
                onChange={(e) => setTpm(e.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="rl-conc">{t("adminRateLimit.concurrency")}</Label>
              <Input
                id="rl-conc"
                type="number"
                min={0}
                inputMode="numeric"
                placeholder="0"
                value={concurrency}
                disabled={isPending}
                onChange={(e) => setConcurrency(e.target.value)}
              />
            </div>
            <p className="text-2xs text-srapi-text-tertiary">{t("adminRateLimit.zeroHint")}</p>
            <div className="flex items-center justify-between">
              <Label htmlFor="rl-enabled" className="mb-0">
                {t("adminRateLimit.enabled")}
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
