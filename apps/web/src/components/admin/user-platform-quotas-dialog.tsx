"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { PageQueryState } from "@/components/layout/page-query-state";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import {
  useUserPlatformQuotas,
  useUpsertUserPlatformQuota,
  useDeleteUserPlatformQuota,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { formatMoney } from "@/lib/admin-format";
import type { UserPlatformQuota } from "@/lib/sdk-types";

interface FormState {
  platform: string;
  daily: string;
  weekly: string;
  monthly: string;
  enabled: boolean;
}

const EMPTY_FORM: FormState = { platform: "", daily: "", weekly: "", monthly: "", enabled: true };

export function UserPlatformQuotasDialog({
  userId,
  userLabel,
  onClose,
}: {
  userId: string;
  userLabel: string;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const query = useUserPlatformQuotas(userId);
  const upsertMut = useUpsertUserPlatformQuota();
  const deleteMut = useDeleteUserPlatformQuota();
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [error, setError] = useState<string | null>(null);
  const busy = upsertMut.isPending || deleteMut.isPending;

  function editQuota(quota: UserPlatformQuota) {
    setForm({
      platform: quota.platform,
      daily: quota.daily_limit ?? "",
      weekly: quota.weekly_limit ?? "",
      monthly: quota.monthly_limit ?? "",
      enabled: quota.enabled,
    });
  }

  async function save() {
    setError(null);
    const platform = form.platform.trim();
    if (!platform) {
      setError(t("adminUserQuota.platformRequired"));
      return;
    }
    try {
      await upsertMut.mutateAsync({
        userId,
        body: {
          platform,
          daily_limit: form.daily.trim() || null,
          weekly_limit: form.weekly.trim() || null,
          monthly_limit: form.monthly.trim() || null,
          enabled: form.enabled,
        },
      });
      toast({ title: t("feedback.updated"), tone: "success" });
      setForm(EMPTY_FORM);
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  async function remove(platform: string) {
    setError(null);
    try {
      await deleteMut.mutateAsync({ userId, platform });
      toast({ title: t("feedback.deleted"), tone: "success" });
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("adminUserQuota.title")}</DialogTitle>
          <DialogDescription>{t("adminUserQuota.subtitle", { user: userLabel })}</DialogDescription>
        </DialogHeader>
        <div className="mt-2 min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain pr-1">
          <PageQueryState query={query} skeleton={<DialogListSkeleton rows={2} />}>
            {(list) =>
              list.data.length === 0 ? (
                <p className="text-sm text-srapi-text-tertiary">{t("adminUserQuota.empty")}</p>
              ) : (
                <div className="space-y-1.5">
                  {list.data.map((quota) => (
                    <div
                      key={quota.platform}
                      className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-srapi-border px-3 py-2"
                    >
                      <span className="font-mono text-xs text-srapi-text-primary">{quota.platform}</span>
                      <QuietBadge
                        status={quota.enabled ? "active" : "disabled"}
                        label={quota.enabled ? t("common.active") : t("common.disabled")}
                      />
                      <span className="ml-auto font-mono text-2xs text-srapi-text-tertiary tabular">
                        {limitLabel(t("adminUserQuota.dailyShort"), quota.daily_limit, quota.currency)}
                        {" · "}
                        {limitLabel(t("adminUserQuota.weeklyShort"), quota.weekly_limit, quota.currency)}
                        {" · "}
                        {limitLabel(t("adminUserQuota.monthlyShort"), quota.monthly_limit, quota.currency)}
                      </span>
                      <button
                        type="button"
                        onClick={() => editQuota(quota)}
                        className="text-2xs text-srapi-text-secondary underline-offset-2 hover:text-srapi-text-primary hover:underline"
                      >
                        {t("common.edit")}
                      </button>
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => void remove(quota.platform)}
                        className="text-2xs text-srapi-error underline-offset-2 hover:underline disabled:opacity-50"
                      >
                        {t("common.delete")}
                      </button>
                    </div>
                  ))}
                </div>
              )
            }
          </PageQueryState>

          <div className="space-y-3 rounded-lg border border-srapi-border p-3.5">
            <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
              {t("adminUserQuota.addEdit")}
            </span>
            <div>
              <Label htmlFor="upq-platform">{t("adminUserQuota.platform")}</Label>
              <Input
                id="upq-platform"
                value={form.platform}
                placeholder="anthropic"
                disabled={busy}
                onChange={(e) => setForm({ ...form, platform: e.target.value })}
              />
            </div>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
              <div>
                <Label htmlFor="upq-daily">{t("adminUserQuota.daily")}</Label>
                <Input
                  id="upq-daily"
                  value={form.daily}
                  placeholder="—"
                  disabled={busy}
                  onChange={(e) => setForm({ ...form, daily: e.target.value })}
                />
              </div>
              <div>
                <Label htmlFor="upq-weekly">{t("adminUserQuota.weekly")}</Label>
                <Input
                  id="upq-weekly"
                  value={form.weekly}
                  placeholder="—"
                  disabled={busy}
                  onChange={(e) => setForm({ ...form, weekly: e.target.value })}
                />
              </div>
              <div>
                <Label htmlFor="upq-monthly">{t("adminUserQuota.monthly")}</Label>
                <Input
                  id="upq-monthly"
                  value={form.monthly}
                  placeholder="—"
                  disabled={busy}
                  onChange={(e) => setForm({ ...form, monthly: e.target.value })}
                />
              </div>
            </div>
            <div className="flex items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <Switch
                  id="upq-enabled"
                  checked={form.enabled}
                  disabled={busy}
                  onCheckedChange={(checked) => setForm({ ...form, enabled: checked })}
                />
                <Label htmlFor="upq-enabled" className="mb-0">
                  {t("adminUserQuota.enabled")}
                </Label>
              </div>
              <Button variant="primary" size="sm" loading={upsertMut.isPending} disabled={busy} onClick={() => void save()}>
                {t("common.save")}
              </Button>
            </div>
            <p className="text-2xs text-srapi-text-tertiary">{t("adminUserQuota.hint")}</p>
          </div>
          {error ? (
            <p role="alert" className="text-sm text-srapi-error">
              {error}
            </p>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function limitLabel(prefix: string, value: string | null | undefined, currency: string): string {
  return `${prefix} ${value ? formatMoney(value, currency) : "∞"}`;
}
