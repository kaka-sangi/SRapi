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
} from "@/hooks/admin-queries";
import type { UserStatus } from "@/lib/sdk-types";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { adminErrorMessage } from "@/lib/admin-api";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney } from "@/lib/admin-format";
import {
  USER_STATUSES,
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
  const users = useAdminUsers({
    page: list.page,
    page_size: list.pageSize,
    q: list.search || undefined,
    status: statusFilter,
  });
  const setEnabled = useSetUserEnabled();
  const bulkEnabled = useBulkSetUsersEnabled();
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

  const isFiltered = Boolean(list.search || statusFilter);

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
