"use client";

import { AdminShell } from "@/components/layout/admin-shell";
import { AffiliateLedgerView } from "@/components/admin/affiliate-ledger-view";
import { useAffiliateRebates } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";

export default function AffiliateRebatesPage() {
  return (
    <AdminShell>
      <Content />
    </AdminShell>
  );
}

function Content() {
  const { t } = useLanguage();
  const query = useAffiliateRebates();
  return (
    <AffiliateLedgerView
      query={query}
      title={t("adminAffiliates.rebatesTitle")}
      subtitle={t("adminAffiliates.rebatesSubtitle")}
    />
  );
}
