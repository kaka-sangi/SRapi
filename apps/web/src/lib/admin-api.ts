"use client";

import type { Auth } from "../../../../packages/sdk/typescript/src/core/auth.gen";
import { client } from "../../../../packages/sdk/typescript/src/client.gen";
import {
  acknowledgeAdminOpsAlert,
  cleanupAdminOpsSystemLogs,
  cleanupAdminUsage,
  addAdminAccountGroupMember,
  batchDisableAdminRedeemCodes,
  batchGenerateAdminRedeemCodes,
  batchActionAdminAccounts,
  batchUpdateAdminAccounts,
  bindAdminAccountProxy,
  bulkImportAdminPricingRules,
  listAdminModelRateLimits,
  upsertAdminModelRateLimit,
  deleteAdminModelRateLimit,
  listAdminGroupRateLimits,
  upsertAdminGroupRateLimit,
  deleteAdminGroupRateLimit,
  clearAdminAccountError,
  createAdminAccount,
  createAdminAccountGroup,
  createAdminAnnouncement,
  createAdminErrorPassthroughRule,
  deleteAdminErrorPassthroughRule,
  listAdminErrorPassthroughRules,
  updateAdminErrorPassthroughRule,
  createAdminPayloadRule,
  deleteAdminPayloadRule,
  listAdminPayloadRules,
  updateAdminPayloadRule,
  createAdminScheduledTestPlan,
  deleteAdminScheduledTestPlan,
  listAdminScheduledTestPlans,
  listAdminScheduledTestPlanRuns,
  runAdminScheduledTestPlan,
  updateAdminScheduledTestPlan,
  createAdminTlsProfile,
  deleteAdminTlsProfile,
  listAdminTlsProfiles,
  updateAdminTlsProfile,
  createAdminRole,
  deleteAdminRole,
  listAdminRoles,
  updateAdminApiKey,
  updateAdminRole,
  createAdminUserAttributeDefinition,
  deleteAdminUserAttributeDefinition,
  listAdminUserAttributeDefinitions,
  updateAdminUserAttributeDefinition,
  listAdminNotificationEmailTemplates,
  updateAdminNotificationEmailTemplate,
  previewAdminNotificationEmailTemplate,
  restoreAdminNotificationEmailTemplate,
  listAdminAccountsAvailability,
  listAdminUserPlatformQuotas,
  upsertAdminUserPlatformQuota,
  deleteAdminUserPlatformQuota,
  createAdminOpsSlo,
  listAdminOpsAlertRules,
  createAdminOpsAlertRule,
  updateAdminOpsAlertRule,
  deleteAdminOpsAlertRule,
  listAdminOpsAlertSilences,
  createAdminOpsAlertSilence,
  deleteAdminOpsAlertSilence,
  createAdminPaymentProvider,
  createAdminPricingRule,
  createAdminPromoCode,
  createAdminProvider,
  createAdminProxy,
  createAdminRedeemCode,
  createAdminSubscriptionPlan,
  updateAdminSubscriptionPlan,
  createAdminUser,
  createAdminUserSubscription,
  deleteAdminAnnouncement,
  deleteAdminPromoCode,
  disableAdminAccount,
  disableAdminUser,
  discoverAdminAccountModels,
  enableAdminAccount,
  enableAdminUser,
  exportAdminAccounts,
  getAdminAccountHealth,
  getAdminAccountProxyQuality,
  fetchAdminAccountQuota,
  getAdminAccountQuota,
  getAdminAccountRpmStatus,
  getAdminConfigSnapshot,
  importAdminConfigSnapshot,
  getAdminDashboardSnapshot,
  getAdminOpsConcurrency,
  getAdminOpsErrorDistribution,
  getAdminOpsErrorTrend,
  getAdminOpsLatencyHistogram,
  getAdminOpsOverview,
  getAdminOpsThroughputTrend,
  getAdminRedeemCodeStats,
  getAdminRiskControlConfig,
  getAdminRiskControlStatus,
  getAdminSettings,
  getAdminCopilotConfig,
  getAdminUsageAggregates,
  getAdminUsageDaily,
  importAdminAccounts,
  importAdminCodexSession,
  installAdminProviderPresets,
  listAdminAccountGroups,
  listAdminAccountGroupMembers,
  listAdminAccounts,
  listAdminAffiliateInvites,
  listAdminAffiliateRebates,
  listAdminAffiliateTransfers,
  listAdminAnnouncements,
  listAdminAuditLogs,
  listAdminBillingLedger,
  listAdminModels,
  createAdminModel,
  createAdminModelAlias,
  createAdminModelMapping,
  updateAdminModel,
  listAdminOpsAlertEvents,
  listAdminOpsAlerts,
  listAdminOpsRealtimeSlots,
  listAdminOpsSlos,
  listAdminOpsSystemLogs,
  listAdminApiKeys,
  getAdminApiKeyUsage,
  listAdminOutboxEvents,
  listAdminPaymentOrders,
  listAdminPaymentProviders,
  listAdminPricingRules,
  listAdminPromoCodes,
  listAdminPromoCodeUsages,
  listAdminProviders,
  listAdminProxies,
  listAdminRedeemCodes,
  listAdminRiskControlLogs,
  listAdminSubscriptionPlans,
  listAdminUsageLogs,
  listAdminUsers,
  listAdminUserSubscriptions,
  refundAdminPaymentOrder,
  removeAdminAccountGroupMember,
  replaySchedulerStrategy,
  recoverAdminAccount,
  sendAdminTestEmail,
  testAdminAccount,
  testAdminPaymentProvider,
  testAdminProvider,
  updateAdminAccount,
  updateAdminAccountGroup,
  updateAdminAnnouncement,
  updateAdminOpsSettings,
  updateAdminOpsSlo,
  updateAdminPaymentProvider,
  updateAdminPromoCode,
  updateAdminProvider,
  updateAdminProxy,
  updateAdminRiskControlConfig,
  updateAdminSettings,
  updateAdminUser,
  updateAdminUserBalance,
} from "../../../../packages/sdk/typescript/src/index";
import type {
  AccountGroup,
  AccountGroupMember,
  AccountHealthSnapshot,
  AccountModelDiscovery,
  AccountProxyQuality,
  AccountQuotaReport,
  AccountQuotaSnapshot,
  AccountRpmStatus,
  AdminCopilotConfig,
  AdminDashboardSnapshot,
  AdminSendTestEmailRequest,
  AdminSettings,
  AdminTestResult,
  AdminUpdateApiKeyRequest,
  ApiKey,
  GatewayUsageResponse,
  PromoCodeUsage,
  AffiliateInviteRecord,
  AffiliateLedgerEntry,
  Announcement,
  AnnouncementStatus,
  CreateErrorPassthroughRuleRequest,
  ErrorPassthroughRule,
  UpdateErrorPassthroughRuleRequest,
  CreatePayloadRuleRequest,
  PayloadRule,
  UpdatePayloadRuleRequest,
  CreateScheduledTestPlanRequest,
  ScheduledTestPlan,
  ScheduledTestPlanRun,
  UpdateScheduledTestPlanRequest,
  CreateTlsProfileRequest,
  TlsProfile,
  UpdateTlsProfileRequest,
  Role,
  CreateRoleRequest,
  UpdateRoleRequest,
  CreateUserAttributeDefinitionRequest,
  UserAttributeDefinition,
  UpdateUserAttributeDefinitionRequest,
  NotificationEmailTemplate,
  NotificationEmailTemplateList,
  NotificationEmailTemplateEventName,
  NotificationEmailTemplatePreview,
  PreviewNotificationEmailTemplateRequest,
  UpdateNotificationEmailTemplateRequest,
  AccountAvailabilitySummary,
  UserPlatformQuota,
  UpsertUserPlatformQuotaRequest,
  AuditLog,
  BillingLedgerEntry,
  BulkImportAdminPricingRulesData,
  BulkPricingRuleImportResult,
  ConfigImportRequest,
  ConfigImportResponse,
  ConfigSnapshotResponse,
  CreateAccountGroupRequest,
  CreateAdminAccountData,
  CreateAdminPaymentProviderData,
  UpdateAdminPaymentProviderData,
  CreateAdminPricingRuleData,
  CreateAdminProxyData,
  CreateAdminSubscriptionPlanData,
  CreateAdminUserSubscriptionData,
  CreateAdminUserData,
  CreateAnnouncementRequest,
  CreateRedeemCodeRequest,
  DiscoverAdminAccountModelsData,
  DomainEventOutbox,
  BatchOperationResult,
  BatchUpdateAccountsResult,
  Id,
  CodexSessionImportResult,
  ImportAdminAccountsData,
  ImportAdminCodexSessionData,
  ListAdminAccountsData,
  ListAdminAffiliateInvitesData,
  ListAdminAffiliateRebatesData,
  ListAdminAffiliateTransfersData,
  ListAdminAnnouncementsData,
  ListAdminAuditLogsData,
  ListAdminBillingLedgerData,
  ListAdminModelsData,
  ListAdminOpsAlertEventsData,
  ListAdminOpsAlertsData,
  ListAdminOpsSystemLogsData,
  ListAdminPaymentOrdersData,
  ListAdminPaymentProvidersData,
  ListAdminPricingRulesData,
  ListAdminPromoCodesData,
  ListAdminProvidersData,
  ListAdminProxiesData,
  ListAdminRedeemCodesData,
  ListAdminRiskControlLogsData,
  ListAdminSubscriptionPlansData,
  ListAdminUsageLogsData,
  ListAdminApiKeysData,
  ListAdminOutboxEventsData,
  ListAdminUsersData,
  ListAdminUserSubscriptionsData,
  Model,
  ModelAlias,
  ModelProviderMapping,
  OpsAlertEvent,
  OpsAlertRule,
  OpsAlertSilence,
  CreateAdminOpsAlertRuleData,
  UpdateAdminOpsAlertRuleData,
  CreateAdminOpsAlertSilenceData,
  OpsConcurrency,
  OpsErrorDistribution,
  OpsErrorTrend,
  OpsLatencyHistogram,
  OpsOverview,
  OpsSlo,
  OpsSettings,
  OpsSloDefinition,
  OpsSystemLog,
  OpsSystemLogCleanupRequest,
  OpsSystemLogCleanupResult,
  UsageCleanupRequest,
  UsageCleanupResult,
  OpsThroughputTrend,
  Pagination,
  ModelRateLimit,
  AccountGroupRateLimit,
  UpsertModelRateLimitRequest,
  UpsertGroupRateLimitRequest,
  PaymentOrder,
  PaymentProviderInstance,
  PricingRule,
  PromoCode,
  Provider,
  ProviderAccount,
  ProviderAccountExportItem,
  ProviderAccountImportResult,
  ProviderAccountStatus,
  ProxyDefinition,
  RealtimeActiveSlot,
  RedeemCode,
  RedeemCodeStats,
  ReplaySchedulerStrategyData,
  RiskControlConfig,
  RiskControlLog,
  RiskControlStatus,
  SchedulerReplayResult,
  SubscriptionPlan,
  UpdateAccountGroupRequest,
  UpdateAdminAccountData,
  UpdateAdminOpsSloData,
  UpdateAdminProviderData,
  UpdateAdminProxyData,
  UpdateAdminUserData,
  UpdateAnnouncementRequest,
  UpdatePromoCodeRequest,
  UpdateUserBalanceRequest,
  UsageAggregate,
  UsageAggregateDimension,
  UsageLog,
  User,
  UserStatus,
  UserSubscription,
} from "../../../../packages/sdk/typescript/src/types.gen";

const CSRF_STORAGE_KEY = "srapi_csrf_token";

export interface AdminListResult<T> {
  data: T[];
  pagination?: Pagination;
}

export interface AdminTimeRange {
  start?: string;
  end?: string;
}

export interface AdminUnsupportedSurface {
  title: string;
  contractPath?: string;
  reason: string;
}

function configuredApiBaseUrl(): string {
  return (process.env.NEXT_PUBLIC_SRAPI_BASE_URL || "").replace(/\/+$/, "");
}

function getStoredCSRFToken(): string | undefined {
  if (typeof window === "undefined") {
    return undefined;
  }
  return localStorage.getItem(CSRF_STORAGE_KEY) || undefined;
}

function resolveAuthToken(auth: Auth): string | undefined {
  if (auth.name === "X-CSRF-Token") {
    return getStoredCSRFToken();
  }
  return undefined;
}

function configureAdminClient() {
  client.setConfig({
    baseUrl: configuredApiBaseUrl(),
    credentials: "include",
    auth: resolveAuthToken,
  });
}

configureAdminClient();

async function unwrapData<T>(request: () => Promise<{ data?: { data?: T } }>): Promise<T> {
  configureAdminClient();
  const response = await request();
  if (!response.data || !("data" in response.data)) {
    throw new Error("Admin API returned an empty response.");
  }
  return response.data.data as T;
}

async function unwrapList<T>(
  request: () => Promise<{ data?: { data?: T[]; pagination?: Pagination } }>,
): Promise<AdminListResult<T>> {
  configureAdminClient();
  const response = await request();
  if (!response.data || !Array.isArray(response.data.data)) {
    throw new Error("Admin API returned an empty list response.");
  }
  return {
    data: response.data.data,
    pagination: response.data.pagination,
  };
}

export function adminErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }

  if (typeof error === "object" && error !== null) {
    const maybe = error as {
      error?: { message?: string };
      message?: string;
      response?: { data?: { error?: { message?: string } } };
    };
    return (
      maybe.response?.data?.error?.message ||
      maybe.error?.message ||
      maybe.message ||
      "Admin API request failed."
    );
  }

  return "Admin API request failed.";
}

export const adminApi = {
  getDashboardSnapshot(query?: AdminTimeRange): Promise<AdminDashboardSnapshot> {
    return unwrapData(() => getAdminDashboardSnapshot({ query, throwOnError: true }));
  },

  getOpsOverview(query?: AdminTimeRange): Promise<OpsOverview> {
    return unwrapData(() => getAdminOpsOverview({ query, throwOnError: true }));
  },

  getOpsThroughputTrend(
    query?: AdminTimeRange & { bucket?: "hour" | "day" },
  ): Promise<OpsThroughputTrend> {
    return unwrapData(() => getAdminOpsThroughputTrend({ query, throwOnError: true }));
  },

  getOpsErrorTrend(query?: AdminTimeRange & { bucket?: "hour" | "day" }): Promise<OpsErrorTrend> {
    return unwrapData(() => getAdminOpsErrorTrend({ query, throwOnError: true }));
  },

  getOpsErrorDistribution(query?: AdminTimeRange): Promise<OpsErrorDistribution> {
    return unwrapData(() => getAdminOpsErrorDistribution({ query, throwOnError: true }));
  },

  getOpsLatencyHistogram(query?: AdminTimeRange): Promise<OpsLatencyHistogram> {
    return unwrapData(() => getAdminOpsLatencyHistogram({ query, throwOnError: true }));
  },

  getOpsConcurrency(): Promise<OpsConcurrency> {
    return unwrapData(() => getAdminOpsConcurrency({ throwOnError: true }));
  },

  listOpsSystemLogs(
    query?: ListAdminOpsSystemLogsData["query"],
  ): Promise<AdminListResult<OpsSystemLog>> {
    return unwrapList(() => listAdminOpsSystemLogs({ query, throwOnError: true }));
  },
  // Bounded deletion of sanitized system logs (requires ≥1 filter; dry_run
  // previews without deleting). Returns matched/deleted counts.
  cleanupOpsSystemLogs(body: OpsSystemLogCleanupRequest): Promise<OpsSystemLogCleanupResult> {
    return unwrapData(() => cleanupAdminOpsSystemLogs({ body, throwOnError: true }));
  },
  // Replace the operational monitoring settings (auto-refresh, alert thresholds,
  // retention). The backend exposes no read endpoint, so this is write-only.
  updateOpsSettings(body: OpsSettings): Promise<OpsSettings> {
    return unwrapData(() => updateAdminOpsSettings({ body, throwOnError: true }));
  },

  listOpsAlerts(query?: ListAdminOpsAlertsData["query"]): Promise<AdminListResult<OpsAlertEvent>> {
    return unwrapList(() => listAdminOpsAlerts({ query, throwOnError: true }));
  },

  listOpsAlertEvents(
    query?: ListAdminOpsAlertEventsData["query"],
  ): Promise<AdminListResult<OpsAlertEvent>> {
    return unwrapList(() => listAdminOpsAlertEvents({ query, throwOnError: true }));
  },

  listOpsRealtimeSlots(): Promise<AdminListResult<RealtimeActiveSlot>> {
    return unwrapList(() => listAdminOpsRealtimeSlots({ throwOnError: true }));
  },

  listOpsSlos(): Promise<AdminListResult<OpsSlo>> {
    return unwrapList(() => listAdminOpsSlos({ throwOnError: true }));
  },

  replaySchedulerStrategy(
    body: ReplaySchedulerStrategyData["body"],
  ): Promise<SchedulerReplayResult> {
    return unwrapData(() => replaySchedulerStrategy({ body, throwOnError: true }));
  },

  acknowledgeAlert(id: Id): Promise<OpsAlertEvent> {
    return unwrapData(() => acknowledgeAdminOpsAlert({ path: { id }, throwOnError: true }));
  },

  listUsers(query?: ListAdminUsersData["query"]): Promise<AdminListResult<User>> {
    return unwrapList(() => listAdminUsers({ query, throwOnError: true }));
  },

  createUser(body: CreateAdminUserData["body"]): Promise<User> {
    return unwrapData(() => createAdminUser({ body, throwOnError: true }));
  },

  updateUser(id: Id, body: UpdateAdminUserData["body"]): Promise<User> {
    return unwrapData(() => updateAdminUser({ path: { id }, body, throwOnError: true }));
  },

  updateUserBalance(id: Id, body: UpdateUserBalanceRequest): Promise<User> {
    return unwrapData(() => updateAdminUserBalance({ path: { id }, body, throwOnError: true }));
  },

  setUserEnabled(user: User): Promise<User> {
    if (user.status === "disabled") {
      return unwrapData(() => enableAdminUser({ path: { id: user.id }, throwOnError: true }));
    }
    return unwrapData(() => disableAdminUser({ path: { id: user.id }, throwOnError: true }));
  },

  setUserEnabledById(id: Id, enabled: boolean): Promise<User> {
    return enabled
      ? unwrapData(() => enableAdminUser({ path: { id }, throwOnError: true }))
      : unwrapData(() => disableAdminUser({ path: { id }, throwOnError: true }));
  },

  listProviders(query?: ListAdminProvidersData["query"]): Promise<AdminListResult<Provider>> {
    return unwrapList(() => listAdminProviders({ query, throwOnError: true }));
  },

  createProvider(body: Parameters<typeof createAdminProvider>[0]["body"]): Promise<Provider> {
    return unwrapData(() => createAdminProvider({ body, throwOnError: true }));
  },

  updateProvider(id: Id, body: UpdateAdminProviderData["body"]): Promise<Provider> {
    return unwrapData(() =>
      updateAdminProvider({ path: { id }, body, throwOnError: true }),
    );
  },

  testProvider(id: Id): Promise<AdminTestResult> {
    return unwrapData(() => testAdminProvider({ path: { id }, throwOnError: true }));
  },
  // Bulk-create any missing built-in provider presets (existing ones are skipped,
  // new ones land disabled). Returns how many were requested/created/failed.
  installProviderPresets(): Promise<BatchOperationResult> {
    return unwrapData(() => installAdminProviderPresets({ throwOnError: true }));
  },

  listModels(query?: ListAdminModelsData["query"]): Promise<AdminListResult<Model>> {
    return unwrapList(() => listAdminModels({ query, throwOnError: true }));
  },

  createModel(body: Parameters<typeof createAdminModel>[0]["body"]): Promise<Model> {
    return unwrapData(() => createAdminModel({ body, throwOnError: true }));
  },

  updateModel(id: Id, body: Parameters<typeof updateAdminModel>[0]["body"]): Promise<Model> {
    return unwrapData(() => updateAdminModel({ path: { id }, body, throwOnError: true }));
  },
  createModelAlias(
    id: Id,
    body: Parameters<typeof createAdminModelAlias>[0]["body"],
  ): Promise<ModelAlias> {
    return unwrapData(() => createAdminModelAlias({ path: { id }, body, throwOnError: true }));
  },
  createModelMapping(
    id: Id,
    body: Parameters<typeof createAdminModelMapping>[0]["body"],
  ): Promise<ModelProviderMapping> {
    return unwrapData(() => createAdminModelMapping({ path: { id }, body, throwOnError: true }));
  },

  listAccounts(query?: ListAdminAccountsData["query"]): Promise<AdminListResult<ProviderAccount>> {
    return unwrapList(() => listAdminAccounts({ query, throwOnError: true }));
  },

  exportAccounts(): Promise<ProviderAccountExportItem[]> {
    return unwrapData(() => exportAdminAccounts({ throwOnError: true }));
  },

  importAccounts(body: ImportAdminAccountsData["body"]): Promise<ProviderAccountImportResult> {
    return unwrapData(() => importAdminAccounts({ body, throwOnError: true }));
  },

  importCodexSession(body: ImportAdminCodexSessionData["body"]): Promise<CodexSessionImportResult> {
    return unwrapData(() => importAdminCodexSession({ body, throwOnError: true }));
  },

  batchUpdateAccounts(body: Parameters<typeof batchUpdateAdminAccounts>[0]["body"]): Promise<BatchUpdateAccountsResult> {
    return unwrapData(() => batchUpdateAdminAccounts({ body, throwOnError: true }));
  },

  batchActionAccounts(body: Parameters<typeof batchActionAdminAccounts>[0]["body"]): Promise<BatchUpdateAccountsResult> {
    return unwrapData(() => batchActionAdminAccounts({ body, throwOnError: true }));
  },

  createAccount(body: CreateAdminAccountData["body"]): Promise<ProviderAccount> {
    return unwrapData(() => createAdminAccount({ body, throwOnError: true }));
  },

  updateAccount(id: Id, body: UpdateAdminAccountData["body"]): Promise<ProviderAccount> {
    return unwrapData(() => updateAdminAccount({ path: { id }, body, throwOnError: true }));
  },

  setAccountStatus(id: Id, status: ProviderAccountStatus): Promise<ProviderAccount> {
    // `status` is the desired TARGET state set by the caller (e.g. row toggle /
    // bulk action). Disable the account when the target is "disabled", enable it
    // otherwise. (Previously inverted: enable/disable were swapped, so the row
    // toggle was a no-op and the bulk buttons did the opposite of their label.)
    if (status === "disabled") {
      return unwrapData(() => disableAdminAccount({ path: { id }, throwOnError: true }));
    }
    return unwrapData(() => enableAdminAccount({ path: { id }, throwOnError: true }));
  },

  testAccount(id: Id): Promise<AdminTestResult> {
    return unwrapData(() => testAdminAccount({ path: { id }, throwOnError: true }));
  },

  discoverAccountModels(
    id: Id,
    body?: DiscoverAdminAccountModelsData["body"],
  ): Promise<AccountModelDiscovery> {
    return unwrapData(() =>
      discoverAdminAccountModels({ path: { id }, body, throwOnError: true }),
    );
  },

  clearAccountError(id: Id): Promise<ProviderAccount> {
    return unwrapData(() => clearAdminAccountError({ path: { id }, throwOnError: true }));
  },

  recoverAccount(id: Id): Promise<ProviderAccount> {
    return unwrapData(() => recoverAdminAccount({ path: { id }, throwOnError: true }));
  },

  bindAccountProxy(id: Id, proxyId: string | null): Promise<ProviderAccount> {
    return unwrapData(() =>
      bindAdminAccountProxy({
        path: { id },
        body: { proxy_id: proxyId },
        throwOnError: true,
      }),
    );
  },

  getAccountProxyQuality(id: Id): Promise<AccountProxyQuality> {
    return unwrapData(() => getAdminAccountProxyQuality({ path: { id }, throwOnError: true }));
  },

  getAccountHealth(id: Id): Promise<AccountHealthSnapshot> {
    return unwrapData(() => getAdminAccountHealth({ path: { id }, throwOnError: true }));
  },

  getAccountQuota(id: Id): Promise<AdminListResult<AccountQuotaSnapshot>> {
    return unwrapList(() => getAdminAccountQuota({ path: { id }, throwOnError: true }));
  },
  // Trigger a live quota pull from the upstream provider (vs the stored
  // snapshots getAccountQuota returns). Returns the fresh provider report.
  fetchAccountQuota(id: Id): Promise<AccountQuotaReport> {
    return unwrapData(() => fetchAdminAccountQuota({ path: { id }, throwOnError: true }));
  },

  getAccountRpmStatus(id: Id): Promise<AccountRpmStatus> {
    return unwrapData(() => getAdminAccountRpmStatus({ path: { id }, throwOnError: true }));
  },

  listProxies(query?: ListAdminProxiesData["query"]): Promise<AdminListResult<ProxyDefinition>> {
    return unwrapList(() => listAdminProxies({ query, throwOnError: true }));
  },

  createProxy(body: CreateAdminProxyData["body"]): Promise<ProxyDefinition> {
    return unwrapData(() => createAdminProxy({ body, throwOnError: true }));
  },

  updateProxy(id: Id, body: UpdateAdminProxyData["body"]): Promise<ProxyDefinition> {
    return unwrapData(() => updateAdminProxy({ path: { id }, body, throwOnError: true }));
  },

  listAccountGroups(): Promise<AdminListResult<AccountGroup>> {
    return unwrapList(() => listAdminAccountGroups({ throwOnError: true }));
  },

  createAccountGroup(body: CreateAccountGroupRequest): Promise<AccountGroup> {
    return unwrapData(() => createAdminAccountGroup({ body, throwOnError: true }));
  },

  updateAccountGroup(id: Id, body: UpdateAccountGroupRequest): Promise<AccountGroup> {
    return unwrapData(() => updateAdminAccountGroup({ path: { id }, body, throwOnError: true }));
  },

  addAccountToGroup(accountId: Id, groupId: Id): Promise<AccountGroupMember> {
    return unwrapData(() =>
      addAdminAccountGroupMember({
        path: { id: groupId, account_id: accountId },
        throwOnError: true,
      }),
    );
  },

  async removeAccountFromGroup(accountId: Id, groupId: Id): Promise<void> {
    configureAdminClient();
    await removeAdminAccountGroupMember({
      path: { id: groupId, account_id: accountId },
      throwOnError: true,
    });
  },

  listAccountGroupMembers(groupId: Id): Promise<AdminListResult<AccountGroupMember>> {
    return unwrapList(() =>
      listAdminAccountGroupMembers({ path: { id: groupId }, throwOnError: true }),
    );
  },

  listUsageLogs(query?: ListAdminUsageLogsData["query"]): Promise<AdminListResult<UsageLog>> {
    return unwrapList(() => listAdminUsageLogs({ query, throwOnError: true }));
  },

  listUsageDaily(query?: AdminTimeRange): Promise<AdminListResult<UsageAggregate>> {
    return unwrapList(() => getAdminUsageDaily({ query, throwOnError: true }));
  },

  listUsageAggregates(
    dimension: UsageAggregateDimension,
    query?: AdminTimeRange,
  ): Promise<AdminListResult<UsageAggregate>> {
    return unwrapList(
      () => getAdminUsageAggregates({
        query: { dimension, ...query },
        throwOnError: true,
      }),
    );
  },

  // Operator on-demand deletion of usage records (the counterpart to the
  // background retention worker). Requires ≥1 bounded filter (model/start/end);
  // dry_run previews the match count without deleting. Returns matched/deleted.
  cleanupUsage(body: UsageCleanupRequest): Promise<UsageCleanupResult> {
    return unwrapData(() => cleanupAdminUsage({ body, throwOnError: true }));
  },

  listAuditLogs(query?: ListAdminAuditLogsData["query"]): Promise<AdminListResult<AuditLog>> {
    return unwrapList(() => listAdminAuditLogs({ query, throwOnError: true }));
  },
  listOutboxEvents(
    query?: ListAdminOutboxEventsData["query"],
  ): Promise<AdminListResult<DomainEventOutbox>> {
    return unwrapList(() => listAdminOutboxEvents({ query, throwOnError: true }));
  },

  listBillingLedger(
    query?: ListAdminBillingLedgerData["query"],
  ): Promise<AdminListResult<BillingLedgerEntry>> {
    return unwrapList(() => listAdminBillingLedger({ query, throwOnError: true }));
  },

  listAffiliateInvites(
    query?: ListAdminAffiliateInvitesData["query"],
  ): Promise<AdminListResult<AffiliateInviteRecord>> {
    return unwrapList(() => listAdminAffiliateInvites({ query, throwOnError: true }));
  },

  listAffiliateRebates(
    query?: ListAdminAffiliateRebatesData["query"],
  ): Promise<AdminListResult<AffiliateLedgerEntry>> {
    return unwrapList(() => listAdminAffiliateRebates({ query, throwOnError: true }));
  },

  listAffiliateTransfers(
    query?: ListAdminAffiliateTransfersData["query"],
  ): Promise<AdminListResult<AffiliateLedgerEntry>> {
    return unwrapList(() => listAdminAffiliateTransfers({ query, throwOnError: true }));
  },

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

  listPaymentOrders(
    query?: ListAdminPaymentOrdersData["query"],
  ): Promise<AdminListResult<PaymentOrder>> {
    return unwrapList(() => listAdminPaymentOrders({ query, throwOnError: true }));
  },

  refundPaymentOrder(id: Id, body: Parameters<typeof refundAdminPaymentOrder>[0]["body"]): Promise<PaymentOrder> {
    return unwrapData(() =>
      refundAdminPaymentOrder({ path: { id }, body, throwOnError: true }),
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

  listUserSubscriptions(
    query?: ListAdminUserSubscriptionsData["query"],
  ): Promise<AdminListResult<UserSubscription>> {
    return unwrapList(() => listAdminUserSubscriptions({ query, throwOnError: true }));
  },

  createUserSubscription(body: CreateAdminUserSubscriptionData["body"]): Promise<UserSubscription> {
    return unwrapData(() => createAdminUserSubscription({ body, throwOnError: true }));
  },

  listPricingRules(query?: ListAdminPricingRulesData["query"]): Promise<AdminListResult<PricingRule>> {
    return unwrapList(() => listAdminPricingRules({ query, throwOnError: true }));
  },

  createPricingRule(body: CreateAdminPricingRuleData["body"]): Promise<PricingRule> {
    return unwrapData(() => createAdminPricingRule({ body, throwOnError: true }));
  },

  bulkImportPricingRules(
    body: BulkImportAdminPricingRulesData["body"],
  ): Promise<BulkPricingRuleImportResult> {
    return unwrapData(() => bulkImportAdminPricingRules({ body, throwOnError: true }));
  },

  createOpsSlo(body: Parameters<typeof createAdminOpsSlo>[0]["body"]): Promise<OpsSloDefinition> {
    return unwrapData(() => createAdminOpsSlo({ body, throwOnError: true }));
  },

  updateOpsSlo(id: Id, body: UpdateAdminOpsSloData["body"]): Promise<OpsSloDefinition> {
    return unwrapData(() => updateAdminOpsSlo({ path: { id }, body, throwOnError: true }));
  },

  listOpsAlertRules(): Promise<AdminListResult<OpsAlertRule>> {
    return unwrapList(() => listAdminOpsAlertRules({ throwOnError: true }));
  },

  createOpsAlertRule(body: CreateAdminOpsAlertRuleData["body"]): Promise<OpsAlertRule> {
    return unwrapData(() => createAdminOpsAlertRule({ body, throwOnError: true }));
  },

  updateOpsAlertRule(id: Id, body: UpdateAdminOpsAlertRuleData["body"]): Promise<OpsAlertRule> {
    return unwrapData(() => updateAdminOpsAlertRule({ path: { id }, body, throwOnError: true }));
  },

  async deleteOpsAlertRule(id: Id): Promise<void> {
    configureAdminClient();
    await deleteAdminOpsAlertRule({ path: { id }, throwOnError: true });
  },

  listOpsAlertSilences(): Promise<AdminListResult<OpsAlertSilence>> {
    return unwrapList(() => listAdminOpsAlertSilences({ throwOnError: true }));
  },

  createOpsAlertSilence(body: CreateAdminOpsAlertSilenceData["body"]): Promise<OpsAlertSilence> {
    return unwrapData(() => createAdminOpsAlertSilence({ body, throwOnError: true }));
  },

  async deleteOpsAlertSilence(id: Id): Promise<void> {
    configureAdminClient();
    await deleteAdminOpsAlertSilence({ path: { id }, throwOnError: true });
  },

  listAnnouncements(
    query?: ListAdminAnnouncementsData["query"],
  ): Promise<AdminListResult<Announcement>> {
    return unwrapList(() => listAdminAnnouncements({ query, throwOnError: true }));
  },

  createAnnouncement(body: CreateAnnouncementRequest): Promise<Announcement> {
    return unwrapData(() => createAdminAnnouncement({ body, throwOnError: true }));
  },

  updateAnnouncement(id: Id, body: UpdateAnnouncementRequest): Promise<Announcement> {
    return unwrapData(() => updateAdminAnnouncement({ path: { id }, body, throwOnError: true }));
  },

  deleteAnnouncement(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminAnnouncement({ path: { id }, throwOnError: true }));
  },

  listErrorPassthroughRules(): Promise<AdminListResult<ErrorPassthroughRule>> {
    return unwrapList(() => listAdminErrorPassthroughRules({ throwOnError: true }));
  },

  createErrorPassthroughRule(
    body: CreateErrorPassthroughRuleRequest,
  ): Promise<ErrorPassthroughRule> {
    return unwrapData(() => createAdminErrorPassthroughRule({ body, throwOnError: true }));
  },

  updateErrorPassthroughRule(
    id: Id,
    body: UpdateErrorPassthroughRuleRequest,
  ): Promise<ErrorPassthroughRule> {
    return unwrapData(() =>
      updateAdminErrorPassthroughRule({ path: { id }, body, throwOnError: true }),
    );
  },

  deleteErrorPassthroughRule(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminErrorPassthroughRule({ path: { id }, throwOnError: true }));
  },

  listPayloadRules(): Promise<AdminListResult<PayloadRule>> {
    return unwrapList(() => listAdminPayloadRules({ throwOnError: true }));
  },

  createPayloadRule(body: CreatePayloadRuleRequest): Promise<PayloadRule> {
    return unwrapData(() => createAdminPayloadRule({ body, throwOnError: true }));
  },

  updatePayloadRule(id: Id, body: UpdatePayloadRuleRequest): Promise<PayloadRule> {
    return unwrapData(() => updateAdminPayloadRule({ path: { id }, body, throwOnError: true }));
  },

  deletePayloadRule(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminPayloadRule({ path: { id }, throwOnError: true }));
  },

  listScheduledTestPlans(): Promise<AdminListResult<ScheduledTestPlan>> {
    return unwrapList(() => listAdminScheduledTestPlans({ throwOnError: true }));
  },

  createScheduledTestPlan(
    body: CreateScheduledTestPlanRequest,
  ): Promise<ScheduledTestPlan> {
    return unwrapData(() => createAdminScheduledTestPlan({ body, throwOnError: true }));
  },

  updateScheduledTestPlan(
    id: Id,
    body: UpdateScheduledTestPlanRequest,
  ): Promise<ScheduledTestPlan> {
    return unwrapData(() =>
      updateAdminScheduledTestPlan({ path: { id }, body, throwOnError: true }),
    );
  },

  deleteScheduledTestPlan(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminScheduledTestPlan({ path: { id }, throwOnError: true }));
  },

  listScheduledTestPlanRuns(
    id: Id,
    limit?: number,
  ): Promise<AdminListResult<ScheduledTestPlanRun>> {
    return unwrapList(() =>
      listAdminScheduledTestPlanRuns({
        path: { id },
        query: limit ? { limit } : {},
        throwOnError: true,
      }),
    );
  },

  runScheduledTestPlan(id: Id): Promise<ScheduledTestPlanRun> {
    return unwrapData(() => runAdminScheduledTestPlan({ path: { id }, throwOnError: true }));
  },

  listTlsProfiles(): Promise<AdminListResult<TlsProfile>> {
    return unwrapList(() => listAdminTlsProfiles({ throwOnError: true }));
  },

  createTlsProfile(body: CreateTlsProfileRequest): Promise<TlsProfile> {
    return unwrapData(() => createAdminTlsProfile({ body, throwOnError: true }));
  },

  listRoles(): Promise<AdminListResult<Role>> {
    return unwrapList(() => listAdminRoles({ throwOnError: true }));
  },

  createRole(body: CreateRoleRequest): Promise<Role> {
    return unwrapData(() => createAdminRole({ body, throwOnError: true }));
  },

  updateRole(id: Id, body: UpdateRoleRequest): Promise<Role> {
    return unwrapData(() => updateAdminRole({ path: { id }, body, throwOnError: true }));
  },

  deleteRole(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminRole({ path: { id }, throwOnError: true }));
  },

  listAdminApiKeys(query?: ListAdminApiKeysData["query"]): Promise<AdminListResult<ApiKey>> {
    return unwrapList(() => listAdminApiKeys({ query, throwOnError: true }));
  },

  updateAdminApiKey(id: Id, body: AdminUpdateApiKeyRequest): Promise<ApiKey> {
    return unwrapData(() => updateAdminApiKey({ path: { id }, body, throwOnError: true }));
  },

  // The usage envelope is returned bare (no { data } wrapper), so call directly.
  async getAdminApiKeyUsage(id: Id, days: number): Promise<GatewayUsageResponse> {
    configureAdminClient();
    const response = await getAdminApiKeyUsage({ path: { id }, query: { days }, throwOnError: true });
    if (!response.data) {
      throw new Error("API key usage returned an empty response.");
    }
    return response.data;
  },

  updateTlsProfile(id: Id, body: UpdateTlsProfileRequest): Promise<TlsProfile> {
    return unwrapData(() => updateAdminTlsProfile({ path: { id }, body, throwOnError: true }));
  },

  deleteTlsProfile(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminTlsProfile({ path: { id }, throwOnError: true }));
  },

  listUserAttributeDefinitions(): Promise<AdminListResult<UserAttributeDefinition>> {
    return unwrapList(() => listAdminUserAttributeDefinitions({ throwOnError: true }));
  },

  createUserAttributeDefinition(
    body: CreateUserAttributeDefinitionRequest,
  ): Promise<UserAttributeDefinition> {
    return unwrapData(() => createAdminUserAttributeDefinition({ body, throwOnError: true }));
  },

  updateUserAttributeDefinition(
    id: Id,
    body: UpdateUserAttributeDefinitionRequest,
  ): Promise<UserAttributeDefinition> {
    return unwrapData(() =>
      updateAdminUserAttributeDefinition({ path: { id }, body, throwOnError: true }),
    );
  },

  deleteUserAttributeDefinition(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() =>
      deleteAdminUserAttributeDefinition({ path: { id }, throwOnError: true }),
    );
  },

  listNotificationEmailTemplates(): Promise<NotificationEmailTemplateList> {
    return unwrapData(() => listAdminNotificationEmailTemplates({ throwOnError: true }));
  },

  updateNotificationEmailTemplate(
    event: NotificationEmailTemplateEventName,
    body: UpdateNotificationEmailTemplateRequest,
  ): Promise<NotificationEmailTemplate> {
    return unwrapData(() =>
      updateAdminNotificationEmailTemplate({ path: { event }, body, throwOnError: true }),
    );
  },

  restoreNotificationEmailTemplate(
    event: NotificationEmailTemplateEventName,
  ): Promise<NotificationEmailTemplate> {
    return unwrapData(() =>
      restoreAdminNotificationEmailTemplate({ path: { event }, throwOnError: true }),
    );
  },

  previewNotificationEmailTemplate(
    body: PreviewNotificationEmailTemplateRequest,
  ): Promise<NotificationEmailTemplatePreview> {
    return unwrapData(() => previewAdminNotificationEmailTemplate({ body, throwOnError: true }));
  },

  listAccountsAvailability(days?: number): Promise<AdminListResult<AccountAvailabilitySummary>> {
    return unwrapList(() => listAdminAccountsAvailability({ query: { days }, throwOnError: true }));
  },

  listUserPlatformQuotas(userId: Id): Promise<AdminListResult<UserPlatformQuota>> {
    return unwrapList(() => listAdminUserPlatformQuotas({ path: { id: userId }, throwOnError: true }));
  },

  upsertUserPlatformQuota(
    userId: Id,
    body: UpsertUserPlatformQuotaRequest,
  ): Promise<UserPlatformQuota> {
    return unwrapData(() =>
      upsertAdminUserPlatformQuota({ path: { id: userId }, body, throwOnError: true }),
    );
  },

  deleteUserPlatformQuota(userId: Id, platform: string): Promise<{ deleted: boolean }> {
    return unwrapData(() =>
      deleteAdminUserPlatformQuota({ path: { id: userId, platform }, throwOnError: true }),
    );
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

  batchDisableRedeemCodes(ids: Id[]): Promise<unknown> {
    return unwrapData(() => batchDisableAdminRedeemCodes({ body: { ids }, throwOnError: true }));
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

  getRiskConfig(): Promise<RiskControlConfig> {
    return unwrapData(() => getAdminRiskControlConfig({ throwOnError: true }));
  },

  updateRiskConfig(body: RiskControlConfig): Promise<RiskControlConfig> {
    return unwrapData(() => updateAdminRiskControlConfig({ body, throwOnError: true }));
  },

  getRiskStatus(): Promise<RiskControlStatus> {
    return unwrapData(() => getAdminRiskControlStatus({ throwOnError: true }));
  },

  listRiskLogs(query?: ListAdminRiskControlLogsData["query"]): Promise<AdminListResult<RiskControlLog>> {
    return unwrapList(() => listAdminRiskControlLogs({ query, throwOnError: true }));
  },

  getSettings(): Promise<AdminSettings> {
    return unwrapData(() => getAdminSettings({ throwOnError: true }));
  },

  updateSettings(body: AdminSettings): Promise<AdminSettings> {
    return unwrapData(() => updateAdminSettings({ body, throwOnError: true }));
  },

  // Deliver a probe email through the effective SMTP config. The write-only SMTP
  // password makes this the only way to confirm the credentials actually work.
  sendTestEmail(body?: AdminSendTestEmailRequest): Promise<AdminTestResult> {
    return unwrapData(() => sendAdminTestEmail({ body: body ?? {}, throwOnError: true }));
  },

  getCopilotConfig(): Promise<AdminCopilotConfig> {
    return unwrapData(() => getAdminCopilotConfig({ throwOnError: true }));
  },

  getConfigSnapshot(): Promise<ConfigSnapshotResponse["data"]> {
    return unwrapData(() => getAdminConfigSnapshot({ throwOnError: true }));
  },

  importConfigSnapshot(
    body: ConfigImportRequest,
    dryRun = false,
  ): Promise<ConfigImportResponse["data"]> {
    return unwrapData(() =>
      importAdminConfigSnapshot({ body, query: { dry_run: dryRun }, throwOnError: true }),
    );
  },

  // Rate limits (per-model & per-account-group TPM/RPM/concurrency). The API keys
  // them by id with no per-id GET, so reads list all and the UI joins by id.
  listModelRateLimits(): Promise<AdminListResult<ModelRateLimit>> {
    return unwrapList(() => listAdminModelRateLimits({ throwOnError: true }));
  },
  upsertModelRateLimit(body: UpsertModelRateLimitRequest): Promise<ModelRateLimit> {
    return unwrapData(() => upsertAdminModelRateLimit({ body, throwOnError: true }));
  },
  async deleteModelRateLimit(modelId: Id): Promise<void> {
    configureAdminClient();
    await deleteAdminModelRateLimit({ path: { modelId }, throwOnError: true });
  },
  listGroupRateLimits(): Promise<AdminListResult<AccountGroupRateLimit>> {
    return unwrapList(() => listAdminGroupRateLimits({ throwOnError: true }));
  },
  upsertGroupRateLimit(body: UpsertGroupRateLimitRequest): Promise<AccountGroupRateLimit> {
    return unwrapData(() => upsertAdminGroupRateLimit({ body, throwOnError: true }));
  },
  async deleteGroupRateLimit(groupId: Id): Promise<void> {
    configureAdminClient();
    await deleteAdminGroupRateLimit({ path: { groupId }, throwOnError: true });
  },
};

export function statusQuery(status: string): { status?: UserStatus | AnnouncementStatus | string } {
  return status === "all" ? {} : { status };
}
