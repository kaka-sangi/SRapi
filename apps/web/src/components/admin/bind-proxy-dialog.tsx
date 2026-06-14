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

  const fields: FieldConfig<BindProxyFormState>[] = [
    {
      name: "proxy",
      label: t("adminAccounts.proxy"),
      type: "select",
      options: [{ value: NO_PROXY, label: t("adminAccounts.noProxy") }, ...proxyOptions],
    },
  ];

  return (
    <ResourceFormDialog<BindProxyFormState, { proxyId: string | null }>
      open={open}
      onOpenChange={onOpenChange}
      title={t("adminAccounts.bindProxy")}
      description={account.name}
      fields={fields}
      initial={{ proxy: NO_PROXY }}
      buildBody={(form) => ({ proxyId: form.proxy === NO_PROXY ? null : form.proxy })}
      submit={(body) => bind.mutateAsync({ id: account.id, proxyId: body.proxyId })}
      successMessage={t("adminAccounts.bindProxyDone")}
      isPending={bind.isPending}
    />
  );
}
