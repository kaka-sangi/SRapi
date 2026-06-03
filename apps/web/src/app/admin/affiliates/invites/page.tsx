"use client";

import { UserPlus } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { useAffiliateInvites } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { formatDateTime } from "@/lib/admin-format";
import type { AffiliateInviteRecord } from "@/lib/sdk-types";

export default function AffiliateInvitesPage() {
  return (
    <AdminShell>
      <InvitesContent />
    </AdminShell>
  );
}

function InvitesContent() {
  const { t } = useLanguage();
  const invites = useAffiliateInvites();

  const columns: Column<AffiliateInviteRecord>[] = [
    {
      key: "inviter",
      header: t("adminAffiliates.inviter"),
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
          invites.data ? (
            <ListCount total={invites.data.pagination?.total ?? invites.data.data.length} />
          ) : undefined
        }
      />
      <AdminListView
        query={invites}
        columns={columns}
        getRowId={(r) => r.id}
        emptyIcon={UserPlus}
        emptyTitle={t("adminAffiliates.emptyTitle")}
        emptyBody={t("adminAffiliates.emptyBody")}
        minWidth={520}
      />
    </>
  );
}
