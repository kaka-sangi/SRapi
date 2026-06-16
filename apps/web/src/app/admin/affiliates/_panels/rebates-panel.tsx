"use client";

import { AffiliateLedgerView } from "@/components/admin/affiliate-ledger-view";
import { useAffiliateRebates } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";

export function RebatesPanel() {
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
