/**
 * Barrel re-export of the generated SDK domain types.
 *
 * Pages live at varying depths under src/app, which makes deep relative paths
 * into packages/sdk error-prone. Import SDK types from "@/lib/sdk-types"
 * instead — this file holds the single known-good relative path.
 */
export type {
  User,
  UserStatus,
  Provider,
  PlatformFamily,
  RuntimeClass,
  Model,
  ProviderAccount,
  AccountGroup,
  ProxyDefinition,
  SubscriptionPlan,
  UserSubscription,
  PaymentOrder,
  UsageLog,
  PromoCode,
  RedeemCode,
  AffiliateInviteRecord,
  AffiliateLedgerEntry,
  AuditLog,
  BillingLedgerEntry,
  ErrorPassthroughRule,
  TlsProfile,
  UserAttributeDefinition,
  NotificationEmailTemplate,
  NotificationEmailTemplateList,
  NotificationEmailTemplateEvent,
  NotificationEmailTemplateEventName,
  AccountAvailabilitySummary,
  UserPlatformQuota,
  Announcement,
  PricingRule,
  RiskControlLog,
  RiskControlStatus,
  SchedulerReplayResult,
  SchedulerStrategyName,
  OpsSloDefinition,
  ModelRateLimit,
  AccountGroupRateLimit,
  UpsertModelRateLimitRequest,
  UpsertGroupRateLimitRequest,
} from "../../../../packages/sdk/typescript/src/types.gen";
