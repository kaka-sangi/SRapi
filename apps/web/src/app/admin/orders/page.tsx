"use client";

import { useState } from "react";
import { ShoppingCart } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AutoRefreshControl } from "@/components/ui/auto-refresh";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu, type RowAction } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
import { useColumnVisibility } from "@/hooks/use-column-visibility";
import { useClientPagedList } from "@/hooks/use-client-list";
import { ColumnToggle } from "@/components/ui/column-toggle";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  useAdminPaymentOrderAuditLogs,
  useAdminPaymentOrders,
  useRefundPaymentOrder,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { quietStatusFor } from "@/lib/status-badge";
import { formatDateTime, formatMoney, safeJson } from "@/lib/admin-format";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  isRefundableOrder,
  refundFormFromOrder,
  buildRefundPaymentOrderBody,
  type RefundOrderFormState,
} from "@/lib/admin-orders-form";
import type { PaymentAuditLog, PaymentOrder } from "@/lib/sdk-types";
import {
  LOG_WINDOW_PRESETS,
  LOG_WINDOW_ALL_LABEL_KEY,
  logWindowSince,
} from "@/lib/log-window-filter";
import { PaymentDashboardPanel } from "./_panels/payment-dashboard-panel";

export default function AdminOrdersPage() {
  return (
    <AdminShell>
      <OrdersContent />
    </AdminShell>
  );
}

const ORDER_STATUSES: PaymentOrder["status"][] = [
  "pending",
  "paid",
  "fulfilled",
  "partially_refunded",
  "refunding",
  "refunded",
  "refund_failed",
  "expired",
  "canceled",
  "failed",
];

function orderMatch(
  order: PaymentOrder,
  term: string,
  filters: Record<string, string>,
): boolean {
  if (filters.status && order.status !== filters.status) return false;
  if (filters.window) {
    const since = logWindowSince(filters.window);
    if (since && order.created_at && new Date(order.created_at) < since) return false;
  }
  if (!term) return true;
  return [order.order_no, String(order.user_id), order.product_type, order.status]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

const orderCompare = (a: PaymentOrder, b: PaymentOrder) =>
  (b.created_at ?? "").localeCompare(a.created_at ?? "");

function OrdersContent() {
  const { t } = useLanguage();
  const list = useAdminList();
  const colVis = useColumnVisibility("admin-orders", ["created_at"]);
  const userLookup = useUserEmailLookup();
  // translate() returns the key path on a miss, so fall back to a humanized
  // token rather than leaking the dotted key into the UI.
  const labelOr = (key: string, fallback: string) => {
    const s = t(key);
    return s === key ? fallback : s;
  };
  const statusLabel = (s: PaymentOrder["status"]) =>
    labelOr(`adminOrders.statuses.${s}`, s.replace(/_/g, " "));
  const productLabel = (p: string) => labelOr(`adminOrders.productTypes.${p}`, p.replace(/_/g, " "));
  const statusOptions = ORDER_STATUSES.map((v) => ({ value: v, label: statusLabel(v) }));
  const statusFilter = (list.filters.status as PaymentOrder["status"]) || undefined;
  const all = useAdminPaymentOrders();
  const { query: orders, total } = useClientPagedList(all, list, {
    match: orderMatch,
    compare: orderCompare,
  });
  const [refundTarget, setRefundTarget] = useState<PaymentOrder | null>(null);
  const [auditTarget, setAuditTarget] = useState<PaymentOrder | null>(null);

  const columns: Column<PaymentOrder>[] = [
    {
      key: "order",
      header: t("adminOrders.order"),
      pinned: true,
      sortValue: (o) => o.order_no,
      render: (o) => (
        <span className="font-mono text-2xs text-srapi-text-secondary">{o.order_no}</span>
      ),
    },
    {
      key: "user",
      header: t("adminOrders.user"),
      hideOnMobile: true,
      render: (o) => <span className="text-srapi-text-secondary">{userLookup.get(o.user_id)}</span>,
    },
    {
      key: "product",
      header: t("adminOrders.product"),
      hideOnMobile: true,
      render: (o) => (
        <span className="text-srapi-text-secondary">{productLabel(o.product_type)}</span>
      ),
    },
    {
      key: "amount",
      header: t("adminOrders.amount"),
      align: "right",
      sortValue: (o) => Number(o.amount),
      render: (o) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(o.amount, o.currency)}
        </span>
      ),
    },
    {
      key: "status",
      header: t("common.active"),
      render: (o) => <QuietBadge status={quietStatusFor(o.status)} label={statusLabel(o.status)} />,
    },
  ];

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminOrders.title")}
        description={t("adminOrders.subtitle")}
        actions={
          <div className="flex items-center gap-3">
            {all.data ? <ListCount total={total} /> : null}
            <AutoRefreshControl
              onRefresh={() => void all.refetch()}
              isRefreshing={all.isFetching}
              storageKey="srapi.autorefresh.admin-orders"
            />
          </div>
        }
      />
      <PaymentDashboardPanel />
      <AdminListView
        query={orders}
        columns={columns}
        columnVisibility={colVis}
        getRowId={(o) => o.id}
        emptyIcon={ShoppingCart}
        emptyTitle={t("adminOrders.emptyTitle")}
        emptyBody={t("adminOrders.emptyBody")}
        minWidth={640}
        isFiltered={Boolean(statusFilter || list.filters.window)}
        onClearFilters={list.clearFilters}
        sort={list.sort}
        onSort={list.toggleSort}
        toolbar={
          <ListToolbar>
            <FilterSelect
              value={statusFilter}
              onChange={(v) => list.setFilter("status", v)}
              options={statusOptions}
              allLabel={t("adminCommon.allStatuses")}
            />
            <FilterSelect
              value={list.filters.window}
              onChange={(v) => list.setFilter("window", v)}
              options={LOG_WINDOW_PRESETS.map((p) => ({ value: p.value, label: t(p.labelKey) }))}
              allLabel={t(LOG_WINDOW_ALL_LABEL_KEY)}
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
          total,
          onPageChange: list.setPage,
        }}
        rowActions={(o) => {
          const actions: RowAction[] = [
            {
              label: t("adminOrders.audit.action"),
              onSelect: () => setAuditTarget(o),
            },
          ];
          if (isRefundableOrder(o)) {
            actions.push({
              label: t("adminOrders.refund"),
              destructive: true,
              onSelect: () => setRefundTarget(o),
            });
          }
          return <RowActionsMenu actions={actions} />;
        }}
      />

      {refundTarget ? (
        <RefundDialog order={refundTarget} onClose={() => setRefundTarget(null)} />
      ) : null}
      {auditTarget ? (
        <AuditDialog order={auditTarget} onClose={() => setAuditTarget(null)} />
      ) : null}
    </>
  );
}

function AuditDialog({ order, onClose }: { order: PaymentOrder; onClose: () => void }) {
  const { t } = useLanguage();
  const logs = useAdminPaymentOrderAuditLogs(order.id);
  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("adminOrders.audit.title")}</DialogTitle>
          <DialogDescription>{order.order_no}</DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] space-y-3 overflow-y-auto pr-1">
          {logs.isLoading ? (
            <p className="text-sm text-srapi-text-tertiary">{t("common.loading")}</p>
          ) : logs.isError ? (
            <p role="alert" className="text-sm text-srapi-error">{t("adminOrders.audit.loadFailed")}</p>
          ) : (logs.data?.data.length ?? 0) === 0 ? (
            <p className="text-sm text-srapi-text-tertiary">{t("adminOrders.audit.empty")}</p>
          ) : (
            logs.data!.data.map((log) => <AuditLogItem key={log.id} log={log} />)
          )}
        </div>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("common.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function AuditLogItem({ log }: { log: PaymentAuditLog }) {
  const { t } = useLanguage();
  return (
    <div className="rounded-lg border border-srapi-border bg-srapi-card-muted p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-mono text-xs text-srapi-text-primary">{log.event_type}</span>
        <QuietBadge
          status={log.signature_valid ? "active" : "error"}
          label={log.signature_valid ? t("adminOrders.audit.signatureValid") : t("adminOrders.audit.signatureInvalid")}
        />
      </div>
      <dl className="mt-2 grid gap-1 text-2xs text-srapi-text-tertiary sm:grid-cols-2">
        <div>
          <dt>{t("adminOutbox.idempotency")}</dt>
          <dd className="break-all font-mono text-srapi-text-secondary">{log.idempotency_key}</dd>
        </div>
        <div>
          <dt>{t("adminCommon.created")}</dt>
          <dd className="font-mono text-srapi-text-secondary">{formatDateTime(log.created_at)}</dd>
        </div>
      </dl>
      <pre className="mt-2 max-h-48 overflow-auto rounded-md border border-srapi-border bg-srapi-card px-3 py-2 text-2xs text-srapi-text-secondary">
        {safeJson(log.payload)}
      </pre>
    </div>
  );
}

/**
 * Refund a paid / partially_refunded order. Surfaces the refundable balance so
 * the operator can't over-refund: `PaymentOrder` has no refunded-amount field,
 * so the remaining refundable amount is the order `amount` (full balance).
 * Reuses the existing refund mutation and body builder to preserve the flow.
 */
function RefundDialog({ order, onClose }: { order: PaymentOrder; onClose: () => void }) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const refundMut = useRefundPaymentOrder();
  const [form, setForm] = useState<RefundOrderFormState>(() => refundFormFromOrder(order));
  const [error, setError] = useState<string | null>(null);

  // No refunded-amount field on PaymentOrder, so the refundable remaining is the
  // full order amount. A blank amount means a full refund; any entered amount
  // must be > 0 and within the refundable balance.
  const refundable = Number(order.amount);
  const entered = form.amount.trim() === "" ? null : Number(form.amount);
  const enteredValid = entered === null || (Number.isFinite(entered) && entered > 0);
  const exceeds = entered !== null && Number.isFinite(entered) && entered > refundable;

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (exceeds) {
      setError(t("adminOrders.refundExceeds"));
      return;
    }
    let body;
    try {
      const { id: _id, ...rest } = buildRefundPaymentOrderBody(form);
      void _id;
      body = rest;
    } catch (err) {
      setError(adminErrorMessage(err));
      return;
    }
    try {
      await refundMut.mutateAsync({ id: order.id, body });
      toast({ title: t("feedback.updated"), tone: "success" });
      onClose();
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t("adminOrders.refundTitle")}</DialogTitle>
            <DialogDescription>{order.order_no}</DialogDescription>
          </DialogHeader>
          <div className="mt-4 space-y-4">
            <dl className="rounded-xl border border-srapi-border bg-srapi-card-muted px-3.5 py-3 text-sm">
              <div className="flex items-center justify-between">
                <dt className="text-srapi-text-tertiary">{t("adminOrders.amount")}</dt>
                <dd className="font-mono text-srapi-text-secondary tabular">
                  {formatMoney(order.amount, order.currency)}
                </dd>
              </div>
              <div className="mt-1.5 flex items-center justify-between">
                <dt className="text-srapi-text-tertiary">{t("adminOrders.refundRemaining")}</dt>
                <dd className="font-mono text-srapi-text-primary tabular">
                  {formatMoney(order.amount, order.currency)}
                </dd>
              </div>
            </dl>
            <div>
              <Label htmlFor="refund-amount">{t("adminOrders.amount")}</Label>
              <Input
                id="refund-amount"
                inputMode="decimal"
                placeholder={t("adminOrders.refundFull")}
                value={form.amount}
                disabled={refundMut.isPending}
                aria-invalid={exceeds || !enteredValid}
                onChange={(e) => setForm((prev) => ({ ...prev, amount: e.target.value }))}
              />
              {exceeds ? (
                <p role="alert" className="mt-1 text-2xs text-srapi-error">
                  {t("adminOrders.refundExceeds")}
                </p>
              ) : null}
            </div>
            <div>
              <Label htmlFor="refund-reason">{t("adminOrders.refundReason")}</Label>
              <Input
                id="refund-reason"
                value={form.reason}
                disabled={refundMut.isPending}
                onChange={(e) => setForm((prev) => ({ ...prev, reason: e.target.value }))}
              />
            </div>
            {error ? (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter className="mt-6">
            <Button type="button" variant="ghost" disabled={refundMut.isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              variant="primary"
              loading={refundMut.isPending}
              disabled={exceeds || !enteredValid}
            >
              {t("adminOrders.refund")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
