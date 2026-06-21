"use client";

import { useQuery } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type B, type P, useAdminMutation } from "./_shared";

// ---- Subscriptions & commerce ----
export function useAdminSubscriptionPlans(params?: P<typeof adminApi.listSubscriptionPlans>) {
  return useQuery({
    queryKey: queryKeys.admin.subscriptionPlans(params),
    queryFn: () => adminApi.listSubscriptionPlans(params),
  });
}

export function useAdminSubscriptions(params?: P<typeof adminApi.listUserSubscriptions>) {
  return useQuery({
    queryKey: queryKeys.admin.userSubscriptions(params),
    queryFn: () => adminApi.listUserSubscriptions(params),
  });
}

export function useAdminPricingRules(params?: P<typeof adminApi.listPricingRules>) {
  return useQuery({
    queryKey: queryKeys.admin.pricingRules(params),
    queryFn: () => adminApi.listPricingRules(params),
  });
}

export function useAdminPricingRulePresets() {
  return useQuery({
    queryKey: queryKeys.admin.pricingRulePresets(),
    queryFn: () => adminApi.listPricingRulePresets(),
  });
}

// Subscription plans & user subscriptions
export function useCreateSubscriptionPlan() {
  return useAdminMutation(
    (body: P<typeof adminApi.createSubscriptionPlan>) => adminApi.createSubscriptionPlan(body),
    ["admin", "subscription-plans"],
  );
}
export function useUpdateSubscriptionPlan() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateSubscriptionPlan> }) =>
      adminApi.updateSubscriptionPlan(vars.id, vars.body),
    ["admin", "subscription-plans"],
  );
}
export function useDeleteSubscriptionPlan() {
  return useAdminMutation(
    (id: string) => adminApi.deleteSubscriptionPlan(id),
    ["admin", "subscription-plans"],
  );
}
export function useCreateUserSubscription() {
  return useAdminMutation(
    (body: P<typeof adminApi.createUserSubscription>) => adminApi.createUserSubscription(body),
    ["admin", "user-subscriptions"],
  );
}
export function useDeleteUserSubscription() {
  return useAdminMutation(
    (id: string) => adminApi.deleteUserSubscription(id),
    ["admin", "user-subscriptions"],
  );
}

// POST /admin/user-subscriptions/batch-assign — bulk-assign a subscription
// plan to N users in one call. Verbatim port of sub2api's
// SubscriptionService.BulkAssignSubscription. Per-row outcome reports
// created / reused / failed.
export function useBatchAssignUserSubscriptions() {
  return useAdminMutation(
    (items: P<typeof adminApi.batchAssignUserSubscriptions>) =>
      adminApi.batchAssignUserSubscriptions(items),
    ["admin", "user-subscriptions"],
  );
}

// Pricing rules
export function useCreatePricingRule() {
  return useAdminMutation(
    (body: P<typeof adminApi.createPricingRule>) => adminApi.createPricingRule(body),
    ["admin", "pricing-rules"],
    queryKeys.admin.gatewayResources(),
  );
}
export function useUpdatePricingRule() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updatePricingRule> }) =>
      adminApi.updatePricingRule(vars.id, vars.body),
    ["admin", "pricing-rules"],
    queryKeys.admin.gatewayResources(),
  );
}
export function useBulkImportPricingRules() {
  return useAdminMutation(
    (body: P<typeof adminApi.bulkImportPricingRules>) => adminApi.bulkImportPricingRules(body),
    ["admin", "pricing-rules"],
    queryKeys.admin.gatewayResources(),
  );
}
export function useInstallPricingRulePresets() {
  return useAdminMutation(
    (body: P<typeof adminApi.installPricingRulePresets> | undefined) =>
      adminApi.installPricingRulePresets(body),
    ["admin", "pricing-rules"],
    queryKeys.admin.gatewayResources(),
  );
}
export function useDeletePricingRule() {
  return useAdminMutation(
    (id: string) => adminApi.deletePricingRule(id),
    ["admin", "pricing-rules"],
    queryKeys.admin.gatewayResources(),
  );
}
