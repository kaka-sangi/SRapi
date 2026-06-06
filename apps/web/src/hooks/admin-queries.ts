"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import type { User, ProviderAccountStatus } from "../../../../packages/sdk/typescript/src/types.gen";

/**
 * Admin data hooks. Pages consume ONLY these (never useEffect+fetch).
 * Everything routes through `adminApi` (lib/admin-api.ts) → generated SDK.
 * All endpoints are admin-only and 403 for regular users — the AppShell role
 * gate keeps non-admins off these pages entirely.
 *
 * Param types are derived from each adminApi method so they always match the
 * generated SDK query shape (status enums etc.).
 */

type P<F extends (...a: never[]) => unknown> = Parameters<F>[0];

// ---- Dashboard ----
export function useAdminDashboard(range?: P<typeof adminApi.getDashboardSnapshot>) {
  return useQuery({
    queryKey: queryKeys.admin.dashboardSnapshot(range),
    queryFn: () => adminApi.getDashboardSnapshot(range),
    // Polling is driven by the page's AutoRefreshControl (single source of truth).
  });
}

// ---- Users ----
export function useAdminUsers(params?: P<typeof adminApi.listUsers>) {
  return useQuery({
    queryKey: queryKeys.admin.users(params),
    queryFn: () => adminApi.listUsers(params),
  });
}

type UserList = Awaited<ReturnType<typeof adminApi.listUsers>>;
const USERS_KEY = ["admin", "users"] as const;

/** Optimistically patch every cached users page (any filter/sort/page), and
 *  return the prior snapshots so onError can roll them all back. */
function optimisticUsers(qc: ReturnType<typeof useQueryClient>, apply: (u: User) => User) {
  const prev = qc.getQueriesData<UserList>({ queryKey: USERS_KEY });
  qc.setQueriesData<UserList>({ queryKey: USERS_KEY }, (old) =>
    old ? { ...old, data: old.data.map(apply) } : old,
  );
  return prev;
}

export function useSetUserEnabled() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (user: User) => adminApi.setUserEnabled(user),
    // 联动: the row's status badge + dim flips the instant you click, well
    // before the server responds. onError restores; onSettled reconciles.
    onMutate: async (user) => {
      await qc.cancelQueries({ queryKey: USERS_KEY });
      const next = user.status === "disabled" ? "active" : "disabled";
      const prev = optimisticUsers(qc, (u) => (u.id === user.id ? { ...u, status: next } : u));
      return { prev };
    },
    onError: (_e, _v, ctx) => ctx?.prev?.forEach(([key, data]) => qc.setQueryData(key, data)),
    onSettled: () => qc.invalidateQueries({ queryKey: USERS_KEY }),
  });
}

export function useBulkSetUsersEnabled() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ ids, enabled }: { ids: string[]; enabled: boolean }) =>
      Promise.all(ids.map((id) => adminApi.setUserEnabledById(id, enabled))),
    onMutate: async ({ ids, enabled }) => {
      await qc.cancelQueries({ queryKey: USERS_KEY });
      const next = enabled ? "active" : "disabled";
      const set = new Set(ids);
      const prev = optimisticUsers(qc, (u) => (set.has(u.id) ? { ...u, status: next } : u));
      return { prev };
    },
    onError: (_e, _v, ctx) => ctx?.prev?.forEach(([key, data]) => qc.setQueryData(key, data)),
    onSettled: () => qc.invalidateQueries({ queryKey: USERS_KEY }),
  });
}

// ---- Provider accounts ----
export function useAdminAccounts(params?: P<typeof adminApi.listAccounts>) {
  return useQuery({
    queryKey: queryKeys.admin.accounts(params),
    queryFn: () => adminApi.listAccounts(params),
  });
}

type AccountList = Awaited<ReturnType<typeof adminApi.listAccounts>>;

export function useSetAccountStatus() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: ProviderAccountStatus }) =>
      adminApi.setAccountStatus(id, status),
    // 联动: flip the account row instantly; rollback on error, reconcile on settle.
    onMutate: async ({ id, status }) => {
      await qc.cancelQueries({ queryKey: ["admin", "accounts"] });
      const prev = qc.getQueriesData<AccountList>({ queryKey: ["admin", "accounts"] });
      qc.setQueriesData<AccountList>({ queryKey: ["admin", "accounts"] }, (old) =>
        old ? { ...old, data: old.data.map((a) => (a.id === id ? { ...a, status } : a)) } : old,
      );
      return { prev };
    },
    onError: (_e, _v, ctx) => ctx?.prev?.forEach(([key, data]) => qc.setQueryData(key, data)),
    onSettled: () => qc.invalidateQueries({ queryKey: ["admin", "accounts"] }),
  });
}

export function useTestAccount() {
  const qc = useQueryClient();
  return useMutation({
    // A connectivity test can flip the account status (e.g. needs_reauth/dead on
    // auth failure), so refetch the list to reflect the new state.
    mutationFn: (id: string) => adminApi.testAccount(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "accounts"] }),
  });
}

// Per-account diagnostics — fetched on demand (when a detail view opens) so the
// list stays cheap; `enabled` gates each query behind a selected account id.
export function useAccountHealth(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountHealth(id ?? ""),
    queryFn: () => adminApi.getAccountHealth(id as string),
    enabled: Boolean(id),
  });
}
export function useAccountQuota(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountQuota(id ?? ""),
    queryFn: () => adminApi.getAccountQuota(id as string),
    enabled: Boolean(id),
  });
}
export function useFetchAccountQuota() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.fetchAccountQuota(id),
    // A live pull refreshes the stored snapshots, so re-read them for this account.
    onSuccess: (_data, id) =>
      qc.invalidateQueries({ queryKey: queryKeys.admin.accountQuota(id) }),
  });
}
export function useAccountRpmStatus(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountRpm(id ?? ""),
    queryFn: () => adminApi.getAccountRpmStatus(id as string),
    enabled: Boolean(id),
  });
}
export function useAccountProxyQuality(id: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountProxyQuality(id ?? ""),
    queryFn: () => adminApi.getAccountProxyQuality(id as string),
    enabled: Boolean(id),
  });
}

// ---- Account groups ----
export function useAdminGroups() {
  return useQuery({
    queryKey: queryKeys.admin.accountGroups(),
    queryFn: () => adminApi.listAccountGroups(),
  });
}

// Members of one group — fetched on demand (when the manage-members dialog opens).
// Keyed under the account-groups prefix so add/remove member mutations refetch it.
export function useGroupMembers(groupId: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.accountGroupMembers(groupId ?? ""),
    queryFn: () => adminApi.listAccountGroupMembers(groupId as string),
    enabled: Boolean(groupId),
  });
}

// ---- Proxies ----
export function useAdminProxies(params?: P<typeof adminApi.listProxies>) {
  return useQuery({
    queryKey: queryKeys.admin.proxies(params),
    queryFn: () => adminApi.listProxies(params),
  });
}

// ---- Providers & models (reference data) ----
export function useAdminProviders(params?: P<typeof adminApi.listProviders>) {
  return useQuery({
    queryKey: queryKeys.admin.providers(params),
    queryFn: () => adminApi.listProviders(params),
  });
}

export function useAdminModels(params?: P<typeof adminApi.listModels>) {
  return useQuery({
    queryKey: queryKeys.admin.models(params),
    queryFn: () => adminApi.listModels(params),
  });
}

// ---- Rate limits (per-model & per-account-group TPM/RPM/concurrency) ----
// The API has no per-id GET, so these list-all and the UI joins by id.
export function useModelRateLimits() {
  return useQuery({
    queryKey: ["admin", "model-rate-limits"],
    queryFn: () => adminApi.listModelRateLimits(),
  });
}
export function useUpsertModelRateLimit() {
  return useAdminMutation(
    (body: P<typeof adminApi.upsertModelRateLimit>) => adminApi.upsertModelRateLimit(body),
    ["admin", "model-rate-limits"],
  );
}
export function useDeleteModelRateLimit() {
  return useAdminMutation(
    (modelId: string) => adminApi.deleteModelRateLimit(modelId),
    ["admin", "model-rate-limits"],
  );
}
export function useGroupRateLimits() {
  return useQuery({
    queryKey: ["admin", "group-rate-limits"],
    queryFn: () => adminApi.listGroupRateLimits(),
  });
}
export function useUpsertGroupRateLimit() {
  return useAdminMutation(
    (body: P<typeof adminApi.upsertGroupRateLimit>) => adminApi.upsertGroupRateLimit(body),
    ["admin", "group-rate-limits"],
  );
}
export function useDeleteGroupRateLimit() {
  return useAdminMutation(
    (groupId: string) => adminApi.deleteGroupRateLimit(groupId),
    ["admin", "group-rate-limits"],
  );
}

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

export function useAdminPaymentOrders(params?: P<typeof adminApi.listPaymentOrders>) {
  return useQuery({
    queryKey: queryKeys.admin.paymentOrders(params),
    queryFn: () => adminApi.listPaymentOrders(params),
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

// ---- Announcements ----
export function useAdminAnnouncements(params?: P<typeof adminApi.listAnnouncements>) {
  return useQuery({
    queryKey: queryKeys.admin.announcements(params),
    queryFn: () => adminApi.listAnnouncements(params),
  });
}
export function useAnnouncementReadStatus(id: string | null, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.admin.announcementReads(id ?? ""),
    queryFn: () => adminApi.getAnnouncementReadStatus(id as string),
    enabled: enabled && Boolean(id),
  });
}

// ---- Error passthrough rules ----
export function useErrorPassthroughRules() {
  return useQuery({
    queryKey: queryKeys.admin.errorPassthroughRules(),
    queryFn: () => adminApi.listErrorPassthroughRules(),
  });
}

// ---- Payload transform rules ----
export function usePayloadRules() {
  return useQuery({
    queryKey: queryKeys.admin.payloadRules(),
    queryFn: () => adminApi.listPayloadRules(),
  });
}

// ---- Scheduled account health-test plans ----
export function useScheduledTestPlans() {
  return useQuery({
    queryKey: queryKeys.admin.scheduledTestPlans(),
    queryFn: () => adminApi.listScheduledTestPlans(),
  });
}

export function useScheduledTestPlanRuns(planId: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.scheduledTestPlanRuns(planId ?? ""),
    queryFn: () => adminApi.listScheduledTestPlanRuns(planId ?? ""),
    enabled: Boolean(planId),
  });
}

// ---- Channel monitors (synthetic probes) ----
export function useChannelMonitors() {
  return useQuery({
    queryKey: queryKeys.admin.channelMonitors(),
    queryFn: () => adminApi.listChannelMonitors(),
  });
}

export function useChannelMonitorRuns(monitorId: string | null) {
  return useQuery({
    queryKey: queryKeys.admin.channelMonitorRuns(monitorId ?? ""),
    queryFn: () => adminApi.listChannelMonitorRuns(monitorId as string),
    enabled: Boolean(monitorId),
  });
}

export function useChannelMonitorTemplates() {
  return useQuery({
    queryKey: queryKeys.admin.channelMonitorTemplates(),
    queryFn: () => adminApi.listChannelMonitorTemplates(),
  });
}

// ---- TLS fingerprint profiles ----
export function useTlsProfiles() {
  return useQuery({
    queryKey: queryKeys.admin.tlsProfiles(),
    queryFn: () => adminApi.listTlsProfiles(),
  });
}

// Custom RBAC roles (read + create; no PATCH/DELETE endpoint yet)
export function useAdminRoles() {
  return useQuery({
    queryKey: queryKeys.admin.roles(),
    queryFn: () => adminApi.listRoles(),
  });
}
export function useCreateAdminRole() {
  return useAdminMutation(
    (body: P<typeof adminApi.createRole>) => adminApi.createRole(body),
    ["admin", "roles"],
  );
}
export function useUpdateAdminRole() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateRole> }) =>
      adminApi.updateRole(vars.id, vars.body),
    ["admin", "roles"],
  );
}
export function useDeleteAdminRole() {
  return useAdminMutation((id: string) => adminApi.deleteRole(id), ["admin", "roles"]);
}
export function useAdminApiKeys(params?: P<typeof adminApi.listAdminApiKeys>) {
  return useQuery({
    queryKey: queryKeys.admin.apiKeys(params),
    queryFn: () => adminApi.listAdminApiKeys(params),
  });
}
export function useUpdateAdminApiKey() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateAdminApiKey> }) =>
      adminApi.updateAdminApiKey(vars.id, vars.body),
    ["admin", "api-keys"],
  );
}
export function useAdminApiKeyUsage(id: string | null, days: number, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.admin.apiKeyUsage(id ?? "", days),
    queryFn: () => adminApi.getAdminApiKeyUsage(id as string, days),
    enabled: enabled && Boolean(id),
  });
}

// ---- Custom user attribute definitions ----
export function useUserAttributeDefinitions() {
  return useQuery({
    queryKey: queryKeys.admin.userAttributes(),
    queryFn: () => adminApi.listUserAttributeDefinitions(),
  });
}

// ---- Notification email templates ----
export function useNotificationEmailTemplates() {
  return useQuery({
    queryKey: queryKeys.admin.notificationEmailTemplates(),
    queryFn: () => adminApi.listNotificationEmailTemplates(),
  });
}

// ---- Account availability (monitoring) ----
export function useAccountsAvailability(days?: number) {
  return useQuery({
    queryKey: queryKeys.admin.accountsAvailability(days),
    queryFn: () => adminApi.listAccountsAvailability(days),
  });
}

// ---- Per-user platform spend quotas ----
export function useUserPlatformQuotas(userId: string, enabled = true) {
  return useQuery({
    queryKey: queryKeys.admin.userPlatformQuotas(userId),
    queryFn: () => adminApi.listUserPlatformQuotas(userId),
    enabled: enabled && Boolean(userId),
  });
}
export function useUpsertUserPlatformQuota() {
  return useAdminMutation(
    (vars: { userId: string; body: B<typeof adminApi.upsertUserPlatformQuota> }) =>
      adminApi.upsertUserPlatformQuota(vars.userId, vars.body),
    ["admin", "user-platform-quotas"],
  );
}
export function useDeleteUserPlatformQuota() {
  return useAdminMutation(
    (vars: { userId: string; platform: string }) =>
      adminApi.deleteUserPlatformQuota(vars.userId, vars.platform),
    ["admin", "user-platform-quotas"],
  );
}

// ---- Ops ----
export function useOpsOverview(range?: P<typeof adminApi.getOpsOverview>) {
  return useQuery({
    queryKey: queryKeys.admin.opsOverview(range),
    queryFn: () => adminApi.getOpsOverview(range),
    refetchInterval: 30_000,
  });
}

export function useOpsSlos() {
  return useQuery({
    queryKey: queryKeys.admin.opsSlos(),
    queryFn: () => adminApi.listOpsSlos(),
    refetchInterval: 30_000,
  });
}
export function useCleanupOpsSystemLogs() {
  return useAdminMutation(
    (body: P<typeof adminApi.cleanupOpsSystemLogs>) => adminApi.cleanupOpsSystemLogs(body),
    ["admin", "ops"],
  );
}
// Operator on-demand usage-record cleanup. Invalidates the usage-* queries so
// the admin usage view refreshes once records are actually deleted.
export function useCleanupAdminUsage() {
  return useAdminMutation(
    (body: P<typeof adminApi.cleanupUsage>) => adminApi.cleanupUsage(body),
    ["admin", "usage-logs"],
  );
}
export function useUpdateOpsSettings() {
  return useAdminMutation(
    (body: P<typeof adminApi.updateOpsSettings>) => adminApi.updateOpsSettings(body),
    ["admin", "ops"],
  );
}

export function useOpsAlerts(params?: P<typeof adminApi.listOpsAlerts>) {
  return useQuery({
    queryKey: queryKeys.admin.opsAlerts(params),
    queryFn: () => adminApi.listOpsAlerts(params),
  });
}

export function useOpsThroughput(params?: P<typeof adminApi.getOpsThroughputTrend>) {
  return useQuery({
    queryKey: queryKeys.admin.opsThroughput(params),
    queryFn: () => adminApi.getOpsThroughputTrend(params),
    refetchInterval: 30_000,
  });
}

export function useOpsErrorTrend(params?: P<typeof adminApi.getOpsErrorTrend>) {
  return useQuery({
    queryKey: queryKeys.admin.opsErrorTrend(params),
    queryFn: () => adminApi.getOpsErrorTrend(params),
    refetchInterval: 30_000,
  });
}

export function useAdminUsageDaily(params?: P<typeof adminApi.listUsageDaily>) {
  return useQuery({
    queryKey: queryKeys.admin.usageDaily(params),
    queryFn: () => adminApi.listUsageDaily(params),
  });
}

export function useAdminUsageAggregates(
  dimension: P<typeof adminApi.listUsageAggregates>,
  range?: B<typeof adminApi.listUsageAggregates>,
) {
  return useQuery({
    queryKey: queryKeys.admin.usageAggregates(dimension, range),
    queryFn: () => adminApi.listUsageAggregates(dimension, range),
  });
}

// ---- Risk control ----
export function useRiskStatus() {
  return useQuery({
    queryKey: queryKeys.admin.riskStatus(),
    queryFn: () => adminApi.getRiskStatus(),
    refetchInterval: 30_000,
  });
}

export function useRiskLogs(params?: P<typeof adminApi.listRiskLogs>) {
  return useQuery({
    queryKey: queryKeys.admin.riskLogs(params),
    queryFn: () => adminApi.listRiskLogs(params),
  });
}

// ---- Audit & billing ----
export function useAuditLogs(params?: P<typeof adminApi.listAuditLogs>) {
  return useQuery({
    queryKey: queryKeys.admin.auditLogs(params),
    queryFn: () => adminApi.listAuditLogs(params),
  });
}
export function useOutboxEvents(params?: P<typeof adminApi.listOutboxEvents>) {
  return useQuery({
    queryKey: queryKeys.admin.outboxEvents(params),
    queryFn: () => adminApi.listOutboxEvents(params),
  });
}

export function useBillingLedger(params?: P<typeof adminApi.listBillingLedger>) {
  return useQuery({
    queryKey: queryKeys.admin.billingLedger(params),
    queryFn: () => adminApi.listBillingLedger(params),
  });
}

// ---- Admin usage ----
export function useAdminUsageLogs(params?: P<typeof adminApi.listUsageLogs>) {
  return useQuery({
    queryKey: queryKeys.admin.usageLogs(params),
    queryFn: () => adminApi.listUsageLogs(params),
  });
}

// ---- Settings ----
export function useAdminSettings() {
  return useQuery({
    queryKey: queryKeys.admin.settings(),
    queryFn: () => adminApi.getSettings(),
  });
}

// ---- Admin AI copilot ----
export function useAdminCopilotConfig() {
  return useQuery({
    queryKey: queryKeys.admin.copilotConfig(),
    queryFn: () => adminApi.getCopilotConfig(),
  });
}

// ---- Risk control config (read) ----
export function useRiskConfig() {
  return useQuery({
    queryKey: queryKeys.admin.riskConfig(),
    queryFn: () => adminApi.getRiskConfig(),
  });
}

// ============================================================
// Mutations (create / update / delete). Each invalidates the broad
// ["admin", <resource>] prefix so every param-scoped query variant
// refetches — the pattern established by useSetAccountStatus above.
// ============================================================

/** Second positional arg of a method, used for `update(id, body)` shapes. */
type B<F extends (...a: never[]) => unknown> = Parameters<F>[1];

function useAdminMutation<TVars, TData>(
  mutationFn: (vars: TVars) => Promise<TData>,
  invalidate: readonly unknown[],
) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => qc.invalidateQueries({ queryKey: invalidate }),
  });
}

// Users
export function useCreateAdminUser() {
  return useAdminMutation(
    (body: P<typeof adminApi.createUser>) => adminApi.createUser(body),
    ["admin", "users"],
  );
}
export function useUpdateAdminUser() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateUser> }) =>
      adminApi.updateUser(vars.id, vars.body),
    ["admin", "users"],
  );
}
export function useUpdateUserBalance() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateUserBalance> }) =>
      adminApi.updateUserBalance(vars.id, vars.body),
    ["admin", "users"],
  );
}

// Provider accounts
export function useCreateAccount() {
  return useAdminMutation(
    (body: P<typeof adminApi.createAccount>) => adminApi.createAccount(body),
    ["admin", "accounts"],
  );
}
export function useUpdateAccount() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateAccount> }) =>
      adminApi.updateAccount(vars.id, vars.body),
    ["admin", "accounts"],
  );
}
export function useBindAccountProxy() {
  return useAdminMutation(
    (vars: { id: string; proxyId: string | null }) =>
      adminApi.bindAccountProxy(vars.id, vars.proxyId),
    ["admin", "accounts"],
  );
}
export function useClearAccountError() {
  return useAdminMutation((id: string) => adminApi.clearAccountError(id), ["admin", "accounts"]);
}
export function useRecoverAccount() {
  return useAdminMutation((id: string) => adminApi.recoverAccount(id), ["admin", "accounts"]);
}
export function useDiscoverAccountModels() {
  return useAdminMutation(
    (vars: { id: string; body?: B<typeof adminApi.discoverAccountModels> }) =>
      adminApi.discoverAccountModels(vars.id, vars.body),
    ["admin", "accounts"],
  );
}
export function useImportAccounts() {
  return useAdminMutation(
    (body: P<typeof adminApi.importAccounts>) => adminApi.importAccounts(body),
    ["admin", "accounts"],
  );
}
/** Import Codex/ChatGPT desktop session blobs as upstream codex_cli accounts. */
export function useImportCodexSession() {
  return useAdminMutation(
    (body: P<typeof adminApi.importCodexSession>) => adminApi.importCodexSession(body),
    ["admin", "accounts"],
  );
}
export function useBatchUpdateAccounts() {
  return useAdminMutation(
    (body: P<typeof adminApi.batchUpdateAccounts>) => adminApi.batchUpdateAccounts(body),
    ["admin", "accounts"],
  );
}
export function useBatchActionAccounts() {
  return useAdminMutation(
    (body: P<typeof adminApi.batchActionAccounts>) => adminApi.batchActionAccounts(body),
    ["admin", "accounts"],
  );
}
/** Export is read-only; expose as a mutation so pages can trigger it on click. */
export function useExportAccounts() {
  return useMutation({ mutationFn: () => adminApi.exportAccounts() });
}

// Proxies
export function useCreateProxy() {
  return useAdminMutation(
    (body: P<typeof adminApi.createProxy>) => adminApi.createProxy(body),
    ["admin", "proxies"],
  );
}
export function useUpdateProxy() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateProxy> }) =>
      adminApi.updateProxy(vars.id, vars.body),
    ["admin", "proxies"],
  );
}

// Providers (upstream platform registry)
export function useCreateProvider() {
  return useAdminMutation(
    (body: P<typeof adminApi.createProvider>) => adminApi.createProvider(body),
    ["admin", "providers"],
  );
}
export function useUpdateProvider() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateProvider> }) =>
      adminApi.updateProvider(vars.id, vars.body),
    ["admin", "providers"],
  );
}
export function useTestProvider() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.testProvider(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "providers"] }),
  });
}
export function useInstallProviderPresets() {
  return useAdminMutation<void, Awaited<ReturnType<typeof adminApi.installProviderPresets>>>(
    () => adminApi.installProviderPresets(),
    ["admin", "providers"],
  );
}

// Models (model registry)
export function useCreateModel() {
  return useAdminMutation(
    (body: P<typeof adminApi.createModel>) => adminApi.createModel(body),
    ["admin", "models"],
  );
}
export function useUpdateModel() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateModel> }) =>
      adminApi.updateModel(vars.id, vars.body),
    ["admin", "models"],
  );
}
export function useCreateModelAlias() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.createModelAlias> }) =>
      adminApi.createModelAlias(vars.id, vars.body),
    ["admin", "models"],
  );
}
export function useCreateModelMapping() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.createModelMapping> }) =>
      adminApi.createModelMapping(vars.id, vars.body),
    ["admin", "models"],
  );
}

// Account groups
export function useCreateGroup() {
  return useAdminMutation(
    (body: P<typeof adminApi.createAccountGroup>) => adminApi.createAccountGroup(body),
    ["admin", "account-groups"],
  );
}
export function useUpdateGroup() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateAccountGroup> }) =>
      adminApi.updateAccountGroup(vars.id, vars.body),
    ["admin", "account-groups"],
  );
}
export function useAddGroupMember() {
  return useAdminMutation(
    (vars: { accountId: string; groupId: string }) =>
      adminApi.addAccountToGroup(vars.accountId, vars.groupId),
    ["admin", "account-groups"],
  );
}
export function useRemoveGroupMember() {
  return useAdminMutation(
    (vars: { accountId: string; groupId: string }) =>
      adminApi.removeAccountFromGroup(vars.accountId, vars.groupId),
    ["admin", "account-groups"],
  );
}

// Announcements
export function useCreateAnnouncement() {
  return useAdminMutation(
    (body: P<typeof adminApi.createAnnouncement>) => adminApi.createAnnouncement(body),
    ["admin", "announcements"],
  );
}
export function useUpdateAnnouncement() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateAnnouncement> }) =>
      adminApi.updateAnnouncement(vars.id, vars.body),
    ["admin", "announcements"],
  );
}
export function useDeleteAnnouncement() {
  return useAdminMutation(
    (id: string) => adminApi.deleteAnnouncement(id),
    ["admin", "announcements"],
  );
}

// Error passthrough rules
export function useCreateErrorPassthroughRule() {
  return useAdminMutation(
    (body: P<typeof adminApi.createErrorPassthroughRule>) =>
      adminApi.createErrorPassthroughRule(body),
    ["admin", "error-passthrough-rules"],
  );
}
export function useUpdateErrorPassthroughRule() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateErrorPassthroughRule> }) =>
      adminApi.updateErrorPassthroughRule(vars.id, vars.body),
    ["admin", "error-passthrough-rules"],
  );
}
export function useDeleteErrorPassthroughRule() {
  return useAdminMutation(
    (id: string) => adminApi.deleteErrorPassthroughRule(id),
    ["admin", "error-passthrough-rules"],
  );
}

// Payload transform rules
export function useCreatePayloadRule() {
  return useAdminMutation(
    (body: P<typeof adminApi.createPayloadRule>) => adminApi.createPayloadRule(body),
    ["admin", "payload-rules"],
  );
}
export function useUpdatePayloadRule() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updatePayloadRule> }) =>
      adminApi.updatePayloadRule(vars.id, vars.body),
    ["admin", "payload-rules"],
  );
}
export function useDeletePayloadRule() {
  return useAdminMutation(
    (id: string) => adminApi.deletePayloadRule(id),
    ["admin", "payload-rules"],
  );
}

// Scheduled account health-test plans
export function useCreateScheduledTestPlan() {
  return useAdminMutation(
    (body: P<typeof adminApi.createScheduledTestPlan>) =>
      adminApi.createScheduledTestPlan(body),
    ["admin", "scheduled-test-plans"],
  );
}
export function useUpdateScheduledTestPlan() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateScheduledTestPlan> }) =>
      adminApi.updateScheduledTestPlan(vars.id, vars.body),
    ["admin", "scheduled-test-plans"],
  );
}
export function useDeleteScheduledTestPlan() {
  return useAdminMutation(
    (id: string) => adminApi.deleteScheduledTestPlan(id),
    ["admin", "scheduled-test-plans"],
  );
}
export function useRunScheduledTestPlan() {
  return useAdminMutation(
    (id: string) => adminApi.runScheduledTestPlan(id),
    ["admin", "scheduled-test-plans"],
  );
}

// Channel monitors (synthetic probes)
export function useCreateChannelMonitor() {
  return useAdminMutation(
    (body: P<typeof adminApi.createChannelMonitor>) => adminApi.createChannelMonitor(body),
    ["admin", "channel-monitors"],
  );
}
export function useUpdateChannelMonitor() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateChannelMonitor> }) =>
      adminApi.updateChannelMonitor(vars.id, vars.body),
    ["admin", "channel-monitors"],
  );
}
export function useDeleteChannelMonitor() {
  return useAdminMutation(
    (id: string) => adminApi.deleteChannelMonitor(id),
    ["admin", "channel-monitors"],
  );
}
export function useRunChannelMonitor() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => adminApi.runChannelMonitor(id),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: queryKeys.admin.channelMonitorRuns(id) });
    },
  });
}
export function useCreateChannelMonitorTemplate() {
  return useAdminMutation(
    (body: P<typeof adminApi.createChannelMonitorTemplate>) =>
      adminApi.createChannelMonitorTemplate(body),
    ["admin", "channel-monitor-templates"],
  );
}
export function useUpdateChannelMonitorTemplate() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateChannelMonitorTemplate> }) =>
      adminApi.updateChannelMonitorTemplate(vars.id, vars.body),
    ["admin", "channel-monitor-templates"],
  );
}
export function useDeleteChannelMonitorTemplate() {
  return useAdminMutation(
    (id: string) => adminApi.deleteChannelMonitorTemplate(id),
    ["admin", "channel-monitor-templates"],
  );
}
export function useApplyChannelMonitorTemplate() {
  return useAdminMutation(
    (vars: { id: string; monitorIds: number[] }) =>
      adminApi.applyChannelMonitorTemplate(vars.id, vars.monitorIds),
    ["admin", "channel-monitors"],
  );
}

// TLS fingerprint profiles
export function useCreateTlsProfile() {
  return useAdminMutation(
    (body: P<typeof adminApi.createTlsProfile>) => adminApi.createTlsProfile(body),
    ["admin", "tls-profiles"],
  );
}
export function useUpdateTlsProfile() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateTlsProfile> }) =>
      adminApi.updateTlsProfile(vars.id, vars.body),
    ["admin", "tls-profiles"],
  );
}
export function useDeleteTlsProfile() {
  return useAdminMutation(
    (id: string) => adminApi.deleteTlsProfile(id),
    ["admin", "tls-profiles"],
  );
}

// Custom user attribute definitions
export function useCreateUserAttributeDefinition() {
  return useAdminMutation(
    (body: P<typeof adminApi.createUserAttributeDefinition>) =>
      adminApi.createUserAttributeDefinition(body),
    ["admin", "user-attributes"],
  );
}
export function useUpdateUserAttributeDefinition() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateUserAttributeDefinition> }) =>
      adminApi.updateUserAttributeDefinition(vars.id, vars.body),
    ["admin", "user-attributes"],
  );
}
export function useDeleteUserAttributeDefinition() {
  return useAdminMutation(
    (id: string) => adminApi.deleteUserAttributeDefinition(id),
    ["admin", "user-attributes"],
  );
}

// Notification email templates
export function useUpdateNotificationEmailTemplate() {
  return useAdminMutation(
    (vars: {
      event: P<typeof adminApi.updateNotificationEmailTemplate>;
      body: B<typeof adminApi.updateNotificationEmailTemplate>;
    }) => adminApi.updateNotificationEmailTemplate(vars.event, vars.body),
    ["admin", "notification-email-templates"],
  );
}
export function useRestoreNotificationEmailTemplate() {
  return useAdminMutation(
    (event: P<typeof adminApi.restoreNotificationEmailTemplate>) =>
      adminApi.restoreNotificationEmailTemplate(event),
    ["admin", "notification-email-templates"],
  );
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
  );
}
export function useBatchGenerateRedeemCodes() {
  return useAdminMutation(
    (body: P<typeof adminApi.batchGenerateRedeemCodes>) => adminApi.batchGenerateRedeemCodes(body),
    ["admin", "redeem-codes"],
  );
}
export function useBatchDisableRedeemCodes() {
  return useAdminMutation(
    (ids: string[]) => adminApi.batchDisableRedeemCodes(ids),
    ["admin", "redeem-codes"],
  );
}
export function useRedeemStats() {
  return useQuery({
    queryKey: queryKeys.admin.redeemStats(),
    queryFn: () => adminApi.getRedeemStats(),
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
export function useCreateUserSubscription() {
  return useAdminMutation(
    (body: P<typeof adminApi.createUserSubscription>) => adminApi.createUserSubscription(body),
    ["admin", "user-subscriptions"],
  );
}

// Pricing rules
export function useCreatePricingRule() {
  return useAdminMutation(
    (body: P<typeof adminApi.createPricingRule>) => adminApi.createPricingRule(body),
    ["admin", "pricing-rules"],
  );
}
export function useBulkImportPricingRules() {
  return useAdminMutation(
    (body: P<typeof adminApi.bulkImportPricingRules>) => adminApi.bulkImportPricingRules(body),
    ["admin", "pricing-rules"],
  );
}
export function useDeletePricingRule() {
  return useAdminMutation(
    (id: string) => adminApi.deletePricingRule(id),
    ["admin", "pricing-rules"],
  );
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

// Risk control
export function useUpdateRiskConfig() {
  return useAdminMutation(
    (body: P<typeof adminApi.updateRiskConfig>) => adminApi.updateRiskConfig(body),
    ["admin", "risk-config"],
  );
}

// Ops alerts
export function useAcknowledgeAlert() {
  return useAdminMutation(
    (id: string) => adminApi.acknowledgeAlert(id),
    ["admin", "ops", "alerts"],
  );
}

// Ops SLO definitions
export function useCreateOpsSlo() {
  return useAdminMutation(
    (body: P<typeof adminApi.createOpsSlo>) => adminApi.createOpsSlo(body),
    ["admin", "ops", "slos"],
  );
}
export function useUpdateOpsSlo() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateOpsSlo> }) =>
      adminApi.updateOpsSlo(vars.id, vars.body),
    ["admin", "ops", "slos"],
  );
}

// Ops alert rules + silences (configurable generic metric alerting)
export function useOpsAlertRules() {
  return useQuery({
    queryKey: queryKeys.admin.opsAlertRules(),
    queryFn: () => adminApi.listOpsAlertRules(),
  });
}
export function useCreateOpsAlertRule() {
  return useAdminMutation(
    (body: P<typeof adminApi.createOpsAlertRule>) => adminApi.createOpsAlertRule(body),
    ["admin", "ops"],
  );
}
export function useUpdateOpsAlertRule() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateOpsAlertRule> }) =>
      adminApi.updateOpsAlertRule(vars.id, vars.body),
    ["admin", "ops"],
  );
}
export function useDeleteOpsAlertRule() {
  return useAdminMutation(
    (id: string) => adminApi.deleteOpsAlertRule(id),
    ["admin", "ops"],
  );
}
export function useOpsAlertSilences() {
  return useQuery({
    queryKey: queryKeys.admin.opsAlertSilences(),
    queryFn: () => adminApi.listOpsAlertSilences(),
  });
}
export function useCreateOpsAlertSilence() {
  return useAdminMutation(
    (body: P<typeof adminApi.createOpsAlertSilence>) => adminApi.createOpsAlertSilence(body),
    ["admin", "ops"],
  );
}
export function useDeleteOpsAlertSilence() {
  return useAdminMutation(
    (id: string) => adminApi.deleteOpsAlertSilence(id),
    ["admin", "ops"],
  );
}

// Settings
export function useUpdateSettings() {
  return useAdminMutation(
    (body: P<typeof adminApi.updateSettings>) => adminApi.updateSettings(body),
    ["admin", "settings"],
  );
}

// Send a probe email to verify SMTP credentials. Returns an AdminTestResult; it
// changes no server state, so there is nothing to invalidate.
export function useSendTestEmail() {
  return useMutation({
    mutationFn: (body?: P<typeof adminApi.sendTestEmail>) => adminApi.sendTestEmail(body),
  });
}

// Config snapshot (backup / restore)
export function useConfigSnapshot() {
  return useQuery({
    queryKey: queryKeys.admin.configSnapshot(),
    queryFn: () => adminApi.getConfigSnapshot(),
    enabled: false, // fetched on demand from the Backup tab
  });
}
export function useImportConfigSnapshot() {
  return useMutation({
    mutationFn: (vars: { body: P<typeof adminApi.importConfigSnapshot>; dryRun?: boolean }) =>
      adminApi.importConfigSnapshot(vars.body, vars.dryRun),
  });
}

// Scheduler strategy replay
export function useReplaySchedulerStrategy() {
  return useMutation({
    mutationFn: (body: P<typeof adminApi.replaySchedulerStrategy>) =>
      adminApi.replaySchedulerStrategy(body),
  });
}
