"use client";

import { useState } from "react";
import { Users } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { UserPlatformQuotasDialog } from "@/components/admin/user-platform-quotas-dialog";
import { UserBalanceHistoryDialog } from "@/components/admin/user-balance-history-dialog";
import { UserAttributeValuesDialog } from "@/components/admin/user-attribute-values-dialog";
import { ListToolbar, SearchInput, FilterSelect } from "@/components/admin/list-toolbar";
import { ColumnToggle } from "@/components/ui/column-toggle";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { useCurrentUserShell } from "@/components/layout/auth-gate";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useAdminUsers,
  useSetUserEnabled,
  useBulkSetUsersEnabled,
  useCreateAdminUser,
  useUpdateAdminUser,
  useDeleteAdminUser,
  useUpdateUserBalance,
  useUserAttributeValuesBatch,
  useUsersSpendingTodayBatch,
} from "@/hooks/admin-queries";
import type { UserStatus } from "@/lib/sdk-types";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney, formatInteger, formatPercent } from "@/lib/admin-format";
import {
  USER_STATUSES,
  USER_FILTER_ROLES,
  BALANCE_OPERATIONS,
  emptyUserCreateForm,
  userEditFormFromUser,
  emptyUserBalanceForm,
  buildCreateUserBody,
  buildUpdateUserBody,
  buildUserBalanceBody,
  type UserCreateFormState,
  type UserEditFormState,
  type UserBalanceFormState,
} from "@/lib/admin-user-form";
import type { User } from "@/lib/sdk-types";

export default function AdminUsersPage() {
  return (
    <AdminShell>
      <UsersContent />
    </AdminShell>
  );
}

function UsersContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-users", ["created_at", "updated_at"]);
  const statusFilter = list.filters.status as UserStatus | undefined;
  const roleFilter = list.filters.role || undefined;
  const users = useAdminUsers({
    page: list.page,
    page_size: list.pageSize,
    q: list.search || undefined,
    status: statusFilter,
    role: roleFilter,
  });
  const setEnabled = useSetUserEnabled();
  const bulkEnabled = useBulkSetUsersEnabled();
  // Attribute values for the current page in one round-trip. Group by user_id
  // so the column can pick up to N chips per row without re-scanning the list.
  const visibleUserIds = (users.data?.data ?? []).map((u) => u.id);
  const attributeBatch = useUserAttributeValuesBatch(visibleUserIds);
  const attributesByUserId = new Map<string, typeof attributeBatch.data>();
  for (const row of attributeBatch.data ?? []) {
    const existing = attributesByUserId.get(row.user_id) ?? [];
    existing.push(row);
    attributesByUserId.set(row.user_id, existing);
  }
  // Same shape as the iter-23 accounts today-stats column, just grouped by
  // user_id. Joined back by id when the column renders below.
  const spendingBatch = useUsersSpendingTodayBatch(visibleUserIds);
  const spendingByUserId = new Map(
    (spendingBatch.data ?? []).map((r) => [r.user_id, r] as const),
  );
  const createMut = useCreateAdminUser();
  const updateMut = useUpdateAdminUser();
  const deleteMut = useDeleteAdminUser();
  const balanceMut = useUpdateUserBalance();
  const currentUser = useCurrentUserShell();
  const selfId = currentUser?.id;

  const [creating, setCreating] = useState(false);
  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [balanceTarget, setBalanceTarget] = useState<User | null>(null);
  const [historyTarget, setHistoryTarget] = useState<User | null>(null);
  const [quotaTarget, setQuotaTarget] = useState<User | null>(null);
  const [attributesTarget, setAttributesTarget] = useState<User | null>(null);
  const [disableTarget, setDisableTarget] = useState<User | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);
  const [bulkDisableOpen, setBulkDisableOpen] = useState(false);

  const isFiltered = Boolean(list.search || statusFilter || roleFilter);

  async function toggleEnabled(u: User) {
    try {
      await setEnabled.mutateAsync(u);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  // Clear the selection only after the bulk call resolves, and surface failures —
  // otherwise a rejected batch left the rows checked, reading as "done".
  async function runBulk(enabled: boolean) {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      await bulkEnabled.mutateAsync({ ids, enabled });
      list.clearSelection();
      toast({ title: t("feedback.saved"), description: `${ids.length}`, tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const createFields: FieldConfig<UserCreateFormState>[] = [
    { name: "email", label: t("adminUsers.email"), required: true },
    { name: "name", label: t("adminUsers.name"), required: true },
    { name: "password", label: t("adminUsers.password"), type: "password", required: true },
    { name: "rolesCsv", label: t("adminUsers.roles"), placeholder: "user, admin" },
    { name: "status", label: t("adminCommon.status"), type: "select", options: enumOptions(USER_STATUSES) },
    { name: "rpmLimit", label: t("adminUsers.rpmLimit"), type: "number", placeholder: t("adminUsers.unlimited") },
  ];

  const editFields: FieldConfig<UserEditFormState>[] = [
    { name: "name", label: t("adminUsers.name") },
    { name: "rolesCsv", label: t("adminUsers.roles"), placeholder: "user, admin" },
    { name: "status", label: t("adminCommon.status"), type: "select", options: enumOptions(USER_STATUSES) },
    { name: "rpmLimit", label: t("adminUsers.rpmLimit"), type: "number", placeholder: t("adminUsers.unlimited") },
  ];

  const balanceFields: FieldConfig<UserBalanceFormState>[] = [
    {
      name: "amount",
      label: t("adminUsers.amount"),
      validate: (value) => {
        const num = Number(value);
        if (num === 0) return "Amount must be greater than zero.";
        return undefined;
      },
    },
    { name: "operation", label: t("adminUsers.operation"), type: "select", options: enumOptions(BALANCE_OPERATIONS) },
    { name: "currency", label: t("adminCommon.currency") },
    { name: "note", label: t("adminUsers.note"), type: "textarea" },
  ];

  const columns: Column<User>[] = [
    {
      key: "email",
      header: t("adminUsers.email"),
      pinned: true,
      sortValue: (u) => u.email,
      render: (u) => (
        <div className="min-w-0">
          <div className="truncate text-srapi-text-primary">{u.name}</div>
          <div className="truncate font-mono text-2xs text-srapi-text-tertiary">{u.email}</div>
        </div>
      ),
    },
    {
      key: "roles",
      header: t("adminUsers.roles"),
      hideOnMobile: true,
      render: (u) => (
        <span className="font-mono text-2xs text-srapi-text-secondary">{u.roles.join(" · ")}</span>
      ),
    },
    {
      key: "balance",
      header: t("adminUsers.balance"),
      align: "right",
      sortValue: (u) => Number(u.balance),
      render: (u) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(u.balance, u.currency)}
        </span>
      ),
    },
    {
      key: "today",
      header: t("adminUsers.today"),
      hideOnMobile: true,
      sortValue: (u) => spendingByUserId.get(u.id)?.requests ?? -1,
      render: (u) => {
        const today = spendingByUserId.get(u.id);
        if (!today) {
          return <span className="font-mono text-2xs text-srapi-text-tertiary">—</span>;
        }
        if (today.requests === 0) {
          return (
            <span className="font-mono text-2xs text-srapi-text-tertiary">
              {t("adminUsers.todayIdle")}
            </span>
          );
        }
        return (
          <div className="flex flex-col gap-0.5">
            <span className="font-mono text-2xs text-srapi-text-secondary tabular">
              {formatInteger(today.requests)} · {formatMoney(today.cost, today.currency)}
            </span>
            <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
              {formatPercent(today.success_rate)}
            </span>
          </div>
        );
      },
    },
    {
      key: "attributes",
      header: t("adminUsers.attributesColumn"),
      hideOnMobile: true,
      render: (u) => {
        const rows = attributesByUserId.get(u.id) ?? [];
        // Only show definitions the operator has actually set — empty values
        // are noise. Cap at 3 chips so the column stays compact; the full
        // editor lives behind the row-actions "Custom attributes" entry.
        const set = rows.filter((r) => r.value !== "");
        if (set.length === 0) {
          return <span className="text-2xs text-srapi-text-tertiary">—</span>;
        }
        const chips = set.slice(0, 3);
        const extra = set.length - chips.length;
        return (
          <div className="flex flex-wrap gap-1">
            {chips.map((row) => (
              <span
                key={row.definition_id}
                className="inline-flex items-center rounded-md border border-srapi-border bg-srapi-card-muted px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary"
                title={`${row.name} (${row.key})`}
              >
                {row.key}: {row.value}
              </span>
            ))}
            {extra > 0 ? (
              <span className="font-mono text-2xs text-srapi-text-tertiary">+{extra}</span>
            ) : null}
          </div>
        );
      },
    },
    {
      key: "status",
      header: t("common.active"),
      render: (u) => <QuietBadge status={quietStatusFor(u.status)} label={statusLabel(t, u.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminUsers.title")}
        description={t("adminUsers.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {users.data ? (
              <ListCount total={users.data.pagination?.total ?? users.data.data.length} />
            ) : null}
            <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
              ＋ {t("adminUsers.create")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={users}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(u) => u.id}
        emptyIcon={Users}
        emptyTitle={t("adminUsers.emptyTitle")}
        emptyBody={t("adminUsers.emptyBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
            ＋ {t("adminUsers.create")}
          </Button>
        }
        dimRow={(u) => u.status === "disabled"}
        isFiltered={isFiltered}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminUsers.searchPlaceholder")}
            />
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={enumOptions(USER_STATUSES)}
              allLabel={t("adminCommon.allStatuses")}
            />
            <FilterSelect
              value={roleFilter}
              onChange={(v) => list.setFilter("role", v)}
              options={USER_FILTER_ROLES.map((r) => ({ value: r, label: r }))}
              allLabel={t("adminUsers.allRoles")}
            />
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
          </ListToolbar>
        }
        selection={{
          selected: list.selected,
          onToggle: list.toggle,
          onTogglePage: list.togglePage,
          bulkActions: (
            <>
              <Button
                variant="outline"
                size="sm"
                loading={bulkEnabled.isPending}
                onClick={() => runBulk(true)}
              >
                {t("adminUsers.enable")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                loading={bulkEnabled.isPending}
                onClick={() => setBulkDisableOpen(true)}
              >
                {t("adminUsers.disable")}
              </Button>
            </>
          ),
        }}
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: users.data?.pagination?.total ?? users.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(u) => (
          <RowActionsMenu
            actions={[
              { label: t("adminUsers.edit"), onSelect: () => setEditTarget(u) },
              { label: t("adminUsers.adjustBalance"), onSelect: () => setBalanceTarget(u) },
              { label: t("adminUsers.balanceHistory"), onSelect: () => setHistoryTarget(u) },
              { label: t("adminUsers.platformQuotas"), onSelect: () => setQuotaTarget(u) },
              { label: t("adminUsers.attributes"), onSelect: () => setAttributesTarget(u) },
              {
                label: u.status === "disabled" ? t("adminUsers.enable") : t("adminUsers.disable"),
                destructive: u.status !== "disabled",
                onSelect: () =>
                  u.status === "disabled" ? void toggleEnabled(u) : setDisableTarget(u),
              },
              // Self-delete is also rejected server-side, but hiding it removes
              // a footgun and a confusing dialog.
              ...(selfId && u.id === selfId
                ? []
                : [
                    {
                      label: t("adminUsers.delete"),
                      destructive: true,
                      onSelect: () => setDeleteTarget(u),
                    },
                  ]),
            ]}
          />
        )}
      />

      {creating ? (
        <ResourceFormDialog
          open
          onOpenChange={setCreating}
          title={t("adminUsers.create")}
          fields={createFields}
          initial={emptyUserCreateForm()}
          buildBody={buildCreateUserBody}
          submit={(body) => createMut.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createMut.isPending}
        />
      ) : null}

      {editTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setEditTarget(null);
          }}
          title={t("adminUsers.edit")}
          description={editTarget.email}
          fields={editFields}
          initial={userEditFormFromUser(editTarget)}
          buildBody={buildUpdateUserBody}
          submit={(body) => updateMut.mutateAsync({ id: editTarget.id, body })}
          successMessage={t("feedback.updated")}
          isPending={updateMut.isPending}
        />
      ) : null}

      {balanceTarget ? (
        <ResourceFormDialog
          open
          onOpenChange={(open) => {
            if (!open) setBalanceTarget(null);
          }}
          title={t("adminUsers.adjustBalance")}
          description={`${balanceTarget.name} · ${formatMoney(balanceTarget.balance, balanceTarget.currency)}`}
          fields={balanceFields}
          initial={emptyUserBalanceForm(balanceTarget.currency)}
          buildBody={buildUserBalanceBody}
          submit={(body) => balanceMut.mutateAsync({ id: balanceTarget.id, body })}
          successMessage={t("feedback.updated")}
          isPending={balanceMut.isPending}
        />
      ) : null}

      {historyTarget ? (
        <UserBalanceHistoryDialog
          userId={historyTarget.id}
          email={historyTarget.email}
          open
          onOpenChange={(open) => {
            if (!open) setHistoryTarget(null);
          }}
        />
      ) : null}

      {quotaTarget ? (
        <UserPlatformQuotasDialog
          userId={quotaTarget.id}
          userLabel={quotaTarget.email}
          onClose={() => setQuotaTarget(null)}
        />
      ) : null}

      {attributesTarget ? (
        <UserAttributeValuesDialog
          userId={attributesTarget.id}
          userLabel={attributesTarget.email}
          onClose={() => setAttributesTarget(null)}
        />
      ) : null}

      <ConfirmDialog
        open={disableTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDisableTarget(null);
        }}
        title={t("adminUsers.disable")}
        body={disableTarget?.email}
        confirmLabel={t("adminUsers.disable")}
        successMessage={t("feedback.saved")}
        isPending={setEnabled.isPending}
        onConfirm={async () => {
          if (disableTarget) await setEnabled.mutateAsync(disableTarget);
        }}
      />

      <ConfirmDialog
        open={bulkDisableOpen}
        onOpenChange={setBulkDisableOpen}
        title={t("adminUsers.disable")}
        confirmLabel={t("adminUsers.disable")}
        isPending={bulkEnabled.isPending}
        onConfirm={() => runBulk(false)}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        tone="danger"
        title={t("adminUsers.delete")}
        body={
          deleteTarget
            ? `${deleteTarget.name} · ${deleteTarget.email}\n${t("adminUsers.deleteWarning")}`
            : undefined
        }
        confirmLabel={t("adminUsers.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteMut.isPending}
        onConfirm={async () => {
          if (deleteTarget) await deleteMut.mutateAsync(deleteTarget.id);
        }}
      />
    </>
  );
}
