"use client";

import { useState } from "react";
import { ShoppingCart, CreditCard } from "lucide-react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { AdminListView, type Column } from "@/components/admin/admin-list-view";
import { RowActionsMenu } from "@/components/admin/row-actions";
import { ConfirmDialog } from "@/components/admin/confirm-dialog";
import {
  useBalance,
  usePaymentMethods,
  useMyOrders,
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
import { meErrorMessage } from "@/lib/me-api";
import type { PaymentOrder, UserSubscription } from "@/lib/sdk-types";

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
  // Holds the chosen provider_instance_id (unique), not the method type — two
  // instances can share a method type, which would give Radix Select duplicate
  // values and break selection.
  const [instanceId, setInstanceId] = useState("");
  const [error, setError] = useState<string | null>(null);

  const methodList = methods.data?.data ?? [];

  async function topUp(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    const selected =
      methodList.find((m) => m.provider_instance_id === instanceId) ?? methodList[0];
    if (!selected) {
      setError(t("billing.noMethods"));
      return;
    }
    try {
      await createMut.mutateAsync({
        method: selected.method,
        amount: amount.trim(),
        product_type: "balance_credit",
      });
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
                  value={instanceId || methodList[0]?.provider_instance_id}
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
