"use client";

import { useState } from "react";
import { CreditCard, Receipt, ShoppingBag, TrendingUp, Users } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { StatCard, StatCardSkeleton } from "@/components/ui/stat-card";
import { Button } from "@/components/ui/button";
import { SectionTitle } from "@/components/ui/section-title";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { useAdminPaymentDashboard } from "@/hooks/admin-queries";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney } from "@/lib/admin-format";

const DAY_OPTIONS = [7, 30, 90] as const;
type DayRange = (typeof DAY_OPTIONS)[number];

export function PaymentDashboardPanel() {
  const { t } = useLanguage();
  const [days, setDays] = useState<DayRange>(30);
  const dashboard = useAdminPaymentDashboard(days);
  // Top-users list resolves id → email via the shared 200-row lookup; falls
  // back to "User #<id>" for ids past the window (more readable than the
  // raw id in a "top spender" context where a name is expected).
  const userLookup = useUserEmailLookup();

  const snapshot = dashboard.data;
  const currency = snapshot?.currency || "USD";

  const dayOptions = DAY_OPTIONS.map((d) => ({
    value: String(d) as `${DayRange}`,
    label: `${d}${t("adminOrders.dashboard.daySuffix")}`,
  }));

  return (
    <section
      aria-labelledby="payment-dashboard-heading"
      className="mb-6 space-y-4"
    >
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <h2
            id="payment-dashboard-heading"
            className="text-lg font-semibold tracking-tight text-srapi-text-primary"
          >
            {t("adminOrders.dashboard.title")}
          </h2>
          <p className="text-sm text-srapi-text-tertiary">
            {t("adminOrders.dashboard.subtitle")}
          </p>
        </div>
        <SegmentedControl
          value={String(days) as `${DayRange}`}
          onChange={(v) => setDays(Number(v) as DayRange)}
          options={dayOptions}
          ariaLabel={t("adminOrders.dashboard.title")}
        />
      </header>

      {dashboard.isError ? (
        <Card>
          <CardContent className="space-y-2 p-4">
            <p role="alert" className="text-sm text-srapi-error">
              {t("adminOrders.dashboard.loadFailed")}
            </p>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => dashboard.refetch()}
            >
              {t("common.retry")}
            </Button>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        {dashboard.isLoading || !snapshot ? (
          <>
            <StatCardSkeleton />
            <StatCardSkeleton />
            <StatCardSkeleton />
          </>
        ) : (
          <>
            <StatCard
              label={t("adminOrders.dashboard.paidAmount")}
              value={Number(snapshot.totals.paid_amount) || 0}
              format={(n) => formatMoney(n.toFixed(2), currency)}
              hint={t("adminOrders.dashboard.windowHint", { days: snapshot.day_range })}
              icon={<TrendingUp aria-hidden />}
            />
            <StatCard
              label={t("adminOrders.dashboard.paidCount")}
              value={snapshot.totals.paid_count}
              hint={t("adminOrders.dashboard.paidHint", { total: snapshot.totals.order_count })}
              icon={<Receipt aria-hidden />}
            />
            <StatCard
              label={t("adminOrders.dashboard.orderCount")}
              value={snapshot.totals.order_count}
              hint={t("adminOrders.dashboard.windowHint", { days: snapshot.day_range })}
              icon={<ShoppingBag aria-hidden />}
            />
          </>
        )}
      </div>

      {snapshot && !dashboard.isLoading ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Card>
            <CardContent className="space-y-3 p-5">
              <SectionTitle
                icon={<CreditCard aria-hidden />}
                label={t("adminOrders.dashboard.paymentMethods")}
              />
              {snapshot.payment_methods.length === 0 ? (
                <p className="text-sm text-srapi-text-tertiary">
                  {t("adminOrders.dashboard.empty")}
                </p>
              ) : (
                <ul className="divide-y divide-srapi-border/70">
                  {snapshot.payment_methods.map((m) => (
                    <li
                      key={m.provider}
                      className="flex items-center justify-between gap-3 py-3 text-sm transition-colors hover:bg-srapi-card-muted/60 -mx-2 px-2 rounded-lg"
                    >
                      <span className="text-srapi-text-secondary">{m.provider}</span>
                      <span className="flex items-baseline gap-2 text-right">
                        <span className="text-sm font-semibold tabular text-srapi-text-primary">
                          {formatMoney(m.amount, currency)}
                        </span>
                        <span className="text-[11px] font-medium tabular text-srapi-text-tertiary">
                          ({m.count})
                        </span>
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardContent className="space-y-3 p-5">
              <SectionTitle
                icon={<Users aria-hidden />}
                label={t("adminOrders.dashboard.topUsers")}
              />
              {snapshot.top_users.length === 0 ? (
                <p className="text-sm text-srapi-text-tertiary">
                  {t("adminOrders.dashboard.empty")}
                </p>
              ) : (
                <ol className="divide-y divide-srapi-border/70">
                  {snapshot.top_users.map((u, idx) => {
                    const email = userLookup.map.get(String(u.user_id));
                    return (
                      <li
                        key={u.user_id}
                        className="flex items-center justify-between gap-3 py-3 text-sm transition-colors hover:bg-srapi-card-muted/60 -mx-2 px-2 rounded-lg"
                      >
                        <span className="flex min-w-0 items-center gap-2.5">
                          <span className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-srapi-card-muted text-[11px] font-medium tabular text-srapi-text-tertiary">
                            {idx + 1}
                          </span>
                          <span className="truncate text-srapi-text-secondary">
                            {email ?? `User #${u.user_id}`}
                          </span>
                        </span>
                        <span className="flex items-baseline gap-2 text-right">
                          <span className="text-sm font-semibold tabular text-srapi-text-primary">
                            {formatMoney(u.amount, currency)}
                          </span>
                          <span className="text-[11px] font-medium tabular text-srapi-text-tertiary">
                            ({u.order_count})
                          </span>
                        </span>
                      </li>
                    );
                  })}
                </ol>
              )}
            </CardContent>
          </Card>
        </div>
      ) : null}
    </section>
  );
}
