"use client";

import { useState } from "react";
import Link from "next/link";
import { KeyRound } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { Button } from "@/components/ui/button";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { CopyButton } from "@/components/ui/copy-button";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { InlineDetailGrid } from "@/components/ui/inline-detail-grid";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { useAdminApiKeys, useResetAdminApiKeyUsage, useUpdateAdminApiKey } from "@/hooks/admin-queries";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
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
  // Audit one user's keys at a time — the shared lookup hook covers the same
  // page-1/200 window the inline fetch used to.
  const userLookup = useUserEmailLookup();
  const userOptions = (userLookup.query.data?.data ?? []).map((u) => ({
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
          <div className="truncate text-srapi-text-primary font-medium">{k.name}</div>
          <div className="flex min-w-0 items-center gap-1">
            <span className="truncate font-mono text-xs text-srapi-text-tertiary">{k.prefix}</span>
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
        <span className="text-xs text-srapi-text-secondary tabular">
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
        <DataTooltip
          title={t("adminApiKeys.rpm")}
          primary={k.rpm_limit != null ? String(k.rpm_limit) : "∞"}
          rows={[
            { label: "tpm", value: k.tpm_limit != null ? String(k.tpm_limit) : "∞", tone: "muted" },
            { label: "concurrency", value: k.concurrency_limit != null ? String(k.concurrency_limit) : "∞", tone: "muted" },
            { label: "5h req", value: k.request_limit_5h != null ? String(k.request_limit_5h) : "∞", tone: "muted" },
            { label: "1d req", value: k.request_limit_1d != null ? String(k.request_limit_1d) : "∞", tone: "muted" },
          ]}
        >
          <span className="text-xs text-srapi-text-tertiary tabular">
            {k.rpm_limit != null ? k.rpm_limit : "∞"}
          </span>
        </DataTooltip>
      ),
    },
    {
      key: "lastUsed",
      header: t("adminApiKeys.lastUsed"),
      hideOnMobile: true,
      render: (k) => (
        <DataTooltip
          title={t("adminApiKeys.lastUsed")}
          primary={k.last_used_at ? formatDateTime(k.last_used_at) : "—"}
          rows={[
            { label: "cost (5h)", value: k.cost_used_5h ?? "0", tone: "muted" },
            { label: "cost (1d)", value: k.cost_used_1d ?? "0", tone: "muted" },
            { label: "cost (7d)", value: k.cost_used_7d ?? "0", tone: "muted" },
            { label: "total cost", value: k.cost_used ?? "0" },
          ]}
        >
          <span className="text-xs text-srapi-text-tertiary tabular">
            {k.last_used_at ? formatDateTime(k.last_used_at) : "—"}
          </span>
        </DataTooltip>
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
      <SectionHero
        eyebrow="Gateway · API Keys"
        title={t("adminApiKeys.title")}
        description={t("adminApiKeys.subtitle")}
        metrics={
          keys.data
            ? [
                {
                  label: t("adminApiKeys.title"),
                  value: String(keys.data.pagination?.total ?? keys.data.data.length),
                },
              ]
            : undefined
        }
        actions={
          <Button asChild size="sm" variant="outline">
            <Link href="/api-keys">{t("adminApiKeys.createCta")}</Link>
          </Button>
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
        enableKeyboardNav
        rowSeverity={(k) => {
          // Expired keys are terminal — make them stand out with an error
          // stripe. Disabled keys are restorable, info-stripe. Active stays
          // implicit (the default visual; per polish guidance, color is sparing).
          if (k.status === "expired") return "error";
          if (k.status === "disabled") return "info";
          return undefined;
        }}
        expandRow={(k) => {
          const ips = k.allowed_ips ?? [];
          const deniedIps = k.denied_ips ?? [];
          const scopes = k.scopes ?? [];
          const allowedModels = k.allowed_models ?? [];
          return (
            <InlineDetailGrid
              sections={[
                {
                  title: t("adminApiKeys.title"),
                  rows: [
                    { label: t("adminApiKeys.owner"), value: k.user_email || (k.user_id != null ? `#${k.user_id}` : "—") },
                    { label: t("adminApiKeys.lastUsed"), value: k.last_used_at ? formatDateTime(k.last_used_at) : "—", tone: k.last_used_at ? "default" : "muted" },
                    { label: t("common.created"), value: k.created_at ? formatDateTime(k.created_at) : "—", tone: "muted" },
                    { label: "expires", value: k.expires_at ? formatDateTime(k.expires_at) : "—", tone: "muted" },
                  ],
                },
                {
                  title: "usage",
                  rows: [
                    { label: "rpm", value: k.rpm_limit != null ? String(k.rpm_limit) : "∞" },
                    { label: "tpm", value: k.tpm_limit != null ? String(k.tpm_limit) : "∞", tone: "muted" },
                    { label: "concurrency", value: k.concurrency_limit != null ? String(k.concurrency_limit) : "∞", tone: "muted" },
                    { label: "total cost", value: k.cost_used ?? "0" },
                    { label: "5h cost", value: k.cost_used_5h ?? "0", tone: "muted" },
                    { label: "1d cost", value: k.cost_used_1d ?? "0", tone: "muted" },
                  ],
                },
                {
                  title: "ips & scopes",
                  rows: [
                    { label: "allowed_ips", value: ips.length === 0 ? "all" : (ips.slice(0, 4).join(", ") + (ips.length > 4 ? ` +${ips.length - 4}` : "")), mono: true, tone: ips.length === 0 ? "muted" : "default" },
                    { label: "denied_ips", value: deniedIps.length === 0 ? "—" : (deniedIps.slice(0, 4).join(", ") + (deniedIps.length > 4 ? ` +${deniedIps.length - 4}` : "")), mono: true, tone: deniedIps.length === 0 ? "muted" : "warning" },
                    { label: "scopes", value: scopes.length === 0 ? "all" : (scopes.slice(0, 4).join(", ") + (scopes.length > 4 ? ` +${scopes.length - 4}` : "")), tone: scopes.length === 0 ? "muted" : "default" },
                    { label: "allowed_models", value: allowedModels.length === 0 ? "all" : (allowedModels.slice(0, 3).join(", ") + (allowedModels.length > 3 ? ` +${allowedModels.length - 3}` : "")), tone: allowedModels.length === 0 ? "muted" : "default" },
                  ],
                },
              ]}
              actions={
                <Button variant="outline" size="sm" onClick={() => setUsageTarget(k)}>
                  {t("apiKeys.usageAction")}
                </Button>
              }
            />
          );
        }}
        toolbar={
          <ListToolbar>
            <SegmentedControl<string>
              value={statusFilter || "__all__"}
              onChange={(v) => list.setFilter("status", v === "__all__" ? undefined : v)}
              ariaLabel={t("adminCommon.status")}
              size="sm"
              options={[
                { value: "__all__", label: t("adminCommon.allStatuses") },
                ...API_KEY_STATUSES.map((s) => ({ value: s, label: s })),
              ]}
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
