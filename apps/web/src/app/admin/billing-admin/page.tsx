"use client";

import { Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AdminShell } from "@/components/layout/admin-shell";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useLanguage } from "@/context/LanguageContext";
import { PlansPanel } from "./_panels/plans-panel";
import { SubscriptionsPanel } from "./_panels/subscriptions-panel";
import { PaymentProvidersPanel } from "./_panels/payment-providers-panel";

// Aggregated billing-admin view — replaces three standalone pages
// (/admin/orders/plans, /admin/subscriptions, /admin/payment-providers)
// with a single tabbed surface. All three configure the "money-in"
// machinery: catalog (plans), active sales (subscriptions), and the
// underlying provider connections (Stripe/WeChat/Alipay/EasyPay).
// /admin/orders (the transactional payment-order list) stays standalone
// — it's an event log, not config.
//
// Standalone routes remain as 308 redirects so deeplinks + bookmarks
// keep working. Tab is URL-synced via ?tab= for share + back-button.
const TABS = ["plans", "subscriptions", "payment-providers"] as const;

type Tab = (typeof TABS)[number];

const DEFAULT_TAB: Tab = "plans";

function isTab(value: string | null): value is Tab {
  return value !== null && (TABS as readonly string[]).includes(value);
}

export default function BillingAdminPage() {
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
    router.replace(`/admin/billing-admin${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  return (
    <>
      <Tabs value={active} onValueChange={setTab}>
        <TabsList className="flex flex-wrap">
          <TabsTrigger value="plans">{t("nav.adminOrdersPlans")}</TabsTrigger>
          <TabsTrigger value="subscriptions">{t("nav.adminSubscriptions")}</TabsTrigger>
          <TabsTrigger value="payment-providers">{t("nav.adminPaymentProviders")}</TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="mt-4">
        {active === "plans" ? <PlansPanel /> : null}
        {active === "subscriptions" ? <SubscriptionsPanel /> : null}
        {active === "payment-providers" ? <PaymentProvidersPanel /> : null}
      </div>
    </>
  );
}
