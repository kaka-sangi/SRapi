"use client";

import { useAuthIdentities, useUnbindAuthIdentity } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { meErrorMessage } from "@/lib/me-api";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { PageQueryState } from "@/components/layout/page-query-state";
import type { CurrentUserAuthIdentity } from "@/lib/sdk-types";

const PROVIDER_LABEL: Record<string, string> = {
  email: "Email",
  oidc: "SSO",
  github: "GitHub",
  google: "Google",
  linuxdo: "LINUX DO",
  wechat: "WeChat",
  dingtalk: "DingTalk",
};

export function LinkedSignInsCard() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const identities = useAuthIdentities();
  const unbindMut = useUnbindAuthIdentity();

  async function unbind(identity: CurrentUserAuthIdentity) {
    if (!identity.id) return;
    if (!window.confirm(t("account.linkedUnbindConfirm"))) return;
    try {
      await unbindMut.mutateAsync(identity.id);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  return (
    <Card>
      <CardContent className="space-y-4">
        <div>
          <h3 className="font-serif text-lg text-srapi-text-primary">{t("account.linkedTitle")}</h3>
          <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("account.linkedHint")}</p>
        </div>
        <PageQueryState
          query={identities}
          skeleton={<DialogListSkeleton rows={2} />}
          isEmpty={(d) => d.data.length === 0}
          emptyTitle={t("account.linkedEmpty")}
        >
          {(list) => (
            <ul className="divide-y divide-srapi-border">
              {list.data.map((identity) => {
                const hint = identity.email || identity.display_name || identity.subject_hint;
                const lastUsed = identity.last_used_at
                  ? `${t("account.linkedLastUsed")} ${identity.last_used_at.slice(0, 10)}`
                  : t("account.linkedNeverUsed");
                return (
                  <li
                    key={identity.id ?? `${identity.provider}:${identity.provider_key}`}
                    className="flex items-center justify-between gap-3 py-3"
                  >
                    <div className="min-w-0">
                      <p className="text-sm text-srapi-text-primary">
                        {PROVIDER_LABEL[identity.provider] ?? identity.provider}
                      </p>
                      <p className="truncate text-2xs text-srapi-text-tertiary">
                        {hint ? `${hint} · ${lastUsed}` : lastUsed}
                      </p>
                    </div>
                    {identity.can_unbind ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={unbindMut.isPending}
                        onClick={() => void unbind(identity)}
                      >
                        {t("account.linkedUnbind")}
                      </Button>
                    ) : (
                      <span className="text-2xs text-srapi-text-tertiary">
                        {t("account.linkedUnbindBlocked")}
                      </span>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </PageQueryState>
      </CardContent>
    </Card>
  );
}
