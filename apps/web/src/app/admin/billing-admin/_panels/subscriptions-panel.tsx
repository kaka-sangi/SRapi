"use client";

import { useState } from "react";
import { CreditCard } from "lucide-react";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import { useClientPagedList } from "@/hooks/use-client-list";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { RowActionsMenu, type RowAction } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import {
  useAdminSubscriptionPlans,
  useAdminSubscriptions,
  useBatchAssignUserSubscriptions,
  useCreateUserSubscription,
  useDeleteUserSubscription,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { SegmentedControl } from "@/components/ui/segmented-control";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { adminErrorMessage } from "@/lib/admin-api";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatDate } from "@/lib/admin-format";
import {
  SubscriptionUsageBars,
  type SubscriptionUsageLabels,
} from "@/components/features/subscription-usage-bars";
import {
  USER_SUBSCRIPTION_STATUSES,
  emptyUserSubscriptionForm,
  buildCreateUserSubscriptionBody,
  type UserSubscriptionFormState,
} from "@/lib/admin-subscription-form";
import type { UserSubscription } from "@/lib/sdk-types";

export function SubscriptionsPanel() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-subscriptions", []);
  const plans = useAdminSubscriptionPlans();
  const allSubs = useAdminSubscriptions();
  // Server endpoint accepts page/page_size/user_id only — no status filter —
  // so the operator-side filtering happens client-side via the shared list
  // helper. The full list ships in one shot today, so the lift is small.
  const { query: subs, total } = useClientPagedList(allSubs, list, {
    match: (row, term, filters) => {
      if (filters.status && row.status !== filters.status) return false;
      if (!term) return true;
      // user_id alone is opaque (numeric); search hits both the raw id and
      // the looked-up email so an operator can paste either.
      const email = userLookup.map.get(String(row.user_id)) ?? "";
      return [String(row.user_id), email, String(row.plan_id), row.status]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(term);
    },
    compare: (a, b) => (b.starts_at ?? "").localeCompare(a.starts_at ?? ""),
  });
  const statusFilter = list.filters.status as UserSubscription["status"] | undefined;
  const userLookup = useUserEmailLookup();
  const createSub = useCreateUserSubscription();
  const deleteSub = useDeleteUserSubscription();
  const batchAssign = useBatchAssignUserSubscriptions();
  const { toast } = useToast();
  const [creatingSub, setCreatingSub] = useState(false);
  const [subToDelete, setSubToDelete] = useState<UserSubscription | null>(null);
  const [bulkAssignOpen, setBulkAssignOpen] = useState(false);

  // applyBulkAssign is the verbatim wiring for the port of sub2api's
  // SubscriptionService.BulkAssignSubscription. The dialog hands parsed items
  // to this method; the server applies them per-row with created/reused/failed
  // outcomes — partial-failure is the default and surfaces in the toast.
  async function applyBulkAssign(items: { user_id: string; plan_id: string }[]) {
    if (items.length === 0) return;
    try {
      const result = await batchAssign.mutateAsync(items);
      const failedCount = result.errors.length;
      const succeededCount = result.created_count + result.reused_count;
      if (failedCount > 0 && succeededCount > 0) {
        toast({
          title: t("feedback.batchPartial", { succeeded: succeededCount, failed: failedCount }),
          tone: "warning",
        });
      } else if (failedCount > 0) {
        toast({ title: t("feedback.batchAllFailed", { count: items.length }), tone: "error" });
      } else {
        toast({ title: t("feedback.batchAllSucceeded", { count: succeededCount }), tone: "success" });
      }
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const subFields: FieldConfig<UserSubscriptionFormState>[] = [
    {
      name: "userId",
      label: t("adminSubscriptions.user"),
      type: "select",
      required: true,
      options: (userLookup.query.data?.data ?? []).map((u) => ({ value: u.id, label: u.email })),
    },
    {
      name: "planId",
      label: t("adminSubscriptions.plan"),
      type: "select",
      required: true,
      options: (plans.data?.data ?? []).map((p) => ({ value: p.id, label: p.name })),
    },
    {
      name: "status",
      label: t("adminCommon.status"),
      type: "select",
      options: enumOptions(USER_SUBSCRIPTION_STATUSES),
    },
    { name: "startsAtLocal", label: t("adminCommon.startsAt"), type: "datetime" },
    { name: "expiresAtLocal", label: t("adminCommon.expiresAt"), type: "datetime" },
  ];

  const subColumns: Column<UserSubscription>[] = [
    {
      key: "user",
      header: t("adminSubscriptions.user"),
      pinned: true,
      render: (s) => <span className="text-srapi-text-secondary">{userLookup.get(s.user_id)}</span>,
    },
    {
      key: "plan",
      header: t("adminSubscriptions.plan"),
      pinned: true,
      render: (s) => (
        <span className="text-sm font-medium tabular text-srapi-text-secondary">{s.plan_id}</span>
      ),
    },
    {
      key: "period",
      header: t("adminSubscriptions.period"),
      hideOnMobile: true,
      render: (s) => (
        <span className="text-[12px] tabular text-srapi-text-tertiary">
          {formatDate(s.starts_at)} – {formatDate(s.expires_at)}
        </span>
      ),
    },
    {
      key: "usage",
      header: t("adminSubscriptions.usage"),
      hideOnMobile: true,
      render: (s) => <SubscriptionUsageBars subscription={s} labels={subscriptionUsageLabels(t)} />,
    },
    {
      key: "status",
      header: t("common.active"),
      render: (s) => <QuietBadge status={quietStatusFor(s.status)} label={statusLabel(t, s.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminSubscriptions.title")}
        description={t("adminSubscriptions.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {allSubs.data ? <ListCount total={total} /> : null}
            <Button
              variant="outline"
              size="sm"
              loading={batchAssign.isPending}
              onClick={() => setBulkAssignOpen(true)}
            >
              {t("adminSubscriptions.bulkAssign")}
            </Button>
            <Button variant="primary" size="sm" onClick={() => setCreatingSub(true)}>
              ＋ {t("adminSubscriptions.createSubscription")}
            </Button>
          </div>
        }
      />
      <AdminListView
        query={subs}
        columns={subColumns}
        columnVisibility={colVis}
        getRowId={(s) => s.id}
        emptyIcon={CreditCard}
        emptyTitle={t("adminSubscriptions.emptySubs")}
        emptyBody={t("adminSubscriptions.emptySubsBody")}
        minWidth={560}
        isFiltered={Boolean(statusFilter || list.search)}
        onClearFilters={list.clearFilters}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminSubscriptions.searchPlaceholder")}
            />
            <SegmentedControl<string>
              value={(list.filters.status as string) ?? "all"}
              onChange={(v) => list.setFilter("status", v === "all" ? "" : v)}
              ariaLabel={t("adminCommon.allStatuses")}
              size="sm"
              options={[
                { value: "all", label: t("adminCommon.allStatuses") },
                ...USER_SUBSCRIPTION_STATUSES.map((s) => ({
                  value: s,
                  label: statusLabel(t, s),
                })),
              ]}
            />
            <ColumnToggle
              columns={subColumns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(s) => {
          const actions: RowAction[] = [
            { label: t("common.delete"), destructive: true, onSelect: () => setSubToDelete(s) },
          ];
          return <RowActionsMenu actions={actions} />;
        }}
      />

      <ConfirmDialog
        open={subToDelete !== null}
        onOpenChange={(open) => {
          if (!open) setSubToDelete(null);
        }}
        title={t("adminSubscriptions.deleteSubTitle")}
        body={t("adminSubscriptions.deleteSubBody")}
        confirmLabel={t("common.delete")}
        successMessage={t("feedback.deleted")}
        isPending={deleteSub.isPending}
        onConfirm={async () => {
          if (subToDelete) await deleteSub.mutateAsync(subToDelete.id);
        }}
      />

      {creatingSub ? (
        <ResourceFormDialog
          open
          onOpenChange={setCreatingSub}
          title={t("adminSubscriptions.createSubscription")}
          fields={subFields}
          initial={emptyUserSubscriptionForm()}
          buildBody={buildCreateUserSubscriptionBody}
          submit={(body) => createSub.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createSub.isPending}
        />
      ) : null}

      {bulkAssignOpen ? (
        <BulkAssignSubscriptionDialog
          isPending={batchAssign.isPending}
          onSubmit={async (items) => {
            await applyBulkAssign(items);
            setBulkAssignOpen(false);
          }}
          onClose={() => setBulkAssignOpen(false)}
        />
      ) : null}
    </>
  );
}

// Operator-friendly dialog for the verbatim port of sub2api's
// SubscriptionService.BulkAssignSubscription. One row per line in
// `user_id,plan_id` format; the server applies them per-row and returns
// per-row outcomes (created / reused / failed).
function BulkAssignSubscriptionDialog({
  isPending,
  onSubmit,
  onClose,
}: {
  isPending: boolean;
  onSubmit: (items: { user_id: string; plan_id: string }[]) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [raw, setRaw] = useState("");
  const [error, setError] = useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const items: { user_id: string; plan_id: string }[] = [];
    const lines = raw.split(/\r?\n/);
    for (const line of lines) {
      const trimmed = line.trim();
      if (trimmed === "") continue;
      const parts = trimmed.split(",").map((s) => s.trim());
      if (parts.length !== 2) {
        setError(t("adminSubscriptions.bulkAssignBadLine") as string);
        return;
      }
      items.push({ user_id: parts[0], plan_id: parts[1] });
    }
    if (items.length === 0) {
      setError(t("adminSubscriptions.bulkAssignEmpty") as string);
      return;
    }
    void onSubmit(items);
  }

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold tracking-tight">
            {t("adminSubscriptions.bulkAssignTitle")}
          </DialogTitle>
          <DialogDescription>{t("adminSubscriptions.bulkAssignBody")}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-3">
          <Label htmlFor="bulk-assign-textarea">
            {t("adminSubscriptions.bulkAssignInputLabel")}
          </Label>
          <textarea
            id="bulk-assign-textarea"
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            rows={6}
            className="w-full rounded-xl border border-srapi-border bg-srapi-card-muted/60 px-3 py-2 text-sm tabular text-srapi-text-primary placeholder:text-srapi-text-tertiary focus:border-srapi-primary/40 focus:outline-none focus:ring-2 focus:ring-srapi-primary/20"
            placeholder="123,5&#10;124,5"
          />
          {error ? (
            <p className="text-xs text-srapi-error">{error}</p>
          ) : null}
          <DialogFooter>
            <Button variant="ghost" type="button" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" loading={isPending}>
              {t("adminSubscriptions.bulkAssignSubmit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function subscriptionUsageLabels(t: ReturnType<typeof useLanguage>["t"]): SubscriptionUsageLabels {
  return {
    daily: t("adminSubscriptions.dailyUsage"),
    weekly: t("adminSubscriptions.weeklyUsage"),
    monthly: t("adminSubscriptions.monthlyUsage"),
    noQuota: t("adminSubscriptions.noCostQuota"),
  };
}
