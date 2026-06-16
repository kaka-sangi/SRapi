"use client";

import { useQuery } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type B, type P, useAdminMutation } from "./_shared";

// ---- Affiliates ----
export function useAffiliateInvites(params?: P<typeof adminApi.listAffiliateInvites>) {
  return useQuery({
    queryKey: queryKeys.admin.affiliateInvites(params),
    queryFn: () => adminApi.listAffiliateInvites(params),
  });
}

export function useAffiliateRebates(params?: P<typeof adminApi.listAffiliateRebates>) {
  return useQuery({
    queryKey: queryKeys.admin.affiliateRebates(params),
    queryFn: () => adminApi.listAffiliateRebates(params),
  });
}

export function useAffiliateTransfers(params?: P<typeof adminApi.listAffiliateTransfers>) {
  return useQuery({
    queryKey: queryKeys.admin.affiliateTransfers(params),
    queryFn: () => adminApi.listAffiliateTransfers(params),
  });
}

// Withdrawals are fetched via the transfers endpoint (same shape) and filtered
// client-side to type === "withdraw". Approve/cancel mutations invalidate this
// cache as well as the broader transfers cache.
export function useAffiliateWithdrawals(params?: P<typeof adminApi.listAffiliateWithdrawals>) {
  return useQuery({
    queryKey: queryKeys.admin.affiliateWithdrawals(params),
    queryFn: () => adminApi.listAffiliateWithdrawals(params),
  });
}

export function useApproveWithdrawal() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.approveWithdrawal> }) =>
      adminApi.approveWithdrawal(vars.id, vars.body),
    ["admin", "affiliates", "withdrawals"],
    ["admin", "affiliates", "transfers"],
  );
}

export function useCancelWithdrawal() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.cancelWithdrawal> }) =>
      adminApi.cancelWithdrawal(vars.id, vars.body),
    ["admin", "affiliates", "withdrawals"],
    ["admin", "affiliates", "transfers"],
  );
}

export function useAffiliateRules(params?: P<typeof adminApi.listAffiliateRules>) {
  return useQuery({
    queryKey: queryKeys.admin.affiliateRules(params),
    queryFn: () => adminApi.listAffiliateRules(params),
  });
}

export function useCreateAffiliateRule() {
  return useAdminMutation(
    (body: P<typeof adminApi.createAffiliateRule>) => adminApi.createAffiliateRule(body),
    ["admin", "affiliates", "rules"],
  );
}

export function useUpdateAffiliateRule() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateAffiliateRule> }) =>
      adminApi.updateAffiliateRule(vars.id, vars.body),
    ["admin", "affiliates", "rules"],
  );
}
