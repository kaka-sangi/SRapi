"use client";

import { useState } from "react";
import { Gift, Copy, Check } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { SectionHero } from "@/components/visual/section-hero";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, SearchInput } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  useAdminRedeemCodes,
  useRedeemStats,
  useCreateRedeemCode,
  useBatchGenerateRedeemCodes,
  useBatchDeleteRedeemCodes,
  useBatchDisableRedeemCodes,
  useBatchEnableRedeemCodes,
  useBatchExtendRedeemCodes,
  useBatchUpdateRedeemCodes,
  useDeleteRedeemCode,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { PageQueryState } from "@/components/layout/page-query-state";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { CopyButton, writeClipboard } from "@/components/ui/copy-button";
import { StatCard } from "@/components/ui/stat-card";
import { Button } from "@/components/ui/button";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney, formatDateTime } from "@/lib/admin-format";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  REDEEM_CODE_TYPES,
  emptyRedeemCodeForm,
  emptyRedeemBatchForm,
  buildCreateRedeemCodeBody,
  buildBatchGenerateRedeemCodesBody,
  redeemDisableStateFromSelection,
  type RedeemCodeFormState,
  type RedeemBatchFormState,
} from "@/lib/admin-commerce-code-form";
import type { RedeemCode } from "@/lib/sdk-types";

export default function AdminRedeemPage() {
  return (
    <AdminShell>
      <RedeemContent />
    </AdminShell>
  );
}

const REDEEM_STATUS_VALUES: RedeemCode["status"][] = ["active", "redeemed", "disabled", "expired"];

function RedeemContent() {
  const { t } = useLanguage();
  const { toast } = useToast();
  // Translate the status filter labels (the badge column already uses
  // statusLabel) instead of showing hardcoded English.
  const statusOptions = REDEEM_STATUS_VALUES.map((value) => ({ value, label: statusLabel(t, value) }));
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-redeem", []);
  const statusFilter = (list.filters.status as RedeemCode["status"]) || undefined;
  const codeSearch = list.search || undefined;
  const codes = useAdminRedeemCodes({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
    code: codeSearch,
  });
  const stats = useRedeemStats();
  const createMut = useCreateRedeemCode();
  const disableMut = useBatchDisableRedeemCodes();
  const enableMut = useBatchEnableRedeemCodes();
  const extendMut = useBatchExtendRedeemCodes();
  const batchDeleteMut = useBatchDeleteRedeemCodes();
  const batchUpdateMut = useBatchUpdateRedeemCodes();
  const deleteMut = useDeleteRedeemCode();

  const [creating, setCreating] = useState(false);
  const [batching, setBatching] = useState(false);
  const [disableTarget, setDisableTarget] = useState<RedeemCode | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<RedeemCode | null>(null);
  const [bulkDisabling, setBulkDisabling] = useState(false);
  const [bulkDeleting, setBulkDeleting] = useState(false);
  const [bulkExtending, setBulkExtending] = useState(false);
  const [bulkEditing, setBulkEditing] = useState(false);

  /** Bulk hard-delete the selection. Unlike disable, this is irreversible —
   * the rows are gone — so it gets a destructive confirm. Failed ids
   * (missing/already-deleted) surface in the toast count. */
  async function confirmBulkDelete() {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    try {
      await batchDeleteMut.mutateAsync(ids);
      toast({ title: t("feedback.batchAllSucceeded", { count: ids.length }), tone: "success" });
      list.clearSelection();
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
      throw err;
    }
  }

  // Disable only ACTIVE selected codes — never re-disable redeemed/expired ones.
  const selectedActive = redeemDisableStateFromSelection(
    (codes.data?.data ?? []).filter((c) => list.selected.has(c.id)),
  );
  // Enable only DISABLED selected codes — the inverse filter from above. Lets
  // the toolbar disable the Enable button when nothing eligible is selected.
  const selectedDisabledIds = (codes.data?.data ?? [])
    .filter((c) => list.selected.has(c.id) && c.status === "disabled")
    .map((c) => c.id);

  // confirmBulkDisable now accepts the operator-supplied audit note from the
  // BulkDisableDialog and surfaces the per-reason breakdown returned by the
  // backend so the operator sees exactly what changed (and what didn't).
  async function confirmBulkDisable(note: string) {
    try {
      const result = await disableMut.mutateAsync({ ids: selectedActive.ids, note });
      const breakdown = result.disabled_reason_breakdown ?? {};
      const description = t("adminPromos.bulkDisableBreakdown", {
        disabled:
          (breakdown.admin_action ?? 0) + (breakdown.expired ?? 0),
        already: breakdown.already_disabled ?? 0,
        expired: breakdown.expired ?? 0,
        notFound: breakdown.not_found ?? 0,
      });
      toast({
        title: t("feedback.batchAllSucceeded", { count: result.succeeded }),
        description,
        tone: "success",
      });
      list.clearSelection();
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
      throw err;
    }
  }

  // Verbatim port of sub2api's BatchUpdate — apply the operator-picked
  // partial fields to every selected code. NotFound is idempotent server-side;
  // per-id failures (already-redeemed gate, invalid value) surface in
  // result.errors[] and are summarized in the toast.
  async function confirmBulkUpdate(payload: {
    amount?: string;
    maxRedemptions?: number;
    expiresAt?: string | null;
    note?: string;
  }) {
    const ids = [...list.selected];
    if (ids.length === 0) return;
    const items = ids.map((id) => ({
      id,
      ...(payload.amount !== undefined ? { amount: payload.amount } : {}),
      ...(payload.maxRedemptions !== undefined
        ? { max_redemptions: payload.maxRedemptions }
        : {}),
      ...(payload.expiresAt !== undefined ? { expires_at: payload.expiresAt } : {}),
      ...(payload.note !== undefined ? { note: payload.note } : {}),
    }));
    try {
      const result = await batchUpdateMut.mutateAsync(items);
      list.clearSelection();
      if (result.errors.length > 0) {
        toast({
          title: t("feedback.batchPartial", {
            succeeded: result.updated_count,
            failed: result.errors.length,
          }),
          tone: "warning",
        });
      } else {
        toast({
          title: t("feedback.batchAllSucceeded", { count: result.updated_count }),
          tone: "success",
        });
      }
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
      throw err;
    }
  }

  async function bulkEnable() {
    if (selectedDisabledIds.length === 0) return;
    try {
      await enableMut.mutateAsync(selectedDisabledIds);
      toast({
        title: t("feedback.batchAllSucceeded", { count: selectedDisabledIds.length }),
        tone: "success",
      });
      list.clearSelection();
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const fields: FieldConfig<RedeemCodeFormState>[] = [
    { name: "code", label: t("adminPromos.code") },
    {
      name: "type",
      label: t("adminCommon.type"),
      type: "select",
      options: enumOptions(REDEEM_CODE_TYPES, t),
    },
    { name: "value", label: t("adminPromos.value") },
    { name: "currency", label: t("adminCommon.currency") },
    { name: "maxRedemptions", label: t("adminPromos.maxRedemptions"), type: "number" },
    { name: "expiresAtLocal", label: t("adminCommon.expiresAt"), type: "datetime" },
  ];

  const columns: Column<RedeemCode>[] = [
    {
      key: "code",
      header: t("adminPromos.code"),
      pinned: true,
      sortValue: (c) => c.code,
      render: (c) => (
        <span className="flex items-center gap-1.5">
          <span className="text-sm font-semibold tabular text-srapi-text-primary">{c.code}</span>
          <CopyButton value={c.code} size="inline" />
        </span>
      ),
    },
    {
      key: "value",
      header: t("adminPromos.value"),
      render: (c) => {
        const numeric = Number(c.value);
        const decimals = String(c.value).split(".")[1]?.length ?? 0;
        return (
          <DataTooltip
            title={t("adminPromos.value")}
            primary={formatMoney(c.value, c.currency)}
            rows={[
              { label: t("adminCommon.currency"), value: (c.currency || "USD").toUpperCase() },
              { label: "Precision", value: `${decimals} dp` },
              ...(c.currency && c.currency.toUpperCase() !== "USD" && Number.isFinite(numeric)
                ? [
                    {
                      label: "≈ USD",
                      value: (() => {
                        const fx: Record<string, number> = {
                          CNY: 0.14,
                          EUR: 1.08,
                          JPY: 0.0066,
                          GBP: 1.27,
                          HKD: 0.13,
                          TWD: 0.031,
                          KRW: 0.00075,
                        };
                        const rate = fx[c.currency.toUpperCase()];
                        return rate ? formatMoney(numeric * rate, "USD") : "—";
                      })(),
                      tone: "muted" as const,
                    },
                  ]
                : []),
              { label: "Type", value: c.type, tone: "muted" as const },
            ]}
          >
            <span className="text-sm font-medium tabular text-srapi-text-primary">
              {formatMoney(c.value, c.currency)}
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "uses",
      header: t("adminPromos.uses"),
      align: "right",
      hideOnMobile: true,
      render: (c) => {
        const used = c.redeemed_count ?? 0;
        const max = c.max_redemptions ?? 0;
        return (
          <DataTooltip
            title={t("adminPromos.uses")}
            primary={`${used}${max ? ` / ${max}` : ""}`}
            rows={[
              { label: "Redeemed", value: String(used) },
              { label: "Cap", value: max > 0 ? String(max) : "Unlimited" },
              ...(max > 0
                ? [
                    {
                      label: "Remaining",
                      value: String(Math.max(0, max - used)),
                      tone: "muted" as const,
                    },
                  ]
                : []),
            ]}
          >
            <span className="text-xs tabular text-srapi-text-tertiary">
              {used}
              {max ? ` / ${max}` : ""}
            </span>
          </DataTooltip>
        );
      },
    },
    {
      key: "expires",
      header: t("adminCommon.expiresAt"),
      hideOnMobile: true,
      // sortValue keeps unset expiries at the bottom on ascending sort (empty
      // string sorts before all timestamps), which matches the typical "when
      // does this expire?" mental model.
      sortValue: (c) => c.expires_at ?? "",
      render: (c) => (
        <span className="text-[12px] tabular text-srapi-text-tertiary">
          {c.expires_at ? formatDateTime(c.expires_at) : "—"}
        </span>
      ),
    },
    {
      key: "status",
      header: t("common.active"),
      render: (c) => <QuietBadge status={quietStatusFor(c.status)} label={statusLabel(t, c.status)} />,
    },
  ];

  return (
    <>
      <SectionHero
        eyebrow={t("hero.eyebrowCommerceRedeem")}
        title={t("adminPromos.redeemTitle")}
        description={t("adminPromos.redeemSubtitle")}
        metrics={
          stats.data
            ? [
                { label: t("adminPromos.statsActive"), value: String(stats.data.active) },
                { label: t("adminPromos.statsRedeemed"), value: String(stats.data.redeemed) },
              ]
            : undefined
        }
        actions={
          <div className="flex items-center gap-3">
            {codes.data ? (
              <ListCount total={codes.data.pagination?.total ?? codes.data.data.length} />
            ) : null}
            <ColumnToggle
              columns={columns.filter((c) => !c.pinned).map((c) => ({ key: c.key, label: c.header }))}
              visibility={colVis}
            />
            <Button variant="outline" size="sm" onClick={() => setBatching(true)}>
              {t("adminPromos.batchGenerate")}
            </Button>
            <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
              ＋ {t("adminPromos.createRedeem")}
            </Button>
          </div>
        }
      />
      <PageQueryState query={stats} skeleton={null}>
        {(s) => (
          <div className="mb-6 grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
            <StatCard label={t("adminPromos.statsTotal")} value={String(s.total)} />
            <StatCard label={t("adminPromos.statsActive")} value={String(s.active)} />
            <StatCard label={t("adminPromos.statsRedeemed")} value={String(s.redeemed)} />
            <StatCard label={t("adminPromos.statsDisabled")} value={String(s.disabled)} />
            <StatCard label={t("adminPromos.statsExpired")} value={String(s.expired)} />
          </div>
        )}
      </PageQueryState>
      <AdminListView
        query={codes}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(c) => c.id}
        emptyIcon={Gift}
        emptyTitle={t("adminPromos.emptyRedeem")}
        emptyBody={t("adminPromos.emptyRedeemBody")}
        emptyAction={
          <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
            ＋ {t("adminPromos.createRedeem")}
          </Button>
        }
        minWidth={560}
        dimRow={(c) => c.status !== "active"}
        isFiltered={Boolean(statusFilter || codeSearch)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        toolbar={
          <ListToolbar>
            <SearchInput
              value={list.searchInput}
              onChange={list.setSearchInput}
              placeholder={t("adminPromos.searchPlaceholder")}
            />
            <SegmentedControl<string>
              value={(statusFilter as string) ?? "all"}
              onChange={(v) => list.setFilter("status", v === "all" ? "" : v)}
              ariaLabel={t("adminCommon.allStatuses")}
              size="sm"
              options={[
                { value: "all", label: t("adminCommon.allStatuses") },
                ...statusOptions,
              ]}
            />
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: codes.data?.pagination?.total ?? codes.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        selection={{
          selected: list.selected,
          onToggle: list.toggle,
          onTogglePage: list.togglePage,
          bulkActions: (
            <>
              <Button
                variant="outline"
                size="sm"
                loading={disableMut.isPending}
                disabled={selectedActive.ids.length === 0}
                onClick={() => setBulkDisabling(true)}
              >
                {t("adminPromos.disableSelected")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                loading={enableMut.isPending}
                disabled={selectedDisabledIds.length === 0}
                onClick={() => void bulkEnable()}
              >
                {t("adminPromos.enableSelected")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                loading={extendMut.isPending}
                disabled={list.selected.size === 0}
                onClick={() => setBulkExtending(true)}
              >
                {t("adminPromos.extendSelected")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                loading={batchUpdateMut.isPending}
                disabled={list.selected.size === 0}
                onClick={() => setBulkEditing(true)}
              >
                {t("adminPromos.bulkEditSelected")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                loading={batchDeleteMut.isPending}
                onClick={() => setBulkDeleting(true)}
              >
                {t("adminPromos.deleteSelected")}
              </Button>
            </>
          ),
        }}
        rowActions={(c) => (
          <RowActionsMenu
            actions={[
              ...(c.status === "active"
                ? [
                    {
                      label: t("adminPromos.disable"),
                      onSelect: () => setDisableTarget(c),
                    },
                  ]
                : []),
              { label: t("common.delete"), destructive: true, onSelect: () => setDeleteTarget(c) },
            ]}
          />
        )}
      />

      {creating ? (
        <ResourceFormDialog
          open
          onOpenChange={setCreating}
          title={t("adminPromos.createRedeem")}
          fields={fields}
          initial={emptyRedeemCodeForm()}
          buildBody={buildCreateRedeemCodeBody}
          submit={(body) => createMut.mutateAsync(body)}
          successMessage={t("feedback.created")}
          isPending={createMut.isPending}
        />
      ) : null}

      {batching ? <RedeemBatchDialog onClose={() => setBatching(false)} /> : null}

      {disableTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDisableTarget(null);
          }}
          title={t("adminPromos.disable")}
          body={t("feedback.confirmDeleteBody")}
          confirmLabel={t("adminPromos.disable")}
          onConfirm={() => disableMut.mutateAsync({ ids: [disableTarget.id] })}
          successMessage={t("feedback.updated")}
          isPending={disableMut.isPending}
        />
      ) : null}

      {bulkDisabling ? (
        <BulkDisableDialog
          count={selectedActive.ids.length}
          isPending={disableMut.isPending}
          onSubmit={async (note) => {
            try {
              await confirmBulkDisable(note);
              setBulkDisabling(false);
            } catch {
              // confirmBulkDisable already surfaced the error toast; keep the
              // dialog open so the operator can retry or close it themselves.
            }
          }}
          onClose={() => setBulkDisabling(false)}
        />
      ) : null}

      {bulkDeleting ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setBulkDeleting(false);
          }}
          tone="danger"
          title={t("adminPromos.deleteSelectedTitle", { count: list.selected.size })}
          body={t("adminPromos.deleteSelectedBody")}
          confirmLabel={t("adminPromos.deleteSelected")}
          onConfirm={confirmBulkDelete}
          isPending={batchDeleteMut.isPending}
        />
      ) : null}

      {bulkExtending ? (
        <RedeemExtendDialog
          count={list.selected.size}
          isPending={extendMut.isPending}
          onSubmit={async (isoExpiresAt) => {
            try {
              await extendMut.mutateAsync({
                ids: [...list.selected],
                expiresAt: isoExpiresAt,
              });
              toast({
                title: t("feedback.batchAllSucceeded", { count: list.selected.size }),
                tone: "success",
              });
              list.clearSelection();
              setBulkExtending(false);
            } catch (err) {
              toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
            }
          }}
          onClose={() => setBulkExtending(false)}
        />
      ) : null}

      {deleteTarget ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("adminPromos.deleteRedeemTitle")}
          body={t("adminPromos.deleteRedeemBody", { code: deleteTarget.code })}
          confirmLabel={t("common.delete")}
          successMessage={t("feedback.deleted")}
          isPending={deleteMut.isPending}
          onConfirm={() => deleteMut.mutateAsync(deleteTarget.id)}
        />
      ) : null}

      {bulkEditing ? (
        <BulkEditDialog
          count={list.selected.size}
          isPending={batchUpdateMut.isPending}
          onSubmit={async (payload) => {
            try {
              await confirmBulkUpdate(payload);
              setBulkEditing(false);
            } catch {
              // toast already shown; keep dialog open so operator can retry
            }
          }}
          onClose={() => setBulkEditing(false)}
        />
      ) : null}
    </>
  );
}

// Bulk-edit dialog: lets the operator pick which fields to change across the
// selection (any subset of amount / max_redemptions / expires_at / note).
// Mirrors the per-row partial-update shape of the batch-update endpoint.
function BulkEditDialog({
  count,
  isPending,
  onSubmit,
  onClose,
}: {
  count: number;
  isPending: boolean;
  onSubmit: (payload: {
    amount?: string;
    maxRedemptions?: number;
    expiresAt?: string | null;
    note?: string;
  }) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [editAmount, setEditAmount] = useState(false);
  const [amount, setAmount] = useState("");
  const [editMax, setEditMax] = useState(false);
  const [maxRedemptions, setMaxRedemptions] = useState("1");
  const [editExpires, setEditExpires] = useState(false);
  const [expiresAt, setExpiresAt] = useState("");
  const [clearExpires, setClearExpires] = useState(false);
  const [editNote, setEditNote] = useState(false);
  const [note, setNote] = useState("");
  const [error, setError] = useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (!editAmount && !editMax && !editExpires && !editNote) {
      setError(t("adminPromos.bulkEditNothingSelected"));
      return;
    }
    const payload: {
      amount?: string;
      maxRedemptions?: number;
      expiresAt?: string | null;
      note?: string;
    } = {};
    if (editAmount) {
      const n = Number.parseFloat(amount.trim());
      if (!Number.isFinite(n) || n <= 0) {
        setError(t("adminPromos.bulkEditInvalidAmount"));
        return;
      }
      payload.amount = amount.trim();
    }
    if (editMax) {
      const n = Number.parseInt(maxRedemptions.trim(), 10);
      if (!Number.isFinite(n) || n <= 0) {
        setError(t("adminPromos.bulkEditInvalidMax"));
        return;
      }
      payload.maxRedemptions = n;
    }
    if (editExpires) {
      if (clearExpires) {
        payload.expiresAt = null;
      } else {
        const trimmed = expiresAt.trim();
        if (!trimmed) {
          setError(t("adminPromos.extendRequired"));
          return;
        }
        const d = new Date(trimmed);
        if (Number.isNaN(d.getTime())) {
          setError(t("adminPromos.extendRequired"));
          return;
        }
        payload.expiresAt = d.toISOString();
      }
    }
    if (editNote) {
      payload.note = note;
    }
    void onSubmit(payload);
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="text-lg font-semibold tracking-tight">
              {t("adminPromos.bulkEditTitle", { count })}
            </DialogTitle>
            <DialogDescription>{t("adminPromos.bulkEditBody")}</DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-4">
            <BulkEditField
              checked={editAmount}
              onCheck={setEditAmount}
              label={t("adminPromos.amount")}
            >
              <Input
                inputMode="decimal"
                value={amount}
                disabled={!editAmount || isPending}
                onChange={(e) => setAmount(e.target.value)}
              />
            </BulkEditField>
            <BulkEditField
              checked={editMax}
              onCheck={setEditMax}
              label={t("adminPromos.maxRedemptions")}
            >
              <Input
                type="number"
                inputMode="numeric"
                min={1}
                value={maxRedemptions}
                disabled={!editMax || isPending}
                onChange={(e) => setMaxRedemptions(e.target.value)}
              />
            </BulkEditField>
            <BulkEditField
              checked={editExpires}
              onCheck={setEditExpires}
              label={t("adminCommon.expiresAt")}
            >
              <Input
                type="datetime-local"
                value={expiresAt}
                disabled={!editExpires || isPending || clearExpires}
                onChange={(e) => setExpiresAt(e.target.value)}
              />
              <label className="mt-1 flex items-center gap-2 text-xs">
                <input
                  type="checkbox"
                  disabled={!editExpires || isPending}
                  checked={clearExpires}
                  onChange={(e) => setClearExpires(e.target.checked)}
                />
                {t("adminPromos.bulkEditClearExpiry")}
              </label>
            </BulkEditField>
            <BulkEditField
              checked={editNote}
              onCheck={setEditNote}
              label={t("adminPromos.bulkDisableNoteLabel")}
            >
              <Textarea
                rows={2}
                value={note}
                disabled={!editNote || isPending}
                onChange={(e) => setNote(e.target.value)}
              />
            </BulkEditField>
            {error ? (
              <p role="alert" className="text-xs text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" disabled={isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={isPending}>
              {t("common.apply")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function BulkEditField({
  checked,
  onCheck,
  label,
  children,
}: {
  checked: boolean;
  onCheck: (v: boolean) => void;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={checked}
          onChange={(e) => onCheck(e.target.checked)}
        />
        <span>{label}</span>
      </label>
      <div className={checked ? "" : "opacity-50"}>{children}</div>
    </div>
  );
}

/** Batch-generate codes, then reveal the generated set once for copy/distribution. */
function RedeemBatchDialog({ onClose }: { onClose: () => void }) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const batchMut = useBatchGenerateRedeemCodes();
  const [form, setForm] = useState<RedeemBatchFormState>(emptyRedeemBatchForm());
  const [error, setError] = useState<string | null>(null);
  const [generated, setGenerated] = useState<RedeemCode[] | null>(null);
  const [copied, setCopied] = useState(false);

  function setField<K extends keyof RedeemBatchFormState>(key: K, value: RedeemBatchFormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    let body;
    try {
      body = buildBatchGenerateRedeemCodesBody(form);
    } catch (err) {
      setError(adminErrorMessage(err));
      return;
    }
    try {
      const result = await batchMut.mutateAsync(body);
      setGenerated(result);
      toast({ title: t("feedback.created"), tone: "success" });
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  async function copyAll() {
    if (!generated) return;
    const ok = await writeClipboard(generated.map((c) => c.code).join("\n"));
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        {generated ? (
          <>
            <DialogHeader>
              <DialogTitle className="text-lg font-semibold tracking-tight">
                {t("adminPromos.generatedTitle")}
              </DialogTitle>
              <DialogDescription>{t("adminPromos.generatedBody")}</DialogDescription>
            </DialogHeader>
            <div className="mt-2 max-h-72 overflow-y-auto rounded-xl border border-srapi-border bg-srapi-card-muted/60 p-4">
              <ul className="space-y-1 text-xs tabular text-srapi-text-primary">
                {generated.map((c) => (
                  <li key={c.id}>{c.code}</li>
                ))}
              </ul>
            </div>
            <DialogFooter className="mt-4">
              <Button variant="outline" onClick={copyAll}>
                {copied ? <Check className="size-4 text-srapi-success" /> : <Copy className="size-4" />}
                {t("common.copy")}
              </Button>
              <Button variant="primary" onClick={onClose}>
                {t("common.close")}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <form onSubmit={submit}>
            <DialogHeader>
              <DialogTitle className="text-lg font-semibold tracking-tight">
                {t("adminPromos.batchGenerate")}
              </DialogTitle>
            </DialogHeader>
            <div className="mt-4 space-y-4">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label htmlFor="batch-prefix">{t("adminCommon.prefix")}</Label>
                  <Input
                    id="batch-prefix"
                    value={form.prefix}
                    onChange={(e) => setField("prefix", e.target.value)}
                  />
                </div>
                <div>
                  <Label htmlFor="batch-count">{t("adminCommon.quantity")}</Label>
                  <Input
                    id="batch-count"
                    type="number"
                    value={form.count}
                    onChange={(e) => setField("count", e.target.value)}
                  />
                </div>
              </div>
              <div>
                <Label htmlFor="batch-type">{t("adminCommon.type")}</Label>
                <Select value={form.type} onValueChange={(v) => setField("type", v as RedeemBatchFormState["type"])}>
                  <SelectTrigger id="batch-type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {REDEEM_CODE_TYPES.map((v) => (
                      <SelectItem key={v} value={v}>
                        {v}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label htmlFor="batch-value">{t("adminPromos.value")}</Label>
                  <Input
                    id="batch-value"
                    value={form.value}
                    onChange={(e) => setField("value", e.target.value)}
                  />
                </div>
                <div>
                  <Label htmlFor="batch-currency">{t("adminCommon.currency")}</Label>
                  <Input
                    id="batch-currency"
                    value={form.currency}
                    onChange={(e) => setField("currency", e.target.value)}
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label htmlFor="batch-max">{t("adminPromos.maxRedemptions")}</Label>
                  <Input
                    id="batch-max"
                    type="number"
                    value={form.maxRedemptions}
                    onChange={(e) => setField("maxRedemptions", e.target.value)}
                  />
                </div>
                <div>
                  <Label htmlFor="batch-expires">{t("adminCommon.expiresAt")}</Label>
                  <Input
                    id="batch-expires"
                    type="datetime-local"
                    value={form.expiresAtLocal}
                    onChange={(e) => setField("expiresAtLocal", e.target.value)}
                  />
                </div>
              </div>
              {error ? (
                <p role="alert" className="text-sm text-srapi-error">
                  {error}
                </p>
              ) : null}
            </div>
            <DialogFooter className="mt-6">
              <Button type="button" variant="ghost" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" variant="primary" loading={batchMut.isPending}>
                {t("adminPromos.batchGenerate")}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

// Bulk-disable confirmation with an optional free-text audit note. Mirrors
// RedeemExtendDialog's shape — count blurb up top, single editable field,
// confirm/cancel. The parent owns the mutation; this dialog just collects the
// note and validates it locally (≤500 chars) before submitting.
function BulkDisableDialog({
  count,
  isPending,
  onSubmit,
  onClose,
}: {
  count: number;
  isPending: boolean;
  onSubmit: (note: string) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [note, setNote] = useState("");
  const [error, setError] = useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const trimmed = note.trim();
    if (trimmed.length > 500) {
      setError(t("adminPromos.bulkDisableNoteTooLong"));
      return;
    }
    void onSubmit(trimmed);
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="text-lg font-semibold tracking-tight">
              {t("adminPromos.disableSelectedTitle", { count })}
            </DialogTitle>
            <DialogDescription>{t("feedback.confirmDeleteBody")}</DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-3">
            <div>
              <Label htmlFor="bulk-disable-note">{t("adminPromos.bulkDisableNoteLabel")}</Label>
              <Textarea
                id="bulk-disable-note"
                rows={3}
                maxLength={500}
                placeholder={t("adminPromos.bulkDisableNotePlaceholder")}
                value={note}
                disabled={isPending}
                aria-invalid={Boolean(error)}
                onChange={(e) => setNote(e.target.value)}
              />
            </div>
            {error ? (
              <p role="alert" className="text-xs text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" disabled={isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={isPending}>
              {t("adminPromos.disableSelected")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// Small focused dialog: one datetime-local input + selected-count blurb.
// Submits the ISO timestamp to the parent; the parent owns the mutation.
function RedeemExtendDialog({
  count,
  isPending,
  onSubmit,
  onClose,
}: {
  count: number;
  isPending: boolean;
  onSubmit: (isoExpiresAt: string) => void | Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);

  function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const trimmed = value.trim();
    if (!trimmed) {
      setError(t("adminPromos.extendRequired"));
      return;
    }
    // datetime-local is `yyyy-MM-ddTHH:mm` in the user's local zone — round-trip
    // through Date so the backend receives a real ISO timestamp.
    const d = new Date(trimmed);
    if (Number.isNaN(d.getTime())) {
      setError(t("adminPromos.extendRequired"));
      return;
    }
    void onSubmit(d.toISOString());
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="text-lg font-semibold tracking-tight">
              {t("adminPromos.extendSelectedTitle", { count })}
            </DialogTitle>
            <DialogDescription>{t("adminPromos.extendSelectedBody")}</DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-3">
            <div>
              <Label htmlFor="extend-expires">{t("adminCommon.expiresAt")}</Label>
              <Input
                id="extend-expires"
                type="datetime-local"
                value={value}
                disabled={isPending}
                aria-invalid={Boolean(error)}
                onChange={(e) => setValue(e.target.value)}
              />
            </div>
            {error ? (
              <p role="alert" className="text-xs text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" disabled={isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" loading={isPending}>
              {t("adminPromos.extendSelected")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
