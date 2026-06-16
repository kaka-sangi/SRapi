"use client";

import { useState } from "react";
import { Gift, Copy, Check } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect, SearchInput } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  ResourceFormDialog,
  enumOptions,
  type FieldConfig,
} from "@/components/admin/resource-form-dialog";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
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
  useDeleteRedeemCode,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { PageQueryState } from "@/components/layout/page-query-state";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { CopyButton } from "@/components/ui/copy-button";
import { StatCard } from "@/components/ui/stat-card";
import { Button } from "@/components/ui/button";
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
  const deleteMut = useDeleteRedeemCode();

  const [creating, setCreating] = useState(false);
  const [batching, setBatching] = useState(false);
  const [disableTarget, setDisableTarget] = useState<RedeemCode | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<RedeemCode | null>(null);
  const [bulkDisabling, setBulkDisabling] = useState(false);
  const [bulkDeleting, setBulkDeleting] = useState(false);
  const [bulkExtending, setBulkExtending] = useState(false);

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
      toast({ title: t("feedback.failed"), tone: "error" });
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

  async function confirmBulkDisable() {
    try {
      await disableMut.mutateAsync(selectedActive.ids);
      toast({ title: t("feedback.batchAllSucceeded", { count: selectedActive.ids.length }), tone: "success" });
      list.clearSelection();
    } catch (err) {
      toast({ title: t("feedback.failed"), tone: "error" });
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
      options: enumOptions(REDEEM_CODE_TYPES),
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
          <span className="font-mono text-srapi-text-primary">{c.code}</span>
          <CopyButton value={c.code} size="inline" />
        </span>
      ),
    },
    {
      key: "value",
      header: t("adminPromos.value"),
      render: (c) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(c.value, c.currency)}
        </span>
      ),
    },
    {
      key: "uses",
      header: t("adminPromos.uses"),
      align: "right",
      hideOnMobile: true,
      render: (c) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {c.redeemed_count ?? 0}
          {c.max_redemptions ? ` / ${c.max_redemptions}` : ""}
        </span>
      ),
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
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
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
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminPromos.redeemTitle")}
        description={t("adminPromos.redeemSubtitle")}
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
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={statusOptions}
              allLabel={t("adminCommon.allStatuses")}
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
          confirmPhrase={disableTarget.code}
          onConfirm={() => disableMut.mutateAsync([disableTarget.id])}
          successMessage={t("feedback.updated")}
          isPending={disableMut.isPending}
        />
      ) : null}

      {bulkDisabling ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setBulkDisabling(false);
          }}
          title={t("adminPromos.disableSelectedTitle", { count: selectedActive.ids.length })}
          body={t("feedback.confirmDeleteBody")}
          confirmLabel={t("adminPromos.disableSelected")}
          onConfirm={confirmBulkDisable}
          isPending={disableMut.isPending}
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
    </>
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
    await navigator.clipboard.writeText(generated.map((c) => c.code).join("\n"));
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        {generated ? (
          <>
            <DialogHeader>
              <DialogTitle>{t("adminPromos.generatedTitle")}</DialogTitle>
              <DialogDescription>{t("adminPromos.generatedBody")}</DialogDescription>
            </DialogHeader>
            <div className="mt-2 max-h-72 overflow-y-auto rounded-xl border border-srapi-border bg-srapi-card-muted p-3">
              <ul className="space-y-1 font-mono text-xs text-srapi-text-primary">
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
              <DialogTitle>{t("adminPromos.batchGenerate")}</DialogTitle>
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
            <DialogTitle>{t("adminPromos.extendSelectedTitle", { count })}</DialogTitle>
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
              <p role="alert" className="text-2xs text-srapi-error">
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
