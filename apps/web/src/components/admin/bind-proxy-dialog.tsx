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
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { useBindAccountProxy } from "@/hooks/admin-queries";
import { adminErrorMessage } from "@/lib/admin-api";
import type { ProviderAccount } from "@/lib/sdk-types";

const NO_PROXY = "__none__";

/**
 * Bind (or clear) an egress proxy on a provider account. Surfaces the previously
 * dead `bindAccountProxy` capability — the account form never collected a proxy.
 */
export function BindProxyDialog({
  open,
  onOpenChange,
  account,
  proxyOptions,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  account: ProviderAccount;
  proxyOptions: { value: string; label: string }[];
}) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const bind = useBindAccountProxy();
  const [value, setValue] = useState<string>(NO_PROXY);
  const [error, setError] = useState<string | null>(null);

  async function confirm() {
    setError(null);
    try {
      await bind.mutateAsync({ id: account.id, proxyId: value === NO_PROXY ? null : value });
      toast({ title: t("adminAccounts.bindProxyDone"), tone: "success" });
      onOpenChange(false);
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t("adminAccounts.bindProxy")}</DialogTitle>
          <DialogDescription>{account.name}</DialogDescription>
        </DialogHeader>
        <div className="mt-2">
          <Label htmlFor="bind-proxy">{t("adminAccounts.proxy")}</Label>
          <Select value={value} onValueChange={setValue} disabled={bind.isPending}>
            <SelectTrigger id="bind-proxy">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_PROXY}>{t("adminAccounts.noProxy")}</SelectItem>
              {proxyOptions.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {error ? (
          <p role="alert" className="mt-3 text-sm text-srapi-error">
            {error}
          </p>
        ) : null}
        <DialogFooter className="mt-6">
          <Button type="button" variant="ghost" disabled={bind.isPending} onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button type="button" variant="primary" loading={bind.isPending} onClick={() => void confirm()}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
