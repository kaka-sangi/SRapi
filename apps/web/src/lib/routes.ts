export const ADMIN_HOME_ROUTE = "/admin/dashboard";
export const USER_HOME_ROUTE = "/dashboard";
export const SIGN_IN_ROUTE = "/";

/** User self-service route paths. */
export const USER_ROUTES = {
  account: "/account",
  billing: "/billing",
  redeem: "/redeem",
  affiliate: "/affiliate",
  playground: "/playground",
  availableChannels: "/available-channels",
} as const;

/** Canonical admin route paths, referenced by the sidebar nav + pages. */
export const ADMIN_ROUTES = {
  dashboard: "/admin/dashboard",
  users: "/admin/users",
  quickSetup: "/admin/quick-setup",
  providers: "/admin/providers",
  models: "/admin/models",
  accounts: "/admin/accounts",
  groups: "/admin/groups",
  proxies: "/admin/proxies",
  subscriptions: "/admin/subscriptions",
  orders: "/admin/orders",
  ordersPlans: "/admin/orders/plans",
  channelsPricing: "/admin/channels/pricing",
  paymentProviders: "/admin/payment-providers",
  promoCodes: "/admin/promo-codes",
  redeem: "/admin/redeem",
  affiliatesInvites: "/admin/affiliates/invites",
  affiliatesRebates: "/admin/affiliates/rebates",
  affiliatesTransfers: "/admin/affiliates/transfers",
  affiliatesRules: "/admin/affiliates/rules",
  announcements: "/admin/announcements",
  ops: "/admin/ops",
  opsStrategy: "/admin/ops?tab=strategy",
  eventsOutbox: "/admin/ops/events",
  diagnostics: "/admin/ops/diagnostics",
  riskControl: "/admin/risk-control",
  auditLogs: "/admin/audit-logs",
  errorLogs: "/admin/error-logs",
  billingLedger: "/admin/billing-ledger",
  errorPassthrough: "/admin/error-passthrough",
  tlsProfiles: "/admin/tls-profiles",
  payloadRules: "/admin/payload-rules",
  roles: "/admin/roles",
  apiKeys: "/admin/api-keys",
  userAttributes: "/admin/user-attributes",
  notificationTemplates: "/admin/notification-templates",
  usage: "/admin/usage",
  copilot: "/admin/copilot",
  settings: "/admin/settings",
} as const;

export function homeRouteForRole(role: "admin" | "user"): string {
  return role === "admin" ? ADMIN_HOME_ROUTE : USER_HOME_ROUTE;
}
