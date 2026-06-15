"use client";

import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import type { AccountAvailabilitySummary } from "@/lib/sdk-types";

/**
 * The set of rolling windows the availability tab fans out across. Mirrors the
 * single-window selector's options (see `WINDOW_OPTIONS` in
 * `ops-channel-monitor.tsx`) so the multi-window and fallback views stay in
 * lock-step. Ordered ascending — the table renders one uptime column per entry.
 */
export const AVAILABILITY_WINDOWS = [7, 14, 30, 90] as const;

export type AvailabilityWindow = (typeof AVAILABILITY_WINDOWS)[number];

/**
 * One account's availability merged across every window. `uptime[d]` is the
 * `overall_uptime` reported for the `d`-day window (absent until that window's
 * query resolves). `status` / `last_checked_at` are point-in-time facts that
 * don't depend on the window, so we keep the freshest copy from any window.
 */
export interface AccountAvailabilityWindows {
  account_id: number;
  account_name: string;
  provider_id: number;
  status: string;
  last_checked_at?: string | null;
  /** Per-window uptime fraction (0..1), keyed by window length in days. */
  uptime: Partial<Record<AvailabilityWindow, number>>;
}

export interface AccountsAvailabilityWindowsResult {
  /** One row per account, with uptime filled in for each resolved window. */
  rows: AccountAvailabilityWindows[];
  /** True while any window's query has not yet produced data. */
  isPending: boolean;
  /** True while at least one window is fetching (initial or refetch). */
  isFetching: boolean;
  /** First error encountered across the windows, if any. */
  error: unknown;
  /** Refetch every window's query. */
  refetch: () => void;
}

/**
 * Fan out parallel `listAccountsAvailability(days)` calls across the supplied
 * windows and merge them into one row per account exposing uptime for every
 * window simultaneously. Reuses the same query keys / fetcher as
 * `useAccountsAvailability`, so the cache is shared with the single-window
 * fallback view — switching modes is free, and either view warms the other.
 */
export function useAccountsAvailabilityWindows(
  windows: readonly AvailabilityWindow[] = AVAILABILITY_WINDOWS,
): AccountsAvailabilityWindowsResult {
  const queries = useQueries({
    queries: windows.map((days) => ({
      queryKey: queryKeys.admin.accountsAvailability(days),
      queryFn: () => adminApi.listAccountsAvailability(days),
    })),
  });

  // `windows` is a stable module-level constant by default, but a caller may
  // pass a fresh array each render; depend on its contents, not its identity.
  const windowsKey = windows.join(",");
  // `queries` is a fresh array every render. Derive stable signature strings so
  // the memo only rebuilds when a window's data or load state actually moved —
  // the rule rejects these as inline (complex) dependency expressions.
  const dataSignature = queries.map((q) => q.dataUpdatedAt).join(",");
  const statusSignature = queries.map((q) => q.status).join(",");

  return useMemo(() => {
    const byAccount = new Map<number, AccountAvailabilityWindows>();

    windows.forEach((days, i) => {
      const data = queries[i]?.data?.data;
      if (!data) return;
      for (const summary of data) {
        mergeSummary(byAccount, days, summary);
      }
    });

    return {
      rows: [...byAccount.values()],
      isPending: queries.some((q) => q.isPending),
      isFetching: queries.some((q) => q.isFetching),
      error: queries.find((q) => q.error)?.error ?? null,
      refetch: () => {
        for (const q of queries) q.refetch();
      },
    };
    // Keyed on the derived signatures above; `queries`/`windows` are read inside
    // but their identities churn every render, so we depend on the signatures.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [windowsKey, dataSignature, statusSignature]);
}

function mergeSummary(
  byAccount: Map<number, AccountAvailabilityWindows>,
  days: AvailabilityWindow,
  summary: AccountAvailabilitySummary,
) {
  const existing = byAccount.get(summary.account_id);
  if (existing) {
    existing.uptime[days] = summary.overall_uptime;
    // Prefer a non-null last_checked_at and a definitive status if this window
    // happens to carry fresher facts than whatever we saw first.
    if (summary.last_checked_at && !existing.last_checked_at) {
      existing.last_checked_at = summary.last_checked_at;
    }
    return;
  }
  byAccount.set(summary.account_id, {
    account_id: summary.account_id,
    account_name: summary.account_name,
    provider_id: summary.provider_id,
    status: summary.status,
    last_checked_at: summary.last_checked_at,
    uptime: { [days]: summary.overall_uptime },
  });
}
