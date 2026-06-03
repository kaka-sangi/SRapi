"use client";

import { useState } from "react";
import { Server } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { useProviderAccounts, useTestProviderAccount } from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { QuotaNotchRail } from "@/components/ui/quota-notch-rail";
import { EmptyState } from "@/components/ui/empty-state";
import { Button } from "@/components/ui/button";
import { CopyableValue } from "@/components/ui/copy-button";
import { Skeleton } from "@/components/ui/skeleton";
import type { ProviderAccountSummary } from "@/lib/srapi-types";

export default function ProviderAccountsPage() {
  return (
    <AppShell allowedRole="admin">
      <ProviderAccountsContent />
    </AppShell>
  );
}

function ProviderAccountsContent() {
  const { t } = useLanguage();
  const accounts = useProviderAccounts();

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionGateway")}
        title={t("providers.title")}
        description={t("providers.subtitle")}
      />
      <PageQueryState
        query={accounts}
        isEmpty={(d) => d.length === 0}
        skeleton={
          <div className="grid gap-4 md:grid-cols-2">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-44 rounded-xl" />
            ))}
          </div>
        }
      >
        {(data) =>
          data.length === 0 ? (
            <EmptyState
              icon={Server}
              title={t("providers.emptyTitle")}
              description={t("providers.emptyBody")}
            />
          ) : (
            <div className="grid gap-4 md:grid-cols-2">
              {data.map((account) => (
                <ProviderAccountCard key={account.id} account={account} />
              ))}
            </div>
          )
        }
      </PageQueryState>
    </>
  );
}

const STATUS_MAP: Record<
  ProviderAccountSummary["status"],
  { status: "active" | "limited" | "disabled"; key: string }
> = {
  active: { status: "active", key: "common.active" },
  limited: { status: "limited", key: "common.limited" },
  disabled: { status: "disabled", key: "common.disabled" },
};

function ProviderAccountCard({ account }: { account: ProviderAccountSummary }) {
  const { t } = useLanguage();
  const test = useTestProviderAccount();
  const [result, setResult] = useState<"ok" | "fail" | null>(null);
  const badge = STATUS_MAP[account.status];
  // The SDK maps an unset endpoint to a sentinel string; only offer copy when a
  // real URL is present, otherwise show a muted, localized placeholder.
  const hasBaseUrl = account.base_url.includes("://");

  async function runTest() {
    setResult(null);
    try {
      const res = await test.mutateAsync(account.id);
      setResult(res?.ok ? "ok" : "fail");
    } catch {
      setResult("fail");
    }
  }

  return (
    <Card className={account.status === "disabled" ? "opacity-60" : ""}>
      <CardHeader>
        <div className="min-w-0">
          <CardTitle className="truncate not-italic font-sans text-base text-srapi-text-primary">
            {account.name}
          </CardTitle>
          <div className="mt-0.5 font-mono text-2xs text-srapi-text-tertiary">
            {account.provider_name}
          </div>
        </div>
        <QuietBadge status={badge.status} label={t(badge.key)} />
      </CardHeader>
      <div className="space-y-4 p-5">
        <div className="flex items-center justify-between gap-2 text-xs">
          <span className="shrink-0 text-srapi-text-secondary">{t("providers.baseUrl")}</span>
          {hasBaseUrl ? (
            <CopyableValue
              value={account.base_url}
              label={t("providers.baseUrl")}
              className="max-w-[60%] text-srapi-text-primary"
            />
          ) : (
            <span className="max-w-[60%] truncate text-srapi-text-tertiary">
              {t("providers.baseUrlUnset")}
            </span>
          )}
        </div>
        <div className="flex items-center justify-between text-xs">
          <span className="text-srapi-text-secondary">{t("providers.avgLatency")}</span>
          <span className="font-mono text-srapi-text-primary tabular">{account.latency} ms</span>
        </div>
        <div>
          <div className="mb-1.5 flex items-center justify-between text-xs">
            <span className="text-srapi-text-secondary">{t("providers.quotaLeft")}</span>
            <span className="font-mono text-srapi-text-primary tabular">
              {Math.round(account.quota_percentage)}%
            </span>
          </div>
          <QuotaNotchRail value={account.quota_percentage} />
        </div>
        <div className="flex items-center gap-2 pt-1">
          <Button variant="outline" size="sm" onClick={runTest} loading={test.isPending}>
            {test.isPending ? t("providers.testing") : t("providers.test")}
          </Button>
          {result === "ok" && (
            <QuietBadge status="active" label={t("providers.testOk")} />
          )}
          {result === "fail" && <QuietBadge status="error" label={t("common.error")} />}
        </div>
      </div>
    </Card>
  );
}
