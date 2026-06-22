"use client";

import { useQuery } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import type { P } from "./_shared";

/**
 * Ops chart data hooks — latency percentile histogram + error distribution.
 *
 * These wrap the read-model endpoints `getAdminOpsLatencyHistogram` and
 * `getAdminOpsErrorDistribution` (exposed via `adminApi.getOpsLatencyHistogram`
 * / `adminApi.getOpsErrorDistribution`). They follow the same shape as the
 * neighbouring throughput/error-trend hooks: param type derived from the
 * adminApi method, a stable query key, and a 30s refetch so the ops overview
 * stays live without manual reloads.
 *
 * Kept in this dedicated module (not the shared `ops.ts`) so the new charts can
 * evolve independently of the rest of the ops admin queries.
 */

// Response shapes derived straight from the adminApi methods so they always
// track the generated SDK (the bucket/item types aren't in the sdk-types
// barrel, so re-export them here for the chart components to consume).
type OpsLatencyHistogramData = Awaited<
  ReturnType<typeof adminApi.getOpsLatencyHistogram>
>;
export type OpsLatencyBucket = OpsLatencyHistogramData["buckets"][number];

type OpsErrorDistributionData = Awaited<
  ReturnType<typeof adminApi.getOpsErrorDistribution>
>;
export type OpsErrorDistributionItem = OpsErrorDistributionData["items"][number];

export function useOpsLatencyHistogram(params?: P<typeof adminApi.getOpsLatencyHistogram>) {
  return useQuery({
    queryKey: queryKeys.admin.opsLatencyHistogram(params),
    queryFn: () => adminApi.getOpsLatencyHistogram(params),
    refetchInterval: 30_000,
  });
}

export function useOpsErrorDistribution(params?: P<typeof adminApi.getOpsErrorDistribution>) {
  return useQuery({
    queryKey: queryKeys.admin.opsErrorDistribution(params),
    queryFn: () => adminApi.getOpsErrorDistribution(params),
    refetchInterval: 30_000,
  });
}
