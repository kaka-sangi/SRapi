"use client";

import { Coins } from "lucide-react";
import type { UseQueryResult } from "@tanstack/react-query";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column, type AdminListResult } from "./admin-list-view";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import type { AffiliateLedgerEntry } from "@/lib/sdk-types";

/**
 * Shared view for the affiliate rebate / transfer ledgers — identical shape,
 * different title + dataset.
 */
export function AffiliateLedgerView({
  query,
  title,
  subtitle,
}: {
  query: UseQueryResult<AdminListResult<AffiliateLedgerEntry>>;
  title: string;
  subtitle: string;
}) {
  const { t } = useLanguage();
  const userLookup = useUserEmailLookup();

  const columns: Column<AffiliateLedgerEntry>[] = [
    {
      key: "user",
      header: t("adminAffiliates.inviter"),
      render: (r) => (
        <span className="text-srapi-text-secondary">{userLookup.get(r.user_id)}</span>
      ),
    },
    {
      key: "type",
      header: t("adminUsers.roles"),
      hideOnMobile: true,
      render: (r) => <span className="text-srapi-text-secondary">{r.type || "—"}</span>,
    },
    {
      key: "amount",
      header: t("adminAffiliates.amount"),
      align: "right",
      render: (r) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(r.amount, r.currency)}
        </span>
      ),
    },
    {
      key: "date",
      header: t("adminAffiliates.date"),
      align: "right",
      hideOnMobile: true,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDateTime(r.created_at)}
        </span>
      ),
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={title}
        description={subtitle}
        actions={
          query.data ? (
            <ListCount total={query.data.pagination?.total ?? query.data.data.length} />
          ) : undefined
        }
      />
      <AdminListView
        query={query}
        columns={columns}
        getRowId={(r) => r.id}
        emptyIcon={Coins}
        emptyTitle={t("adminAffiliates.emptyTitle")}
        emptyBody={t("adminAffiliates.emptyBody")}
        minWidth={560}
      />
    </>
  );
}
