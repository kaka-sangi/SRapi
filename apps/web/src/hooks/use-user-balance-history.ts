"use client";

import { useQuery } from "@tanstack/react-query";
import { getAdminUserBalanceHistory } from "../../../../packages/sdk/typescript/src/index";
import type {
  BillingLedgerEntry,
  Id,
  Pagination,
} from "../../../../packages/sdk/typescript/src/types.gen";
import { configureSdkClient } from "@/lib/sdk-client";

/**
 * Balance-history (billing-ledger) query for the admin user-balance dialog.
 *
 * This lives in its own dedicated hook file (rather than the shared
 * `admin-queries/*` modules) so the usage- and users-page agents can both
 * consume it without contending on the same file. It talks to the generated
 * SDK directly, mirroring the `unwrapList` helper in `lib/admin-api/_shared`.
 *
 * The SDK endpoint (`GET /admin/users/{id}/balance-history`) is page-only — it
 * has no `type` query param — so the ledger-entry `type` filter is applied
 * client-side over the current page.
 */

export interface BalanceHistoryResult {
  entries: BillingLedgerEntry[];
  pagination?: Pagination;
}

/** Ledger entry `type` values, surfaced as the dialog's filter options. */
export const BALANCE_HISTORY_TYPES: BillingLedgerEntry["type"][] = [
  "usage_charge",
  "payment_credit",
  "refund",
  "adjustment",
  "compensation",
  "affiliate_transfer",
  "redeem_code_credit",
];

/** Credit-style movements that count toward "total recharged". */
const RECHARGE_TYPES = new Set<BillingLedgerEntry["type"]>([
  "payment_credit",
  "redeem_code_credit",
]);

export function useUserBalanceHistory(
  userId: Id | null,
  page: number,
  pageSize: number,
  enabled = true,
) {
  return useQuery({
    queryKey: ["admin", "user-balance-history", String(userId ?? ""), page, pageSize],
    enabled: enabled && userId != null && userId !== "",
    queryFn: async (): Promise<BalanceHistoryResult> => {
      configureSdkClient();
      const response = await getAdminUserBalanceHistory({
        path: { id: userId as Id },
        query: { page, page_size: pageSize },
        throwOnError: true,
      });
      const body = response.data;
      if (!body || !Array.isArray(body.data)) {
        throw new Error("Admin API returned an empty balance-history response.");
      }
      return { entries: body.data, pagination: body.pagination };
    },
  });
}

/** Sum of credit-style movements on the supplied entries (best-effort, page-scoped). */
export function sumRecharged(entries: BillingLedgerEntry[]): number {
  return entries.reduce((acc, entry) => {
    if (!RECHARGE_TYPES.has(entry.type)) return acc;
    const amount = Number(entry.amount);
    return Number.isFinite(amount) && amount > 0 ? acc + amount : acc;
  }, 0);
}
