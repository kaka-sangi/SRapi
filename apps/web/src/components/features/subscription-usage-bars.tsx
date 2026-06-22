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
  const dailyQuota = costQuota(subscription.entitlements_snapshot, "daily_cost_quota");
  const weeklyQuota = costQuota(subscription.entitlements_snapshot, "weekly_cost_quota");
  const monthlyQuota = costQuota(subscription.entitlements_snapshot, "monthly_cost_quota");

  return (
    <div className="min-w-48 space-y-1.5">
      <UsageBar label={labels.daily} value={subscription.daily_usage_usd} limit={dailyQuota} emptyLimit={labels.noQuota} />
      <UsageBar label={labels.weekly} value={subscription.weekly_usage_usd} limit={weeklyQuota} emptyLimit={labels.noQuota} />
      <UsageBar label={labels.monthly} value={subscription.monthly_usage_usd} limit={monthlyQuota} emptyLimit={labels.noQuota} />
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
      <div className="flex items-center justify-between gap-2 text-[11px]">
        <span className="text-srapi-text-secondary">{label}</span>
        <span className="tabular text-srapi-text-tertiary">
          {formatMoney(value, "USD")}
          {secondary}
        </span>
      </div>
      <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-srapi-card-muted">
        <div className="h-full rounded-full bg-srapi-primary" style={{ width: `${percent}%` }} />
      </div>
    </div>
  );
}

function costQuota(entitlements: Record<string, unknown>, key: "daily_cost_quota" | "weekly_cost_quota" | "monthly_cost_quota"): string | undefined {
  const value = entitlements[key];
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
