"use client";

import { useState } from "react";
import { Wallet } from "lucide-react";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { PageHeader } from "@/components/layout/page-header";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { useAffiliateWithdrawals, useApproveWithdrawal, useCancelWithdrawal } from "@/hooks/admin-queries";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import type { AffiliateLedgerEntry } from "@/lib/sdk-types";

export function WithdrawalsPanel() {
  const { t } = useLanguage();
  const query = useAffiliateWithdrawals();
  const approveMut = useApproveWithdrawal();
  const cancelMut = useCancelWithdrawal();
  const userLookup = useUserEmailLookup();

  const [approveTarget, setApproveTarget] = useState<AffiliateLedgerEntry | null>(null);
  const [cancelTarget, setCancelTarget] = useState<AffiliateLedgerEntry | null>(null);

  // Client-side filter: only show withdraw-type ledger entries (the backend
  // endpoint returns the full transfers ledger including non-withdraw rows).
  const withdrawalsQuery = {
    ...query,
    data: query.data
      ? {
          ...query.data,
          data: query.data.data.filter((r) => r.type === "withdraw"),
        }
      : undefined,
  } as typeof query;

  const columns: Column<AffiliateLedgerEntry>[] = [
    {
      key: "user",
      header: t("adminAffiliates.inviter"),
      render: (r) => (
        <span className="text-srapi-text-secondary">{userLookup.get(r.user_id)}</span>
      ),
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
      key: "status",
      header: t("adminAffiliates.withdrawalStatus"),
      render: (r) => <span className="text-srapi-text-secondary">{r.status}</span>,
    },
    {
      key: "destination",
      header: t("adminAffiliates.withdrawalDestination"),
      hideOnMobile: true,
      render: (r) => {
        const dest =
          r.metadata && typeof r.metadata === "object" && "destination" in r.metadata
            ? String((r.metadata as Record<string, unknown>).destination ?? "")
            : "";
        return <span className="text-srapi-text-secondary">{dest || "—"}</span>;
      },
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
        title={t("adminAffiliates.withdrawalsTitle")}
        description={t("adminAffiliates.withdrawalsSubtitle")}
        actions={
          <div className="flex items-center gap-3">
            {withdrawalsQuery.data ? (
              <ListCount
                total={
                  withdrawalsQuery.data.pagination?.total ?? withdrawalsQuery.data.data.length
                }
              />
            ) : null}
            <AutoRefreshControl
              onRefresh={() => void query.refetch()}
              isRefreshing={query.isFetching}
              storageKey="srapi.autorefresh.admin-affiliate-withdrawals"
            />
          </div>
        }
      />
      <AdminListView
        query={withdrawalsQuery}
        columns={columns}
        getRowId={(r) => r.id}
        emptyIcon={Wallet}
        emptyTitle={t("adminAffiliates.emptyWithdrawals")}
        emptyBody={t("adminAffiliates.emptyWithdrawalsBody")}
        minWidth={560}
        rowActions={(r) =>
          r.status === "pending" ? (
            <RowActionsMenu
              actions={[
                {
                  label: t("adminAffiliates.approveWithdrawal"),
                  onSelect: () => setApproveTarget(r),
                },
                {
                  label: t("adminAffiliates.cancelWithdrawal"),
                  destructive: true,
                  onSelect: () => setCancelTarget(r),
                },
              ]}
            />
          ) : null
        }
      />

      <ConfirmDialog
        open={approveTarget !== null}
        onOpenChange={(open) => {
          if (!open) setApproveTarget(null);
        }}
        tone="default"
        title={t("adminAffiliates.approveWithdrawal")}
        body={
          approveTarget
            ? `${userLookup.get(approveTarget.user_id)} · ${formatMoney(approveTarget.amount, approveTarget.currency)}`
            : undefined
        }
        confirmLabel={t("adminAffiliates.approveWithdrawal")}
        successMessage={t("feedback.saved")}
        isPending={approveMut.isPending}
        onConfirm={async () => {
          if (approveTarget) {
            await approveMut.mutateAsync({ id: approveTarget.id, body: {} });
          }
        }}
      />

      <ConfirmDialog
        open={cancelTarget !== null}
        onOpenChange={(open) => {
          if (!open) setCancelTarget(null);
        }}
        tone="danger"
        title={t("adminAffiliates.cancelWithdrawal")}
        body={
          cancelTarget
            ? `${userLookup.get(cancelTarget.user_id)} · ${formatMoney(cancelTarget.amount, cancelTarget.currency)}`
            : undefined
        }
        confirmLabel={t("adminAffiliates.cancelWithdrawal")}
        successMessage={t("feedback.saved")}
        isPending={cancelMut.isPending}
        onConfirm={async () => {
          if (cancelTarget) {
            await cancelMut.mutateAsync({ id: cancelTarget.id, body: {} });
          }
        }}
      />
    </>
  );
}
