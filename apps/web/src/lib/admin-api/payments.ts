"use client";

import {
  batchAssignAdminUserSubscriptions,
  bulkImportAdminPricingRules,
  createAdminPaymentProvider,
  deleteAdminPaymentProvider,
  getAdminPaymentDashboard,
  createAdminPricingRule,
  updateAdminPricingRule,
  deleteAdminPricingRule,
  createAdminPromoCode,
  createAdminRedeemCode,
  deleteAdminRedeemCode,
  createAdminSubscriptionPlan,
  deleteAdminSubscriptionPlan,
  updateAdminSubscriptionPlan,
  createAdminUserSubscription,
  deleteAdminUserSubscription,
  deleteAdminPromoCode,
  getAdminRedeemCodeStats,
  batchDeleteAdminRedeemCodes,
  batchDisableAdminRedeemCodes,
  batchEnableAdminRedeemCodes,
  batchExtendAdminRedeemCodes,
  batchUpdateAdminRedeemCodes,
  batchGenerateAdminRedeemCodes,
  listAdminPaymentOrders,
  listAdminPaymentOrderAuditLogs,
  listAdminPaymentProviders,
  listAdminPricingRules,
  listAdminPromoCodes,
  listAdminPromoCodeUsages,
  listAdminRedeemCodes,
  listAdminSubscriptionPlans,
  listAdminUserSubscriptions,
  refundAdminPaymentOrder,
  testAdminPaymentProvider,
  updateAdminPaymentProvider,
  updateAdminPromoCode,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  AdminPaymentDashboard,
  AdminTestResult,
  BatchAssignAdminUserSubscriptionItem,
  BatchAssignAdminUserSubscriptionsResult,
  BatchOperationResult,
  BatchUpdateRedeemCodeItem,
  BatchUpdateRedeemCodesResult,
  PromoCodeUsage,
  BulkImportAdminPricingRulesData,
  BulkPricingRuleImportResult,
  CreateAdminPaymentProviderData,
  UpdateAdminPaymentProviderData,
  CreateAdminPricingRuleData,
  UpdateAdminPricingRuleData,
  CreateAdminSubscriptionPlanData,
  CreateAdminUserSubscriptionData,
  CreateRedeemCodeRequest,
  Id,
  ListAdminPaymentOrdersData,
  ListAdminPaymentProvidersData,
  ListAdminPricingRulesData,
  ListAdminPromoCodesData,
  ListAdminRedeemCodesData,
  ListAdminSubscriptionPlansData,
  ListAdminUserSubscriptionsData,
  PaymentOrder,
  PaymentAuditLog,
  PaymentProviderInstance,
  PricingRule,
  PromoCode,
  RedeemCode,
  RedeemCodeStats,
  SubscriptionPlan,
  UpdatePromoCodeRequest,
  UserSubscription,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const paymentsApi = {
  listPaymentProviders(
    query?: ListAdminPaymentProvidersData["query"],
  ): Promise<AdminListResult<PaymentProviderInstance>> {
    return unwrapList(() => listAdminPaymentProviders({ query, throwOnError: true }));
  },

  createPaymentProvider(
    body: CreateAdminPaymentProviderData["body"],
  ): Promise<PaymentProviderInstance> {
    return unwrapData(() => createAdminPaymentProvider({ body, throwOnError: true }));
  },
  updatePaymentProvider(
    id: Id,
    body: UpdateAdminPaymentProviderData["body"],
  ): Promise<PaymentProviderInstance> {
    return unwrapData(() =>
      updateAdminPaymentProvider({ path: { id }, body, throwOnError: true }),
    );
  },
  testPaymentProvider(id: Id): Promise<AdminTestResult> {
    return unwrapData(() => testAdminPaymentProvider({ path: { id }, throwOnError: true }));
  },
  deletePaymentProvider(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminPaymentProvider({ path: { id }, throwOnError: true }));
  },

  listPaymentOrders(
    query?: ListAdminPaymentOrdersData["query"],
  ): Promise<AdminListResult<PaymentOrder>> {
    return unwrapList(() => listAdminPaymentOrders({ query, throwOnError: true }));
  },

  listPaymentOrderAuditLogs(id: Id): Promise<AdminListResult<PaymentAuditLog>> {
    return unwrapList(() =>
      listAdminPaymentOrderAuditLogs({ path: { id }, throwOnError: true }),
    );
  },

  refundPaymentOrder(id: Id, body: Parameters<typeof refundAdminPaymentOrder>[0]["body"]): Promise<PaymentOrder> {
    return unwrapData(() =>
      refundAdminPaymentOrder({ path: { id }, body, throwOnError: true }),
    );
  },

  getPaymentDashboard(days?: number): Promise<AdminPaymentDashboard> {
    return unwrapData(() =>
      getAdminPaymentDashboard({
        query: typeof days === "number" ? { days } : undefined,
        throwOnError: true,
      }),
    );
  },

  listSubscriptionPlans(
    query?: ListAdminSubscriptionPlansData["query"],
  ): Promise<AdminListResult<SubscriptionPlan>> {
    return unwrapList(() => listAdminSubscriptionPlans({ query, throwOnError: true }));
  },

  createSubscriptionPlan(body: CreateAdminSubscriptionPlanData["body"]): Promise<SubscriptionPlan> {
    return unwrapData(() => createAdminSubscriptionPlan({ body, throwOnError: true }));
  },

  updateSubscriptionPlan(
    id: Id,
    body: Parameters<typeof updateAdminSubscriptionPlan>[0]["body"],
  ): Promise<SubscriptionPlan> {
    return unwrapData(() => updateAdminSubscriptionPlan({ path: { id }, body, throwOnError: true }));
  },
  deleteSubscriptionPlan(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminSubscriptionPlan({ path: { id }, throwOnError: true }));
  },

  listUserSubscriptions(
    query?: ListAdminUserSubscriptionsData["query"],
  ): Promise<AdminListResult<UserSubscription>> {
    return unwrapList(() => listAdminUserSubscriptions({ query, throwOnError: true }));
  },

  createUserSubscription(body: CreateAdminUserSubscriptionData["body"]): Promise<UserSubscription> {
    return unwrapData(() => createAdminUserSubscription({ body, throwOnError: true }));
  },

  deleteUserSubscription(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminUserSubscription({ path: { id }, throwOnError: true }));
  },

  // Bulk-assign a subscription plan to N users in one call (verbatim port of
  // sub2api's SubscriptionService.BulkAssignSubscription). Per-row outcome
  // reports created / reused / failed; idempotent on matching
  // (source_type, source_id). Per-row failures come back in result.errors[].
  batchAssignUserSubscriptions(
    items: BatchAssignAdminUserSubscriptionItem[],
  ): Promise<BatchAssignAdminUserSubscriptionsResult> {
    return unwrapData(() =>
      batchAssignAdminUserSubscriptions({ body: { items }, throwOnError: true }),
    );
  },

  listPricingRules(query?: ListAdminPricingRulesData["query"]): Promise<AdminListResult<PricingRule>> {
    return unwrapList(() => listAdminPricingRules({ query, throwOnError: true }));
  },

  createPricingRule(body: CreateAdminPricingRuleData["body"]): Promise<PricingRule> {
    return unwrapData(() => createAdminPricingRule({ body, throwOnError: true }));
  },

  updatePricingRule(
    id: Id,
    body: UpdateAdminPricingRuleData["body"],
  ): Promise<PricingRule> {
    return unwrapData(() => updateAdminPricingRule({ path: { id }, body, throwOnError: true }));
  },

  deletePricingRule(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminPricingRule({ path: { id }, throwOnError: true }));
  },

  bulkImportPricingRules(
    body: BulkImportAdminPricingRulesData["body"],
  ): Promise<BulkPricingRuleImportResult> {
    return unwrapData(() => bulkImportAdminPricingRules({ body, throwOnError: true }));
  },

  listRedeemCodes(query?: ListAdminRedeemCodesData["query"]): Promise<AdminListResult<RedeemCode>> {
    return unwrapList(() => listAdminRedeemCodes({ query, throwOnError: true }));
  },

  createRedeemCode(body: CreateRedeemCodeRequest): Promise<RedeemCode> {
    return unwrapData(() => createAdminRedeemCode({ body, throwOnError: true }));
  },

  batchGenerateRedeemCodes(
    body: Parameters<typeof batchGenerateAdminRedeemCodes>[0]["body"],
  ): Promise<RedeemCode[]> {
    return unwrapList(() => batchGenerateAdminRedeemCodes({ body, throwOnError: true })).then(
      (result) => result.data,
    );
  },

  // batchDisableRedeemCodes returns the full BatchOperationResult so the caller
  // can surface the per-item reason breakdown to the operator (admin_action /
  // already_disabled / expired / not_found) instead of a generic count. The
  // optional note is the operator's audit comment — written onto every code
  // that actually gets disabled.
  batchDisableRedeemCodes(
    ids: Id[],
    note?: string,
  ): Promise<BatchOperationResult> {
    const body: { ids: Id[]; note?: string } = { ids };
    if (note !== undefined) body.note = note;
    return unwrapData(() =>
      batchDisableAdminRedeemCodes({ body, throwOnError: true }),
    );
  },

  batchEnableRedeemCodes(ids: Id[]): Promise<unknown> {
    return unwrapData(() => batchEnableAdminRedeemCodes({ body: { ids }, throwOnError: true }));
  },

  batchExtendRedeemCodes(ids: Id[], expiresAt: string): Promise<unknown> {
    return unwrapData(() =>
      batchExtendAdminRedeemCodes({ body: { ids, expires_at: expiresAt }, throwOnError: true }),
    );
  },

  // Hard delete (vs the soft batch-disable above which keeps the audit row).
  // Reuses the BatchDisableRedeemCodesRequest body shape so the type is shared.
  batchDeleteRedeemCodes(ids: Id[]): Promise<unknown> {
    return unwrapData(() => batchDeleteAdminRedeemCodes({ body: { ids }, throwOnError: true }));
  },

  // Verbatim port of sub2api's RedeemService.BatchUpdate — per-row partial
  // update across N redeem codes in one call. Per-id failures surface in
  // result.errors[]. NotFound is idempotent server-side; already-redeemed
  // codes are rejected per sub2api's TouchesUsedSensitiveFields gate.
  batchUpdateRedeemCodes(
    items: BatchUpdateRedeemCodeItem[],
  ): Promise<BatchUpdateRedeemCodesResult> {
    return unwrapData(() =>
      batchUpdateAdminRedeemCodes({ body: { items }, throwOnError: true }),
    );
  },

  deleteRedeemCode(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminRedeemCode({ path: { id }, throwOnError: true }));
  },

  getRedeemStats(): Promise<RedeemCodeStats> {
    return unwrapData(() => getAdminRedeemCodeStats({ throwOnError: true }));
  },

  listPromoCodes(query?: ListAdminPromoCodesData["query"]): Promise<AdminListResult<PromoCode>> {
    return unwrapList(() => listAdminPromoCodes({ query, throwOnError: true }));
  },

  createPromoCode(body: Parameters<typeof createAdminPromoCode>[0]["body"]): Promise<PromoCode> {
    return unwrapData(() => createAdminPromoCode({ body, throwOnError: true }));
  },

  updatePromoCode(id: Id, body: UpdatePromoCodeRequest): Promise<PromoCode> {
    return unwrapData(() => updateAdminPromoCode({ path: { id }, body, throwOnError: true }));
  },

  deletePromoCode(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminPromoCode({ path: { id }, throwOnError: true }));
  },

  listPromoCodeUsages(id: Id): Promise<AdminListResult<PromoCodeUsage>> {
    return unwrapList(() => listAdminPromoCodeUsages({ path: { id }, throwOnError: true }));
  },
};
