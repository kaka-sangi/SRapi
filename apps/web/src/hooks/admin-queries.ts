"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi, type AdminTimeRange } from "@/lib/admin-api";
import { diffAccountGroupIds } from "@/lib/admin-account-form";
import { queryKeys } from "@/lib/query-keys";
import type {
  AdminSettings,
  AnnouncementStatus,
  Id,
  PaymentOrderStatus,
  ProviderAccount,
  ProviderAccountStatus,
  Provider,
  ProxyDefinition,
  ProxyDefinitionStatus,
  PromoCodeStatus,
  RedeemCodeStatus,
  RiskControlConfig,
  SchedulerReplayRequest,
  SchedulerReplayResult,
  User,
  UserStatus,
  UsageAggregateDimension,
} from "../../../../packages/sdk/typescript/src/types.gen";

const DEFAULT_PAGE_SIZE = 25;

export function useAdminDashboardSnapshot(range?: AdminTimeRange) {
  return useQuery({
    queryKey: queryKeys.admin.dashboardSnapshot(range),
    queryFn: () => adminApi.getDashboardSnapshot(range),
  });
}

export function useAdminOps(
  range?: AdminTimeRange & { bucket?: "hour" | "day"; refetchIntervalMs?: number | false },
) {
  const bucket = range?.bucket ?? "hour";
  const queryRange = { start: range?.start, end: range?.end };
  const refetchInterval = range?.refetchIntervalMs ?? 30_000;

  return {
    overview: useQuery({
      queryKey: queryKeys.admin.opsOverview(queryRange),
      queryFn: () => adminApi.getOpsOverview(queryRange),
      refetchInterval,
    }),
    throughput: useQuery({
      queryKey: queryKeys.admin.opsThroughput({ ...queryRange, bucket }),
      queryFn: () => adminApi.getOpsThroughputTrend({ ...queryRange, bucket }),
      refetchInterval,
    }),
    errorTrend: useQuery({
      queryKey: queryKeys.admin.opsErrorTrend({ ...queryRange, bucket }),
      queryFn: () => adminApi.getOpsErrorTrend({ ...queryRange, bucket }),
      refetchInterval,
    }),
    errorDistribution: useQuery({
      queryKey: queryKeys.admin.opsErrorDistribution(queryRange),
      queryFn: () => adminApi.getOpsErrorDistribution(queryRange),
      refetchInterval,
    }),
    latencyHistogram: useQuery({
      queryKey: queryKeys.admin.opsLatencyHistogram(queryRange),
      queryFn: () => adminApi.getOpsLatencyHistogram(queryRange),
      refetchInterval,
    }),
    concurrency: useQuery({
      queryKey: queryKeys.admin.opsConcurrency(),
      queryFn: () => adminApi.getOpsConcurrency(),
      refetchInterval,
    }),
    logs: useQuery({
      queryKey: queryKeys.admin.opsLogs({ page: 1, page_size: 20 }),
      queryFn: () => adminApi.listOpsSystemLogs({ page: 1, page_size: 20 }),
      refetchInterval,
    }),
    alerts: useQuery({
      queryKey: queryKeys.admin.opsAlerts({ page: 1, page_size: 20 }),
      queryFn: () => adminApi.listOpsAlerts({ page: 1, page_size: 20 }),
      refetchInterval,
    }),
    realtimeSlots: useQuery({
      queryKey: queryKeys.admin.opsRealtimeSlots(),
      queryFn: () => adminApi.listOpsRealtimeSlots(),
      refetchInterval,
    }),
    slos: useQuery({
      queryKey: queryKeys.admin.opsSlos(),
      queryFn: () => adminApi.listOpsSlos(),
      refetchInterval: 60_000,
    }),
  };
}

export function useAdminOpsMutations() {
  const qc = useQueryClient();
  const invalidateSlos = () => {
    void qc.invalidateQueries({ queryKey: queryKeys.admin.opsSlos() });
    void qc.invalidateQueries({ queryKey: ["admin", "ops", "alerts"] });
  };

  return {
    acknowledgeAlert: useMutation({
      mutationFn: adminApi.acknowledgeAlert,
      onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "ops", "alerts"] }),
    }),
    createSlo: useMutation({
      mutationFn: adminApi.createOpsSlo,
      onSuccess: invalidateSlos,
    }),
    updateSlo: useMutation({
      mutationFn: ({ id, body }: { id: Id; body: Parameters<typeof adminApi.updateOpsSlo>[1] }) =>
        adminApi.updateOpsSlo(id, body),
      onSuccess: invalidateSlos,
    }),
  };
}

export function useAdminSchedulerReplay() {
  return useMutation<SchedulerReplayResult, Error, SchedulerReplayRequest>({
    mutationFn: (body) => adminApi.replaySchedulerStrategy(body),
  });
}

export function useAdminUsers(filters: { page?: number; q?: string; status?: UserStatus | "all" }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    q: filters.q || undefined,
    status: filters.status && filters.status !== "all" ? filters.status : undefined,
  };

  return useQuery({
    queryKey: queryKeys.admin.users(query),
    queryFn: () => adminApi.listUsers(query),
  });
}

export function useAdminUserMutations() {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin", "users"] });

  return {
    create: useMutation({
      mutationFn: adminApi.createUser,
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: Id; body: Parameters<typeof adminApi.updateUser>[1] }) =>
        adminApi.updateUser(id, body),
      onSuccess: invalidate,
    }),
    balance: useMutation({
      mutationFn: ({ id, body }: { id: Id; body: Parameters<typeof adminApi.updateUserBalance>[1] }) =>
        adminApi.updateUserBalance(id, body),
      onSuccess: invalidate,
    }),
    toggle: useMutation({
      mutationFn: (user: User) => adminApi.setUserEnabled(user),
      onSuccess: invalidate,
    }),
  };
}

export function useAdminProviders(filters: { page?: number; q?: string; status?: string } = {}) {
  const query = {
    page: filters.page ?? 1,
    page_size: 100,
    q: filters.q || undefined,
    status: filters.status && filters.status !== "all" ? filters.status : undefined,
  };
  return useQuery({
    queryKey: queryKeys.admin.providers(query),
    queryFn: () => adminApi.listProviders(query),
  });
}

export function useAdminProviderMutations() {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin", "providers"] });

  return {
    create: useMutation({
      mutationFn: adminApi.createProvider,
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: Id; body: Parameters<typeof adminApi.updateProvider>[1] }) =>
        adminApi.updateProvider(id, body),
      onSuccess: invalidate,
    }),
    test: useMutation({
      mutationFn: (provider: Provider) => adminApi.testProvider(provider.id),
    }),
  };
}

export function useAdminModels() {
  return useQuery({
    queryKey: queryKeys.admin.models(),
    queryFn: () => adminApi.listModels(),
  });
}

export function useAdminAccounts(filters: { page?: number; status?: ProviderAccountStatus | "all" }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    status: filters.status && filters.status !== "all" ? filters.status : undefined,
  };

  return useQuery({
    queryKey: queryKeys.admin.accounts(query),
    queryFn: () => adminApi.listAccounts(query),
  });
}

export function useAdminAccountMutations() {
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["admin", "accounts"] });
    void qc.invalidateQueries({ queryKey: queryKeys.admin.accountGroups() });
  };

  return {
    create: useMutation({
      mutationFn: adminApi.createAccount,
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: Id; body: Parameters<typeof adminApi.updateAccount>[1] }) =>
        adminApi.updateAccount(id, body),
      onSuccess: invalidate,
    }),
    toggle: useMutation({
      mutationFn: (account: ProviderAccount) => adminApi.setAccountStatus(account.id, account.status),
      onSuccess: invalidate,
    }),
    exportAccounts: useMutation({
      mutationFn: adminApi.exportAccounts,
    }),
    importAccounts: useMutation({
      mutationFn: adminApi.importAccounts,
      onSuccess: invalidate,
    }),
    batchUpdate: useMutation({
      mutationFn: adminApi.batchUpdateAccounts,
      onSuccess: invalidate,
    }),
    test: useMutation({
      mutationFn: (account: ProviderAccount) => adminApi.testAccount(account.id),
    }),
    discoverModels: useMutation({
      mutationFn: ({ account, persist }: { account: ProviderAccount; persist: boolean }) =>
        adminApi.discoverAccountModels(account.id, { persist }),
      onSuccess: invalidate,
    }),
    clearError: useMutation({
      mutationFn: (account: ProviderAccount) => adminApi.clearAccountError(account.id),
      onSuccess: invalidate,
    }),
    recover: useMutation({
      mutationFn: (account: ProviderAccount) => adminApi.recoverAccount(account.id),
      onSuccess: invalidate,
    }),
    syncGroups: useMutation({
      mutationFn: async ({
        accountId,
        currentGroupIds,
        nextGroupIds,
      }: {
        accountId: Id;
        currentGroupIds: Id[];
        nextGroupIds: Id[];
      }) => {
        const { add, remove } = diffAccountGroupIds(currentGroupIds, nextGroupIds);

        await Promise.all(remove.map((groupId) => adminApi.removeAccountFromGroup(accountId, groupId)));
        await Promise.all(add.map((groupId) => adminApi.addAccountToGroup(accountId, groupId)));
      },
      onSuccess: invalidate,
    }),
  };
}

export function useAdminAccountProxyQuality(accountIds: Id[]) {
  const ids = [...new Set(accountIds)].filter(Boolean);
  return useQuery({
    queryKey: ["admin", "account-proxy-quality", ids],
    queryFn: async () => {
      const entries = await Promise.all(
        ids.map(async (id) => [id, await adminApi.getAccountProxyQuality(id)] as const),
      );
      return Object.fromEntries(entries);
    },
    enabled: ids.length > 0,
  });
}

export function useAdminAccountRuntime(accountId: Id | null) {
  return {
    health: useQuery({
      queryKey: ["admin", "account-runtime", accountId, "health"],
      queryFn: () => adminApi.getAccountHealth(accountId ?? ""),
      enabled: Boolean(accountId),
    }),
    quota: useQuery({
      queryKey: ["admin", "account-runtime", accountId, "quota"],
      queryFn: () => adminApi.getAccountQuota(accountId ?? ""),
      enabled: Boolean(accountId),
    }),
    rpm: useQuery({
      queryKey: ["admin", "account-runtime", accountId, "rpm"],
      queryFn: () => adminApi.getAccountRpmStatus(accountId ?? ""),
      enabled: Boolean(accountId),
    }),
    proxyQuality: useQuery({
      queryKey: ["admin", "account-runtime", accountId, "proxy-quality"],
      queryFn: () => adminApi.getAccountProxyQuality(accountId ?? ""),
      enabled: Boolean(accountId),
    }),
  };
}

export function useAdminAccountProxyMutations() {
  const qc = useQueryClient();
  return {
    bind: useMutation({
      mutationFn: ({ id, proxyId }: { id: Id; proxyId: string | null }) =>
        adminApi.bindAccountProxy(id, proxyId),
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: ["admin", "accounts"] });
        void qc.invalidateQueries({ queryKey: ["admin", "account-proxy-quality"] });
      },
    }),
  };
}

export function useAdminProxies(filters: { page?: number; status?: ProxyDefinitionStatus | "all" }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    status: filters.status && filters.status !== "all" ? filters.status : undefined,
  };

  return useQuery({
    queryKey: queryKeys.admin.proxies(query),
    queryFn: () => adminApi.listProxies(query),
  });
}

export function useAdminProxyMutations() {
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["admin", "proxies"] });
    void qc.invalidateQueries({ queryKey: ["admin", "accounts"] });
    void qc.invalidateQueries({ queryKey: ["admin", "account-proxy-quality"] });
  };

  return {
    create: useMutation({
      mutationFn: adminApi.createProxy,
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: ProxyDefinition["id"]; body: Parameters<typeof adminApi.updateProxy>[1] }) =>
        adminApi.updateProxy(id, body),
      onSuccess: invalidate,
    }),
  };
}

export function useAdminAccountGroups() {
  return useQuery({
    queryKey: queryKeys.admin.accountGroups(),
    queryFn: () => adminApi.listAccountGroups(),
  });
}

export function useAdminAccountGroupMutations() {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: queryKeys.admin.accountGroups() });

  return {
    create: useMutation({
      mutationFn: adminApi.createAccountGroup,
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: Id; body: Parameters<typeof adminApi.updateAccountGroup>[1] }) =>
        adminApi.updateAccountGroup(id, body),
      onSuccess: invalidate,
    }),
  };
}

export function useAdminUsageLogs(filters: { page?: number; userId?: Id; model?: string }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    user_id: filters.userId || undefined,
    model: filters.model || undefined,
  };

  return useQuery({
    queryKey: queryKeys.admin.usageLogs(query),
    queryFn: () => adminApi.listUsageLogs(query),
  });
}

export function useAdminUsageAggregates(dimension: UsageAggregateDimension, range?: AdminTimeRange) {
  return useQuery({
    queryKey: queryKeys.admin.usageAggregates(dimension, range),
    queryFn: () => adminApi.listUsageAggregates(dimension, range),
  });
}

export function useAdminAffiliateInvites(filters: { page?: number }) {
  const query = { page: filters.page ?? 1, page_size: DEFAULT_PAGE_SIZE };
  return useQuery({
    queryKey: queryKeys.admin.affiliateInvites(query),
    queryFn: () => adminApi.listAffiliateInvites(query),
  });
}

export function useAdminAffiliateRebates(filters: { page?: number; userId?: Id }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    user_id: filters.userId || undefined,
  };
  return useQuery({
    queryKey: queryKeys.admin.affiliateRebates(query),
    queryFn: () => adminApi.listAffiliateRebates(query),
  });
}

export function useAdminAffiliateTransfers(filters: { page?: number; userId?: Id }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    user_id: filters.userId || undefined,
  };
  return useQuery({
    queryKey: queryKeys.admin.affiliateTransfers(query),
    queryFn: () => adminApi.listAffiliateTransfers(query),
  });
}

export function useAdminAnnouncements(filters: { page?: number; status?: AnnouncementStatus | "all" }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    status: filters.status && filters.status !== "all" ? filters.status : undefined,
  };

  return useQuery({
    queryKey: queryKeys.admin.announcements(query),
    queryFn: () => adminApi.listAnnouncements(query),
  });
}

export function useAdminAnnouncementMutations() {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin", "announcements"] });

  return {
    create: useMutation({
      mutationFn: adminApi.createAnnouncement,
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: Id; body: Parameters<typeof adminApi.updateAnnouncement>[1] }) =>
        adminApi.updateAnnouncement(id, body),
      onSuccess: invalidate,
    }),
    remove: useMutation({
      mutationFn: adminApi.deleteAnnouncement,
      onSuccess: invalidate,
    }),
  };
}

export function useAdminRedeemCodes(filters: { page?: number; status?: RedeemCodeStatus | "all" }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    status: filters.status && filters.status !== "all" ? filters.status : undefined,
  };

  return {
    codes: useQuery({
      queryKey: queryKeys.admin.redeemCodes(query),
      queryFn: () => adminApi.listRedeemCodes(query),
    }),
    stats: useQuery({
      queryKey: queryKeys.admin.redeemStats(),
      queryFn: () => adminApi.getRedeemStats(),
    }),
  };
}

export function useAdminRedeemMutations() {
  const qc = useQueryClient();
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["admin", "redeem-codes"] });
    void qc.invalidateQueries({ queryKey: queryKeys.admin.redeemStats() });
  };

  return {
    create: useMutation({
      mutationFn: adminApi.createRedeemCode,
      onSuccess: invalidate,
    }),
    batchGenerate: useMutation({
      mutationFn: adminApi.batchGenerateRedeemCodes,
      onSuccess: invalidate,
    }),
    batchDisable: useMutation({
      mutationFn: adminApi.batchDisableRedeemCodes,
      onSuccess: invalidate,
    }),
  };
}

export function useAdminPromoCodes(filters: { page?: number; status?: PromoCodeStatus | "all" }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    status: filters.status && filters.status !== "all" ? filters.status : undefined,
  };
  return useQuery({
    queryKey: queryKeys.admin.promoCodes(query),
    queryFn: () => adminApi.listPromoCodes(query),
  });
}

export function useAdminPromoCodeMutations() {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin", "promo-codes"] });

  return {
    create: useMutation({
      mutationFn: adminApi.createPromoCode,
      onSuccess: invalidate,
    }),
    update: useMutation({
      mutationFn: ({ id, body }: { id: Id; body: Parameters<typeof adminApi.updatePromoCode>[1] }) =>
        adminApi.updatePromoCode(id, body),
      onSuccess: invalidate,
    }),
    remove: useMutation({
      mutationFn: adminApi.deletePromoCode,
      onSuccess: invalidate,
    }),
  };
}

export function useAdminRiskControl(filters: { page?: number }) {
  const logsQuery = { page: filters.page ?? 1, page_size: DEFAULT_PAGE_SIZE };
  return {
    config: useQuery({
      queryKey: queryKeys.admin.riskConfig(),
      queryFn: () => adminApi.getRiskConfig(),
    }),
    status: useQuery({
      queryKey: queryKeys.admin.riskStatus(),
      queryFn: () => adminApi.getRiskStatus(),
    }),
    logs: useQuery({
      queryKey: queryKeys.admin.riskLogs(logsQuery),
      queryFn: () => adminApi.listRiskLogs(logsQuery),
    }),
  };
}

export function useAdminRiskMutations() {
  const qc = useQueryClient();
  return {
    updateConfig: useMutation({
      mutationFn: (body: RiskControlConfig) => adminApi.updateRiskConfig(body),
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: queryKeys.admin.riskConfig() });
        void qc.invalidateQueries({ queryKey: queryKeys.admin.riskStatus() });
      },
    }),
  };
}

export function useAdminSettings() {
  return useQuery({
    queryKey: queryKeys.admin.settings(),
    queryFn: () => adminApi.getSettings(),
  });
}

export function useAdminSettingsMutation() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: AdminSettings) => adminApi.updateSettings(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.admin.settings() }),
  });
}

export function useAdminSubscriptionPlans(filters: { page?: number }) {
  const query = { page: filters.page ?? 1, page_size: DEFAULT_PAGE_SIZE };
  return useQuery({
    queryKey: queryKeys.admin.subscriptionPlans(query),
    queryFn: () => adminApi.listSubscriptionPlans(query),
  });
}

export function useAdminSubscriptionPlanMutations() {
  const qc = useQueryClient();
  return {
    create: useMutation({
      mutationFn: adminApi.createSubscriptionPlan,
      onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "subscription-plans"] }),
    }),
  };
}

export function useAdminUserSubscriptions(filters: { page?: number; userId?: Id }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    user_id: filters.userId || undefined,
  };
  return useQuery({
    queryKey: queryKeys.admin.userSubscriptions(query),
    queryFn: () => adminApi.listUserSubscriptions(query),
  });
}

export function useAdminUserSubscriptionMutations() {
  const qc = useQueryClient();
  return {
    create: useMutation({
      mutationFn: adminApi.createUserSubscription,
      onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "user-subscriptions"] }),
    }),
  };
}

export function useAdminPricingRules(filters: { page?: number }) {
  const query = { page: filters.page ?? 1, page_size: DEFAULT_PAGE_SIZE };
  return useQuery({
    queryKey: queryKeys.admin.pricingRules(query),
    queryFn: () => adminApi.listPricingRules(query),
  });
}

export function useAdminPricingRuleMutations() {
  const qc = useQueryClient();
  return {
    create: useMutation({
      mutationFn: adminApi.createPricingRule,
      onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "pricing-rules"] }),
    }),
    bulkImport: useMutation({
      mutationFn: adminApi.bulkImportPricingRules,
      onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "pricing-rules"] }),
    }),
  };
}

export function useAdminPaymentOrders(filters: { page?: number; status?: PaymentOrderStatus | "all" }) {
  const query = {
    page: filters.page ?? 1,
    page_size: DEFAULT_PAGE_SIZE,
    status: filters.status && filters.status !== "all" ? filters.status : undefined,
  };
  return useQuery({
    queryKey: queryKeys.admin.paymentOrders(query),
    queryFn: () => adminApi.listPaymentOrders(query),
  });
}

export function useAdminPaymentOrderMutations() {
  const qc = useQueryClient();
  return {
    refund: useMutation({
      mutationFn: ({ id, body }: { id: Id; body: Parameters<typeof adminApi.refundPaymentOrder>[1] }) =>
        adminApi.refundPaymentOrder(id, body),
      onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "payment-orders"] }),
    }),
  };
}

export function useAdminPaymentProviders(filters: { page?: number }) {
  const query = { page: filters.page ?? 1, page_size: DEFAULT_PAGE_SIZE };
  return useQuery({
    queryKey: queryKeys.admin.paymentProviders(query),
    queryFn: () => adminApi.listPaymentProviders(query),
  });
}

export function useAdminPaymentProviderMutations() {
  const qc = useQueryClient();
  return {
    create: useMutation({
      mutationFn: adminApi.createPaymentProvider,
      onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "payment-providers"] }),
    }),
  };
}
