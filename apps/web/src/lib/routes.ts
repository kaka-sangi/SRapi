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
} as const;

/** Canonical admin route paths, referenced by the sidebar nav + pages. */
export const ADMIN_ROUTES = {
  dashboard: "/admin/dashboard",
  users: "/admin/users",
  providers: "/admin/providers",
  models: "/admin/models",
  accounts: "/admin/accounts",
  groups: "/admin/groups",
  proxies: "/admin/proxies",
  subscriptions: "/admin/subscriptions",
  orders: "/admin/orders",
  ordersPlans: "/admin/orders/plans",
  ordersDashboard: "/admin/orders/dashboard",
  channelsPricing: "/admin/channels/pricing",
  channelsMonitor: "/admin/channels/monitor",
  paymentProviders: "/admin/payment-providers",
  promoCodes: "/admin/promo-codes",
  redeem: "/admin/redeem",
  affiliatesInvites: "/admin/affiliates/invites",
  affiliatesRebates: "/admin/affiliates/rebates",
  affiliatesTransfers: "/admin/affiliates/transfers",
  announcements: "/admin/announcements",
  ops: "/admin/ops",
  opsStrategy: "/admin/ops/strategy",
  eventsOutbox: "/admin/ops/events",
  riskControl: "/admin/risk-control",
  auditLogs: "/admin/audit-logs",
  billingLedger: "/admin/billing-ledger",
  errorPassthrough: "/admin/error-passthrough",
  tlsProfiles: "/admin/tls-profiles",
  payloadRules: "/admin/payload-rules",
  roles: "/admin/roles",
  userAttributes: "/admin/user-attributes",
  notificationTemplates: "/admin/notification-templates",
  usage: "/admin/usage",
  copilot: "/admin/copilot",
  settings: "/admin/settings",
} as const;

export const ADMIN_ROUTE_SMOKE_TARGETS = [
  { path: "/admin/dashboard", heading: "Admin Dashboard" },
  { path: "/admin/ops", heading: "Operations Center" },
  { path: "/admin/ops/strategy", heading: "Strategy Comparison" },
  { path: "/admin/users", heading: "User Management" },
  { path: "/admin/groups", heading: "Group Management" },
  { path: "/admin/subscriptions", heading: "Subscription Management" },
  { path: "/admin/accounts", heading: "Account Pool" },
  { path: "/admin/announcements", heading: "Announcement Management" },
  { path: "/admin/proxies", heading: "Proxy Management" },
  { path: "/admin/risk-control", heading: "Risk Control" },
  { path: "/admin/audit-logs", heading: "Audit logs" },
  { path: "/admin/ops/events", heading: "Event outbox" },
  { path: "/admin/billing-ledger", heading: "Billing ledger" },
  { path: "/admin/error-passthrough", heading: "Error passthrough" },
  { path: "/admin/tls-profiles", heading: "TLS profiles" },
  { path: "/admin/payload-rules", heading: "Payload rules" },
  { path: "/admin/user-attributes", heading: "User attributes" },
  { path: "/admin/roles", heading: "Role Management" },
  { path: "/admin/notification-templates", heading: "Email templates" },
  { path: "/admin/redeem", heading: "Redeem Codes" },
  { path: "/admin/promo-codes", heading: "Promo Codes" },
  { path: "/admin/usage", heading: "Usage Records" },
  { path: "/admin/settings", heading: "System Settings" },
  { path: "/admin/channels/pricing", heading: "Channel Pricing" },
  { path: "/admin/channels/monitor", heading: "Channel Monitor" },
  { path: "/admin/affiliates/invites", heading: "Affiliate Invites" },
  { path: "/admin/affiliates/rebates", heading: "Affiliate Rebates" },
  { path: "/admin/affiliates/transfers", heading: "Affiliate Transfers" },
  { path: "/admin/orders/dashboard", heading: "Payment Dashboard" },
  { path: "/admin/orders", heading: "Orders" },
  { path: "/admin/orders/plans", heading: "Order Plans" },
] as const;

export function homeRouteForRole(role: "admin" | "user"): string {
  return role === "admin" ? ADMIN_HOME_ROUTE : USER_HOME_ROUTE;
}
