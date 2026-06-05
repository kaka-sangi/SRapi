"use client";

import { useState } from "react";
import { KeyRound } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { enumOptions } from "@/components/admin/resource-form-dialog";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { useAdminList } from "@/hooks/use-admin-list";
import { useAdminApiKeys, useUpdateAdminApiKey } from "@/hooks/admin-queries";
import { ApiKeyUsageDialog } from "@/components/features/api-key-usage-dialog";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { formatDateTime } from "@/lib/admin-format";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import type { ApiKey } from "@/lib/sdk-types";

const API_KEY_STATUSES = ["active", "disabled", "expired"];

export default function AdminApiKeysPage() {
  return (
    <AdminShell>
      <ApiKeysContent />
    </AdminShell>
  );
}

/**
 * Cross-user API-key console. The user-facing /api-keys page only ever shows the
 * caller's own keys; this lists every user's keys (attributed by owner email)
 * so an admin can audit and revoke. Revoke = disable (soft) — keys are never
 * hard-deleted, preserving the audit trail.
 */
function ApiKeysContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList();
  const statusFilter = (list.filters.status as ApiKey["status"]) || undefined;
  const keys = useAdminApiKeys({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
  });
  const updateMut = useUpdateAdminApiKey();
  const [revokeTarget, setRevokeTarget] = useState<ApiKey | null>(null);
  const [usageTarget, setUsageTarget] = useState<ApiKey | null>(null);

  async function enableKey(key: ApiKey) {
    try {
      await updateMut.mutateAsync({ id: String(key.id), body: { status: "active" } });
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const columns: Column<ApiKey>[] = [
    {
      key: "key",
      header: t("adminApiKeys.key"),
      render: (k) => (
        <div className="min-w-0">
          <div className="truncate text-srapi-text-primary">{k.name}</div>
          <div className="truncate font-mono text-2xs text-srapi-text-tertiary">{k.prefix}</div>
        </div>
      ),
      sortValue: (k) => k.name,
    },
    {
      key: "owner",
      header: t("adminApiKeys.owner"),
      render: (k) => (
        <span className="font-mono text-2xs text-srapi-text-secondary">
          {k.user_email || (k.user_id != null ? `#${k.user_id}` : "—")}
        </span>
      ),
    },
    {
      key: "rpm",
      header: t("adminApiKeys.rpm"),
      align: "right",
      hideOnMobile: true,
      render: (k) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {k.rpm_limit != null ? k.rpm_limit : "∞"}
        </span>
      ),
    },
    {
      key: "lastUsed",
      header: t("adminApiKeys.lastUsed"),
      hideOnMobile: true,
      render: (k) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {k.last_used_at ? formatDateTime(k.last_used_at) : "—"}
        </span>
      ),
    },
    {
      key: "status",
      header: t("adminCommon.status"),
      sortValue: (k) => k.status,
      render: (k) => <QuietBadge status={quietStatusFor(k.status)} label={statusLabel(t, k.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminApiKeys.title")}
        description={t("adminApiKeys.subtitle")}
        actions={
          keys.data ? (
            <ListCount total={keys.data.pagination?.total ?? keys.data.data.length} />
          ) : undefined
        }
      />
      <AdminListView
        query={keys}
        columns={columns}
        getRowId={(k) => String(k.id)}
        emptyIcon={KeyRound}
        emptyTitle={t("adminApiKeys.emptyTitle")}
        emptyBody={t("adminApiKeys.emptyBody")}
        minWidth={680}
        isFiltered={Boolean(statusFilter)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        dimRow={(k) => k.status !== "active"}
        toolbar={
          <ListToolbar>
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={enumOptions(API_KEY_STATUSES)}
              allLabel={t("adminCommon.allStatuses")}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: keys.data?.pagination?.total ?? keys.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(k) => (
          <RowActionsMenu
            actions={[
              { label: t("apiKeys.usageAction"), onSelect: () => setUsageTarget(k) },
              ...(k.status === "expired"
                ? []
                : k.status === "disabled"
                  ? [{ label: t("adminApiKeys.enable"), onSelect: () => void enableKey(k) }]
                  : [
                      {
                        label: t("adminApiKeys.revoke"),
                        destructive: true,
                        onSelect: () => setRevokeTarget(k),
                      },
                    ]),
            ]}
          />
        )}
      />

      <ApiKeyUsageDialog
        keyId={usageTarget ? String(usageTarget.id) : null}
        keyName={usageTarget?.name ?? ""}
        variant="admin"
        open={usageTarget !== null}
        onOpenChange={(open) => {
          if (!open) setUsageTarget(null);
        }}
      />

      {revokeTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setRevokeTarget(null);
          }}
          title={t("adminApiKeys.revokeTitle")}
          body={t("adminApiKeys.revokeBody", {
            name: revokeTarget.name,
            prefix: revokeTarget.prefix,
          })}
          confirmLabel={t("adminApiKeys.revoke")}
          onConfirm={() =>
            updateMut.mutateAsync({ id: String(revokeTarget.id), body: { status: "disabled" } })
          }
          successMessage={t("feedback.saved")}
          isPending={updateMut.isPending}
        />
      ) : null}
    </>
  );
}
