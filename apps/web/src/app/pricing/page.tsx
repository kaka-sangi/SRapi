"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { CheckCircle2, Sparkles, Star, CreditCard, Zap } from "lucide-react";
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
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { SectionHero } from "@/components/visual/section-hero";
import { AppShell } from "@/components/layout/app-shell";
import { cn } from "@/lib/cn";
import { formatMoney } from "@/lib/admin-format";
import type { PaymentOrder, SubscriptionPlan } from "@/lib/sdk-types";

export default function PricingPage() {
  return (
    <AppShell>
      <PricingContent />
    </AppShell>
  );
}

function PricingContent() {
  const { t } = useLanguage();
  const plans = usePublicSubscriptionPlans();
  const [selected, setSelected] = useState<SubscriptionPlan | null>(null);

  const sortedPlans = useMemo(() => {
    const list = plans.data?.data ?? [];
    return [...list].sort((a, b) => (a.sort_order ?? 999) - (b.sort_order ?? 999));
  }, [plans.data]);

  const highlightIndex = sortedPlans.length >= 3 ? Math.floor(sortedPlans.length / 2) : -1;

  return (
    <>
      <SectionHero
        eyebrow={t("pricing.eyebrow")}
        title={t("pricing.title")}
        description={t("pricing.subtitle")}
      />

      {plans.isLoading ? (
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          <Skeleton className="h-80 rounded-2xl" />
          <Skeleton className="h-80 rounded-2xl" />
          <Skeleton className="h-80 rounded-2xl" />
        </div>
      ) : plans.error ? (
        <p className="text-center text-sm text-srapi-error">{meErrorMessage(plans.error)}</p>
      ) : sortedPlans.length === 0 ? (
        <IllustratedEmptyState
          illust="chart"
          title={t("pricing.emptyTitle")}
          description={t("pricing.emptyBody")}
        />
      ) : (
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
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

      <SubscribeDialog
        key={selected?.id ?? "none"}
        plan={selected}
        onClose={() => setSelected(null)}
      />
    </>
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
    <Card className={cn(
      "group relative flex h-full flex-col overflow-hidden rounded-2xl transition-shadow hover:shadow-lg",
      highlight
        ? "border-2 border-srapi-primary shadow-md ring-1 ring-srapi-primary/20"
        : "border border-srapi-border",
    )}>
      {/* Decorative gradient top bar */}
      <div className={cn(
        "h-1.5 w-full",
        highlight
          ? "bg-gradient-to-r from-srapi-primary via-srapi-primary/70 to-srapi-accent"
          : "bg-gradient-to-r from-srapi-border-strong/60 to-srapi-border-strong/20",
      )} />

      <CardContent className="flex flex-1 flex-col p-6">
        {/* Header */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2">
            <div className={cn(
              "grid size-9 place-items-center rounded-lg",
              highlight ? "bg-srapi-primary/10 text-srapi-primary" : "bg-srapi-card-muted text-srapi-text-tertiary",
            )}>
              {highlight ? <Zap className="size-4" /> : <CreditCard className="size-4" />}
            </div>
            <h3 className="text-lg font-semibold tracking-tight text-srapi-text-primary">
              {plan.name}
            </h3>
          </div>
          {highlight && (
            <DataPill tone="accent" size="sm">
              <Star className="size-3" aria-hidden />
              {t("pricing.popular")}
            </DataPill>
          )}
        </div>

        {plan.description && (
          <p className="mt-3 text-sm leading-relaxed text-srapi-text-secondary">
            {plan.description}
          </p>
        )}

        {/* Price */}
        <div className="mt-5 flex items-baseline gap-1">
          <span className="text-3xl font-bold tracking-tight tabular text-srapi-text-primary">
            {formatMoney(plan.price, plan.currency)}
          </span>
          <span className="text-sm text-srapi-text-tertiary">
            / {plan.validity_days}{t("pricing.days")}
          </span>
        </div>

        {/* Divider */}
        <div className="my-5 h-px bg-srapi-border/70" />

        {/* Entitlements */}
        {entitlements.length > 0 && (
          <ul className="space-y-2.5">
            {entitlements.map((line) => (
              <li key={line} className="flex items-start gap-2.5 text-sm text-srapi-text-secondary">
                <CheckCircle2 className={cn(
                  "mt-0.5 size-4 shrink-0",
                  highlight ? "text-srapi-primary" : "text-srapi-success",
                )} aria-hidden />
                <span>{line}</span>
              </li>
            ))}
          </ul>
        )}

        <div className="flex-1" />

        {/* CTA */}
        <Button
          type="button"
          variant={highlight ? "primary" : "outline"}
          size="lg"
          className={cn("mt-6 w-full rounded-xl", highlight && "shadow-sm")}
          onClick={onSubscribe}
        >
          {highlight && <Sparkles className="size-4" aria-hidden />}
          {t("pricing.subscribe")}
        </Button>
      </CardContent>
    </Card>
  );
}

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
  const selectedMethod =
    methodList.find((m) => m.provider_instance_id === effectiveInstanceId) ?? methodList[0] ?? null;

  async function subscribe(event: React.FormEvent) {
    event.preventDefault();
    if (!plan) return;
    setError(null);
    if (!selectedMethod) {
      setError(t("billing.noMethods"));
      return;
    }
    try {
      const order = await createMut.mutateAsync({
        amount: plan.price,
        currency: plan.currency,
        method: selectedMethod.provider_instance_id,
        product_type: "subscription_plan",
        product_id: plan.id,
      });
      if (order) {
        setCreatedOrder(order as PaymentOrder);
      }
    } catch (err) {
      setError(meErrorMessage(err));
    }
  }

  if (createdOrder) {
    return <CheckoutRedirect order={createdOrder} />;
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>{t("pricing.subscribeTo", { plan: plan?.name ?? "" })}</DialogTitle>
          <DialogDescription>{t("pricing.subscribeHint")}</DialogDescription>
        </DialogHeader>
        {!isAuthed ? (
          <div className="space-y-4 py-2 text-center">
            <p className="text-sm text-srapi-text-secondary">{t("pricing.signInRequired")}</p>
            <Button variant="primary" onClick={() => router.push("/login?from=/pricing")}>
              {t("pricing.signIn")}
            </Button>
          </div>
        ) : methodList.length === 0 ? (
          <div className="space-y-2 py-4 text-center">
            <CreditCard className="mx-auto size-8 text-srapi-text-tertiary" />
            <p className="text-sm font-medium text-srapi-text-secondary">{t("billing.noMethodsTitle")}</p>
            <p className="text-xs text-srapi-text-tertiary">{t("billing.noMethodsHint")}</p>
          </div>
        ) : (
          <form onSubmit={subscribe} className="space-y-4">
            <div>
              <Label>{t("billing.method")}</Label>
              <Select value={effectiveInstanceId} onValueChange={setInstanceId}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {methodList.map((m) => (
                    <SelectItem key={m.provider_instance_id} value={m.provider_instance_id}>
                      {m.name || m.method}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {error && <p className="text-sm text-srapi-error">{error}</p>}
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
              <Button type="submit" variant="primary" loading={createMut.isPending}>
                {t("pricing.confirmPay", { amount: formatMoney(plan?.price ?? "0", plan?.currency ?? "USD") })}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

function entitlementLines(plan: SubscriptionPlan): string[] {
  const lines: string[] = [];
  const e = (plan.entitlements ?? {}) as Record<string, unknown>;
  const reqQuota = Number(e.request_quota ?? e.included_request_quota ?? 0);
  if (reqQuota > 0) lines.push(`${reqQuota.toLocaleString()} requests`);
  const tokenQuota = Number(e.token_quota ?? e.included_token_quota ?? 0);
  if (tokenQuota > 0) lines.push(`${tokenQuota.toLocaleString()} tokens`);
  const credit = String(e.balance_credit ?? e.included_balance_credit ?? "0");
  if (parseFloat(credit) > 0) lines.push(`${formatMoney(credit, plan.currency)} balance credit`);
  const features = e.features;
  if (Array.isArray(features)) {
    for (const f of features) {
      if (typeof f === "string" && f.trim()) lines.push(f.trim());
    }
  }
  const desc = e.description;
  if (typeof desc === "string" && desc.trim()) {
    for (const line of desc.split("\n")) {
      if (line.trim()) lines.push(line.trim());
    }
  }
  return lines;
}
