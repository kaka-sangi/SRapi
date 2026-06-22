"use client";

import { useLanguage } from "@/context/LanguageContext";
import { useBindAccountProxy } from "@/hooks/admin-queries";
import {
  ResourceFormDialog,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import type { ProviderAccount } from "@/lib/sdk-types";

const NO_PROXY = "__none__";

interface BindProxyFormState {
  proxy: string;
}

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
  const bind = useBindAccountProxy();

  const currentProxy = account.proxy_id ? String(account.proxy_id) : NO_PROXY;

  const fields: FieldConfig<BindProxyFormState>[] = [
    {
      name: "proxy",
      label: t("adminAccounts.proxy"),
      type: "select",
      options: [{ value: NO_PROXY, label: t("adminAccounts.noProxy") }, ...proxyOptions],
      // Inline contextual help — the proxy column is one click deep so the
      // operator needs a refresher on the consequence here.
      help: t("adminAccounts.bindProxyHelp") ?? t("adminAccounts.bindProxy"),
      hint:
        currentProxy === NO_PROXY
          ? t("adminAccounts.noProxy")
          : (proxyOptions.find((opt) => opt.value === currentProxy)?.label ?? undefined),
    },
  ];

  return (
    <ResourceFormDialog<BindProxyFormState, { proxyId: string | null }>
      open={open}
      onOpenChange={onOpenChange}
      title={t("adminAccounts.bindProxy")}
      description={account.name}
      fields={fields}
      // Pre-select the account's current proxy so the dialog reflects state on
      // open — previously it always reset to "no proxy", which made a no-op
      // submit silently clear the binding.
      initial={{ proxy: currentProxy }}
      buildBody={(form) => ({ proxyId: form.proxy === NO_PROXY ? null : form.proxy })}
      submit={(body) => bind.mutateAsync({ id: account.id, proxyId: body.proxyId })}
      successMessage={t("adminAccounts.bindProxyDone")}
      isPending={bind.isPending}
    />
  );
}
