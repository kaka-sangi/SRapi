"use client";

import { useQuery } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";

/**
 * Per-account usage hooks — the 5h/7d windows, the "today" stat row, and the
 * 30-day daily series. Each wraps a read-model endpoint via `adminApi` and is
 * gated behind a selected account `id` (the detail sheet opens on demand), so
 * the account list itself stays cheap. Keyed under the per-account
 * `["admin", "accounts", id, …]` prefix like the neighbouring health/quota/rpm
 * diagnostics hooks.
 */

/** Default lookback for the daily series — matches the sub2api 30-day mini-table. */
export const ACCOUNT_USAGE_DAILY_DAYS = 30;

export function useAccountUsageWindows(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountUsageWindows(id ?? ""),
    queryFn: () => adminApi.getAccountUsageWindows(id as string),
    enabled: Boolean(id),
    staleTime: 30_000,
  });
}

export function useAccountUsageToday(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountUsageToday(id ?? ""),
    queryFn: () => adminApi.getAccountUsageToday(id as string),
    enabled: Boolean(id),
    staleTime: 30_000,
  });
}

// useAccountsUsageTodayBatch fetches today's usage for the currently visible
// account ids in a single round-trip — used by the accounts list "Today" column
// so a list of N rows costs one HTTP call instead of N. The id list is sorted
// for cache-key stability so consecutive renders share the same query.
export function useAccountsUsageTodayBatch(ids: string[]) {
  const sortedIds = [...ids].sort();
  return useQuery({
    queryKey: queryKeys.admin.accountsUsageTodayBatch(sortedIds),
    queryFn: () => adminApi.batchGetAccountsUsageToday(sortedIds),
    enabled: sortedIds.length > 0,
    staleTime: 30_000,
  });
}

export function useAccountUsageDaily(id: string | null, days = ACCOUNT_USAGE_DAILY_DAYS) {
  return useQuery({
    queryKey: queryKeys.admin.accountUsageDaily(id ?? "", days),
    queryFn: () => adminApi.getAccountUsageDaily(id as string, { days }),
    enabled: Boolean(id),
    staleTime: 60_000,
  });
}
