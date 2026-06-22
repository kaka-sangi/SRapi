"use client";

import { Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { AdminShell } from "@/components/layout/admin-shell";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { useLanguage } from "@/context/LanguageContext";
import { ErrorPassthroughPanel } from "./_panels/error-passthrough-panel";
import { PayloadRulesPanel } from "./_panels/payload-rules-panel";
import { TlsProfilesPanel } from "./_panels/tls-profiles-panel";

// Aggregated gateway-edge policy admin — replaces three previously
// standalone pages (/admin/error-passthrough, /admin/payload-rules,
// /admin/tls-profiles) with a single tabbed surface. All three share the
// "ordered list of rules that change how the gateway talks to a provider"
// shape, so they belong in one place; an operator tuning egress headers,
// payload transforms, and error visibility for the same provider no longer
// needs to context-switch across three nav items.
//
// Standalone routes remain as 308 redirects so deeplinks + bookmarks keep
// working. Tab is URL-synced via ?tab= for share + back-button.
const TABS = ["error-passthrough", "payload-rules", "tls-profiles"] as const;

type Tab = (typeof TABS)[number];

const DEFAULT_TAB: Tab = "error-passthrough";

function isTab(value: string | null): value is Tab {
  return value !== null && (TABS as readonly string[]).includes(value);
}

export default function GatewayPoliciesAdminPage() {
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

  function setTab(next: Tab) {
    const q = new URLSearchParams();
    if (next !== DEFAULT_TAB) q.set("tab", next);
    const qs = q.toString();
    router.replace(`/admin/gateway-policies${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  return (
    <>
      <SegmentedControl<Tab>
        value={active}
        onChange={setTab}
        ariaLabel={t("nav.adminGatewayPolicies")}
        options={[
          { value: "error-passthrough", label: t("nav.adminErrorPassthrough") },
          { value: "payload-rules", label: t("nav.adminPayloadRules") },
          { value: "tls-profiles", label: t("nav.adminTlsProfiles") },
        ]}
      />

      <div className="mt-4">
        {active === "error-passthrough" ? <ErrorPassthroughPanel /> : null}
        {active === "payload-rules" ? <PayloadRulesPanel /> : null}
        {active === "tls-profiles" ? <TlsProfilesPanel /> : null}
      </div>
    </>
  );
}
