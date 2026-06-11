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
  startAdminAccountOAuthAuthorizeUrl,
  exchangeAdminAccountOAuthCode,
  startAdminAccountOAuthDeviceCode,
  pollAdminAccountOAuthDeviceCode,
  createAdminAccountGroup,
  deleteAdminAccountGroup,
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
  listAdminChannelMonitors,
  createAdminChannelMonitor,
  updateAdminChannelMonitor,
  deleteAdminChannelMonitor,
  runAdminChannelMonitor,
  listAdminChannelMonitorRuns,
  listAdminChannelMonitorTemplates,
  createAdminChannelMonitorTemplate,
  updateAdminChannelMonitorTemplate,
  deleteAdminChannelMonitorTemplate,
  applyAdminChannelMonitorTemplate,
  createAdminTlsProfile,
  deleteAdminTlsProfile,
  listAdminTlsProfiles,
  updateAdminTlsProfile,
  getAdminPermissionCatalog,
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
  deleteAdminOpsSlo,
  listAdminOpsAlertRules,
  createAdminOpsAlertRule,
  updateAdminOpsAlertRule,
  deleteAdminOpsAlertRule,
  listAdminOpsAlertSilences,
  createAdminOpsAlertSilence,
  deleteAdminOpsAlertSilence,
  createAdminPaymentProvider,
  deleteAdminPaymentProvider,
  createAdminPricingRule,
  updateAdminPricingRule,
  deleteAdminPricingRule,
  createAdminAffiliateRule,
  createAdminPromoCode,
  createAdminProvider,
  deleteAdminAccount,
  deleteAdminProvider,
  createAdminProxy,
  deleteAdminProxy,
  createAdminRedeemCode,
  deleteAdminRedeemCode,
  createAdminSubscriptionPlan,
  deleteAdminSubscriptionPlan,
  updateAdminSubscriptionPlan,
  createAdminUser,
  createAdminUserSubscription,
  deleteAdminUserSubscription,
  deleteAdminAnnouncement,
  getAdminAnnouncementReadStatus,
  deleteAdminPromoCode,
  disableAdminAccount,
  disableAdminUser,
  discoverAdminAccountModels,
  enableAdminAccount,
  enableAdminUser,
  exportAdminAccounts,
  getAdminAccountHealth,
  getAdminAccountProxyQuality,
  getAdminAccountsHealthSummary,
  fetchAdminAccountQuota,
  getAdminAccountQuota,
  getAdminAccountRpmStatus,
  getAdminProviderOAuthConfig,
  getAdminConfigSnapshot,
  importAdminConfigSnapshot,
  getAdminDashboardSnapshot,
  getAdminContentSafetyConfig,
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
  listAdminAffiliateRules,
  listAdminAffiliateTransfers,
  listAdminAnnouncements,
  listAdminAuditLogs,
  listAdminBillingLedger,
  listAdminModels,
  createAdminModel,
  createAdminModelAlias,
  quickMapAdminModels,
  listAdminModelAliases,
  deleteAdminModelAlias,
  createAdminModelMapping,
  listAdminModelMappings,
  deleteAdminModelMapping,
  updateAdminModel,
  deleteAdminModel,
  listAdminOpsAlertEvents,
  listAdminOpsAlerts,
  listAdminOpsRealtimeSlots,
  listAdminOpsSlos,
  listAdminOpsSystemLogs,
  listAdminApiKeys,
  getAdminApiKeyUsage,
  listAdminOutboxEvents,
  listAdminPaymentOrders,
  listAdminPaymentOrderAuditLogs,
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
  getAdminSchedulerOverview,
  listSchedulerStrategies,
  createSchedulerStrategy,
  updateSchedulerStrategy,
  deprecateSchedulerStrategy,
  activateSchedulerStrategy,
  simulateSchedulerStrategy,
  recoverAdminAccount,
  resetAdminAccountQuota,
  runAdminQuickSetup,
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
  updateAdminAffiliateRule,
  updateAdminPromoCode,
  updateAdminProvider,
  updateAdminProxy,
  updateAdminContentSafetyConfig,
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
  AdminAccountTestRequest,
  AdminQuickMapModelsResult,
  AdminQuickSetupResult,
  AdminTestResult,
  AdminUpdateApiKeyRequest,
  ApiKey,
  ContentSafetyConfig,
  GatewayUsageResponse,
  PromoCodeUsage,
  AffiliateInviteRecord,
  AffiliateLedgerEntry,
  AffiliateRule,
  Announcement,
  AnnouncementStatus,
  AnnouncementReadStatus,
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
  ChannelMonitor,
  CreateChannelMonitorRequest,
  UpdateChannelMonitorRequest,
  ChannelMonitorRun,
  ChannelMonitorTemplate,
  CreateChannelMonitorTemplateRequest,
  UpdateChannelMonitorTemplateRequest,
  CreateTlsProfileRequest,
  TlsProfile,
  PermissionDefinition,
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
  AccountOAuthProviderConfig,
  AccountOAuthAuthorizeUrl,
  AccountOAuthCredential,
  AccountOAuthDeviceCode,
  AccountOAuthPending,
  ProviderOAuthConfig,
  CreateAdminPaymentProviderData,
  UpdateAdminPaymentProviderData,
  CreateAdminPricingRuleData,
  UpdateAdminPricingRuleData,
  CreateAdminProxyData,
  CreateAdminSubscriptionPlanData,
  CreateAdminUserSubscriptionData,
  CreateAdminUserData,
  CreateAnnouncementRequest,
  CreateAffiliateRuleRequest,
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
  ListAdminAffiliateRulesData,
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
  PaymentAuditLog,
  PaymentProviderInstance,
  PricingRule,
  PromoCode,
  Provider,
  ProviderAccount,
  ProviderAccountExportItem,
  ProviderAccountImportResult,
  ProviderAccountStatus,
  ProxyDefinition,
  QuickMapAdminModelsData,
  RealtimeActiveSlot,
  RedeemCode,
  RedeemCodeStats,
  ReplaySchedulerStrategyData,
  SimulateSchedulerStrategyData,
  CreateSchedulerStrategyData,
  UpdateSchedulerStrategyData,
  RiskControlConfig,
  RiskControlLog,
  RiskControlStatus,
  SchedulerOverview,
  SchedulerStrategy,
  SchedulerReplayResult,
  SchedulerSimulationResult,
  SubscriptionPlan,
  RunAdminQuickSetupData,
  UpdateAccountGroupRequest,
  UpdateAdminAccountData,
  UpdateAdminOpsSloData,
  UpdateAdminProviderData,
  UpdateAdminProxyData,
  UpdateAdminUserData,
  UpdateAnnouncementRequest,
  UpdateAffiliateRuleRequest,
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
  schedulerOverview(): Promise<SchedulerOverview> {
    return unwrapData(() => getAdminSchedulerOverview({ throwOnError: true }));
  },
  listSchedulerStrategies(): Promise<AdminListResult<SchedulerStrategy>> {
    return unwrapList(() => listSchedulerStrategies({ throwOnError: true }));
  },
  createSchedulerStrategy(body: CreateSchedulerStrategyData["body"]): Promise<SchedulerStrategy> {
    return unwrapData(() => createSchedulerStrategy({ body, throwOnError: true }));
  },
  updateSchedulerStrategy(
    id: Id,
    body: UpdateSchedulerStrategyData["body"],
  ): Promise<SchedulerStrategy> {
    return unwrapData(() =>
      updateSchedulerStrategy({ path: { id }, body, throwOnError: true }),
    );
  },
  deprecateSchedulerStrategy(id: Id): Promise<SchedulerStrategy> {
    return unwrapData(() => deprecateSchedulerStrategy({ path: { id }, throwOnError: true }));
  },
  activateSchedulerStrategy(id: Id): Promise<SchedulerStrategy> {
    return unwrapData(() => activateSchedulerStrategy({ path: { id }, throwOnError: true }));
  },
  simulateSchedulerStrategy(
    body: SimulateSchedulerStrategyData["body"],
  ): Promise<SchedulerSimulationResult> {
    return unwrapData(() => simulateSchedulerStrategy({ body, throwOnError: true }));
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

  deleteProvider(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminProvider({ path: { id }, throwOnError: true }));
  },

  testProvider(id: Id): Promise<AdminTestResult> {
    return unwrapData(() => testAdminProvider({ path: { id }, throwOnError: true }));
  },
  // Bulk-create any missing built-in provider presets (existing ones are skipped,
  // new ones land disabled). Returns how many were requested/created/failed.
  installProviderPresets(): Promise<BatchOperationResult> {
    return unwrapData(() => installAdminProviderPresets({ throwOnError: true }));
  },

  runQuickSetup(body: RunAdminQuickSetupData["body"]): Promise<AdminQuickSetupResult> {
    return unwrapData(() => runAdminQuickSetup({ body, throwOnError: true }));
  },

  getProviderOAuthConfig(id: Id): Promise<ProviderOAuthConfig> {
    return unwrapData(() => getAdminProviderOAuthConfig({ path: { id }, throwOnError: true }));
  },

  listModels(query?: ListAdminModelsData["query"]): Promise<AdminListResult<Model>> {
    return unwrapList(() => listAdminModels({ query, throwOnError: true }));
  },

  quickMapModels(body: QuickMapAdminModelsData["body"]): Promise<AdminQuickMapModelsResult> {
    return unwrapData(() => quickMapAdminModels({ body, throwOnError: true }));
  },

  createModel(body: Parameters<typeof createAdminModel>[0]["body"]): Promise<Model> {
    return unwrapData(() => createAdminModel({ body, throwOnError: true }));
  },

  updateModel(id: Id, body: Parameters<typeof updateAdminModel>[0]["body"]): Promise<Model> {
    return unwrapData(() => updateAdminModel({ path: { id }, body, throwOnError: true }));
  },

  deleteModel(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminModel({ path: { id }, throwOnError: true }));
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
  listModelAliases(id: Id): Promise<AdminListResult<ModelAlias>> {
    return unwrapList(() => listAdminModelAliases({ path: { id }, throwOnError: true }));
  },
  deleteModelAlias(id: Id, aliasId: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminModelAlias({ path: { id, aliasId }, throwOnError: true }));
  },
  listModelMappings(id: Id): Promise<AdminListResult<ModelProviderMapping>> {
    return unwrapList(() => listAdminModelMappings({ path: { id }, throwOnError: true }));
  },
  deleteModelMapping(id: Id, mappingId: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminModelMapping({ path: { id, mappingId }, throwOnError: true }));
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

  // Interactive upstream-account OAuth provisioning (replaces hand-pasting
  // access_token/refresh_token). The minted credential is returned write-only
  // and immediately fed into createAccount — it is never persisted server-side.
  startAccountOAuthAuthorizeUrl(config: AccountOAuthProviderConfig): Promise<AccountOAuthAuthorizeUrl> {
    return unwrapData(() =>
      startAdminAccountOAuthAuthorizeUrl({ body: { config }, throwOnError: true }),
    );
  },

  exchangeAccountOAuthCode(input: {
    sessionId: string;
    code: string;
    state: string;
  }): Promise<AccountOAuthCredential> {
    return unwrapData(() =>
      exchangeAdminAccountOAuthCode({
        body: { session_id: input.sessionId, code: input.code, state: input.state },
        throwOnError: true,
      }),
    );
  },

  startAccountOAuthDeviceCode(config: AccountOAuthProviderConfig): Promise<AccountOAuthDeviceCode> {
    return unwrapData(() =>
      startAdminAccountOAuthDeviceCode({ body: { config }, throwOnError: true }),
    );
  },

  // Polls once. Returns the minted credential on success, or a pending status
  // (status === "pending") that the caller keeps polling on a fixed interval.
  // The endpoint returns a union (200 credential / 202 pending), so we unwrap
  // the envelope directly rather than through the single-type unwrapData.
  async pollAccountOAuthDeviceCode(
    sessionId: string,
  ): Promise<AccountOAuthCredential | AccountOAuthPending> {
    configureAdminClient();
    const response = await pollAdminAccountOAuthDeviceCode({
      body: { session_id: sessionId },
      throwOnError: true,
    });
    const data = response.data?.data;
    if (!data) {
      throw new Error("Device-code poll returned an empty response.");
    }
    return data;
  },

  updateAccount(id: Id, body: UpdateAdminAccountData["body"]): Promise<ProviderAccount> {
    return unwrapData(() => updateAdminAccount({ path: { id }, body, throwOnError: true }));
  },

  deleteAccount(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminAccount({ path: { id }, throwOnError: true }));
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

  testAccount(id: Id, body?: AdminAccountTestRequest): Promise<AdminTestResult> {
    return unwrapData(() => testAdminAccount({ path: { id }, body, throwOnError: true }));
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

  resetAccountQuota(id: Id): Promise<ProviderAccount> {
    return unwrapData(() => resetAdminAccountQuota({ path: { id }, throwOnError: true }));
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

  getAccountsHealthSummary(): Promise<AccountHealthSnapshot[]> {
    return unwrapData(() => getAdminAccountsHealthSummary({ throwOnError: true }));
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

  deleteProxy(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminProxy({ path: { id }, throwOnError: true }));
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

  deleteAccountGroup(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminAccountGroup({ path: { id }, throwOnError: true }));
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

  listAffiliateRules(
    query?: ListAdminAffiliateRulesData["query"],
  ): Promise<AdminListResult<AffiliateRule>> {
    return unwrapList(() => listAdminAffiliateRules({ query, throwOnError: true }));
  },

  createAffiliateRule(body: CreateAffiliateRuleRequest): Promise<AffiliateRule> {
    return unwrapData(() => createAdminAffiliateRule({ body, throwOnError: true }));
  },

  updateAffiliateRule(id: Id, body: UpdateAffiliateRuleRequest): Promise<AffiliateRule> {
    return unwrapData(() => updateAdminAffiliateRule({ path: { id }, body, throwOnError: true }));
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

  createOpsSlo(body: Parameters<typeof createAdminOpsSlo>[0]["body"]): Promise<OpsSloDefinition> {
    return unwrapData(() => createAdminOpsSlo({ body, throwOnError: true }));
  },

  updateOpsSlo(id: Id, body: UpdateAdminOpsSloData["body"]): Promise<OpsSloDefinition> {
    return unwrapData(() => updateAdminOpsSlo({ path: { id }, body, throwOnError: true }));
  },

  deleteOpsSlo(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminOpsSlo({ path: { id }, throwOnError: true }));
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

  getAnnouncementReadStatus(id: Id): Promise<AnnouncementReadStatus> {
    return unwrapData(() => getAdminAnnouncementReadStatus({ path: { id }, throwOnError: true }));
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

  listChannelMonitors(): Promise<AdminListResult<ChannelMonitor>> {
    return unwrapList(() => listAdminChannelMonitors({ throwOnError: true }));
  },

  createChannelMonitor(body: CreateChannelMonitorRequest): Promise<ChannelMonitor> {
    return unwrapData(() => createAdminChannelMonitor({ body, throwOnError: true }));
  },

  updateChannelMonitor(id: Id, body: UpdateChannelMonitorRequest): Promise<ChannelMonitor> {
    return unwrapData(() => updateAdminChannelMonitor({ path: { id }, body, throwOnError: true }));
  },

  deleteChannelMonitor(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminChannelMonitor({ path: { id }, throwOnError: true }));
  },

  runChannelMonitor(id: Id): Promise<ChannelMonitorRun> {
    return unwrapData(() => runAdminChannelMonitor({ path: { id }, throwOnError: true }));
  },

  listChannelMonitorRuns(id: Id, limit?: number): Promise<AdminListResult<ChannelMonitorRun>> {
    return unwrapList(() =>
      listAdminChannelMonitorRuns({ path: { id }, query: { limit }, throwOnError: true }),
    );
  },

  listChannelMonitorTemplates(): Promise<AdminListResult<ChannelMonitorTemplate>> {
    return unwrapList(() => listAdminChannelMonitorTemplates({ throwOnError: true }));
  },

  createChannelMonitorTemplate(
    body: CreateChannelMonitorTemplateRequest,
  ): Promise<ChannelMonitorTemplate> {
    return unwrapData(() => createAdminChannelMonitorTemplate({ body, throwOnError: true }));
  },

  updateChannelMonitorTemplate(
    id: Id,
    body: UpdateChannelMonitorTemplateRequest,
  ): Promise<ChannelMonitorTemplate> {
    return unwrapData(() =>
      updateAdminChannelMonitorTemplate({ path: { id }, body, throwOnError: true }),
    );
  },

  deleteChannelMonitorTemplate(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminChannelMonitorTemplate({ path: { id }, throwOnError: true }));
  },

  applyChannelMonitorTemplate(
    id: Id,
    monitorIds: number[],
  ): Promise<AdminListResult<ChannelMonitor>> {
    return unwrapList(() =>
      applyAdminChannelMonitorTemplate({
        path: { id },
        body: { monitor_ids: monitorIds },
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

  listPermissionCatalog(): Promise<PermissionDefinition[]> {
    return unwrapData(() => getAdminPermissionCatalog({ throwOnError: true }));
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

  getContentSafetyConfig(): Promise<ContentSafetyConfig> {
    return unwrapData(() => getAdminContentSafetyConfig({ throwOnError: true }));
  },

  updateContentSafetyConfig(body: ContentSafetyConfig): Promise<ContentSafetyConfig> {
    return unwrapData(() => updateAdminContentSafetyConfig({ body, throwOnError: true }));
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
