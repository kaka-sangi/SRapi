"use client";

import { useState } from "react";
import Link from "next/link";
import { KeyRound } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { enumOptions } from "@/components/admin/resource-form-dialog";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { CopyButton } from "@/components/ui/copy-button";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { useAdminApiKeys, useAdminUsers, useResetAdminApiKeyUsage, useUpdateAdminApiKey } from "@/hooks/admin-queries";
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
  const colVis = useColumnVisibility("admin-api-keys", ["created_at"]);
  const statusFilter = (list.filters.status as ApiKey["status"]) || undefined;
  const userFilter = list.filters.userId || undefined;
  const keys = useAdminApiKeys({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
    user_id: userFilter,
  });
  // Audit one user's keys at a time — page-1/200 covers typical installs and
  // matches the lookup pattern used by error-logs / payment-dashboard panels.
  const users = useAdminUsers({ page: 1, page_size: 200 });
  const userOptions = (users.data?.data ?? []).map((u) => ({
    value: String(u.id),
    label: u.email,
  }));
  const updateMut = useUpdateAdminApiKey();
  const resetUsageMut = useResetAdminApiKeyUsage();
  const [revokeTarget, setRevokeTarget] = useState<ApiKey | null>(null);
  const [resetUsageTarget, setResetUsageTarget] = useState<ApiKey | null>(null);
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
      pinned: true,
      render: (k) => (
        <div className="min-w-0">
          <div className="truncate text-srapi-text-primary">{k.name}</div>
          <div className="flex min-w-0 items-center gap-1">
            <span className="truncate font-mono text-2xs text-srapi-text-tertiary">{k.prefix}</span>
            {k.prefix ? <CopyButton value={k.prefix} size="inline" /> : null}
          </div>
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
      render: (k) => {
        const badge = (
          <QuietBadge status={quietStatusFor(k.status)} label={statusLabel(t, k.status)} />
        );
        // Expired is terminal — show the badge only. Active routes through the
        // existing revoke ConfirmDialog (clicking should not silently kill a
        // live key). Disabled is restorative and safe to flip in one click.
        if (k.status === "expired") return badge;
        const onClick =
          k.status === "disabled" ? () => void enableKey(k) : () => setRevokeTarget(k);
        const title =
          k.status === "disabled" ? t("adminApiKeys.enable") : t("adminApiKeys.revoke");
        return (
          <button
            type="button"
            onClick={onClick}
            disabled={updateMut.isPending}
            className="cursor-pointer disabled:cursor-wait disabled:opacity-60"
            title={title}
          >
            {badge}
          </button>
        );
      },
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminApiKeys.title")}
        description={t("adminApiKeys.subtitle")}
        actions={
          <div className="flex items-center gap-2">
            {keys.data ? (
              <ListCount total={keys.data.pagination?.total ?? keys.data.data.length} />
            ) : null}
            <Button asChild size="sm" variant="outline">
              <Link href="/api-keys">{t("adminApiKeys.createCta")}</Link>
            </Button>
          </div>
        }
      />
      <AdminListView
        query={keys}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(k) => String(k.id)}
        emptyIcon={KeyRound}
        emptyTitle={t("adminApiKeys.emptyTitle")}
        emptyBody={t("adminApiKeys.emptyBody")}
        minWidth={680}
        isFiltered={Boolean(statusFilter || userFilter)}
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
            <FilterSelect
              value={userFilter}
              onChange={(v) => list.setFilter("userId", v)}
              options={userOptions}
              allLabel={t("adminApiKeys.allUsers")}
            />
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
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
              { label: t("adminApiKeys.resetUsage"), onSelect: () => setResetUsageTarget(k) },
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

      {resetUsageTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setResetUsageTarget(null);
          }}
          tone="default"
          title={t("adminApiKeys.resetUsageTitle")}
          body={t("adminApiKeys.resetUsageBody", {
            name: resetUsageTarget.name,
            prefix: resetUsageTarget.prefix,
          })}
          confirmLabel={t("adminApiKeys.resetUsage")}
          onConfirm={() => resetUsageMut.mutateAsync(String(resetUsageTarget.id))}
          successMessage={t("feedback.saved")}
          isPending={resetUsageMut.isPending}
        />
      ) : null}
    </>
  );
}
