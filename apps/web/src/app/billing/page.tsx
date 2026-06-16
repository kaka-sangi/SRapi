"use client";

import { useState } from "react";
import { ShoppingCart, CreditCard, Receipt } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import { CheckoutRedirect } from "@/components/features/checkout-redirect";
import { useAdminList } from "@/hooks/use-admin-list";
import {
  useBalance,
  usePaymentMethods,
  useMyOrders,
  useMyBillingHistory,
  useCreateOrder,
  useCancelOrder,
  useMySubscriptions,
} from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
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
import { Skeleton } from "@/components/ui/skeleton";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { quietStatusFor, statusLabel } from "@/lib/status-badge";
import { formatMoney, formatDate } from "@/lib/admin-format";
import {
  SubscriptionUsageBars,
  type SubscriptionUsageLabels,
} from "@/components/features/subscription-usage-bars";
import { meErrorMessage } from "@/lib/me-api";
import type {
  BillingLedgerEntry,
  PaymentMethod,
  PaymentOrder,
  UserSubscription,
} from "@/lib/sdk-types";

export default function BillingPage() {
  return (
    <AppShell allowedRole="user">
      <BillingContent />
    </AppShell>
  );
}

function BillingContent() {
  const { t } = useLanguage();

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAccount")}
        title={t("billing.title")}
        description={t("billing.subtitle")}
      />
      <Tabs defaultValue="balance">
        <TabsList>
          <TabsTrigger value="balance">{t("billing.tabBalance")}</TabsTrigger>
          <TabsTrigger value="orders">{t("billing.tabOrders")}</TabsTrigger>
          <TabsTrigger value="subs">{t("billing.tabSubscriptions")}</TabsTrigger>
          <TabsTrigger value="ledger">{t("billing.tabLedger")}</TabsTrigger>
        </TabsList>
        <TabsContent value="balance">
          <BalanceTab />
        </TabsContent>
        <TabsContent value="orders">
          <OrdersTab />
        </TabsContent>
        <TabsContent value="subs">
          <SubscriptionsTab />
        </TabsContent>
        <TabsContent value="ledger">
          <LedgerTab />
        </TabsContent>
      </Tabs>
    </>
  );
}

function BalanceTab() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const balance = useBalance();
  const methods = usePaymentMethods();
  const createMut = useCreateOrder();
  const [amount, setAmount] = useState("10");
  const [payerOpenID, setPayerOpenID] = useState("");
  const [payerClientIP, setPayerClientIP] = useState("");
  const [createdOrder, setCreatedOrder] = useState<PaymentOrder | null>(null);
  // Holds the chosen provider_instance_id (unique), not the method type — two
  // instances can share a method type, which would give Radix Select duplicate
  // values and break selection.
  const [instanceId, setInstanceId] = useState("");
  const [error, setError] = useState<string | null>(null);

  const methodList = methods.data?.data ?? [];
  const effectiveInstanceId = instanceId || methodList[0]?.provider_instance_id || "";
  const selected =
    methodList.find((m) => m.provider_instance_id === effectiveInstanceId) ?? methodList[0] ?? null;
  const feePreview = selected ? paymentFeePreview(amount, selected) : null;
  const paymentCurrency = balance.data?.currency ?? "USD";
  const needsWeChatPayer = selected
    ? selected.provider === "wechat" && /jsapi|h5/i.test(selected.method)
    : false;

  async function topUp(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (!selected) {
      setError(t("billing.noMethods"));
      return;
    }
    try {
      const order = await createMut.mutateAsync({
        method: selected.method,
        amount: amount.trim(),
        product_type: "balance_credit",
        payer_openid: optionalTrim(payerOpenID),
        payer_client_ip: optionalTrim(payerClientIP),
      });
      setCreatedOrder(order);
      toast({ title: t("feedback.created"), tone: "success" });
    } catch (err) {
      setError(meErrorMessage(err));
    }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardContent>
          <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
            {t("billing.currentBalance")}
          </span>
          {balance.isLoading ? (
            <Skeleton className="mt-3 h-10 w-40" />
          ) : (
            <div className="mt-2 font-serif text-4xl text-srapi-text-primary tabular">
              {balance.data ? formatMoney(balance.data.balance, balance.data.currency) : "—"}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardContent>
          <form onSubmit={topUp} className="space-y-4">
            <h3 className="font-serif text-lg text-srapi-text-primary">{t("billing.topUp")}</h3>
            <div>
              <Label htmlFor="amount">{t("billing.amount")}</Label>
              <Input
                id="amount"
                inputMode="decimal"
                value={amount}
                onChange={(e) => setAmount(e.target.value)}
                disabled={createMut.isPending}
              />
            </div>
            <div>
              <Label htmlFor="method">{t("billing.method")}</Label>
              {methodList.length === 0 ? (
                <p className="text-2xs text-srapi-text-tertiary">{t("billing.noMethods")}</p>
              ) : (
                <Select
                  value={effectiveInstanceId}
                  onValueChange={setInstanceId}
                >
                  <SelectTrigger id="method">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {methodList.map((m) => (
                      <SelectItem key={m.provider_instance_id} value={m.provider_instance_id}>
                        {m.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            </div>
            {selected ? (
              <dl className="rounded-lg border border-srapi-border bg-srapi-card-muted px-3.5 py-3 text-sm">
                <div className="flex items-center justify-between">
                  <dt className="text-srapi-text-tertiary">{t("billing.feeCredit")}</dt>
                  <dd className="font-mono text-srapi-text-secondary tabular">
                    {formatMoney(amount, paymentCurrency)}
                  </dd>
                </div>
                <div className="mt-1.5 flex items-center justify-between">
                  <dt className="text-srapi-text-tertiary">{t("billing.feeChannel")}</dt>
                  <dd className="font-mono text-srapi-text-secondary tabular">
                    {feePreview ? formatMoney(feePreview.fee, paymentCurrency) : "-"}
                  </dd>
                </div>
                <div className="mt-1.5 flex items-center justify-between border-t border-srapi-border pt-1.5">
                  <dt className="font-medium text-srapi-text-primary">{t("billing.feePayable")}</dt>
                  <dd className="font-mono text-srapi-text-primary tabular">
                    {feePreview ? formatMoney(feePreview.payable, paymentCurrency) : "-"}
                  </dd>
                </div>
              </dl>
            ) : null}
            {needsWeChatPayer ? (
              <div className="grid gap-3 sm:grid-cols-2">
                <div>
                  <Label htmlFor="payer-openid">{t("billing.payerOpenID")}</Label>
                  <Input
                    id="payer-openid"
                    value={payerOpenID}
                    onChange={(e) => setPayerOpenID(e.target.value)}
                    disabled={createMut.isPending}
                  />
                </div>
                <div>
                  <Label htmlFor="payer-client-ip">{t("billing.payerClientIP")}</Label>
                  <Input
                    id="payer-client-ip"
                    value={payerClientIP}
                    onChange={(e) => setPayerClientIP(e.target.value)}
                    disabled={createMut.isPending}
                  />
                </div>
              </div>
            ) : null}
            {createdOrder ? (
              <CheckoutRedirect key={createdOrder.id} order={createdOrder} variant="card" />
            ) : null}
            {error ? (
              <p role="alert" className="text-sm text-srapi-error">
                {error}
              </p>
            ) : null}
            <Button
              type="submit"
              variant="primary"
              loading={createMut.isPending}
              disabled={methodList.length === 0 || !amount.trim()}
            >
              {t("billing.createOrder")}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}

function optionalTrim(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed ? trimmed : undefined;
}

function paymentFeePreview(amount: string, method: PaymentMethod): { fee: string; payable: string } | null {
  const rate = typeof method.metadata?.fee_rate === "string" ? method.metadata.fee_rate : "0";
  const amountUnits = parseDecimalToUnits(amount, 8);
  const rateUnits = parseDecimalToUnits(rate, 8);
  if (amountUnits === null || rateUnits === null || amountUnits <= BigInt(0) || rateUnits < BigInt(0)) {
    return null;
  }
  const scale = BigInt(10) ** BigInt(8);
  const feeUnits = (amountUnits * rateUnits) / scale;
  return {
    fee: formatUnits(feeUnits, 8),
    payable: formatUnits(amountUnits + feeUnits, 8),
  };
}

function parseDecimalToUnits(value: string, scale: number): bigint | null {
  const trimmed = value.trim();
  if (!/^[0-9]+(\.[0-9]+)?$/.test(trimmed)) {
    return null;
  }
  const [whole, fractional = ""] = trimmed.split(".");
  if (fractional.length > scale) {
    return null;
  }
  const padded = fractional.padEnd(scale, "0").slice(0, scale);
  return BigInt(`${whole}${padded}`.replace(/^0+(?=\d)/, "") || "0");
}

function formatUnits(units: bigint, scale: number): string {
  const raw = units.toString().padStart(scale + 1, "0");
  const whole = raw.slice(0, -scale);
  const fractional = raw.slice(-scale);
  return `${whole}.${fractional}`;
}

function OrdersTab() {
  const { t } = useLanguage();
  const orders = useMyOrders();
  const cancelMut = useCancelOrder();
  const [cancelTarget, setCancelTarget] = useState<PaymentOrder | null>(null);

  const columns: Column<PaymentOrder>[] = [
    {
      key: "order",
      header: t("billing.order"),
      render: (o) => <span className="font-mono text-2xs text-srapi-text-secondary">{o.order_no}</span>,
    },
    {
      key: "amount",
      header: t("billing.amount"),
      align: "right",
      render: (o) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(o.amount, o.currency)}
        </span>
      ),
    },
    {
      key: "status",
      header: t("billing.status"),
      render: (o) => <QuietBadge status={quietStatusFor(o.status)} label={statusLabel(t, o.status)} />,
    },
  ];

  return (
    <>
      <AdminListView
        query={orders}
        columns={columns}
        getRowId={(o) => o.id}
        emptyIcon={ShoppingCart}
        emptyTitle={t("billing.emptyOrders")}
        emptyBody={t("billing.emptyOrdersBody")}
        minWidth={480}
        rowActions={(o) =>
          o.status === "pending" ? (
            <RowActionsMenu
              actions={[
                {
                  label: t("billing.cancel"),
                  destructive: true,
                  onSelect: () => setCancelTarget(o),
                },
              ]}
            />
          ) : null
        }
      />
      <ConfirmDialog
        open={cancelTarget !== null}
        onOpenChange={(open) => {
          if (!open) setCancelTarget(null);
        }}
        title={t("billing.cancelTitle")}
        body={cancelTarget?.order_no}
        confirmLabel={t("billing.cancel")}
        onConfirm={() => cancelMut.mutateAsync(cancelTarget!.id)}
        successMessage={t("feedback.saved")}
        isPending={cancelMut.isPending}
      />
    </>
  );
}

function SubscriptionsTab() {
  const { t } = useLanguage();
  const subs = useMySubscriptions();

  const columns: Column<UserSubscription>[] = [
    {
      key: "plan",
      header: t("billing.plan"),
      render: (s) => <span className="font-mono text-2xs text-srapi-text-secondary">{s.plan_id}</span>,
    },
    {
      key: "period",
      header: t("billing.period"),
      hideOnMobile: true,
      render: (s) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDate(s.starts_at)} – {formatDate(s.expires_at)}
        </span>
      ),
    },
    {
      key: "usage",
      header: t("billing.usage"),
      hideOnMobile: true,
      render: (s) => <SubscriptionUsageBars subscription={s} labels={subscriptionUsageLabels(t)} />,
    },
    {
      key: "status",
      header: t("billing.status"),
      render: (s) => <QuietBadge status={quietStatusFor(s.status)} label={statusLabel(t, s.status)} />,
    },
  ];

  return (
    <AdminListView
      query={subs}
      columns={columns}
      getRowId={(s) => s.id}
      emptyIcon={CreditCard}
      emptyTitle={t("billing.emptySubs")}
      emptyBody={t("billing.emptySubsBody")}
      minWidth={480}
    />
  );
}

function subscriptionUsageLabels(t: ReturnType<typeof useLanguage>["t"]): SubscriptionUsageLabels {
  return {
    daily: t("billing.dailyUsage"),
    weekly: t("billing.weeklyUsage"),
    monthly: t("billing.monthlyUsage"),
    noQuota: t("billing.noCostQuota"),
  };
}

// Authenticated billing ledger for the session user. The server scopes by
// user_id so the user only ever sees their own rows; it does NOT depend on
// any client-side filter. Pagination is real (LIMIT/OFFSET at the DB layer
// via billing.Store.ListPage).
function LedgerTab() {
  const { t } = useLanguage();
  const list = useAdminList();
  const ledger = useMyBillingHistory({ page: list.page, page_size: list.pageSize });

  const columns: Column<BillingLedgerEntry>[] = [
    {
      key: "date",
      header: t("billing.date"),
      render: (row) => (
        <span className="whitespace-nowrap font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatDate(row.created_at)}
        </span>
      ),
    },
    {
      key: "type",
      header: t("billing.entryType"),
      render: (row) => <span className="text-srapi-text-secondary">{row.type}</span>,
    },
    {
      key: "amount",
      header: t("billing.amount"),
      align: "right",
      render: (row) => (
        <span className="font-mono text-srapi-text-secondary tabular">
          {formatMoney(row.amount, row.currency)}
        </span>
      ),
    },
    {
      key: "balance",
      header: t("billing.runningBalance"),
      align: "right",
      hideOnMobile: true,
      render: (row) => (
        <span className="font-mono text-2xs text-srapi-text-tertiary tabular">
          {formatMoney(row.balance_after, row.currency)}
        </span>
      ),
    },
    {
      key: "reference",
      header: t("billing.reference"),
      hideOnMobile: true,
      render: (row) =>
        row.reference_type ? (
          <span className="font-mono text-2xs text-srapi-text-tertiary">
            {row.reference_type}
            {row.reference_id ? ` · ${row.reference_id}` : ""}
          </span>
        ) : (
          <span className="text-2xs text-srapi-text-tertiary">—</span>
        ),
    },
  ];

  return (
    <AdminListView
      query={ledger}
      columns={columns}
      getRowId={(r) => r.id}
      emptyIcon={Receipt}
      emptyTitle={t("billing.emptyLedger")}
      emptyBody={t("billing.emptyLedgerBody")}
      minWidth={560}
      pagination={{
        page: list.page,
        pageSize: list.pageSize,
        total: ledger.data?.pagination?.total ?? ledger.data?.data.length ?? 0,
        onPageChange: list.setPage,
      }}
    />
  );
}
