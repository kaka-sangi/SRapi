"use client";

import { useState } from "react";
import { Card } from "@/components/ui/card";
import { StatCard, StatCardSkeleton } from "@/components/ui/stat-card";
import { Button } from "@/components/ui/button";
import { useAdminPaymentDashboard } from "@/hooks/admin-queries";
import { useUserEmailLookup } from "@/hooks/use-user-email-lookup";
import { useLanguage } from "@/context/LanguageContext";
import { formatMoney } from "@/lib/admin-format";
import { cn } from "@/lib/cn";

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

  return (
    <section
      aria-labelledby="payment-dashboard-heading"
      className="mb-6 space-y-4"
    >
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2
            id="payment-dashboard-heading"
            className="font-serif text-lg text-srapi-text-primary"
          >
            {t("adminOrders.dashboard.title")}
          </h2>
          <p className="text-sm text-srapi-text-tertiary">
            {t("adminOrders.dashboard.subtitle")}
          </p>
        </div>
        <div className="flex items-center gap-1 rounded-lg border border-srapi-border bg-srapi-card-muted p-1">
          {DAY_OPTIONS.map((d) => (
            <button
              key={d}
              type="button"
              onClick={() => setDays(d)}
              className={cn(
                "px-3 py-1 font-mono text-2xs uppercase tabular transition-colors",
                d === days
                  ? "rounded-md bg-srapi-card text-srapi-text-primary shadow-sm"
                  : "text-srapi-text-tertiary hover:text-srapi-text-secondary",
              )}
              aria-pressed={d === days}
            >
              {d}
              {t("adminOrders.dashboard.daySuffix")}
            </button>
          ))}
        </div>
      </header>

      {dashboard.isError ? (
        <Card className="p-4">
          <p role="alert" className="text-sm text-srapi-error">
            {t("adminOrders.dashboard.loadFailed")}
          </p>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="mt-2"
            onClick={() => dashboard.refetch()}
          >
            {t("common.retry")}
          </Button>
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
            />
            <StatCard
              label={t("adminOrders.dashboard.paidCount")}
              value={snapshot.totals.paid_count}
              hint={t("adminOrders.dashboard.paidHint", { total: snapshot.totals.order_count })}
            />
            <StatCard
              label={t("adminOrders.dashboard.orderCount")}
              value={snapshot.totals.order_count}
              hint={t("adminOrders.dashboard.windowHint", { days: snapshot.day_range })}
            />
          </>
        )}
      </div>

      {snapshot && !dashboard.isLoading ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Card className="p-4">
            <h3 className="mb-3 font-mono text-2xs uppercase text-srapi-text-tertiary">
              {t("adminOrders.dashboard.paymentMethods")}
            </h3>
            {snapshot.payment_methods.length === 0 ? (
              <p className="text-sm text-srapi-text-tertiary">
                {t("adminOrders.dashboard.empty")}
              </p>
            ) : (
              <ul className="space-y-2">
                {snapshot.payment_methods.map((m) => (
                  <li
                    key={m.provider}
                    className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-srapi-card-muted"
                  >
                    <span className="text-srapi-text-secondary">{m.provider}</span>
                    <span className="text-right">
                      <span className="font-mono text-srapi-text-primary tabular">
                        {formatMoney(m.amount, currency)}
                      </span>
                      <span className="ml-2 font-mono text-2xs text-srapi-text-tertiary tabular">
                        ({m.count})
                      </span>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Card>

          <Card className="p-4">
            <h3 className="mb-3 font-mono text-2xs uppercase text-srapi-text-tertiary">
              {t("adminOrders.dashboard.topUsers")}
            </h3>
            {snapshot.top_users.length === 0 ? (
              <p className="text-sm text-srapi-text-tertiary">
                {t("adminOrders.dashboard.empty")}
              </p>
            ) : (
              <ol className="space-y-1.5">
                {snapshot.top_users.map((u, idx) => {
                  const email = userLookup.map.get(String(u.user_id));
                  return (
                    <li
                      key={u.user_id}
                      className="flex items-center justify-between rounded-md px-2 py-1.5 text-sm hover:bg-srapi-card-muted"
                    >
                      <span className="flex items-center gap-2">
                        <span className="inline-flex h-5 w-5 items-center justify-center rounded-full bg-srapi-card-muted font-mono text-2xs text-srapi-text-tertiary tabular">
                          {idx + 1}
                        </span>
                        <span className="text-srapi-text-secondary">
                          {email ?? `User #${u.user_id}`}
                        </span>
                      </span>
                      <span className="text-right">
                        <span className="font-mono text-srapi-text-primary tabular">
                          {formatMoney(u.amount, currency)}
                        </span>
                        <span className="ml-2 font-mono text-2xs text-srapi-text-tertiary tabular">
                          ({u.order_count})
                        </span>
                      </span>
                    </li>
                  );
                })}
              </ol>
            )}
          </Card>
        </div>
      ) : null}
    </section>
  );
}
