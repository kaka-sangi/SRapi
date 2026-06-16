"use client";

import { AffiliateLedgerView } from "@/components/admin/affiliate-ledger-view";
import { useAffiliateTransfers } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";

export function TransfersPanel() {
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
