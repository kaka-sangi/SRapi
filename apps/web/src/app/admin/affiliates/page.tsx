"use client";

import { Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AdminShell } from "@/components/layout/admin-shell";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useLanguage } from "@/context/LanguageContext";
import { InvitesPanel } from "./_panels/invites-panel";
import { RebatesPanel } from "./_panels/rebates-panel";
import { TransfersPanel } from "./_panels/transfers-panel";
import { WithdrawalsPanel } from "./_panels/withdrawals-panel";
import { RulesPanel } from "./_panels/rules-panel";
import { ManualAdjustmentsPanel } from "./_panels/manual-adjustments-panel";

// Aggregated affiliate admin view — replaces six previously standalone pages
// (/admin/affiliates/{invites,rebates,transfers,withdrawals,rules,
// manual-adjustments}) with a single tabbed page. The standalone routes
// remain as 308 redirects so deeplinks + bookmarks keep working.
//
// Tab is URL-synced via ?tab= so the page is shareable and surfaces in the
// browser back-button. Default tab is invites (matches the legacy nav
// order). Wrapped in Suspense because useSearchParams suspends during SSR.
const TABS = [
  "invites",
  "rebates",
  "transfers",
  "withdrawals",
  "rules",
  "manual-adjustments",
] as const;

type Tab = (typeof TABS)[number];

const DEFAULT_TAB: Tab = "invites";

function isTab(value: string | null): value is Tab {
  return value !== null && (TABS as readonly string[]).includes(value);
}

export default function AffiliatesAdminPage() {
  return (
    <AdminShell>
      <Suspense>
        <Content />
      </Suspense>
    </AdminShell>
  );
}

function Content() {
  const { t } = useLanguage();
  const router = useRouter();
  const params = useSearchParams();
  const raw = params.get("tab");
  const active: Tab = isTab(raw) ? raw : DEFAULT_TAB;

  function setTab(next: string) {
    const q = new URLSearchParams();
    if (next !== DEFAULT_TAB) q.set("tab", next);
    const qs = q.toString();
    router.replace(`/admin/affiliates${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  return (
    <>
      <Tabs value={active} onValueChange={setTab}>
        <TabsList className="flex flex-wrap">
          <TabsTrigger value="invites">{t("nav.adminAffiliatesInvites")}</TabsTrigger>
          <TabsTrigger value="rebates">{t("nav.adminAffiliatesRebates")}</TabsTrigger>
          <TabsTrigger value="transfers">{t("nav.adminAffiliatesTransfers")}</TabsTrigger>
          <TabsTrigger value="withdrawals">{t("nav.adminAffiliatesWithdrawals")}</TabsTrigger>
          <TabsTrigger value="rules">{t("nav.adminAffiliatesRules")}</TabsTrigger>
          <TabsTrigger value="manual-adjustments">
            {t("nav.adminAffiliatesManualAdjustments")}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="mt-4">
        {active === "invites" ? <InvitesPanel /> : null}
        {active === "rebates" ? <RebatesPanel /> : null}
        {active === "transfers" ? <TransfersPanel /> : null}
        {active === "withdrawals" ? <WithdrawalsPanel /> : null}
        {active === "rules" ? <RulesPanel /> : null}
        {active === "manual-adjustments" ? <ManualAdjustmentsPanel /> : null}
      </div>
    </>
  );
}
