"use client";

import { useState } from "react";
import { ShoppingCart } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, ListCount, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ListToolbar, FilterSelect } from "@/components/admin/list-toolbar";
import { useAdminList } from "@/hooks/use-admin-list";
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
import { useAdminPaymentOrders, useRefundPaymentOrder } from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { quietStatusFor } from "@/lib/status-badge";
import { formatMoney } from "@/lib/admin-format";
import { adminErrorMessage } from "@/lib/admin-api";
import {
  isRefundableOrder,
  refundFormFromOrder,
  buildRefundPaymentOrderBody,
  type RefundOrderFormState,
} from "@/lib/admin-orders-form";
import type { PaymentOrder } from "@/lib/sdk-types";

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
  "refunded",
  "expired",
  "canceled",
  "failed",
];

function OrdersContent() {
  const { t } = useLanguage();
  const list = useAdminList();
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
  const orders = useAdminPaymentOrders({
    page: list.page,
    page_size: list.pageSize,
    status: statusFilter,
  });
  const [refundTarget, setRefundTarget] = useState<PaymentOrder | null>(null);

  const columns: Column<PaymentOrder>[] = [
    {
      key: "order",
      header: t("adminOrders.order"),
      sortValue: (o) => o.order_no,
      render: (o) => (
        <span className="font-mono text-2xs text-srapi-text-secondary">{o.order_no}</span>
      ),
    },
    {
      key: "user",
      header: t("adminOrders.user"),
      hideOnMobile: true,
      render: (o) => <span className="font-mono text-2xs text-srapi-text-tertiary">{o.user_id}</span>,
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
          orders.data ? (
            <ListCount total={orders.data.pagination?.total ?? orders.data.data.length} />
          ) : undefined
        }
      />
      <AdminListView
        query={orders}
        columns={columns}
        getRowId={(o) => o.id}
        emptyIcon={ShoppingCart}
        emptyTitle={t("adminOrders.emptyTitle")}
        emptyBody={t("adminOrders.emptyBody")}
        minWidth={640}
        isFiltered={Boolean(statusFilter)}
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
          </ListToolbar>
        }
        pagination={{
          page: list.page,
          pageSize: list.pageSize,
          total: orders.data?.pagination?.total ?? orders.data?.data.length ?? 0,
          onPageChange: list.setPage,
        }}
        rowActions={(o) =>
          isRefundableOrder(o) ? (
            <RowActionsMenu
              actions={[
                {
                  label: t("adminOrders.refund"),
                  destructive: true,
                  onSelect: () => setRefundTarget(o),
                },
              ]}
            />
          ) : null
        }
      />

      {refundTarget ? (
        <RefundDialog order={refundTarget} onClose={() => setRefundTarget(null)} />
      ) : null}
    </>
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
                aria-invalid={exceeds || !enteredValid || undefined}
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
