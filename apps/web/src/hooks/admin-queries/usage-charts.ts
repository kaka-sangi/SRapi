"use client";

import { useQuery } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import type { P } from "./_shared";

/**
 * Admin usage chart data hooks — the multi-series usage TREND (tokens / cost /
 * requests over time, one series per top-N model|account|source_endpoint) and
 * the usage error distribution (counts grouped by error_class).
 *
 * These wrap the read-model endpoints `getAdminUsageTrends` /
 * `getAdminUsageErrorDistribution` (exposed via `adminApi.getUsageTrends` /
 * `adminApi.getUsageErrorDistribution`). They mirror the neighbouring
 * `ops-charts` hooks: a param type derived from the adminApi method, a stable
 * query key, and a 30s refetch so the usage page stays live without manual
 * reloads.
 *
 * Kept in this dedicated module (not the shared `ops.ts`) so the usage charts
 * can evolve independently of the rest of the admin usage queries — same split
 * the ops latency/error charts use.
 */

// Response shapes derived straight from the adminApi methods so they always
// track the generated SDK. The trend point/series and error-bucket types aren't
// in the sdk-types barrel, so re-export them here for the chart components.
export type UsageTrendsData = Awaited<ReturnType<typeof adminApi.getUsageTrends>>;
export type UsageTrendSeries = UsageTrendsData["series"][number];
export type UsageTrendPoint = UsageTrendSeries["points"][number];

export type UsageErrorDistributionData = Awaited<
  ReturnType<typeof adminApi.getUsageErrorDistribution>
>;
export type UsageErrorBucketItem = UsageErrorDistributionData[number];

export function useAdminUsageTrends(params?: P<typeof adminApi.getUsageTrends>) {
  return useQuery({
    queryKey: queryKeys.admin.usageTrends(params),
    queryFn: () => adminApi.getUsageTrends(params),
    refetchInterval: 30_000,
  });
}

export function useAdminUsageErrorDistribution(
  params?: P<typeof adminApi.getUsageErrorDistribution>,
) {
  return useQuery({
    queryKey: queryKeys.admin.usageErrorDistribution(params),
    queryFn: () => adminApi.getUsageErrorDistribution(params),
    refetchInterval: 30_000,
  });
}
