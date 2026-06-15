"use client";

import { useQuery } from "@tanstack/react-query";
import {
  getCurrentUserUsageThroughput,
  getCurrentUserUsageModels,
  getCurrentUserUsageTrend,
  getCurrentUserUsageCacheMetrics,
} from "../../../../packages/sdk/typescript/src/index";
import type {
  UsageThroughput,
  UsageModelShare,
  UsageTrendPoint,
  UsageCacheMetrics,
} from "@/lib/sdk-types";
import { configureSdkClient } from "@/lib/sdk-client";

/**
 * User-facing usage-dashboard aggregate hooks.
 *
 * These back the enriched end-user dashboard (RPM/TPM/cache-hit KPIs, the usage
 * trend chart and the model-distribution list). They are USER endpoints
 * (`/user/usage/dashboard/*`), not admin — so they talk to the generated SDK
 * directly behind `configureSdkClient()`, exactly like `use-user-balance-history`
 * and the public `/key-usage` flow, rather than routing through the admin-api
 * layer. Living in their own file keeps the dashboard agent off the shared
 * `queries.ts` / `admin-queries/*` modules.
 *
 * Each hook unwraps the `{ data, request_id }` envelope and returns just the
 * payload so consumers never reach through `.data.data`.
 */

/** Aggregates refresh on a slow cadence — they are derived rollups, not live logs. */
const DASHBOARD_STALE_TIME = 30_000;
const DASHBOARD_REFETCH_INTERVAL = 60_000;

const userDashboardKeys = {
  throughput: () => ["me", "usage-dashboard", "throughput"] as const,
  models: (days: number) => ["me", "usage-dashboard", "models", days] as const,
  trend: (days: number, bucket: "day" | "hour") =>
    ["me", "usage-dashboard", "trend", days, bucket] as const,
  cacheMetrics: () => ["me", "usage-dashboard", "cache-metrics"] as const,
};

/** Live throughput (RPM/TPM, peaks, window totals) for the signed-in user. */
export function useUserUsageThroughput() {
  return useQuery({
    queryKey: userDashboardKeys.throughput(),
    staleTime: DASHBOARD_STALE_TIME,
    refetchInterval: DASHBOARD_REFETCH_INTERVAL,
    queryFn: async (): Promise<UsageThroughput> => {
      configureSdkClient();
      const response = await getCurrentUserUsageThroughput({ throwOnError: true });
      return response.data.data;
    },
  });
}

/** Per-model usage share (requests / tokens / cost) over the last `days` days. */
export function useUserUsageModels(days = 7) {
  return useQuery({
    queryKey: userDashboardKeys.models(days),
    staleTime: DASHBOARD_STALE_TIME,
    queryFn: async (): Promise<UsageModelShare[]> => {
      configureSdkClient();
      const response = await getCurrentUserUsageModels({
        query: { days },
        throwOnError: true,
      });
      return response.data.data;
    },
  });
}

/** Bucketed usage trend (requests / tokens / cost per bucket) over `days` days. */
export function useUserUsageTrend(days = 7, bucket: "day" | "hour" = "day") {
  return useQuery({
    queryKey: userDashboardKeys.trend(days, bucket),
    staleTime: DASHBOARD_STALE_TIME,
    queryFn: async (): Promise<UsageTrendPoint[]> => {
      configureSdkClient();
      const response = await getCurrentUserUsageTrend({
        query: { days, bucket },
        throwOnError: true,
      });
      return response.data.data;
    },
  });
}

/** Prompt-cache metrics (hit rate, cached vs. total input tokens, cost saved). */
export function useUserUsageCacheMetrics() {
  return useQuery({
    queryKey: userDashboardKeys.cacheMetrics(),
    staleTime: DASHBOARD_STALE_TIME,
    refetchInterval: DASHBOARD_REFETCH_INTERVAL,
    queryFn: async (): Promise<UsageCacheMetrics> => {
      configureSdkClient();
      const response = await getCurrentUserUsageCacheMetrics({ throwOnError: true });
      return response.data.data;
    },
  });
}
