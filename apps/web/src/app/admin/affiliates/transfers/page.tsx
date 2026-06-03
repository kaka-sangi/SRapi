"use client";

import { AdminShell } from "@/components/layout/admin-shell";
import { AffiliateLedgerView } from "@/components/admin/affiliate-ledger-view";
import { useAffiliateTransfers } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";

export default function AffiliateTransfersPage() {
  return (
    <AdminShell>
      <Content />
    </AdminShell>
  );
}

function Content() {
  const { t } = useLanguage();
  const query = useAffiliateTransfers();
  return (
    <AffiliateLedgerView
      query={query}
      title={t("adminAffiliates.transfersTitle")}
      subtitle={t("adminAffiliates.transfersSubtitle")}
    />
  );
}
