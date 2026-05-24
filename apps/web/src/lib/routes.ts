export const ADMIN_HOME_ROUTE = "/admin/dashboard";
export const USER_HOME_ROUTE = "/dashboard";

export const ADMIN_ROUTE_SMOKE_TARGETS = [
  { path: "/admin/dashboard", heading: "Admin Dashboard" },
  { path: "/admin/ops", heading: "Operations Center" },
  { path: "/admin/users", heading: "User Management" },
  { path: "/admin/groups", heading: "Group Management" },
  { path: "/admin/subscriptions", heading: "Subscription Management" },
  { path: "/admin/accounts", heading: "Account Pool" },
  { path: "/admin/announcements", heading: "Announcement Management" },
  { path: "/admin/proxies", heading: "Proxy Management" },
  { path: "/admin/risk-control", heading: "Risk Control" },
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
