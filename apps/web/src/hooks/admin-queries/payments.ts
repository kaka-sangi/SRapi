"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type B, type P, useAdminMutation } from "./_shared";

export function useAdminPaymentOrders(params?: P<typeof adminApi.listPaymentOrders>) {
  return useQuery({
    queryKey: queryKeys.admin.paymentOrders(params),
    queryFn: () => adminApi.listPaymentOrders(params),
  });
}

export function useAdminPaymentDashboard(days?: number) {
  return useQuery({
    queryKey: queryKeys.admin.paymentDashboard(days),
    queryFn: () => adminApi.getPaymentDashboard(days),
  });
}

export function useAdminPaymentOrderAuditLogs(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.paymentOrderAuditLogs(id ?? ""),
    queryFn: () => adminApi.listPaymentOrderAuditLogs(id as string),
    enabled: Boolean(id),
  });
}

export function useAdminPaymentProviders(params?: P<typeof adminApi.listPaymentProviders>) {
  return useQuery({
    queryKey: queryKeys.admin.paymentProviders(params),
    queryFn: () => adminApi.listPaymentProviders(params),
  });
}

// ---- Promotions ----
export function useAdminPromoCodes(params?: P<typeof adminApi.listPromoCodes>) {
  return useQuery({
    queryKey: queryKeys.admin.promoCodes(params),
    queryFn: () => adminApi.listPromoCodes(params),
  });
}
export function useAdminPromoCodeUsages(id: string | null, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.admin.promoCodeUsages(id ?? ""),
    queryFn: () => adminApi.listPromoCodeUsages(id as string),
    enabled: enabled && Boolean(id),
  });
}

export function useAdminRedeemCodes(params?: P<typeof adminApi.listRedeemCodes>) {
  return useQuery({
    queryKey: queryKeys.admin.redeemCodes(params),
    queryFn: () => adminApi.listRedeemCodes(params),
  });
}

// Promo codes
export function useCreatePromoCode() {
  return useAdminMutation(
    (body: P<typeof adminApi.createPromoCode>) => adminApi.createPromoCode(body),
    ["admin", "promo-codes"],
  );
}
export function useUpdatePromoCode() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updatePromoCode> }) =>
      adminApi.updatePromoCode(vars.id, vars.body),
    ["admin", "promo-codes"],
  );
}
export function useDeletePromoCode() {
  return useAdminMutation(
    (id: string) => adminApi.deletePromoCode(id),
    ["admin", "promo-codes"],
  );
}

// Redeem codes
export function useCreateRedeemCode() {
  return useAdminMutation(
    (body: P<typeof adminApi.createRedeemCode>) => adminApi.createRedeemCode(body),
    ["admin", "redeem-codes"],
    queryKeys.admin.redeemStats(),
  );
}
export function useBatchGenerateRedeemCodes() {
  return useAdminMutation(
    (body: P<typeof adminApi.batchGenerateRedeemCodes>) => adminApi.batchGenerateRedeemCodes(body),
    ["admin", "redeem-codes"],
    queryKeys.admin.redeemStats(),
  );
}
export function useBatchDisableRedeemCodes() {
  return useAdminMutation(
    (ids: string[]) => adminApi.batchDisableRedeemCodes(ids),
    ["admin", "redeem-codes"],
    queryKeys.admin.redeemStats(),
  );
}
// Hard-delete a selection — the row is gone (vs disable which keeps history).
// Same invalidation set as batch-disable so both refresh the same list views.
export function useBatchDeleteRedeemCodes() {
  return useAdminMutation(
    (ids: string[]) => adminApi.batchDeleteRedeemCodes(ids),
    ["admin", "redeem-codes"],
    queryKeys.admin.redeemStats(),
  );
}
export function useDeleteRedeemCode() {
  return useAdminMutation(
    (id: string) => adminApi.deleteRedeemCode(id),
    ["admin", "redeem-codes"],
    queryKeys.admin.redeemStats(),
  );
}
export function useRedeemStats() {
  return useQuery({
    queryKey: queryKeys.admin.redeemStats(),
    queryFn: () => adminApi.getRedeemStats(),
  });
}

// Payment orders & providers
export function useRefundPaymentOrder() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.refundPaymentOrder> }) =>
      adminApi.refundPaymentOrder(vars.id, vars.body),
    ["admin", "payment-orders"],
  );
}
export function useCreatePaymentProvider() {
  return useAdminMutation(
    (body: P<typeof adminApi.createPaymentProvider>) => adminApi.createPaymentProvider(body),
    ["admin", "payment-providers"],
  );
}
export function useUpdatePaymentProvider() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updatePaymentProvider> }) =>
      adminApi.updatePaymentProvider(vars.id, vars.body),
    ["admin", "payment-providers"],
  );
}
export function useTestPaymentProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.testPaymentProvider(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "payment-providers"] }),
  });
}
export function useDeletePaymentProvider() {
  return useAdminMutation(
    (id: string) => adminApi.deletePaymentProvider(id),
    ["admin", "payment-providers"],
  );
}
