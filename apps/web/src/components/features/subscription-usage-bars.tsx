"use client";

import { formatMoney } from "@/lib/admin-format";
import type { UserSubscription } from "@/lib/sdk-types";

export interface SubscriptionUsageLabels {
  daily: string;
  weekly: string;
  monthly: string;
  noQuota: string;
}

export function SubscriptionUsageBars({
  subscription,
  labels,
}: {
  subscription: UserSubscription;
  labels: SubscriptionUsageLabels;
}) {
  const quota = monthlyCostQuota(subscription.entitlements_snapshot);

  return (
    <div className="min-w-48 space-y-1.5">
      <UsageBar label={labels.daily} value={subscription.daily_usage_usd} />
      <UsageBar label={labels.weekly} value={subscription.weekly_usage_usd} />
      <UsageBar label={labels.monthly} value={subscription.monthly_usage_usd} limit={quota} emptyLimit={labels.noQuota} />
    </div>
  );
}

function UsageBar({
  label,
  value,
  limit,
  emptyLimit,
}: {
  label: string;
  value: string;
  limit?: string;
  emptyLimit?: string;
}) {
  const valueNumber = moneyNumber(value);
  const limitNumber = moneyNumber(limit);
  const percent = limitNumber && limitNumber > 0 ? Math.min(100, (valueNumber / limitNumber) * 100) : 0;
  const secondary = limitNumber && limitNumber > 0 ? ` / ${formatMoney(limit, "USD")}` : emptyLimit ? ` / ${emptyLimit}` : "";

  return (
    <div>
      <div className="flex items-center justify-between gap-2 text-2xs">
        <span className="text-srapi-text-secondary">{label}</span>
        <span className="font-mono text-srapi-text-tertiary">
          {formatMoney(value, "USD")}
          {secondary}
        </span>
      </div>
      <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-srapi-card-muted">
        <div className="h-full rounded-full bg-srapi-accent" style={{ width: `${percent}%` }} />
      </div>
    </div>
  );
}

function monthlyCostQuota(entitlements: Record<string, unknown>): string | undefined {
  const value = entitlements.monthly_cost_quota;
  if (typeof value === "string" && value.trim() !== "") {
    return value;
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    return String(value);
  }
  return undefined;
}

function moneyNumber(value?: string): number {
  if (!value) return 0;
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}
