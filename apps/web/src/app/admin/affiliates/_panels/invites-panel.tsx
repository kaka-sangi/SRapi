"use client";

import { useMemo } from "react";
import { UserPlus } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useAdminUsers, useAffiliateInvites } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime } from "@/lib/admin-format";
import type { AffiliateInviteRecord } from "@/lib/sdk-types";

// Best-effort email lookup against the first page of users — same pattern
// the payment-dashboard panel uses to avoid an N+1 server-side join.
// Operators with users past row 200 will still see the raw user_id.
const USER_LOOKUP_PAGE_SIZE = 200;

// Panel body for /admin/affiliates?tab=invites. Identical UI to the legacy
// /admin/affiliates/invites page; the standalone page is now a redirect.
export function InvitesPanel() {
  const { t } = useLanguage();
  const invites = useAffiliateInvites();
  const users = useAdminUsers({ page: 1, page_size: USER_LOOKUP_PAGE_SIZE });
  const colVis = useColumnVisibility("admin-affiliate-invites", []);

  const emailByUserId = useMemo(() => {
    const map = new Map<string, string>();
    for (const u of users.data?.data ?? []) {
      map.set(String(u.id), u.email);
    }
    return map;
  }, [users.data]);

  const formatUser = (id: string | null | undefined) => {
    if (!id) return "—";
    return emailByUserId.get(String(id)) ?? String(id);
  };

  const columns: Column<AffiliateInviteRecord>[] = [
    {
      key: "inviter",
      header: t("adminAffiliates.inviter"),
      pinned: true,
      render: (r) => (
        <span className="text-srapi-text-secondary">{formatUser(r.inviter_user_id)}</span>
      ),
    },
    {
      key: "invitee",
      header: t("adminAffiliates.invitee"),
      render: (r) => (
        <span className="text-srapi-text-secondary">{formatUser(r.invitee_user_id)}</span>
      ),
    },
    {
      key: "date",
      header: t("adminAffiliates.date"),
      align: "right",
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
        title={t("adminAffiliates.invitesTitle")}
        description={t("adminAffiliates.invitesSubtitle")}
        actions={
          <div className="flex items-center gap-3">
            {invites.data ? (
              <ListCount total={invites.data.pagination?.total ?? 0} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
          </div>
        }
      />
      <AdminListView
        query={invites}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(r) => r.id}
        emptyIcon={UserPlus}
        emptyTitle={t("adminAffiliates.emptyTitle")}
        emptyBody={t("adminAffiliates.emptyBody")}
        minWidth={520}
      />
    </>
  );
}
