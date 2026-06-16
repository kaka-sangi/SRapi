"use client";

import { UserPlus } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useAffiliateInvites } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime } from "@/lib/admin-format";
import type { AffiliateInviteRecord } from "@/lib/sdk-types";

// Panel body for /admin/affiliates?tab=invites. Identical UI to the legacy
// /admin/affiliates/invites page; the standalone page is now a redirect.
export function InvitesPanel() {
  const { t } = useLanguage();
  const invites = useAffiliateInvites();
  const colVis = useColumnVisibility("admin-affiliate-invites", []);

  const columns: Column<AffiliateInviteRecord>[] = [
    {
      key: "inviter",
      header: t("adminAffiliates.inviter"),
      pinned: true,
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-secondary">{r.inviter_user_id}</span>
      ),
    },
    {
      key: "invitee",
      header: t("adminAffiliates.invitee"),
      render: (r) => (
        <span className="font-mono text-2xs text-srapi-text-secondary">
          {r.invitee_user_id || "—"}
        </span>
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
