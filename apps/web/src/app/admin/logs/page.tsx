"use client";

import { Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AdminShell } from "@/components/layout/admin-shell";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useLanguage } from "@/context/LanguageContext";
import { AuditLogsPanel } from "./_panels/audit-logs-panel";
import { BillingLedgerPanel } from "./_panels/billing-ledger-panel";
import { ErrorLogsPanel } from "./_panels/error-logs-panel";

// Aggregated logs view — replaces three standalone pages (/admin/audit-logs,
// /admin/billing-ledger, /admin/error-logs) with one tabbed surface. They
// share the same operator question — "what happened, when, to whom?" — just
// at three different layers: admin actions (audit), money movement (ledger),
// and request failures (error). Grouping reduces sidebar clutter and gives
// one place for postmortems.
//
// Standalone routes remain as 308 redirects so deeplinks + bookmarks keep
// working. Tab is URL-synced via ?tab= for share + back-button.
const TABS = ["audit", "billing-ledger", "error"] as const;

type Tab = (typeof TABS)[number];

const DEFAULT_TAB: Tab = "audit";

function isTab(value: string | null): value is Tab {
  return value !== null && (TABS as readonly string[]).includes(value);
}

export default function LogsAdminPage() {
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
    router.replace(`/admin/logs${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  return (
    <>
      <Tabs value={active} onValueChange={setTab}>
        <TabsList className="flex flex-wrap">
          <TabsTrigger value="audit">{t("nav.adminAuditLogs")}</TabsTrigger>
          <TabsTrigger value="billing-ledger">{t("nav.adminBillingLedger")}</TabsTrigger>
          <TabsTrigger value="error">{t("nav.adminErrorLogs")}</TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="mt-4">
        {active === "audit" ? <AuditLogsPanel /> : null}
        {active === "billing-ledger" ? <BillingLedgerPanel /> : null}
        {active === "error" ? <ErrorLogsPanel /> : null}
      </div>
    </>
  );
}
