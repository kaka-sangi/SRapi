"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { CheckCircle2, Sparkles, Star } from "lucide-react";
import { apiService } from "@/lib/api";
import { CheckoutRedirect } from "@/components/features/checkout-redirect";
import {
  useCreateOrder,
  usePaymentMethods,
  usePublicSubscriptionPlans,
} from "@/hooks/queries";
import { meErrorMessage } from "@/lib/me-api";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { DataPill } from "@/components/ui/data-pill";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";
import { formatMoney } from "@/lib/admin-format";
import type { PaymentOrder, SubscriptionPlan } from "@/lib/sdk-types";

// Public storefront. No AppShell — that gates by auth. The page is intentionally
// renderable without a session so visitors can browse pricing first.
export default function PricingPage() {
  const { t } = useLanguage();
  const plans = usePublicSubscriptionPlans();
  const [selected, setSelected] = useState<SubscriptionPlan | null>(null);

  // Sorted by sort_order; plans without one fall to the end.
  const sortedPlans = useMemo(() => {
    const list = plans.data?.data ?? [];
    return [...list].sort((a, b) => (a.sort_order ?? 999) - (b.sort_order ?? 999));
  }, [plans.data]);

  // Highlight the middle plan as the "popular" tier when 3+ are available;
  // the visual treatment uses a soft accent border-l-4 instead of any badge.
  const highlightIndex = sortedPlans.length >= 3 ? Math.floor(sortedPlans.length / 2) : -1;

  return (
    <div className="relative flex min-h-dvh flex-col">
      <header className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-6">
        <Link href="/" className="text-2xl font-semibold leading-none tracking-tight text-srapi-text-primary">
          SRapi
        </Link>
        <div className="flex items-center gap-2">
          <LanguageToggle />
          <ThemeToggle />
          <Link href="/">
            <Button variant="outline" size="sm">
              {t("pricing.signIn")}
            </Button>
          </Link>
        </div>
      </header>

      <main className="mx-auto w-full max-w-6xl flex-1 px-6 pb-16">
        <div className="mx-auto max-w-2xl text-center">
          <h1 className="text-4xl font-semibold tracking-tight text-srapi-text-primary sm:text-5xl">
            {t("pricing.title")}
          </h1>
          <p className="mt-3 text-srapi-text-secondary">{t("pricing.subtitle")}</p>
        </div>

        <div className="mt-12">
          {plans.isLoading ? (
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              <Skeleton className="h-72" />
              <Skeleton className="h-72" />
              <Skeleton className="h-72" />
            </div>
          ) : plans.error ? (
            <p className="text-center text-sm text-srapi-error">{meErrorMessage(plans.error)}</p>
          ) : sortedPlans.length === 0 ? (
            <EmptyState />
          ) : (
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              {sortedPlans.map((plan, idx) => (
                <PlanCard
                  key={plan.id}
                  plan={plan}
                  highlight={idx === highlightIndex}
                  onSubscribe={() => setSelected(plan)}
                />
              ))}
            </div>
          )}
        </div>
      </main>

      {/* Re-keying on the plan ID resets the dialog's internal state (selected
          payment method, error, created order) whenever the user opens it for
          a different plan or re-opens it after closing — no setState-in-effect
          dance required. */}
      <SubscribeDialog
        key={selected?.id ?? "none"}
        plan={selected}
        onClose={() => setSelected(null)}
      />
    </div>
  );
}

function PlanCard({
  plan,
  highlight = false,
  onSubscribe,
}: {
  plan: SubscriptionPlan;
  highlight?: boolean;
  onSubscribe: () => void;
}) {
  const { t } = useLanguage();
  const entitlements = entitlementLines(plan);

  return (
    <Card
      className={`flex h-full flex-col ${
        highlight ? "card-raised border-l-4 border-l-srapi-primary" : ""
      }`}
    >
      <CardContent className="flex flex-1 flex-col">
        <div className="flex items-start justify-between gap-3">
          <h3 className="text-xl font-semibold tracking-tight text-srapi-text-primary">
            {plan.name}
          </h3>
          {highlight ? (
            <DataPill tone="accent" size="sm">
              <Star className="size-3" aria-hidden />
              {t("pricing.popular") || "Popular"}
            </DataPill>
          ) : null}
        </div>
        {plan.description ? (
          <p className="mt-2 text-sm leading-relaxed text-srapi-text-secondary">
            {plan.description}
          </p>
        ) : null}
        <div className="mt-6 flex items-baseline gap-2">
          <span className="text-3xl font-semibold tracking-tight tabular text-srapi-text-primary">
            {formatMoney(plan.price, plan.currency)}
          </span>
        </div>
        <p className="mt-1 text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
          {t("pricing.perPeriod", { days: plan.validity_days })}
        </p>

        {entitlements.length > 0 ? (
          <ul className="mt-6 space-y-2">
            {entitlements.map((line) => (
              <li key={line} className="flex items-start gap-2 text-sm text-srapi-text-secondary">
                <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-srapi-success" aria-hidden />
                <span>{line}</span>
              </li>
            ))}
          </ul>
        ) : null}

        <div className="flex-1" />

        <Button
          type="button"
          variant="primary"
          size="lg"
          className="mt-6 w-full"
          onClick={onSubscribe}
        >
          <Sparkles className="size-4" aria-hidden />
          {t("pricing.subscribe")}
        </Button>
      </CardContent>
    </Card>
  );
}

function EmptyState() {
  const { t } = useLanguage();
  return (
    <IllustratedEmptyState
      illust="chart"
      title={t("pricing.emptyTitle")}
      description={t("pricing.emptyBody")}
    />
  );
}

// Subscribe flow: if no session, redirect to login with ?next=/pricing so the
// user lands back here after sign-in. If signed in, pick a payment method and
// create an order — the backend's POST /payment/orders with
// product_type='subscription_plan' is already wired and the webhook fulfillment
// path activates the subscription on payment receipt.
function SubscribeDialog({
  plan,
  onClose,
}: {
  plan: SubscriptionPlan | null;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const router = useRouter();
  const methods = usePaymentMethods();
  const createMut = useCreateOrder();
  const [instanceId, setInstanceId] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [createdOrder, setCreatedOrder] = useState<PaymentOrder | null>(null);

  const open = plan !== null;
  const isAuthed = apiService.getCurrentUser() !== null;
  const methodList = methods.data?.data ?? [];
  const effectiveInstanceId = instanceId || methodList[0]?.provider_instance_id || "";
  const selected =
    methodList.find((m) => m.provider_instance_id === effectiveInstanceId) ?? methodList[0] ?? null;

  function goSignIn() {
    router.push("/?next=/pricing");
  }

  async function subscribe(event: React.FormEvent) {
    event.preventDefault();
    if (!plan) return;
    setError(null);
    if (!selected) {
      setError(t("billing.noMethods"));
      return;
    }
    try {
      const order = await createMut.mutateAsync({
        method: selected.method,
        amount: plan.price,
        currency: plan.currency,
        product_type: "subscription_plan",
        product_id: String(plan.id),
      });
      setCreatedOrder(order);
    } catch (err) {
      setError(meErrorMessage(err));
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {plan
              ? t("pricing.subscribeTo", { name: plan.name })
              : t("pricing.subscribe")}
          </DialogTitle>
          {plan ? (
            <DialogDescription>
              {formatMoney(plan.price, plan.currency)} · {t("pricing.perPeriod", { days: plan.validity_days })}
            </DialogDescription>
          ) : null}
        </DialogHeader>

        {!isAuthed ? (
          <div className="space-y-4 py-2">
            <p className="text-sm text-srapi-text-secondary">{t("pricing.signInRequired")}</p>
            <Button type="button" variant="primary" className="w-full" onClick={goSignIn}>
              {t("pricing.signIn")}
            </Button>
          </div>
        ) : createdOrder ? (
          <SubscriptionOrderStatus order={createdOrder} onClose={onClose} />
        ) : (
          <form onSubmit={subscribe} className="space-y-4 py-2">
            <div>
              <Label htmlFor="pricing-method">{t("billing.paymentMethod")}</Label>
              {methods.isLoading ? (
                <Skeleton className="mt-1 h-10 w-full" />
              ) : methodList.length === 0 ? (
                <p className="mt-1 text-xs text-srapi-text-tertiary">{t("billing.noMethods")}</p>
              ) : (
                <Select
                  value={effectiveInstanceId}
                  onValueChange={(value) => setInstanceId(value)}
                >
                  <SelectTrigger id="pricing-method" className="mt-1">
                    <SelectValue placeholder={t("billing.paymentMethod")} />
                  </SelectTrigger>
                  <SelectContent>
                    {methodList.map((m) => (
                      <SelectItem key={m.provider_instance_id} value={m.provider_instance_id}>
                        {m.name || m.method}
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
              className="w-full"
              loading={createMut.isPending}
              disabled={methodList.length === 0 || !selected}
            >
              {t("pricing.subscribe")}
            </Button>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

function SubscriptionOrderStatus({
  order,
  onClose,
}: {
  order: PaymentOrder;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  return (
    <div className="py-2">
      <CheckoutRedirect
        order={order}
        variant="card"
        successAction={
          <Button asChild variant="primary" className="w-full">
            <Link href="/billing">{t("pricing.viewBilling")}</Link>
          </Button>
        }
        failureAction={
          <Button type="button" variant="outline" className="w-full" onClick={onClose}>
            {t("pricing.close")}
          </Button>
        }
      />
    </div>
  );
}

function entitlementLines(plan: SubscriptionPlan): string[] {
  if (!plan.entitlements) return [];
  const lines: string[] = [];
  for (const [key, raw] of Object.entries(plan.entitlements)) {
    if (raw === null || raw === undefined || raw === false) continue;
    if (raw === true) {
      lines.push(prettifyKey(key));
      continue;
    }
    lines.push(`${prettifyKey(key)}: ${String(raw)}`);
  }
  return lines;
}

function prettifyKey(key: string): string {
  return key.replace(/[_-]+/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}
